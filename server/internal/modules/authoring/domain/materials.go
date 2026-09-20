package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const MaterialSchemaVersion = 1
const EntryProgram = "program"

// PackageMetadata is source material, not the mutable public problem projection.
// Tests are ordered by stable IDs. Program and data references use entry IDs so
// that renaming a file does not silently change what is executed.
type PackageMetadata struct {
	Requirements      []MaterialRequirement `json:"requirements"`
	ResourceMode      string                `json:"resourceMode"`
	Difficulty        int                   `json:"difficulty"`
	SchemaVersion     int                   `json:"schemaVersion"`
	Title             string                `json:"title"`
	StatementLanguage string                `json:"statementLanguage"`
	TimeLimitMs       int                   `json:"timeLimitMs"`
	MemoryLimitKB     int                   `json:"memoryLimitKb"`
	JudgeType         string                `json:"judgeType"`
	Source            string                `json:"source"`
	License           string                `json:"license"`
	RightsOwner       string                `json:"rightsOwner"`
	Tags              []string              `json:"tags"`
	Comparison        OutputComparison      `json:"comparison"`
	InputValidators   []string              `json:"inputValidators"`
	OutputValidator   string                `json:"outputValidator"`
	MainSolution      string                `json:"mainSolution"`
	TestOrder         []string              `json:"testOrder"`
}

type OutputComparison struct {
	Kind              string  `json:"kind"`
	CaseSensitive     bool    `json:"caseSensitive"`
	SpaceSensitive    bool    `json:"spaceSensitive"`
	FloatingPoint     bool    `json:"floatingPoint"`
	AbsoluteTolerance float64 `json:"absoluteTolerance"`
	RelativeTolerance float64 `json:"relativeTolerance"`
}

type ProgramMaterial struct {
	Directory        string   `json:"directory"`
	SchemaVersion    int      `json:"schemaVersion"`
	Name             string   `json:"name"`
	Role             string   `json:"role"`
	Language         string   `json:"language"`
	Protocol         string   `json:"protocol"`
	Files            []string `json:"files"`
	EntryPoint       string   `json:"entryPoint"`
	Arguments        []string `json:"arguments"`
	ExpectedVerdicts []string `json:"expectedVerdicts"`
}

type TestMaterial struct {
	SchemaVersion int        `json:"schemaVersion"`
	Name          string     `json:"name"`
	Group         string     `json:"group"`
	IsSample      bool       `json:"isSample"`
	IsPretest     bool       `json:"isPretest"`
	Input         TestInput  `json:"input"`
	Answer        TestAnswer `json:"answer"`
	TimeLimitMs   int        `json:"timeLimitMs"`
	MemoryLimitKB int        `json:"memoryLimitKb"`
	Points        float64    `json:"points"`
	Description   string     `json:"description"`
}

type TestInput struct {
	Kind      string   `json:"kind"`
	Entry     string   `json:"entry"`
	Generator string   `json:"generator"`
	Arguments []string `json:"arguments"`
}

type TestAnswer struct {
	Kind     string `json:"kind"`
	Entry    string `json:"entry"`
	Solution string `json:"solution"`
}

type GroupMaterial struct {
	SchemaVersion int      `json:"schemaVersion"`
	Name          string   `json:"name"`
	Aggregation   string   `json:"aggregation"`
	MaxScore      float64  `json:"maxScore"`
	Prerequisites []string `json:"prerequisites"`
	Description   string   `json:"description"`
}

type MaterialContent struct {
	Entry TreeEntry
	Data  []byte
}

// MaterialView is a typed per-entry projection for forms, never a second source
// of truth. Source code, statements and large data use the separate blob route.
type MaterialView struct {
	Validation *ValidationMaterial `json:"validation,omitempty"`
	Position   int                 `json:"position,omitempty"`
	Error      string              `json:"error,omitempty"`
	Entry      TreeEntry           `json:"entry"`
	Metadata   *PackageMetadata    `json:"metadata,omitempty"`
	Program    *ProgramMaterial    `json:"program,omitempty"`
	Test       *TestMaterial       `json:"test,omitempty"`
	Group      *GroupMaterial      `json:"group,omitempty"`
}

