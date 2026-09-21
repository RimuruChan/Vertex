package packages

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

type xmlElement struct {
	XMLName  xml.Name
	Attrs    []xml.Attr   `xml:",any,attr"`
	Children []xmlElement `xml:",any"`
	Text     string       `xml:",chardata"`
}

func (node xmlElement) attr(name string) string {
	for _, attr := range node.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}
func (node xmlElement) child(name string) xmlElement {
	for _, child := range node.Children {
		if child.XMLName.Local == name {
			return child
		}
	}
	return xmlElement{}
}
func (node xmlElement) children(name string) []xmlElement {
	values := []xmlElement{}
	for _, child := range node.Children {
		if child.XMLName.Local == name {
			values = append(values, child)
		}
	}
	return values
}

func readXML(data []byte) (xmlElement, error) {
	if len(data) > 1<<20 {
		return xmlElement{}, domain.ErrPackageTooBig
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, count, roots := 0, 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xmlElement{}, invalid("无效 XML：%s", err)
		}
		count++
		if count > 100000 {
			return xmlElement{}, invalid("XML 结构过于复杂")
		}
		switch current := token.(type) {
		case xml.Directive:
			return xmlElement{}, invalid("不允许 XML DTD 或外部实体")
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots > 1 {
					return xmlElement{}, invalid("XML 必须只包含一个根元素")
				}
			}
			attributes := map[xml.Name]bool{}
			for _, attribute := range current.Attr {
				if attributes[attribute.Name] {
					return xmlElement{}, invalid("XML 属性重复：%s", attribute.Name.Local)
				}
				attributes[attribute.Name] = true
			}
			depth++
			if depth > 64 {
				return xmlElement{}, invalid("XML 嵌套过深")
			}
		case xml.EndElement:
			depth--
		}
	}
	var root xmlElement
	if err := xml.Unmarshal(data, &root); err != nil {
		return root, err
	}
	return root, nil
}

