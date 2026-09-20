package packages

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"go.yaml.in/yaml/v3"
)

type ReadContent func(domain.BlobRef) (io.ReadCloser, error)
type ExportOptions struct {
	Format   string
	Identity string
	ReadTest func(string, bool) (io.ReadCloser, error)
}
type ExportResult struct {
	Filename string
	Data     []byte
	Format   string
	Issues   []domain.CompatibilityIssue
}

var standardComponent = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,254}$`)

type exportFile struct {
	mode fs.FileMode
	ref  *domain.BlobRef
	body []byte
	open func() (io.ReadCloser, error)
}

func Export(tree domain.ContentTree, options ExportOptions, read ReadContent) (*ExportResult, error) {
	tree, err := tree.Canonical()
	if err != nil {
		return nil, err
	}
	files := map[string]exportFile{}
	issues := []domain.CompatibilityIssue{}
	if options.Format == "vertex" {
		manifest, err := json.Marshal(struct {
			SchemaVersion int                `json:"schemaVersion"`
			Tree          domain.ContentTree `json:"tree"`
		}{1, tree})
		if err != nil {
			return nil, err
		}
		files["vertex-package.json"] = exportFile{body: manifest}
		for _, entry := range tree.Entries {
			ref := entry.Blob
			files["blobs/"+ref.SHA256] = exportFile{ref: &ref}
		}
	} else if options.Format == "luogu-data" {
		files, issues, err = exportData(tree, options, read)
		if err != nil {
			return nil, err
		}
	} else {
		files, issues, err = exportKattis(tree, options, read)
		if err != nil {
			return nil, err
		}
	}
	var buffer limitedBuffer
	buffer.limit = MaxArchiveBytes
	writer := zip.NewWriter(&buffer)
	root := ""
	filename := "problem-vertex.zip"
	if options.Format == "luogu-data" {
		filename = "problem-data.zip"
	} else if options.Format != "vertex" {
		identity := options.Identity
		if identity == "" {
			identity, _ = tree.Hash()
		}
		digest := sha256.Sum256([]byte("vertex-package-root:" + identity))
		root = "p" + hex.EncodeToString(digest[:6]) + "/"
		filename = strings.TrimSuffix(root, "/") + ".zip"
	}
	names := sortedKeys(files)
	seenNames := map[string]bool{}
	for _, name := range names {
		folded := strings.ToLower(name)
		if seenNames[folded] {
			return nil, invalid("导出路径大小写冲突：%s", name)
		}
		seenNames[folded] = true
	}
	for _, name := range names {
		parts := strings.Split(strings.ToLower(name), "/")
		for index := 1; index < len(parts); index++ {
			if seenNames[strings.Join(parts[:index], "/")] {
				return nil, invalid("导出文件与目录冲突：%s", name)
			}
		}
	}
	remaining := MaxExpandedBytes
	for _, name := range names {
		if err := domain.ValidatePackagePath(name); err != nil {
			return nil, err
		}
		if root != "" {
			for _, component := range strings.Split(name, "/") {
				if !standardComponent.MatchString(component) {
					return nil, invalid("标准格式会忽略此文件名，请先重命名：%s", name)
				}
			}
		}
		item := files[name]
		header := &zip.FileHeader{Name: root + name, Method: zip.Deflate}
		header.SetMode(0644)
		if item.mode != 0 {
			header.SetMode(item.mode)
		}
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		destination, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		var source io.ReadCloser
		if item.ref != nil {
			source, err = read(*item.ref)
		} else if item.open != nil {
			source, err = item.open()
		} else {
			source = io.NopCloser(bytes.NewReader(item.body))
		}
		if err != nil {
			return nil, err
		}
		hash := sha256.New()
		size, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(source, remaining+1))
		source.Close()
		if err != nil {
			return nil, err
		}
		if size > remaining {
			return nil, domain.ErrPackageTooBig
		}
		remaining -= size
		if item.ref != nil && (size != item.ref.Bytes || hex.EncodeToString(hash.Sum(nil)) != item.ref.SHA256) {
			return nil, invalid("导出材料摘要不匹配：%s", name)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return &ExportResult{Data: buffer.Bytes(), Format: options.Format, Issues: issues, Filename: filename}, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int64
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if int64(len(data)) > b.limit-int64(b.Len()) {
		return 0, domain.ErrPackageTooBig
	}
	return b.Buffer.Write(data)
}

func exportKattis(tree domain.ContentTree, options ExportOptions, read ReadContent) (map[string]exportFile, []domain.CompatibilityIssue, error) {
	version := strings.TrimPrefix(options.Format, "kattis-")
	if version != "legacy" && version != "legacy-icpc" && version != "2025-09" && options.Format != "domjudge" {
		return nil, nil, invalid("未知导出格式：%s", options.Format)
	}
	if options.Format == "domjudge" {
		version = "legacy-icpc"
	}
	readDocument := func(ref domain.BlobRef) ([]byte, error) {
		if ref.Bytes > 1<<20 {
			return nil, domain.ErrPackageTooBig
		}
		reader, err := read(ref)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		if err != nil {
			return nil, err
		}
		if err := domain.ValidateBlob(ref, data); err != nil {
			return nil, err
		}
		return data, nil
	}
	metadataEntry := domain.TreeEntry{}
	entries := map[string]domain.TreeEntry{}
	for _, entry := range tree.Entries {
		entries[entry.ID] = entry
		if entry.ID == "problem" {
			metadataEntry = entry
		}
	}
	if metadataEntry.ID == "" {
		return nil, nil, invalid("缺少题目基本设置")
	}
	data, err := readDocument(metadataEntry.Blob)
	if err != nil {
		return nil, nil, err
	}
	view, err := domain.DecodeMaterial(metadataEntry, data)
	if err != nil {
		return nil, nil, err
	}
	meta := *view.Metadata
	inspection, _, err := domain.InspectMaterials(tree, readDocument)
	if err != nil {
		return nil, nil, err
	}
	if !inspection.CanBuild {
		return nil, nil, invalid("材料引用检查尚未通过，不能导出标准题包")
	}
	if len(inspection.PublicationIssues) > 0 {
		return nil, nil, invalid("目标格式尚不能完整表达当前评测规则：%s；可导出原生归档", inspection.PublicationIssues[0].Message)
	}
	if len(meta.Requirements) > 0 {
		return nil, nil, invalid("存在未解决的兼容要求；可先导出 Vertex 原生归档保留所有材料")
	}
	if meta.JudgeType != "normal" || meta.ResourceMode != "exact" {
		return nil, nil, invalid("标准题包导出要求传统题与精确资源限制")
	}
	files := map[string]exportFile{}
	issues := []domain.CompatibilityIssue{}
	config := map[string]any{"problem_format_version": version, "uuid": packageUUID(options.Identity), "name": meta.Title, "license": meta.License, "limits": map[string]any{"memory": meta.MemoryLimitKB / 1024}}
	if meta.MemoryLimitKB%1024 != 0 {
		return nil, nil, invalid("内存限制不能精确表示为 MiB")
	}
	if meta.Source != "" {
		config["source"] = meta.Source
	}
	if meta.RightsOwner != "" {
		config["rights_owner"] = meta.RightsOwner
	}
	if version == "2025-09" {
		config["type"] = "pass-fail"
		config["uuid"] = packageUUID(options.Identity)
		config["limits"].(map[string]any)["time_limit"] = float64(meta.TimeLimitMs) / 1000
		config["limits"].(map[string]any)["time_resolution"] = 0.001
		config["keywords"] = meta.Tags
	} else {
		config["validation"] = "default"
		config["keywords"] = strings.Join(meta.Tags, " ")
		files["domjudge-problem.ini"] = exportFile{body: []byte("timelimit = " + strconv.FormatFloat(float64(meta.TimeLimitMs)/1000, 'f', -1, 64) + "\n")}
		issues = append(issues, domain.CompatibilityIssue{Severity: "warning", Code: "legacy.timing", Path: "domjudge-problem.ini", Message: "绝对时限写入 DOMjudge 扩展，其他 legacy 消费者可能按参考解重新计算时限"})
	}
	flags := []string{}
	if meta.Comparison.Kind == "tokens" {
		flags = comparisonFlags(meta.Comparison)
	}
	if len(flags) > 0 {
		if version == "2025-09" {
			encoded, _ := yaml.Marshal(map[string]any{"output_validator_args": flags})
			files["data/secret/test_group.yaml"] = exportFile{body: encoded}
			files["data/sample/test_group.yaml"] = exportFile{body: encoded}
		} else {
			config["validator_flags"] = strings.Join(flags, " ")
		}
	}
	statements := 0
	namesByLanguage := map[string]string{}
	secretTests := 0
	statementDirectory := "problem_statement"
	if version == "2025-09" {
		statementDirectory = "statement"
	}
	polygonStatements := polygonLocations(tree, statementDirectory)
	for _, entry := range tree.Entries {
		if entry.Kind == domain.EntryStatement {
			language := entry.Attributes["language"]
			title := entry.Attributes["title"]
			if title == "" || language == meta.StatementLanguage {
				title = meta.Title
			}
			namesByLanguage[language] = title
			if location, ok := polygonStatements[entry.ID]; ok {
				wrapper, err := polygonWrapper(entry, location)
				if err != nil {
					return nil, nil, err
				}
				target := statementDirectory + "/problem." + language + ".tex"
				source := location.targetDirectory + "/" + path.Base(entry.Path)
				if _, exists := files[target]; exists {
					return nil, nil, invalid("Polygon 题面导出路径冲突")
				}
				if _, exists := files[source]; exists {
					return nil, nil, invalid("Polygon 题面导出路径冲突")
				}
				ref := entry.Blob
				files[target] = exportFile{body: []byte(wrapper)}
				files[source] = exportFile{ref: &ref}
				issues = append(issues, domain.CompatibilityIssue{Severity: "warning", Code: "statement.polygon_layout", Path: entry.Path, Message: "Polygon 原始题面保持不变，通过兼容排版封装导出；原文样例保留，目标渲染器可能另外附加数据样例"})
				statements++
				continue
			}
			if entry.Attributes["format"] != "pdf" && !strings.HasPrefix(entry.Path, "statement/") && !strings.HasPrefix(entry.Path, "problem_statement/") {
				return nil, nil, invalid("题面需放在标准题面目录中以保留相对引用")
			}
			format := entry.Attributes["format"]
			extension := map[string]string{"markdown": "md", "tex": "tex", "pdf": "pdf"}[format]
			if extension == "" {
				return nil, nil, invalid("未知题面格式")
			}
			if version != "2025-09" && format == "markdown" {
				body, err := readDocument(entry.Blob)
				if err != nil {
					return nil, nil, err
				}
				converted, err := markdownToTeX(body, title, entry.Path, entries)
				if err != nil {
					return nil, nil, invalid("%s：%v", entry.Path, err)
				}
				target := statementDirectory + "/problem." + entry.Attributes["language"] + ".tex"
				if _, exists := files[target]; exists {
					return nil, nil, invalid("题面导出路径冲突：%s", target)
				}
				files[target] = exportFile{body: converted.data}
				for name, ref := range converted.images {
					ref := ref
					if prior, exists := files[statementDirectory+"/"+name]; exists && (prior.ref == nil || *prior.ref != ref) {
						return nil, nil, invalid("图片导出路径冲突：%s", name)
					}
					files[statementDirectory+"/"+name] = exportFile{ref: &ref}
				}
				issues = append(issues, domain.CompatibilityIssue{Severity: "warning", Code: "statement.converted", Path: entry.Path, Message: "Markdown 已转换为 TeX；原始材料仍保留在工作副本，可用原生归档备份"})
				if converted.unicode {
					issues = append(issues, domain.CompatibilityIssue{Severity: "warning", Code: "statement.fonts", Path: entry.Path, Message: "TeX 包含非 ASCII 文字；目标渲染器需要对应的 Unicode 字体支持"})
				}
				if converted.links {
					issues = append(issues, domain.CompatibilityIssue{Severity: "warning", Code: "statement.links", Path: entry.Path, Message: "链接以可阅读的文字地址保留，未生成 PDF 超链接"})
				}
				statements++
				continue
			}
			directory := "problem_statement"
			if version == "2025-09" {
				directory = "statement"
			}
			ref := entry.Blob
			target := directory + "/problem." + entry.Attributes["language"] + "." + extension
			if _, exists := files[target]; exists {
				return nil, nil, invalid("题面导出路径冲突：%s", target)
			}
			files[target] = exportFile{ref: &ref}
			if version == "2025-09" && format == "markdown" {
				body, err := readDocument(entry.Blob)
				if err != nil {
					return nil, nil, err
				}
				normalized, err := standardMarkdown(body, title, entry.Path, entries)
				if err != nil {
					return nil, nil, err
				}
				files[target] = exportFile{body: normalized}
				if !bytes.Equal(body, normalized) {
					issues = append(issues, domain.CompatibilityIssue{Severity: "warning", Code: "statement.title", Path: entry.Path, Message: "标准 Markdown 由题包元数据提供标题，导出时移除重复的首行标题；工作副本原文不变"})
				}
			}
			statements++
		} else if entry.Kind == domain.EntryAsset {
			if visibility := entry.Attributes["visibility"]; visibility != "" && visibility != "public" {
				return nil, nil, invalid("私有附件不能导出到标准公开目录：%s；可使用原生归档", entry.Path)
			}
			ref := entry.Blob
			mapped := false
			for _, location := range polygonStatements {
				if name := location.resource(entry.Path); name != "" {
					if _, exists := files[name]; exists {
						return nil, nil, invalid("Polygon 题面附件路径冲突：%s", name)
					}
					files[name] = exportFile{ref: &ref}
					mapped = true
				}
			}
			if mapped {
				continue
			}
			name := entry.Path
			if strings.HasPrefix(name, "statement/") || strings.HasPrefix(name, "problem_statement/") {
				name = statementDirectory + "/" + strings.TrimPrefix(strings.TrimPrefix(name, "problem_statement/"), "statement/")
			} else if !strings.HasPrefix(name, "attachments/") {
				name = "attachments/" + path.Base(name)
			}
			if version == "2025-09" && strings.HasPrefix(name, "attachments/") && strings.Contains(strings.TrimPrefix(name, "attachments/"), "/") {
				return nil, nil, invalid("附件目录需先打包为 ZIP；代码模板目录尚未实现语义映射")
			}
			if _, exists := files[name]; exists {
				return nil, nil, invalid("附件导出路径冲突")
			}
			files[name] = exportFile{ref: &ref}
		} else if entry.Kind == domain.EntryResource && (strings.HasPrefix(entry.Path, "statement/") || strings.HasPrefix(entry.Path, "problem_statement/")) {
			if entry.Attributes["purpose"] == "statement-support" && (path.Ext(entry.Path) == ".tex" || path.Ext(entry.Path) == ".sty" || path.Ext(entry.Path) == ".cls") {
				name := statementDirectory + "/" + strings.TrimPrefix(strings.TrimPrefix(entry.Path, "problem_statement/"), "statement/")
				if _, exists := files[name]; exists {
					return nil, nil, invalid("题面依赖路径冲突：%s", name)
				}
				ref := entry.Blob
				files[name] = exportFile{ref: &ref}
				continue
			}
			return nil, nil, invalid("题面目录中的私有材料需明确设为公开附件后导出：%s；可使用原生归档保留", entry.Path)
		}
	}
	if statements == 0 {
		return nil, nil, invalid("缺少题面")
	}
	if version == "2025-09" && (len(namesByLanguage) != 1 || namesByLanguage["en"] == "") {
		config["name"] = namesByLanguage
	}
	for index, id := range meta.TestOrder {
		entry, ok := entries[id]
		if !ok || entry.Kind != domain.EntryTest {
			return nil, nil, invalid("测试顺序引用无效材料")
		}
		data, err := readDocument(entry.Blob)
		if err != nil {
			return nil, nil, err
		}
		document, err := domain.DecodeMaterial(entry, data)
		if err != nil {
			return nil, nil, err
		}
		test := document.Test
		if test.Group != "" || test.IsPretest || test.TimeLimitMs != 0 || test.MemoryLimitKB != 0 {
			return nil, nil, invalid("测试分组、预测试或独立限制需要额外格式映射")
		}
		directory := "data/secret"
		if test.IsSample {
			directory = "data/sample"
		} else {
			secretTests++
		}
		for _, answer := range []bool{false, true} {
			suffix := ".in"
			kind, reference := test.Input.Kind, test.Input.Entry
			if answer {
				suffix = ".ans"
				kind, reference = test.Answer.Kind, test.Answer.Entry
			}
			name := fmt.Sprintf("%s/%04d%s", directory, index+1, suffix)
			if kind == "file" {
				source, ok := entries[reference]
				if !ok {
					return nil, nil, invalid("测试数据引用缺失")
				}
				ref := source.Blob
				files[name] = exportFile{ref: &ref}
			} else if options.ReadTest != nil {
				testID, isAnswer := id, answer
				files[name] = exportFile{open: func() (io.ReadCloser, error) { return options.ReadTest(testID, isAnswer) }}
			} else {
				return nil, nil, invalid("生成型测试需要匹配的成功检查产物才能导出")
			}
		}
	}
	if err := exportValidation(tree, version, files, readDocument); err != nil {
		return nil, nil, err
	}
	if secretTests == 0 {
		return nil, nil, invalid("标准题包至少需要一个秘密测试点")
	}
	selected := map[string]string{}
	for _, id := range meta.InputValidators {
		selected[id] = "input_validators"
	}
	if len(meta.InputValidators) == 0 {
		return nil, nil, invalid("标准题包需要输入校验器")
	}
	if meta.Comparison.Kind == "kattis" || meta.Comparison.Kind == "testlib" {
		if meta.OutputValidator == "" {
			return nil, nil, invalid("缺少输出校验器")
		}
		selected[meta.OutputValidator] = "output_validators"
		if version == "2025-09" {
			selected[meta.OutputValidator] = "output_validator"
		} else {
			config["validation"] = "custom"
		}
	}
	if meta.Comparison.Kind == "exact" {
		directory := "output_validators/vertex_exact"
		if version == "2025-09" {
			directory = "output_validator"
		} else {
			config["validation"] = "custom"
		}
		files[directory+"/main.cpp"] = exportFile{body: exactValidator}
		issues = append(issues, domain.CompatibilityIssue{Severity: "warning", Code: "validator.exact_adapter", Path: directory, Message: "逐字节比较已导出为独立 Kattis 输出校验器，不忽略空白、大小写或末尾换行"})
	}
	accepted := 0
	for _, entry := range tree.Entries {
		if entry.Kind != domain.EntryProgram {
			continue
		}
		data, err := readDocument(entry.Blob)
		if err != nil {
			return nil, nil, err
		}
		document, err := domain.DecodeMaterial(entry, data)
		if err != nil {
			return nil, nil, err
		}
		program := document.Program
		directory, chosen := selected[entry.ID]
		if program.Role == "solution" {
			if len(program.ExpectedVerdicts) != 1 {
				return nil, nil, invalid("参考解的多重预期判定需要额外映射")
			}
			category := map[string]string{"Accepted": "accepted", "Wrong Answer": "wrong_answer", "Time Limit Exceeded": "time_limit_exceeded", "Runtime Error": "run_time_error"}[program.ExpectedVerdicts[0]]
			if category == "" {
				return nil, nil, invalid("目标格式无法表示此参考解预期")
			}
			directory = "submissions/" + category
			chosen = true
			if category == "accepted" {
				accepted++
			}
		}
		if program.Role == "generator" {
			directory = "generators"
			chosen = true
		}
		if !chosen {
			continue
		}
		if program.Role == "input-validator" || program.Role == "output-validator" {
			if program.Protocol != "kattis" && program.Protocol != "testlib" {
				return nil, nil, invalid("校验器需要 Kattis 调用协议适配")
			}
		}
		if program.Language != "cpp" && program.Language != "c" && program.Language != "python" {
			return nil, nil, invalid("目标语言尚未提供互操作验证")
		}
		if len(program.Arguments) > 0 {
			return nil, nil, invalid("程序参数需要映射到目标格式配置")
		}
		prefix := directory + "/p" + strings.TrimPrefix(entry.ID, "program-")
		if version == "2025-09" && program.Role == "output-validator" {
			prefix = directory
		}
		for _, id := range program.Files {
			source, ok := entries[id]
			if !ok {
				return nil, nil, invalid("程序文件引用缺失")
			}
			name := source.Path
			if program.Directory != "" {
				root := program.Directory + "/"
				if !strings.HasPrefix(name, root) {
					return nil, nil, invalid("程序文件不在声明目录中")
				}
				name = strings.TrimPrefix(name, root)
			}
			ref := source.Blob
			files[prefix+"/"+name] = exportFile{ref: &ref}
		}
		if (program.Role == "input-validator" || program.Role == "output-validator") && program.Protocol == "testlib" {
			if err := adaptTestlib(files, prefix, *program, entries); err != nil {
				return nil, nil, err
			}
			issues = append(issues, domain.CompatibilityIssue{Severity: "warning", Code: "validator.testlib_adapter", Path: prefix, Message: "testlib 程序以固定版本头文件和 Kattis 协议适配器导出；原始源码保留，判定器故障不会当作通过"})
		}
	}
	if accepted == 0 {
		return nil, nil, invalid("标准题包需要正确参考解")
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		return nil, nil, err
	}
	files["problem.yaml"] = exportFile{body: encoded}
	return files, issues, nil
}

func comparisonFlags(policy domain.OutputComparison) []string {
	result := []string{}
	if policy.CaseSensitive {
		result = append(result, "case_sensitive")
	}
	if policy.SpaceSensitive {
		result = append(result, "space_change_sensitive")
	}
	if policy.FloatingPoint {
		result = append(result, "float_absolute_tolerance", strconv.FormatFloat(policy.AbsoluteTolerance, 'g', -1, 64), "float_relative_tolerance", strconv.FormatFloat(policy.RelativeTolerance, 'g', -1, 64))
	}
	return result
}
func packageUUID(identity string) string {
	digest := sha256.Sum256([]byte("vertex-exchange:" + identity))
	digest[6] = (digest[6] & 15) | 80
	digest[8] = (digest[8] & 63) | 128
	hexed := hex.EncodeToString(digest[:16])
	return hexed[:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" + hexed[16:20] + "-" + hexed[20:]
}
