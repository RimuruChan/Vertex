package packages

import (
	"encoding/json"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

var statementName = regexp.MustCompile(`^problem(?:\.([a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*))?\.(md|tex|pdf)$`)
var latexName = regexp.MustCompile(`\\problemname\{([^{}]+)\}`)

func (value *importer) kattis(options domain.ImportOptions) error {
	data, err := value.archive.read("problem.yaml", 1<<20)
	if err != nil {
		return err
	}
	config, err := readYAML(data)
	if err != nil {
		return err
	}
	for _, key := range []string{"type", "keywords", "validator_flags"} {
		if raw, exists := config[key]; exists && !validStringList(raw) {
			return invalid("%s 必须为字符串或字符串列表", key)
		}
	}
	version := text(config["problem_format_version"])
	if version == "" {
		version = "legacy"
	}
	if version != "legacy" && version != "legacy-icpc" && version != "2025-09" {
		return invalid("不支持的 Kattis 格式版本：%s", version)
	}
	value.plan.Format = "kattis-" + version
	value.metadata = domain.PackageMetadata{SchemaVersion: 1, Difficulty: 1, ResourceMode: "exact", Requirements: []domain.MaterialRequirement{}, Title: text(config["name"]), StatementLanguage: "en", TimeLimitMs: 1000, MemoryLimitKB: 262144, JudgeType: "normal", License: text(config["license"]), RightsOwner: text(config["rights_owner"]), Source: sourceName(config["source"]), Tags: []string{}, InputValidators: []string{}, TestOrder: []string{}, Comparison: domain.OutputComparison{Kind: "tokens"}}
	if value.metadata.License == "" {
		value.metadata.License = "unknown"
	}
	if names, ok := config["name"].(map[string]any); ok {
		keys := sortedKeys(names)
		if len(keys) > 0 {
			value.metadata.StatementLanguage = keys[0]
			value.metadata.Title = text(names[keys[0]])
		}
	}
	if value.metadata.Title == "" {
		value.metadata.Title = "导入题目"
	}
	for _, kind := range stringList(config["type"]) {
		if kind != "pass-fail" {
			value.require("kattis.type", "problem.yaml", "当前执行器尚不能完整处理题型 "+kind, "build")
			if kind == "interactive" {
				value.metadata.JudgeType = "interactive"
			}
		}
	}
	known := map[string]bool{}
	for _, key := range strings.Fields("problem_format_version name uuid version author source source_url license rights_owner keywords validation validator_flags limits type credits") {
		known[key] = true
	}
	for _, key := range sortedKeys(config) {
		if !known[key] {
			value.require("kattis.metadata."+key, "problem.yaml", "保留尚未映射的配置："+key, "build")
		}
	}
	value.metadata.Tags = stringList(config["keywords"])
	if version == "2025-09" {
		for _, key := range []string{"validation", "validator_flags"} {
			if _, exists := config[key]; exists {
				value.require("kattis.deprecated_metadata", "problem.yaml", "2025-09 使用逐点/组配置，不能沿用 "+key, "build")
			}
		}
	}
	limits, _ := config["limits"].(map[string]any)
	timeLimit := 0
	for _, key := range sortedKeys(limits) {
		switch key {
		case "memory":
			number, ok := integer(limits[key])
			if !ok || number <= 0 || number > 1<<20 {
				return invalid("memory 必须为正整数 MiB")
			}
			value.metadata.MemoryLimitKB = number * 1024
		case "time_limit":
			if version != "2025-09" {
				value.require("kattis.legacy_time", "problem.yaml", "legacy 标准未定义 time_limit 字段", "build")
			}
			seconds, ok := number(limits[key])
			if !ok || seconds <= 0 || seconds > 3600 || math.Abs(seconds*1000-math.Round(seconds*1000)) > 1e-7 {
				return invalid("time_limit 无法精确表示为毫秒")
			}
			timeLimit = int(math.Round(seconds * 1000))
		case "time_multiplier", "time_safety_margin", "time_multipliers", "time_resolution":
			value.issue("warning", "timing.calibration", "problem.yaml", "保留原时限校准参数；导入采用预检中明确选择的固定时限")
		default:
			value.require("kattis.limit."+key, "problem.yaml", "尚未完整实现指定的资源约束："+key, "build")
		}
	}
	if value.archive.has("domjudge-problem.ini") {
		ini, err := value.archive.read("domjudge-problem.ini", 64<<10)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(ini), "\n") {
			key, raw, ok := strings.Cut(line, "=")
			if !ok || strings.TrimSpace(key) != "timelimit" {
				continue
			}
			seconds, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(raw), "\"'"), 64)
			if err != nil || seconds <= 0 || seconds > 3600 || math.Abs(seconds*1000-math.Round(seconds*1000)) > 1e-7 {
				return invalid("无效的 DOMjudge 固定时限")
			}
			if timeLimit != 0 && timeLimit != int(math.Round(seconds*1000)) {
				return invalid("problem.yaml 与 DOMjudge 时限冲突")
			}
			timeLimit = int(math.Round(seconds * 1000))
		}
	}
	if timeLimit == 0 && options.TimeLimitMs > 0 {
		timeLimit = options.TimeLimitMs
		value.issue("warning", "timing.selected", "problem.yaml", "原包未给出绝对时限，采用明确选择的固定时限")
	}
	if timeLimit == 0 {
		value.require("timing.unspecified", "problem.yaml", "需要在导入预检中明确指定固定时限", "build")
	} else {
		value.metadata.TimeLimitMs = timeLimit
	}
	validation := strings.Fields(text(config["validation"]))
	custom := false
	for _, word := range validation {
		switch word {
		case "default":
		case "custom":
			custom = true
		default:
			value.require("kattis.validation", "problem.yaml", "当前未支持 validation 模式："+word, "build")
		}
	}
	if version == "2025-09" {
		for _, name := range value.archive.names {
			custom = custom || strings.HasPrefix(name, "output_validator/")
		}
	}
	if custom {
		value.metadata.Comparison.Kind = "kattis"
	}
	flags := stringList(config["validator_flags"])
	if !custom {
		if err := parseComparisonFlags(flags, &value.metadata.Comparison); err != nil {
			value.require("kattis.validator_flags", "problem.yaml", err.Error(), "build")
		}
	}
	statementDirectory := "problem_statement/"
	if version == "2025-09" {
		statementDirectory = "statement/"
	}
	languages := []string{}
	rootFlags := ""
	rootFlagPaths := map[string]bool{}
	for _, name := range value.archive.names {
		if !strings.HasPrefix(name, statementDirectory) {
			continue
		}
		relative := strings.TrimPrefix(name, statementDirectory)
		match := statementName.FindStringSubmatch(relative)
		if match == nil {
			continue
		}
		language := match[1]
		if language == "" {
			language = "en"
		}
		format := match[2]
		if format == "md" {
			format = "markdown"
		}
		if version != "2025-09" && format == "markdown" {
			value.require("kattis.statement_format", name, "legacy 题面必须使用 TeX 或 PDF", "publish")
		}
		attributes := map[string]string{"language": language, "format": format, "originFormat": value.plan.Format}
		if names, ok := config["name"].(map[string]any); ok {
			if title := text(names[language]); title != "" {
				attributes["title"] = title
			}
		}
		if _, err := value.file(name, domain.EntryStatement, attributes); err != nil {
			return err
		}
		languages = append(languages, language)
		if value.metadata.Title == "导入题目" && format == "tex" {
			body, err := value.archive.read(name, 1<<20)
			if err != nil {
				return err
			}
			if found := latexName.FindSubmatch(body); found != nil {
				value.metadata.Title = string(found[1])
			}
		}
	}
	if len(languages) == 0 {
		value.require("statement.missing", statementDirectory, "没有找到标准格式题面", "publish")
	} else {
		sort.Strings(languages)
		found := false
		for _, language := range languages {
			found = found || language == value.metadata.StatementLanguage
		}
		if !found {
			value.metadata.StatementLanguage = languages[0]
		}
	}
	validators, err := value.programs("input_validators/", "input-validator", "kattis", nil, false)
	if err != nil {
		return err
	}
	value.metadata.InputValidators = validators
	if len(validators) == 0 {
		value.require("kattis.input_validator", "input_validators/", "标准题包缺少输入校验器", "build")
	}
	outputDirectory := "output_validators/"
	if version == "2025-09" {
		outputDirectory = "output_validator/"
	}
	checkers, err := value.programs(outputDirectory, "output-validator", "kattis", nil, version == "2025-09")
	if err != nil {
		return err
	}
	if custom {
		if len(checkers) != 1 {
			value.require("kattis.output_validators", outputDirectory, "当前要求明确选择一个输出校验器", "build")
		} else {
			value.metadata.OutputValidator = checkers[0]
		}
	}
	if custom && len(flags) > 0 {
		value.require("kattis.custom_flags", "problem.yaml", "需映射自定义校验器参数后再检查", "build")
	}
	for _, item := range []struct{ dir, verdict string }{{"accepted", "Accepted"}, {"wrong_answer", "Wrong Answer"}, {"time_limit_exceeded", "Time Limit Exceeded"}, {"run_time_error", "Runtime Error"}, {"runtime_error", "Runtime Error"}, {"rejected", "Any Rejection"}} {
		ids, err := value.programs("submissions/"+item.dir+"/", "solution", "stdio", []string{item.verdict}, false)
		if err != nil {
			return err
		}
		if item.dir == "accepted" && len(ids) > 0 {
			value.metadata.MainSolution = ids[0]
		}
	}
	if value.metadata.MainSolution == "" {
		value.require("kattis.reference", "submissions/accepted/", "题包没有可用的正确参考解", "build")
	}
	if err := value.validationCases(version); err != nil {
		return err
	}
	for _, name := range value.archive.names {
		if (strings.HasPrefix(name, "data/sample/") || strings.HasPrefix(name, "data/secret/")) && strings.HasSuffix(name, ".in") {
			answer := strings.TrimSuffix(name, ".in") + ".ans"
			if !value.archive.has(answer) {
				return invalid("测试点缺少 .ans 文件：%s", name)
			}
			inputEntry, err := value.file(name, domain.EntryInput, nil)
			if err != nil {
				return err
			}
			answerEntry, err := value.file(answer, domain.EntryAnswer, nil)
			if err != nil {
				return err
			}
			id := stableID("test", strings.TrimSuffix(name, ".in"))
			test := domain.TestMaterial{SchemaVersion: 1, Name: path.Base(strings.TrimSuffix(name, ".in")), IsSample: strings.HasPrefix(name, "data/sample/"), Input: domain.TestInput{Kind: "file", Entry: inputEntry.ID}, Answer: domain.TestAnswer{Kind: "file", Entry: answerEntry.ID}}
			if err := value.document(id, "vertex/tests/"+id+".json", domain.EntryTest, test); err != nil {
				return err
			}
			value.metadata.TestOrder = append(value.metadata.TestOrder, id)
		}
		if strings.HasPrefix(name, "data/") && (strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			mapped, err := value.validationConfiguration(name)
			if err != nil {
				return err
			}
			if version == "2025-09" && (name == "data/sample/test_group.yaml" || name == "data/secret/test_group.yaml") && value.metadata.Comparison.Kind == "tokens" {
				data, err := value.archive.read(name, 1<<20)
				if err != nil {
					return err
				}
				group, err := readYAML(data)
				if err != nil {
					return err
				}
				if len(group) == 1 && group["output_validator_args"] != nil {
					if !validStringList(group["output_validator_args"]) {
						return invalid("output_validator_args 必须为字符串列表")
					}
					args := stringList(group["output_validator_args"])
					encoded, _ := json.Marshal(args)
					if rootFlags != "" && rootFlags != string(encoded) {
						value.require("kattis.comparison_groups", name, "样例和秘密数据使用不同的比较参数，当前须单独映射", "build")
					} else if err := parseComparisonFlags(args, &value.metadata.Comparison); err != nil {
						value.require("kattis.validator_flags", name, err.Error(), "build")
					}
					rootFlags = string(encoded)
					rootFlagPaths[name] = true
					mapped = true
				}
			}
			if !mapped {
				value.require("kattis.test_configuration", name, "测试组或逐点配置已保留，须完成语义映射后才能执行", "build")
			}
		}
		if strings.HasPrefix(name, "data/") && ((strings.HasSuffix(name, ".out") && validationMode(name) == "") || strings.HasSuffix(name, ".interaction") || strings.Contains(name, ".files/")) {
			value.require("kattis.presentation_override", name, "共享样例覆盖或逐点附加文件需映射后才能发布", "publish")
		}
		for _, prefix := range []string{"static_validator/", "include/", "graders/"} {
			if strings.HasPrefix(name, prefix) {
				value.require("kattis.additional_material", name, "附加验证或执行材料已保留，当前尚未执行其语义", "build")
				break
			}
		}
	}
	if err := value.preserveRemaining(); err != nil {
		return err
	}
	if version == "2025-09" {
		entries := map[string]domain.TreeEntry{}
		for _, entry := range value.plan.Tree.Entries {
			entries[entry.ID] = entry
		}
		for _, entry := range value.plan.Tree.Entries {
			if entry.Kind == domain.EntryStatement && entry.Attributes["format"] == "markdown" {
				body, err := value.archive.read(entry.Path, 1<<20)
				if err != nil {
					return err
				}
				if _, err := standardMarkdown(body, value.metadata.Title, entry.Path, entries); err != nil {
					value.require("kattis.markdown_presentation", entry.Path, err.Error(), "publish")
				}
			}
			if strings.HasPrefix(entry.Path, "attachments/") && strings.Contains(strings.TrimPrefix(entry.Path, "attachments/"), "/") {
				value.require("kattis.attachment_directory", entry.Path, "附件子目录需作为 ZIP 文件提供；代码模板语义尚未映射", "publish")
			}
		}
	}
	if rootFlags != "" && rootFlags != "[]" {
		hasSamples, hasSecrets := false, false
		for _, name := range value.archive.names {
			if strings.HasSuffix(name, ".in") {
				hasSamples = hasSamples || strings.HasPrefix(name, "data/sample/")
				hasSecrets = hasSecrets || strings.HasPrefix(name, "data/secret/")
			}
		}
		if hasSamples && hasSecrets && (!rootFlagPaths["data/sample/test_group.yaml"] || !rootFlagPaths["data/secret/test_group.yaml"]) {
			value.require("kattis.comparison_groups", "data/", "样例与秘密数据比较参数不同，需分别映射", "build")
		}
	}
	return value.document("problem", "vertex/problem.json", domain.EntryMetadata, value.metadata)
}

