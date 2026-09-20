package artifact

import "github.com/RimuruChan/Vertex/worker/internal/checker"

const CheckProtocol = "vertex-authoring-4"

type BlobRef struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type FrozenFile struct {
	ID   string  `json:"id"`
	Path string  `json:"path"`
	Blob BlobRef `json:"blob"`
}
type ProgramDefinition struct {
	SchemaVersion    int      `json:"schemaVersion"`
	Directory        string   `json:"directory"`
	Name             string   `json:"name"`
	Role             string   `json:"role"`
	Language         string   `json:"language"`
	Protocol         string   `json:"protocol"`
	Files            []string `json:"files"`
	EntryPoint       string   `json:"entryPoint"`
	Arguments        []string `json:"arguments"`
	ExpectedVerdicts []string `json:"expectedVerdicts"`
}
type FrozenProgram struct {
	ID         string            `json:"id"`
	Definition ProgramDefinition `json:"definition"`
	Files      []FrozenFile      `json:"files"`
	EntryPoint string            `json:"entryPoint"`
}
type TestDefinition struct {
	SchemaVersion int    `json:"schemaVersion"`
	Name          string `json:"name"`
	Group         string `json:"group"`
	IsSample      bool   `json:"isSample"`
	IsPretest     bool   `json:"isPretest"`
	Input         struct {
		Kind      string   `json:"kind"`
		Entry     string   `json:"entry"`
		Generator string   `json:"generator"`
		Arguments []string `json:"arguments"`
	} `json:"input"`
	Answer struct {
		Kind     string `json:"kind"`
		Entry    string `json:"entry"`
		Solution string `json:"solution"`
	} `json:"answer"`
	TimeLimitMs   int     `json:"timeLimitMs"`
	MemoryLimitKB int     `json:"memoryLimitKb"`
	Points        float64 `json:"points"`
	Description   string  `json:"description"`
}
type FrozenTest struct {
	ID         string         `json:"id"`
	Definition TestDefinition `json:"definition"`
	Input      *BlobRef       `json:"input,omitempty"`
	Answer     *BlobRef       `json:"answer,omitempty"`
}
type FrozenGroup struct {
	ID         string `json:"id"`
	Definition struct {
		SchemaVersion int      `json:"schemaVersion"`
		Name          string   `json:"name"`
		Aggregation   string   `json:"aggregation"`
		MaxScore      float64  `json:"maxScore"`
		Prerequisites []string `json:"prerequisites"`
		Description   string   `json:"description"`
	} `json:"definition"`
}
type FrozenMetadata struct {
	ResourceMode      string                `json:"resourceMode"`
	Requirements      []MaterialRequirement `json:"requirements"`
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
	Comparison        checker.Policy        `json:"comparison"`
	InputValidators   []string              `json:"inputValidators"`
	OutputValidator   string                `json:"outputValidator"`
	MainSolution      string                `json:"mainSolution"`
	TestOrder         []string              `json:"testOrder"`
}

type MaterialRequirement struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Stage   string `json:"stage"`
}
type FrozenSnapshot struct {
	Validation    []FrozenValidation `json:"validation,omitempty"`
	Statements    []FrozenStatement  `json:"statements,omitempty"`
	SchemaVersion int                `json:"schemaVersion"`
	TreeHash      string             `json:"treeHash"`
	PolicyVersion string             `json:"policyVersion"`
	DataHash      string             `json:"dataHash"`
	Metadata      FrozenMetadata     `json:"metadata"`
	Programs      []FrozenProgram    `json:"programs"`
	Tests         []FrozenTest       `json:"tests"`
	Groups        []FrozenGroup      `json:"groups"`
}

type ValidationDefinition struct {
	SchemaVersion int    `json:"schemaVersion"`
	Name          string `json:"name"`
	Mode          string `json:"mode"`
	Input         string `json:"input"`
	Answer        string `json:"answer,omitempty"`
	Output        string `json:"output,omitempty"`
	Description   string `json:"description,omitempty"`
}
type FrozenValidation struct {
	ID         string               `json:"id"`
	Definition ValidationDefinition `json:"definition"`
	Input      *BlobRef             `json:"input,omitempty"`
	Answer     *BlobRef             `json:"answer,omitempty"`
	Output     *BlobRef             `json:"output,omitempty"`
}

type ArtifactManifest struct {
	Statements    []ArtifactStatement `json:"statements,omitempty"`
	Dependencies  []FrozenFile        `json:"dependencies,omitempty"`
	SchemaVersion int                 `json:"schemaVersion"`
	ToolchainKey  string              `json:"toolchainKey"`
	Snapshot      FrozenSnapshot      `json:"snapshot"`
	Tests         []ArtifactTest      `json:"tests"`
}
type FrozenStatement struct {
	Title    string       `json:"title"`
	Dialect  string       `json:"dialect,omitempty"`
	ID       string       `json:"id"`
	Language string       `json:"language"`
	Source   FrozenFile   `json:"source"`
	Files    []FrozenFile `json:"files"`
}
type ArtifactStatement struct {
	ID  string  `json:"id"`
	PDF BlobRef `json:"pdf"`
}
type ArtifactTest struct {
	ID     string  `json:"id"`
	Input  BlobRef `json:"input"`
	Answer BlobRef `json:"answer"`
}
