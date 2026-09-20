package packages

import (
	"fmt"
	"io"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"go.yaml.in/yaml/v3"
)

// This is intentionally a data transfer, not a complete judging package. Only
// declared test inputs/answers and explicit per-case limits enter the archive.
func exportData(tree domain.ContentTree, options ExportOptions, read ReadContent) (map[string]exportFile, []domain.CompatibilityIssue, error) {
	entries := map[string]domain.TreeEntry{}
	for _, entry := range tree.Entries {
		entries[entry.ID] = entry
	}
	document := func(entry domain.TreeEntry) (*domain.MaterialView, error) {
		if entry.Blob.Bytes > 1<<20 {
			return nil, domain.ErrPackageTooBig
		}
		stream, err := read(entry.Blob)
		if err != nil {
			return nil, err
		}
		defer stream.Close()
		data, err := io.ReadAll(io.LimitReader(stream, (1<<20)+1))
		if err != nil {
			return nil, err
		}
		if err := domain.ValidateBlob(entry.Blob, data); err != nil {
			return nil, err
		}
		return domain.DecodeMaterial(entry, data)
	}
	meta, err := document(entries["problem"])
	if err != nil {
		return nil, nil, err
	}
	if meta.Metadata == nil || len(meta.Metadata.TestOrder) == 0 {
		return nil, nil, invalid("没有可导出的测试数据")
	}
	testCount := 0
	for _, entry := range tree.Entries {
		if entry.Kind == domain.EntryTest {
			testCount++
		}
	}
	if testCount != len(meta.Metadata.TestOrder) {
		return nil, nil, invalid("测试顺序没有覆盖全部测试点，请先修复再导出")
	}
	files := map[string]exportFile{}
	config := map[string]map[string]int{}
	for index, id := range meta.Metadata.TestOrder {
		entry, exists := entries[id]
		if !exists || entry.Kind != domain.EntryTest {
			return nil, nil, invalid("测试顺序引用不存在的测试")
		}
		view, err := document(entry)
		if err != nil {
			return nil, nil, err
		}
		test := view.Test
		for _, answer := range []bool{false, true} {
			suffix, sourceKind, reference, fileKind := "in", test.Input.Kind, test.Input.Entry, domain.EntryInput
			if answer {
				suffix, sourceKind, reference, fileKind = "out", test.Answer.Kind, test.Answer.Entry, domain.EntryAnswer
			}
			name := fmt.Sprintf("%04d.%s", index+1, suffix)
			if sourceKind == "file" {
				source, ok := entries[reference]
				if !ok || source.Kind != fileKind {
					return nil, nil, invalid("数据文件引用缺失或类型不正确")
				}
				ref := source.Blob
				files[name] = exportFile{ref: &ref}
			} else if options.ReadTest != nil {
				testID, isAnswer := id, answer
				files[name] = exportFile{open: func() (io.ReadCloser, error) { return options.ReadTest(testID, isAnswer) }}
			} else {
				return nil, nil, invalid("生成型数据需要匹配的成功检查")
			}
		}
		limits := map[string]int{}
		if test.TimeLimitMs > 0 {
			limits["timeLimit"] = test.TimeLimitMs
		}
		if test.MemoryLimitKB > 0 {
			limits["memoryLimit"] = test.MemoryLimitKB
		}
		if len(limits) > 0 {
			config[fmt.Sprintf("%04d.in", index+1)] = limits
		}
	}
	if len(config) > 0 {
		data, err := yaml.Marshal(config)
		if err != nil {
			return nil, nil, err
		}
		files["config.yml"] = exportFile{body: data}
	}
	issues := []domain.CompatibilityIssue{{Severity: "warning", Code: "data.only", Message: "仅导出按当前顺序排列的输入、答案和显式逐点限制。题面、程序、样例标记、分组计分及全局评测策略不包含在数据 ZIP 中；完整备份请使用原生归档。"}}
	return files, issues, nil
}