func (value *importer) programs(prefix, role, protocol string, expected []string, single bool) ([]string, error) {
	groups := map[string][]string{}
	for _, name := range value.archive.names {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		relative := strings.TrimPrefix(name, prefix)
		first, _, nested := strings.Cut(relative, "/")
		key := name
		if single {
			key = strings.TrimSuffix(prefix, "/")
		} else if nested {
			key = prefix + first
		}
		groups[key] = append(groups[key], name)
	}
	ids := []string{}
	for _, group := range sortedKeys(groups) {
		if id, recognized, err := value.importTestlibAdapter(group, role); recognized {
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
			continue
		}
		files := groups[group]
		sort.Strings(files)
		sources := []string{}
		language := ""
		entryName := ""
		for _, name := range files {
			ext := strings.ToLower(path.Ext(name))
			lang := ""
			switch ext {
			case ".cpp", ".cc", ".cxx":
				lang = "cpp"
			case ".c":
				lang = "c"
			case ".py":
				lang = "python"
			case ".java":
				lang = "java"
			case ".rs":
				lang = "rust"
			}
			if path.Base(name) == "build" || path.Base(name) == "run" {
				value.require("program.script", name, "自定义 build/run 脚本已保留，尚不自动执行", "build")
			}
			if lang != "" {
				if language != "" && language != lang {
					value.require("program.mixed_languages", group, "程序包含多种语言，须明确构建方式", "build")
				}
				language = lang
				sources = append(sources, name)
				if entryName == "" || path.Base(name) == "main.py" || path.Base(name) == "__main__.py" || path.Base(name) == "main.cpp" {
					entryName = name
				}
			}
		}
		if len(sources) == 0 {
			continue
		}
		if language != "cpp" && language != "c" && language != "python" {
			value.require("program.language", group, "当前 Worker 未配置语言："+language, "build")
		}
		if language == "python" && len(sources) > 1 && path.Base(entryName) != "__main__.py" && path.Base(entryName) != "main.py" {
			value.require("program.entrypoint", group, "多文件 Python 程序需要明确入口", "build")
		}
		directory := group
		if value.archive.has(group) {
			directory = path.Dir(group)
		}
		definition := domain.ProgramMaterial{SchemaVersion: 1, Name: path.Base(group), Directory: directory, Role: role, Language: language, Protocol: protocol, Files: []string{}, ExpectedVerdicts: expected, Arguments: []string{}}
		for _, name := range files {
			entry, err := value.file(name, domain.EntrySource, map[string]string{"language": language, "originFormat": value.plan.Format})
			if err != nil {
				return nil, err
			}
			definition.Files = append(definition.Files, entry.ID)
			if name == entryName {
				definition.EntryPoint = entry.ID
			}
		}
		id := stableID("program", group)
		if err := value.document(id, "vertex/programs/"+id+".json", domain.EntryProgram, definition); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func parseComparisonFlags(flags []string, policy *domain.OutputComparison) error {
	seen := map[string]bool{}
	for index := 0; index < len(flags); index++ {
		flag := flags[index]
		if seen[flag] {
			return invalid("校验参数重复：%s", flag)
		}
		seen[flag] = true
		switch flag {
		case "case_sensitive":
			policy.CaseSensitive = true
		case "space_change_sensitive":
			policy.SpaceSensitive = true
		case "float_tolerance", "float_absolute_tolerance", "float_relative_tolerance":
			if index+1 >= len(flags) {
				return invalid("缺少浮点误差值")
			}
			index++
			tolerance, err := strconv.ParseFloat(flags[index], 64)
			if err != nil || math.IsNaN(tolerance) || math.IsInf(tolerance, 0) || tolerance < 0 {
				return invalid("无效浮点误差")
			}
			policy.FloatingPoint = true
			if flag != "float_relative_tolerance" {
				policy.AbsoluteTolerance = tolerance
			}
			if flag != "float_absolute_tolerance" {
				policy.RelativeTolerance = tolerance
			}
		default:
			return invalid("未识别校验参数：%s", flag)
		}
	}
	if seen["float_tolerance"] && (seen["float_absolute_tolerance"] || seen["float_relative_tolerance"]) {
		return invalid("float_tolerance 与单独误差参数不能同时使用")
	}
	return nil
}
func text(value any) string { result, _ := value.(string); return result }
func number(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint64:
		return float64(v), v <= 1<<53
	case float64:
		return v, !math.IsNaN(v) && !math.IsInf(v, 0)
	}
	return 0, false
}
func integer(value any) (int, bool) {
	v, ok := number(value)
	return int(v), ok && v >= 0 && v <= 1<<30 && math.Trunc(v) == v
}
func stringList(value any) []string {
	if s, ok := value.(string); ok {
		return strings.Fields(s)
	}
	out := []string{}
	if values, ok := value.([]any); ok {
		for _, v := range values {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func validStringList(value any) bool {
	if _, ok := value.(string); ok {
		return true
	}
	if values, ok := value.([]any); ok {
		for _, item := range values {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	}
	return false
}
func sourceName(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	if m, ok := value.(map[string]any); ok {
		return text(m["name"])
	}
	return ""
}
func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