func (value *importer) polygon() error {
	data, err := value.archive.read("problem.xml", 1<<20)
	if err != nil {
		return err
	}
	root, err := readXML(data)
	if err != nil {
		return err
	}
	if root.XMLName.Local != "problem" {
		return invalid("不是 Polygon problem.xml")
	}
	value.plan.Format = "polygon"
	value.metadata = domain.PackageMetadata{SchemaVersion: 1, Difficulty: 1, ResourceMode: "exact", Title: root.attr("short-name"), StatementLanguage: "en", TimeLimitMs: 1000, MemoryLimitKB: 262144, JudgeType: "normal", License: "unknown", Comparison: domain.OutputComparison{Kind: "tokens", CaseSensitive: true}, InputValidators: []string{}, TestOrder: []string{}, Requirements: []domain.MaterialRequirement{}}
	for _, name := range root.child("names").children("name") {
		if value.metadata.Title == "" || name.attr("language") == "english" {
			value.metadata.Title = name.attr("value")
			value.metadata.StatementLanguage = polygonLanguage(name.attr("language"))
		}
	}
	if value.metadata.Title == "" {
		value.metadata.Title = "导入题目"
	}
	judging := root.child("judging")
	for _, node := range judging.Children {
		if node.XMLName.Local != "testset" {
			value.require("polygon.judging."+node.XMLName.Local, "problem.xml", "未映射的评测配置："+node.XMLName.Local, "build")
		}
	}
	sets := judging.children("testset")
	if len(sets) == 0 {
		return invalid("Polygon 题包缺少测试集")
	}
	selected := sets[0]
	for _, set := range sets {
		if set.attr("name") == "tests" {
			selected = set
		}
	}
	if len(sets) > 1 {
		value.require("polygon.testsets", "problem.xml", "多个测试集需映射预测试/系统测试语义", "build")
	}
	for _, key := range []string{"input-file", "output-file"} {
		if name := judging.attr(key); name != "" && name != "stdin" && name != "stdout" {
			value.require("polygon.file_io", "problem.xml", "文件输入输出尚需执行协议适配", "build")
		}
	}
	if judging.attr("interactive") == "true" {
		value.metadata.JudgeType = "interactive"
	}
	if raw := strings.TrimSpace(selected.child("time-limit").Text); raw != "" {
		number, err := strconv.Atoi(raw)
		if err != nil || number <= 0 {
			return invalid("无效 Polygon 时限")
		}
		value.metadata.TimeLimitMs = number
	}
	if raw := strings.TrimSpace(selected.child("memory-limit").Text); raw != "" {
		number, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || number <= 0 || number%1024 != 0 || number/1024 > 1<<30 {
			return invalid("Polygon 内存限制不能精确表示为 KiB")
		}
		value.metadata.MemoryLimitKB = int(number / 1024)
	}
	statements := root.child("statements").children("statement")
	declaredStatements := map[string]bool{}
	for _, statement := range statements {
		declaredStatements[statement.attr("path")] = true
	}
	languages := map[string]bool{}
	for _, statement := range statements {
		language := polygonLanguage(statement.attr("language"))
		name := statement.attr("path")
		format := map[string]string{"application/x-tex": "tex", "application/pdf": "pdf", "text/markdown": "markdown", "text/x-markdown": "markdown"}[statement.attr("type")]
		if format == "" {
			continue
		}
		if languages[language] {
			continue
		}
		if !value.archive.has(name) {
			return invalid("缺少题面文件：%s", name)
		}
		attributes := map[string]string{"language": language, "format": format, "originFormat": "polygon"}
		if format == "tex" {
			attributes["dialect"] = "polygon"
		}
		if _, err := value.file(name, domain.EntryStatement, attributes); err != nil {
			return err
		}
		directory := path.Dir(name)
		if directory != "." {
			for _, asset := range value.archive.names {
				if !strings.HasPrefix(asset, directory+"/") || declaredStatements[asset] {
					continue
				}
				switch strings.ToLower(path.Ext(asset)) {
				case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".pdf":
					if _, err := value.file(asset, domain.EntryAsset, map[string]string{"visibility": "public", "purpose": "statement-support", "originFormat": "polygon"}); err != nil {
						return err
					}
				}
			}
		}
		languages[language] = true
	}
	if len(languages) == 0 {
		value.require("polygon.statement", "problem.xml", "未找到可编辑或保留的 TeX/PDF/Markdown 题面", "publish")
	}
	assets := root.child("assets")
	for _, name := range []string{"interactor", "grader"} {
		if assets.child(name).XMLName.Local != "" || root.child(name).XMLName.Local != "" {
			value.require("polygon."+name, "problem.xml", "尚未支持的程序协议："+name, "build")
		}
	}
	checker := assets.child("checker")
	if checker.XMLName.Local == "" {
		checker = root.child("checker")
	}
	if checker.XMLName.Local != "" {
		id, err := value.polygonProgram(checker.child("source"), "output-validator", nil)
		if err != nil {
			return err
		}
		value.metadata.OutputValidator = id
		value.metadata.Comparison.Kind = "testlib"
		if checker.attr("type") != "testlib" {
			value.require("polygon.checker", "problem.xml", "未知 Polygon checker 类型", "build")
		}
		if checker.child("testset").XMLName.Local != "" {
			value.require("polygon.checker_tests", "problem.xml", "checker 自测数据已保留，尚需映射", "build")
		}
	}
	validator := assets.child("validator")
	if validator.XMLName.Local == "" {
		validator = root.child("validator")
	}
	if validator.XMLName.Local != "" {
		id, err := value.polygonProgram(validator.child("source"), "input-validator", nil)
		if err != nil {
			return err
		}
		value.metadata.InputValidators = append(value.metadata.InputValidators, id)
		if validator.child("testset").XMLName.Local != "" {
			value.require("polygon.validator_tests", "problem.xml", "validator 自测数据已保留，尚需映射", "build")
		}
	}
	solutions := assets.child("solutions")
	if solutions.XMLName.Local == "" {
		solutions = root.child("solutions")
	}
	for _, solution := range solutions.children("solution") {
		tag := solution.attr("tag")
		expected := map[string]string{"main": "Accepted", "accepted": "Accepted", "wrong-answer": "Wrong Answer", "time-limit-exceeded": "Time Limit Exceeded", "runtime-error": "Runtime Error", "rejected": "Any Rejection"}[tag]
		if expected == "" {
			value.require("polygon.solution_tag", "problem.xml", "未知参考解标签："+tag, "build")
			expected = "Any Rejection"
		}
		id, err := value.polygonProgram(solution.child("source"), "solution", []string{expected})
		if err != nil {
			return err
		}
		if tag == "main" {
			value.metadata.MainSolution = id
		}
	}
	if value.metadata.MainSolution == "" {
		value.require("polygon.main_solution", "problem.xml", "未指定主参考解", "build")
	}
	tests := selected.child("tests").children("test")
	if raw := strings.TrimSpace(selected.child("test-count").Text); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil || count != len(tests) {
			return invalid("Polygon 测试点数量与描述不一致")
		}
	}
	for index, test := range tests {
		input, err := expandPolygonPattern(strings.TrimSpace(selected.child("input-path-pattern").Text), index+1)
		if err != nil {
			return err
		}
		answer, err := expandPolygonPattern(strings.TrimSpace(selected.child("answer-path-pattern").Text), index+1)
		if err != nil {
			return err
		}
		if !value.archive.has(input) || !value.archive.has(answer) {
			return invalid("需要包含已生成输入和答案的 Polygon 题包：%s", input)
		}
		in, err := value.file(input, domain.EntryInput, nil)
		if err != nil {
			return err
		}
		out, err := value.file(answer, domain.EntryAnswer, nil)
		if err != nil {
			return err
		}
		id := stableID("test", input)
		definition := domain.TestMaterial{SchemaVersion: 1, Name: fmt.Sprintf("%d", index+1), IsSample: test.attr("sample") == "true", Input: domain.TestInput{Kind: "file", Entry: in.ID}, Answer: domain.TestAnswer{Kind: "file", Entry: out.ID}}
		if test.attr("group") != "" || test.attr("points") != "" {
			value.require("polygon.test_groups", "problem.xml", "分组/分值配置已保留，尚需映射", "build")
		}
		if test.attr("method") == "generated" {
			value.issue("warning", "polygon.materialized", input, "采用包内已生成数据，原生成命令保留在 problem.xml")
		}
		if err := value.document(id, "vertex/tests/"+id+".json", domain.EntryTest, definition); err != nil {
			return err
		}
		value.metadata.TestOrder = append(value.metadata.TestOrder, id)
	}
	if err := value.preserveRemaining(); err != nil {
		return err
	}
	return value.document("problem", "vertex/problem.json", domain.EntryMetadata, value.metadata)
}

