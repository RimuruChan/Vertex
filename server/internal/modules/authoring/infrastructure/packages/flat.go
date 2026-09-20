package packages

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

var dataNumber = regexp.MustCompile(`[0-9]+`)

// A data-only archive augments a draft; it never supplies a replacement title,
// statement, checker, reference solution, or global judging policy.
func (value *importer) flatData() error {
	value.plan.Format, value.plan.Scope = "luogu-data", "data"
	value.metadata = domain.PackageMetadata{SchemaVersion: 1, Title: "数据导入", Difficulty: 1, TimeLimitMs: 1000, MemoryLimitKB: 262144, JudgeType: "normal", StatementLanguage: "zh", Comparison: domain.OutputComparison{Kind: "tokens"}}
	inputs := []string{}
	for _, name := range value.archive.names {
		if strings.Contains(name, "/") {
			return invalid("普通数据包需要平铺的 .in/.out 文件：%s", name)
		}
		if strings.HasSuffix(name, ".in") {
			inputs = append(inputs, name)
		}
		if name != "config.yml" && !strings.HasSuffix(name, ".in") && !strings.HasSuffix(name, ".out") && !strings.HasSuffix(name, ".ans") {
			return invalid("数据包包含无法识别的文件：%s", name)
		}
	}
	if len(inputs) == 0 {
		return invalid("无法识别题包格式，普通数据包需要成对的 .in/.out 或 .in/.ans")
	}
	// Numeric identifiers sort numerically (2 before 10), without integer overflow.
	sort.SliceStable(inputs, func(i, j int) bool {
		a, b := dataNumber.FindString(inputs[i]), dataNumber.FindString(inputs[j])
		a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		if a != b {
			return a < b
		}
		return inputs[i] < inputs[j]
	})
	config := map[string]any{}
	if value.archive.has("config.yml") {
		data, err := value.archive.read("config.yml", 1<<20)
		if err != nil {
			return err
		}
		config, err = readYAML(data)
		if err != nil {
			return err
		}
	}
	consumed := map[string]bool{}
	for _, input := range inputs {
		stem := strings.TrimSuffix(input, ".in")
		answer := stem + ".out"
		if value.archive.has(stem + ".ans") {
			if value.archive.has(answer) {
				return invalid("同一输入有两个答案文件：%s", input)
			}
			answer = stem + ".ans"
		}
		if !value.archive.has(answer) {
			return invalid("缺少答案文件：%s", input)
		}
		if len(dataNumber.FindAllString(stem, -1)) != 1 {
			return invalid("数据文件名必须含一段连续数字：%s", input)
		}
		in, err := value.file(input, domain.EntryInput, nil)
		if err != nil {
			return err
		}
		out, err := value.file(answer, domain.EntryAnswer, nil)
		if err != nil {
			return err
		}
		consumed[input], consumed[answer] = true, true
		id := stableID("test", input)
		test := domain.TestMaterial{SchemaVersion: 1, Name: stem, Input: domain.TestInput{Kind: "file", Entry: in.ID}, Answer: domain.TestAnswer{Kind: "file", Entry: out.ID}}
		settings, hasInput := config[input]
		answerSettings, hasAnswer := config[answer]
		if hasInput && hasAnswer {
			return invalid("输入和输出重复配置同一测试点：%s", input)
		}
		if hasAnswer {
			settings = answerSettings
		}
		if hasInput || hasAnswer {
			fields, ok := settings.(map[string]any)
			if !ok {
				return invalid("无效测试点配置：%s", input)
			}
			for key, raw := range fields {
				switch key {
				case "timeLimit", "memoryLimit":
					number, ok := raw.(int)
					if !ok || number <= 0 {
						return invalid("无效 %s：%s", key, input)
					}
					if key == "timeLimit" {
						test.TimeLimitMs = number
					} else {
						test.MemoryLimitKB = number
					}
				default:
					value.require("luogu.config."+key, "config.yml", "测试点 "+input+" 的 "+key+" 尚需映射，原配置已保留", "build")
				}
			}
		}
		if err := value.document(id, "vertex/tests/"+id+".json", domain.EntryTest, test); err != nil {
			return err
		}
		value.metadata.TestOrder = append(value.metadata.TestOrder, id)
	}
	for name := range config {
		if !consumed[name] {
			return invalid("配置引用不存在的测试点：%s", name)
		}
	}
	for _, name := range value.archive.names {
		if name != "config.yml" && !consumed[name] {
			return invalid("没有匹配输入的答案：%s", name)
		}
	}
	if err := value.preserveRemaining(); err != nil {
		return err
	}
	value.issue("info", "data.merge", "", "只添加或更新同名导入数据，保留已有题面、程序、其他测试及全局设置")
	return value.document("problem", "vertex/problem.json", domain.EntryMetadata, value.metadata)
}