func DecodeMaterial(entry TreeEntry, data []byte) (*MaterialView, error) {
	canonical, err := NormalizeMaterial(entry.Kind, data)
	if err != nil {
		return nil, err
	}
	view := &MaterialView{Entry: entry}
	var target any
	switch entry.Kind {
	case EntryMetadata:
		view.Metadata = &PackageMetadata{}
		target = view.Metadata
	case EntryProgram:
		view.Program = &ProgramMaterial{}
		target = view.Program
	case EntryTest:
		view.Test = &TestMaterial{}
		target = view.Test
	case EntryGroup:
		view.Group = &GroupMaterial{}
		target = view.Group
	case EntryValidation:
		view.Validation = &ValidationMaterial{}
		target = view.Validation
	}
	if err := json.Unmarshal(canonical, target); err != nil {
		return nil, err
	}
	return view, nil
}

// InitialMaterials does not create a history revision. The immutable initial
// tree is the common ancestor for authors editing before the first commit.
func InitialMaterials(title, statement, language, source, judgeType string, timeMs, memoryKB int, difficulty ...int) ([]MaterialContent, error) {
	if language == "" {
		language = "zh"
	}
	metadata := PackageMetadata{SchemaVersion: MaterialSchemaVersion, Title: title, StatementLanguage: language, Source: source,
		TimeLimitMs: timeMs, MemoryLimitKB: memoryKB, JudgeType: judgeType, License: "unknown", Tags: []string{},
		Comparison: OutputComparison{Kind: "tokens", CaseSensitive: true}, InputValidators: []string{}, TestOrder: []string{}}
	if len(difficulty) > 0 {
		metadata.Difficulty = difficulty[0]
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	encoded, err = NormalizeMaterial(EntryMetadata, encoded)
	if err != nil {
		return nil, err
	}
	statementBytes := []byte(statement)
	return []MaterialContent{
		{Entry: TreeEntry{ID: "problem", Path: "vertex/problem.json", Kind: EntryMetadata, Blob: Reference(encoded), Attributes: map[string]string{"format": "json"}}, Data: encoded},
		{Entry: TreeEntry{ID: "statement-" + language, Path: "statement/problem." + language + ".md", Kind: EntryStatement, Blob: Reference(statementBytes), Attributes: map[string]string{"format": "markdown", "language": language}}, Data: statementBytes},
	}, nil
}

// NormalizeMaterial only checks a document's local invariants. Missing linked
// programs/tests remain editable drafts and are reported by package checking.
// Unknown fields are rejected here rather than disappearing on the next save.
func NormalizeMaterial(kind string, data []byte) ([]byte, error) {
	if len(data) > 1<<20 || !utf8.Valid(data) || !json.Valid(data) {
		return nil, InvalidInput("material document must be valid JSON of at most 1 MiB")
	}
	var value any
	switch kind {
	case EntryMetadata:
		value = &PackageMetadata{}
	case EntryProgram:
		value = &ProgramMaterial{}
	case EntryTest:
		value = &TestMaterial{}
	case EntryGroup:
		value = &GroupMaterial{}
	case EntryValidation:
		value = &ValidationMaterial{}
	default:
		return nil, InvalidInput("entry is not a structured material document")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return nil, InvalidInput("invalid material document: " + err.Error())
	}
	var err error
	switch item := value.(type) {
	case *PackageMetadata:
		err = item.Validate()
	case *ProgramMaterial:
		err = item.Validate()
	case *TestMaterial:
		err = item.Validate()
	case *GroupMaterial:
		err = item.Validate()
	case *ValidationMaterial:
		err = item.Validate()
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func (value *PackageMetadata) Validate() error {
	if value.ResourceMode == "" {
		value.ResourceMode = "language-scaled"
	}
	if value.ResourceMode != "exact" && value.ResourceMode != "language-scaled" {
		return InvalidInput("unknown resource limit mode")
	}
	if len(value.Requirements) > 256 {
		return InvalidInput("too many package requirements")
	}
	for _, requirement := range value.Requirements {
		if requirement.Code == "" || len(requirement.Code) > 128 || len(requirement.Path) > 1024 || len(requirement.Message) > 4096 || (requirement.Stage != "build" && requirement.Stage != "publish") {
			return InvalidInput("invalid package requirement")
		}
	}
	if value.Requirements == nil {
		value.Requirements = []MaterialRequirement{}
	}
	if value.Difficulty == 0 {
		value.Difficulty = 1
	}
	if value.Difficulty < 1 || value.Difficulty > 10 {
		return InvalidInput("difficulty must be between 1 and 10")
	}
	if err := materialIdentity(value.SchemaVersion, value.Title); err != nil {
		return err
	}
	if len(value.StatementLanguage) > 64 || !languagePattern.MatchString(value.StatementLanguage) {
		return InvalidInput("invalid default statement language")
	}
	if value.TimeLimitMs < 1 || value.TimeLimitMs > 3600000 || value.MemoryLimitKB < 1 || value.MemoryLimitKB > 1<<30 {
		return InvalidInput("invalid problem resource limits")
	}
	if value.JudgeType != "normal" && value.JudgeType != "interactive" {
		return InvalidInput("unknown problem judge type")
	}
	if len(value.Source) > 4096 || len(value.RightsOwner) > 4096 || len(value.License) > 128 {
		return InvalidInput("problem attribution is too long")
	}
	if err := value.Comparison.Validate(); err != nil {
		return err
	}
	if err := materialIDs(value.InputValidators); err != nil {
		return err
	}
	if err := materialIDs(value.TestOrder); err != nil {
		return err
	}
	for _, id := range []string{value.OutputValidator, value.MainSolution} {
		if id != "" && !contentIDPattern.MatchString(id) {
			return InvalidInput("invalid program reference")
		}
	}
	if len(value.Tags) > 100 {
		return InvalidInput("too many tags")
	}
	for _, tag := range value.Tags {
		if strings.TrimSpace(tag) == "" || len(tag) > 128 {
			return InvalidInput("invalid problem tag")
		}
	}
	if value.Tags == nil {
		value.Tags = []string{}
	}
	if value.InputValidators == nil {
		value.InputValidators = []string{}
	}
	if value.TestOrder == nil {
		value.TestOrder = []string{}
	}
	return nil
}

func (value OutputComparison) Validate() error {
	switch value.Kind {
	case "tokens", "exact", "testlib", "kattis":
	default:
		return InvalidInput("unknown output comparison protocol")
	}
	if !finiteScore(value.AbsoluteTolerance) || !finiteScore(value.RelativeTolerance) {
		return InvalidInput("floating-point tolerances must be finite and non-negative")
	}
	if value.Kind != "tokens" && (value.FloatingPoint || value.AbsoluteTolerance != 0 || value.RelativeTolerance != 0) {
		return InvalidInput("floating-point tolerances require token comparison")
	}
	if !value.FloatingPoint && (value.AbsoluteTolerance != 0 || value.RelativeTolerance != 0) {
		return InvalidInput("enable floating-point comparison before setting tolerances")
	}
	return nil
}

func (value *ProgramMaterial) Validate() error {
	if err := materialIdentity(value.SchemaVersion, value.Name); err != nil {
		return err
	}
	if value.Directory == "." {
		value.Directory = ""
	}
	if value.Directory != "" {
		if err := ValidatePackagePath(value.Directory); err != nil {
			return InvalidInput("invalid program directory")
		}
	}
	switch value.Role {
	case "solution", "generator", "input-validator", "output-validator", "interactor", "static-validator":
	default:
		return InvalidInput("unknown program role")
	}
	if !contentIDPattern.MatchString(value.Language) {
		return InvalidInput("invalid program language")
	}
	switch value.Protocol {
	case "stdio", "testlib", "kattis":
	default:
		return InvalidInput("unknown program protocol")
	}
	if (value.Role == "solution" || value.Role == "generator") && value.Protocol != "stdio" {
		return InvalidInput("solutions and generators use the stdio protocol")
	}
	if err := materialIDs(value.Files); err != nil {
		return err
	}
	if value.EntryPoint != "" {
		found := false
		for _, id := range value.Files {
			found = found || id == value.EntryPoint
		}
		if !found {
			return InvalidInput("program entry point must be one of its files")
		}
	}
	if err := validateArguments(value.Arguments); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, verdict := range value.ExpectedVerdicts {
		if !judgeVerdicts[verdict] || seen[verdict] {
			return InvalidInput("invalid or duplicate expected verdict")
		}
		seen[verdict] = true
	}
	if value.Role != "solution" && len(value.ExpectedVerdicts) != 0 {
		return InvalidInput("only solutions declare expected verdicts")
	}
	if value.Files == nil {
		value.Files = []string{}
	}
	if value.Arguments == nil {
		value.Arguments = []string{}
	}
	if value.ExpectedVerdicts == nil {
		value.ExpectedVerdicts = []string{}
	}
	return nil
}

func (value *TestMaterial) Validate() error {
	if err := materialIdentity(value.SchemaVersion, value.Name); err != nil {
		return err
	}
	if value.Group != "" && !contentIDPattern.MatchString(value.Group) {
		return InvalidInput("invalid test group reference")
	}
	switch value.Input.Kind {
	case "file":
		if value.Input.Generator != "" || len(value.Input.Arguments) != 0 {
			return InvalidInput("file inputs cannot specify generator commands")
		}
	case "generator":
		if value.Input.Entry != "" {
			return InvalidInput("generated inputs cannot specify an input file")
		}
	default:
		return InvalidInput("unknown test input source")
	}
	switch value.Answer.Kind {
	case "file":
		if value.Answer.Solution != "" {
			return InvalidInput("imported answers cannot specify a generating solution")
		}
	case "solution":
		if value.Answer.Entry != "" {
			return InvalidInput("generated answers cannot specify an answer file")
		}
	default:
		return InvalidInput("unknown test answer source")
	}
	for _, id := range []string{value.Input.Entry, value.Input.Generator, value.Answer.Entry, value.Answer.Solution} {
		if id != "" && !contentIDPattern.MatchString(id) {
			return InvalidInput("invalid test material reference")
		}
	}
	if err := validateArguments(value.Input.Arguments); err != nil {
		return err
	}
	if value.TimeLimitMs < 0 || value.TimeLimitMs > 3600000 || value.MemoryLimitKB < 0 || value.MemoryLimitKB > 1<<30 || !finiteScore(value.Points) {
		return InvalidInput("invalid per-test limits or points")
	}
	if len(value.Description) > 8192 {
		return InvalidInput("test description is too long")
	}
	if value.Input.Arguments == nil {
		value.Input.Arguments = []string{}
	}
	return nil
}

func (value *GroupMaterial) Validate() error {
	if err := materialIdentity(value.SchemaVersion, value.Name); err != nil {
		return err
	}
	switch value.Aggregation {
	case "pass-fail", "sum", "min":
	default:
		return InvalidInput("unknown test group aggregation")
	}
	if !finiteScore(value.MaxScore) {
		return InvalidInput("group maximum score must be finite and non-negative")
	}
	if err := materialIDs(value.Prerequisites); err != nil {
		return err
	}
	if len(value.Description) > 8192 {
		return InvalidInput("group description is too long")
	}
	if value.Prerequisites == nil {
		value.Prerequisites = []string{}
	}
	return nil
}

func materialIdentity(version int, name string) error {
	if version != MaterialSchemaVersion {
		return InvalidInput(fmt.Sprintf("unsupported material schema version %d", version))
	}
	if strings.TrimSpace(name) == "" || len(name) > 240 || strings.ContainsAny(name, "\r\n\x00") {
		return InvalidInput("material name must contain 1-240 bytes on one line")
	}
	return nil
}
func materialIDs(ids []string) error {
	if len(ids) > MaxTreeEntries {
		return InvalidInput("too many material references")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !contentIDPattern.MatchString(id) || seen[id] {
			return InvalidInput("invalid or duplicate material reference")
		}
		seen[id] = true
	}
	return nil
}
func validateArguments(args []string) error {
	if len(args) > 128 {
		return InvalidInput("too many program arguments")
	}
	for _, arg := range args {
		if len(arg) > 4096 || strings.ContainsRune(arg, 0) || !utf8.ValidString(arg) {
			return InvalidInput("invalid program argument")
		}
	}
	return nil
}
func finiteScore(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