func (value *importer) polygonProgram(source xmlElement, role string, expected []string) (string, error) {
	name := source.attr("path")
	if name == "" || !value.archive.has(name) {
		return "", invalid("缺少 Polygon 程序源码：%s", name)
	}
	language := ""
	kind := source.attr("type")
	switch {
	case strings.HasPrefix(kind, "cpp"):
		language = "cpp"
	case strings.HasPrefix(kind, "c."):
		language = "c"
	case strings.HasPrefix(kind, "python"):
		language = "python"
	default:
		language = "unknown"
		value.require("polygon.language", name, "未映射的 Polygon 工具链："+kind, "build")
	}
	if strings.HasPrefix(kind, "cpp") && kind != "cpp.g++" && kind != "cpp.g++17" {
		value.require("polygon.toolchain", name, "需要匹配指定的 C++ 标准："+kind, "build")
	}
	protocol := "stdio"
	if role != "solution" && role != "generator" {
		protocol = "testlib"
	}
	directory := path.Dir(name)
	if directory == "." {
		directory = ""
	}
	definition := domain.ProgramMaterial{SchemaVersion: 1, Name: path.Base(name), Directory: directory, Role: role, Language: language, Protocol: protocol, Files: []string{}, ExpectedVerdicts: expected, Arguments: []string{}}
	for _, file := range value.archive.names {
		inside := directory == "" || strings.HasPrefix(file, directory+"/")
		if file != name && !(inside && (strings.HasSuffix(file, ".h") || strings.HasSuffix(file, ".hpp"))) {
			continue
		}
		entry, err := value.file(file, domain.EntrySource, map[string]string{"language": language, "originFormat": "polygon"})
		if err != nil {
			return "", err
		}
		definition.Files = append(definition.Files, entry.ID)
		if file == name {
			definition.EntryPoint = entry.ID
		}
	}
	id := stableID("program", name+":"+role)
	return id, value.document(id, "vertex/programs/"+id+".json", domain.EntryProgram, definition)
}

var polygonNumberPattern = regexp.MustCompile(`%0?([1-9][0-9]?)?d`)

func expandPolygonPattern(pattern string, index int) (string, error) {
	matches := polygonNumberPattern.FindAllStringSubmatchIndex(pattern, -1)
	if len(matches) != 1 {
		return "", invalid("不支持的测试路径模式：%s", pattern)
	}
	match := matches[0]
	width := 0
	if match[2] >= 0 {
		width, _ = strconv.Atoi(pattern[match[2]:match[3]])
	}
	if width > 8 {
		return "", invalid("测试路径编号宽度过大")
	}
	before, after := pattern[:match[0]], pattern[match[1]:]
	if strings.Contains(before+after, "%") {
		return "", invalid("路径模式含其他格式参数")
	}
	name := before + fmt.Sprintf("%0*d", width, index) + after
	if err := domain.ValidatePackagePath(name); err != nil {
		return "", err
	}
	return name, nil
}
func polygonLanguage(language string) string {
	if mapped := map[string]string{"english": "en", "russian": "ru", "chinese": "zh", "french": "fr", "spanish": "es"}[language]; mapped != "" {
		return mapped
	}
	return language
}