// MergeData keeps the selected copy as the base. Reimporting a changed data file
// updates its stable identity; unrelated path/identity collisions are rejected.
func MergeData(base domain.ContentTree, plan *domain.ImportPlan, read ReadContent, put PutContent) error {
	if plan.Scope != "data" {
		return nil
	}
	loadMetadata := func(tree domain.ContentTree) (domain.PackageMetadata, domain.TreeEntry, error) {
		for _, entry := range tree.Entries {
			if entry.ID != "problem" || entry.Kind != domain.EntryMetadata {
				continue
			}
			stream, err := read(entry.Blob)
			if err != nil {
				return domain.PackageMetadata{}, entry, err
			}
			data, err := io.ReadAll(io.LimitReader(stream, (1<<20)+1))
			stream.Close()
			if err != nil {
				return domain.PackageMetadata{}, entry, err
			}
			if len(data) > 1<<20 {
				return domain.PackageMetadata{}, entry, domain.ErrPackageTooBig
			}
			var meta domain.PackageMetadata
			if err := json.Unmarshal(data, &meta); err != nil {
				return meta, entry, err
			}
			return meta, entry, nil
		}
		return domain.PackageMetadata{}, domain.TreeEntry{}, invalid("工作副本缺少元信息")
	}
	meta, metaEntry, err := loadMetadata(base)
	if err != nil {
		return err
	}
	imported, _, err := loadMetadata(plan.Tree)
	if err != nil {
		return err
	}
	byID := map[string]int{}
	entries := append([]domain.TreeEntry{}, base.Entries...)
	for index, entry := range entries {
		byID[entry.ID] = index
	}
	for _, entry := range plan.Tree.Entries {
		if entry.ID == "problem" {
			continue
		}
		if index, exists := byID[entry.ID]; exists {
			old := entries[index]
			if old.Path != entry.Path || old.Kind != entry.Kind {
				return invalid("导入数据与已有材料身份冲突：%s", entry.Path)
			}
			entries[index] = entry
		} else {
			entries = append(entries, entry)
		}
	}
	ordered := map[string]bool{}
	for _, id := range meta.TestOrder {
		ordered[id] = true
	}
	for _, id := range imported.TestOrder {
		if !ordered[id] {
			meta.TestOrder = append(meta.TestOrder, id)
			ordered[id] = true
		}
	}
	for _, requirement := range imported.Requirements {
		found := false
		for _, old := range meta.Requirements {
			if old == requirement {
				found = true
				break
			}
		}
		if !found {
			meta.Requirements = append(meta.Requirements, requirement)
		}
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	data, err = domain.NormalizeMaterial(domain.EntryMetadata, data)
	if err != nil {
		return err
	}
	metaEntry.Blob, err = put(bytes.NewReader(data))
	if err != nil {
		return err
	}
	entries[byID["problem"]] = metaEntry
	plan.Tree, err = (domain.ContentTree{Entries: entries}).Canonical()
	plan.FileCount = len(plan.Tree.Entries)
	return err
}
