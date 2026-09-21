package domain

import (
	"encoding/json"
	"slices"
	"strings"
)

const CheckPolicyVersion = "vertex-authoring-4"

type MaterialIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	EntryID  string `json:"entryId,omitempty"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

// CanBuild reports structural readiness, never successful execution.
type MaterialInspection struct {
	ValidationCount   int             `json:"validationCount"`
	Revision          *int64          `json:"revision,omitempty"`
	ETag              string          `json:"etag,omitempty"`
	TreeHash          string          `json:"treeHash"`
	DataHash          string          `json:"dataHash,omitempty"`
	PolicyVersion     string          `json:"policyVersion"`
	CanBuild          bool            `json:"canBuild"`
	ProgramCount      int             `json:"programCount"`
	TestCount         int             `json:"testCount"`
	SampleCount       int             `json:"sampleCount"`
	Issues            []MaterialIssue `json:"issues"`
	PublicationIssues []MaterialIssue `json:"publicationIssues"`
}

type SnapshotProgram struct {
	ID         string          `json:"id"`
	Definition ProgramMaterial `json:"definition"`
	Files      []SnapshotFile  `json:"files"`
	EntryPoint string          `json:"entryPoint"`
}
type SnapshotFile struct {
	ID   string  `json:"id"`
	Path string  `json:"path"`
	Blob BlobRef `json:"blob"`
}
type SnapshotTest struct {
	ID         string       `json:"id"`
	Definition TestMaterial `json:"definition"`
	Input      *BlobRef     `json:"input,omitempty"`
	Answer     *BlobRef     `json:"answer,omitempty"`
}
type SnapshotGroup struct {
	ID         string        `json:"id"`
	Definition GroupMaterial `json:"definition"`
}

// Only executable TeX statements are frozen into checks. Markdown/PDF wording
// can be published without rerunning judging. Only declared statement
// dependencies are staged; programs and test data remain excluded.
type SnapshotStatement struct {
	Title    string         `json:"title"`
	Dialect  string         `json:"dialect,omitempty"`
	ID       string         `json:"id"`
	Language string         `json:"language"`
	Source   SnapshotFile   `json:"source"`
	Files    []SnapshotFile `json:"files"`
}

// CheckSnapshot seals semantic build inputs. Data and sources stay as immutable
// references; worker downloads must be authorized through a live build lease.
type CheckSnapshot struct {
	Validation    []SnapshotValidation `json:"validation,omitempty"`
	Statements    []SnapshotStatement  `json:"statements,omitempty"`
	SchemaVersion int                  `json:"schemaVersion"`
	TreeHash      string               `json:"treeHash"`
	PolicyVersion string               `json:"policyVersion"`
	DataHash      string               `json:"dataHash"`
	Metadata      PackageMetadata      `json:"metadata"`
	Programs      []SnapshotProgram    `json:"programs"`
	Tests         []SnapshotTest       `json:"tests"`
	Groups        []SnapshotGroup      `json:"groups"`
}

type MaterialLoader func(BlobRef) ([]byte, error)

// InspectMaterials reads bounded descriptor documents without executing source
// code or loading test data. Storage corruption is a failure, not a missing-file
// warning that could accidentally permit publication.
func InspectMaterials(tree ContentTree, load MaterialLoader) (MaterialInspection, *CheckSnapshot, error) {
	tree, err := tree.Canonical()
	if err != nil {
		return MaterialInspection{}, nil, err
	}
	hash, err := tree.Hash()
	if err != nil {
		return MaterialInspection{}, nil, err
	}
	check := materialInspector{
		report:   MaterialInspection{TreeHash: hash, PolicyVersion: CheckPolicyVersion, CanBuild: true, Issues: []MaterialIssue{}},
		snapshot: &CheckSnapshot{SchemaVersion: 1, TreeHash: hash, PolicyVersion: CheckPolicyVersion, Programs: []SnapshotProgram{}, Tests: []SnapshotTest{}, Groups: []SnapshotGroup{}},
		entries:  map[string]TreeEntry{}, programs: map[string]ProgramMaterial{}, tests: map[string]TestMaterial{}, groups: map[string]GroupMaterial{},
		validation: map[string]ValidationMaterial{},
	}
	metadataCount := 0
	documentBytes := int64(0)
	for _, entry := range tree.Entries {
		check.entries[entry.ID] = entry
		switch entry.Kind {
		case EntryMetadata, EntryProgram, EntryTest, EntryGroup, EntryValidation, EntryGeneration:
		default:
			continue
		}
		if entry.Blob.Bytes > 1<<20 || documentBytes > (32<<20)-entry.Blob.Bytes {
			check.issue("error", "material.too_large", entry.ID, "", "材料文档超过检查大小限制")
			continue
		}
		documentBytes += entry.Blob.Bytes
		data, err := load(entry.Blob)
		if err != nil {
			return check.report, nil, err
		}
		if err := ValidateBlob(entry.Blob, data); err != nil {
			return check.report, nil, err
		}
		view, err := DecodeMaterial(entry, data)
		if err != nil {
			check.issue("error", "material.invalid", entry.ID, "", err.Error())
			continue
		}
		switch entry.Kind {
		case EntryMetadata:
			metadataCount++
			if entry.ID != "problem" || entry.Path != "vertex/problem.json" {
				check.issue("error", "metadata.identity", entry.ID, "", "基本设置必须使用固定的 problem 材料")
			}
			check.snapshot.Metadata = *view.Metadata
		case EntryProgram:
			check.programs[entry.ID] = *view.Program
		case EntryTest:
			check.tests[entry.ID] = *view.Test
		case EntryGroup:
			check.groups[entry.ID] = *view.Group
		case EntryValidation:
			check.validation[entry.ID] = *view.Validation
		}
	}
	if metadataCount != 1 {
		check.issue("error", "metadata.required", "problem", "", "必须有且只有一份有效的题目基本设置")
	}
	check.statements(tree)
	check.preparePrograms(tree)
	if metadataCount == 1 {
		check.judgingPolicy()
	}
	check.prepareTests(tree)
	seenTests := map[string]string{}
	for _, test := range check.snapshot.Tests {
		if test.Input == nil || test.Answer == nil {
			continue
		}
		key := test.Input.SHA256 + ":" + test.Answer.SHA256
		if prior, exists := seenTests[key]; exists {
			check.issue("warning", "test.duplicate", test.ID, "input", "输入和答案与测试「"+prior+"」完全相同，请确认是否需要重复保留")
		} else {
			seenTests[key] = test.Definition.Name
		}
	}
	check.prepareGroups(tree)
	check.prepareValidation(tree)
	solutions := 0
	for _, program := range check.programs {
		if program.Role == "solution" {
			solutions++
		}
	}
	if len(check.tests) > 0 && solutions > 100000/len(check.tests) {
		check.issue("error", "check.matrix_limit", "problem", "", "参考解检查矩阵超过 100000 次执行")
	}
	check.report.ProgramCount = len(check.programs)
	check.report.TestCount = len(check.tests)
	data, err := check.snapshot.DataFingerprint()
	if err != nil {
		return check.report, nil, err
	}
	check.snapshot.DataHash = data
	check.report.DataHash = data
	check.report.PublicationIssues = check.snapshot.PublicationIssues()
	return check.report, check.snapshot, nil
}

// Checking programs/data is useful before every judging mode is implemented.
// Publication must separately reject semantics the submission runner would drop.
func (snapshot *CheckSnapshot) PublicationIssues() []MaterialIssue {
	issues := []MaterialIssue{}
	for _, test := range snapshot.Tests {
		if test.Definition.Points != 0 {
			issues = append(issues, MaterialIssue{Severity: "error", Code: "publication.scoring", EntryID: test.ID, Field: "points", Message: "逐点计分尚未接入提交汇总，处理计分规则后才能发布"})
			break
		}
	}
	for _, test := range snapshot.Tests {
		if test.Definition.IsPretest {
			issues = append(issues, MaterialIssue{Severity: "error", Code: "publication.pretests", EntryID: test.ID, Field: "isPretest", Message: "预测试与系统测试阶段尚未接入提交评测，暂不能发布"})
			break
		}
	}
	if len(snapshot.Groups) > 0 {
		issues = append(issues, MaterialIssue{Severity: "error", Code: "publication.groups", EntryID: snapshot.Groups[0].ID, Field: "aggregation", Message: "分组计分和依赖尚未接入提交汇总，暂不能发布"})
	}
	return issues
}

func (check *materialInspector) statements(tree ContentTree) {
	found := false
	languages := map[string]bool{}
	for _, entry := range tree.Entries {
		if entry.Kind != EntryStatement {
			continue
		}
		language, format := entry.Attributes["language"], entry.Attributes["format"]
		if !languagePattern.MatchString(language) {
			check.issue("error", "statement.language", entry.ID, "language", "题面语言标识无效")
		}
		if languages[language] {
			check.issue("error", "statement.duplicate", entry.ID, "language", "同一种语言存在多份题面")
		}
		languages[language] = true
		if format != "markdown" && format != "tex" && format != "pdf" {
			check.issue("error", "statement.format", entry.ID, "format", "未知题面格式")
		}
		if entry.Blob.Bytes == 0 {
			check.issue("error", "statement.empty", entry.ID, "", "题面内容为空")
		}
		if language == check.snapshot.Metadata.StatementLanguage {
			found = true
		}
		if format == "tex" {
			statement := SnapshotStatement{ID: entry.ID, Language: language, Source: SnapshotFile{ID: entry.ID, Path: entry.Path, Blob: entry.Blob}, Files: []SnapshotFile{}}
			statement.Title = entry.Attributes["title"]
			if statement.Title == "" || language == check.snapshot.Metadata.StatementLanguage {
				statement.Title = check.snapshot.Metadata.Title
			}
			statement.Dialect = entry.Attributes["dialect"]
			if statement.Dialect != "" && statement.Dialect != "polygon" {
				check.issue("error", "statement.dialect", entry.ID, "dialect", "未知 TeX 题面方言")
			}
			for _, asset := range tree.Entries {
				if asset.Kind == EntryAsset && (asset.Attributes["visibility"] == "" || asset.Attributes["visibility"] == "public") || asset.Kind == EntryResource && asset.Attributes["purpose"] == "statement-support" {
					statement.Files = append(statement.Files, SnapshotFile{ID: asset.ID, Path: asset.Path, Blob: asset.Blob})
				}
			}
			check.snapshot.Statements = append(check.snapshot.Statements, statement)
		}
	}
	if !found {
		check.issue("error", "statement.required", "problem", "statementLanguage", "缺少默认语言的题面")
	}
	if len(check.snapshot.Statements) > 20 {
		check.issue("error", "statement.render_limit", "problem", "", "一次检查最多编译 20 份 TeX 题面")
	}
}

func (check *materialInspector) preparePrograms(tree ContentTree) {
	references := 0
	for _, entry := range tree.Entries {
		program, exists := check.programs[entry.ID]
		if !exists {
			continue
		}
		if len(program.Files) > 4*MaxTreeEntries-references {
			check.issue("error", "program.reference_limit", entry.ID, "files", "程序源文件引用总数超过检查上限")
			continue
		}
		references += len(program.Files)
		if len(program.Files) == 0 {
			check.issue("error", "program.files", entry.ID, "files", "程序没有源文件")
		}
		if program.EntryPoint == "" {
			check.issue("error", "program.entrypoint", entry.ID, "entryPoint", "程序没有入口文件")
		}
		prepared := SnapshotProgram{ID: entry.ID, Definition: program, Files: []SnapshotFile{}}
		for _, id := range program.Files {
			check.requireEntry(id, EntrySource, entry.ID, "files")
		}
		root := program.Directory
		if root != "" {
			root += "/"
		}
		for _, id := range program.Files {
			source, ok := check.entries[id]
			if !ok || source.Kind != EntrySource {
				continue
			}
			if !strings.HasPrefix(source.Path, root) {
				check.issue("error", "program.directory", entry.ID, "directory", "源文件不在程序目录内："+source.Path)
				continue
			}
			name := strings.TrimPrefix(source.Path, root)
			prepared.Files = append(prepared.Files, SnapshotFile{ID: id, Path: name, Blob: source.Blob})
			if id == program.EntryPoint {
				prepared.EntryPoint = name
			}
		}
		slices.SortFunc(prepared.Files, func(a, b SnapshotFile) int { return strings.Compare(a.Path, b.Path) })
		if program.Role == "solution" && len(program.ExpectedVerdicts) == 0 {
			check.issue("error", "solution.expectation", entry.ID, "expectedVerdicts", "参考解必须声明预期判定")
		}
		check.snapshot.Programs = append(check.snapshot.Programs, prepared)
	}
}

func (check *materialInspector) judgingPolicy() {
	meta := check.snapshot.Metadata
	for _, requirement := range meta.Requirements {
		if requirement.Stage == "build" {
			check.issue("error", requirement.Code, "problem", "requirements", requirement.Message)
		}
	}
	check.requireProgram(meta.MainSolution, "solution", "problem", "mainSolution")
	if main, ok := check.programs[meta.MainSolution]; ok && (len(main.ExpectedVerdicts) != 1 || main.ExpectedVerdicts[0] != "Accepted") {
		check.issue("error", "solution.main_expectation", meta.MainSolution, "expectedVerdicts", "主参考解的预期结果必须是全部通过")
	}
	for _, id := range meta.InputValidators {
		check.requireProgram(id, "input-validator", "problem", "inputValidators")
	}
	if len(meta.InputValidators) == 0 {
		check.issue("warning", "validator.missing", "problem", "inputValidators", "未配置输入校验器，无法自动验证输入约束")
	}
	if meta.Comparison.Kind == "testlib" || meta.Comparison.Kind == "kattis" {
		check.requireProgram(meta.OutputValidator, "output-validator", "problem", "outputValidator")
		if validator, ok := check.programs[meta.OutputValidator]; ok && validator.Protocol != meta.Comparison.Kind {
			check.issue("error", "validator.protocol", meta.OutputValidator, "protocol", "输出校验器协议与比较方式不一致")
		}
	} else if meta.OutputValidator != "" {
		check.issue("error", "validator.unused", "problem", "outputValidator", "当前比较方式不会执行所选输出校验器")
	}
	if meta.JudgeType != "normal" {
		check.issue("error", "runtime.interactive", "problem", "judgeType", "交互题材料可以保存；当前构建与发布尚不支持交互执行")
	}
}

func (check *materialInspector) prepareTests(tree ContentTree) {
	ordered := map[string]bool{}
	for _, id := range check.snapshot.Metadata.TestOrder {
		definition, exists := check.tests[id]
		if !exists {
			check.issue("error", "test.order_reference", "problem", "testOrder", "测试顺序引用了不存在的测试点："+id)
			continue
		}
		ordered[id] = true
		prepared := SnapshotTest{ID: id, Definition: definition}
		if definition.Group != "" {
			if _, ok := check.groups[definition.Group]; !ok {
				check.issue("error", "test.group", id, "group", "测试点关联的分组不存在")
			}
		}
		if definition.Input.Kind == "file" {
			if file := check.requireEntry(definition.Input.Entry, EntryInput, id, "input.entry"); file != nil {
				ref := file.Blob
				prepared.Input = &ref
			}
		} else {
			check.requireProgram(definition.Input.Generator, "generator", id, "input.generator")
		}
		if definition.Answer.Kind == "file" {
			if file := check.requireEntry(definition.Answer.Entry, EntryAnswer, id, "answer.entry"); file != nil {
				ref := file.Blob
				prepared.Answer = &ref
			}
		} else {
			solution := definition.Answer.Solution
			if solution == "" {
				solution = check.snapshot.Metadata.MainSolution
			}
			check.requireProgram(solution, "solution", id, "answer.solution")
		}
		if definition.IsSample {
			check.report.SampleCount++
		}
		check.snapshot.Tests = append(check.snapshot.Tests, prepared)
	}
	for _, entry := range tree.Entries {
		if _, exists := check.tests[entry.ID]; exists && !ordered[entry.ID] {
			check.issue("error", "test.unordered", entry.ID, "", "测试点尚未加入测试顺序")
		}
	}
	if len(check.tests) == 0 {
		check.issue("error", "test.required", "problem", "testOrder", "尚未添加测试点")
	}
}

func (check *materialInspector) prepareGroups(tree ContentTree) {
	colors := map[string]int{}
	var visit func(string)
	visit = func(id string) {
		if colors[id] == 2 {
			return
		}
		if colors[id] == 1 {
			check.issue("error", "group.cycle", id, "prerequisites", "测试组依赖形成了循环")
			return
		}
		colors[id] = 1
		for _, dependency := range check.groups[id].Prerequisites {
			if _, exists := check.groups[dependency]; !exists {
				check.issue("error", "group.reference", id, "prerequisites", "依赖的测试组不存在："+dependency)
			} else {
				visit(dependency)
			}
		}
		colors[id] = 2
	}
	for _, entry := range tree.Entries {
		if definition, exists := check.groups[entry.ID]; exists {
			visit(entry.ID)
			check.snapshot.Groups = append(check.snapshot.Groups, SnapshotGroup{ID: entry.ID, Definition: definition})
		}
	}
}

// DataFingerprint excludes editorial wording/attribution, but includes every
// judging setting, program, file name, argument, group and input/answer digest.
// Executed-result reuse additionally requires an identical worker toolchain key.
func (snapshot CheckSnapshot) DataFingerprint() (string, error) {
	key := struct {
		Policy          string               `json:"policy"`
		Time            int                  `json:"time"`
		Memory          int                  `json:"memory"`
		JudgeType       string               `json:"judgeType"`
		ResourceMode    string               `json:"resourceMode"`
		Comparison      OutputComparison     `json:"comparison"`
		Main            string               `json:"main"`
		InputValidators []string             `json:"inputValidators"`
		OutputValidator string               `json:"outputValidator"`
		Programs        []SnapshotProgram    `json:"programs"`
		Tests           []SnapshotTest       `json:"tests"`
		Groups          []SnapshotGroup      `json:"groups"`
		Statements      []SnapshotStatement  `json:"statements,omitempty"`
		Validation      []SnapshotValidation `json:"validation,omitempty"`
	}{snapshot.PolicyVersion, snapshot.Metadata.TimeLimitMs, snapshot.Metadata.MemoryLimitKB, snapshot.Metadata.JudgeType, snapshot.Metadata.ResourceMode, snapshot.Metadata.Comparison, snapshot.Metadata.MainSolution, snapshot.Metadata.InputValidators, snapshot.Metadata.OutputValidator, snapshot.Programs, snapshot.Tests, snapshot.Groups, snapshot.Statements, snapshot.Validation}
	data, err := json.Marshal(key)
	if err != nil {
		return "", err
	}
	return Digest(data), nil
}

type materialInspector struct {
	validation map[string]ValidationMaterial
	report     MaterialInspection
	snapshot   *CheckSnapshot
	entries    map[string]TreeEntry
	programs   map[string]ProgramMaterial
	tests      map[string]TestMaterial
	groups     map[string]GroupMaterial
}

func (check *materialInspector) issue(severity, code, id, field, message string) {
	check.report.Issues = append(check.report.Issues, MaterialIssue{severity, code, id, field, message})
	if severity == "error" {
		check.report.CanBuild = false
	}
}

func (check *materialInspector) requireEntry(id, kind, owner, field string) *TreeEntry {
	entry, ok := check.entries[id]
	if !ok || entry.Kind != kind {
		check.issue("error", "reference.missing", owner, field, "关联材料不存在或类型不匹配："+id)
		return nil
	}
	return &entry
}
func (check *materialInspector) requireProgram(id, role, owner, field string) {
	program, ok := check.programs[id]
	if !ok || program.Role != role {
		check.issue("error", "program.role", owner, field, "请选择有效的 "+role+" 程序："+id)
	}
}
