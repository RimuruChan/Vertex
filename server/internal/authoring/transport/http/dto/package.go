// Package dto maps the authoring domain onto the HTTP wire contract. Source
// code and test inputs travel as plain JSON strings so the workspace editor
// never has to deal with a second transport format.
package dto

import (
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
)

// ---------- statements ----------

type StatementResponse struct {
	Language     string    `json:"language"`
	Name         string    `json:"name"`
	Legend       string    `json:"legend"`
	InputFormat  string    `json:"inputFormat"`
	OutputFormat string    `json:"outputFormat"`
	Notes        string    `json:"notes"`
	Tutorial     string    `json:"tutorial"`
	Scoring      string    `json:"scoring"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type StatementUpsertRequest struct {
	Name         string `json:"name,omitempty"`
	Legend       string `json:"legend,omitempty"`
	InputFormat  string `json:"inputFormat,omitempty"`
	OutputFormat string `json:"outputFormat,omitempty"`
	Notes        string `json:"notes,omitempty"`
	Tutorial     string `json:"tutorial,omitempty"`
	Scoring      string `json:"scoring,omitempty"`
}

// StatementPreviewResponse carries the Markdown the public problem page would
// render for the submitted statement draft.
type StatementPreviewResponse struct {
	StatementMD string `json:"statementMd"`
}

func FromStatement(value authoringdomain.Statement) StatementResponse {
	return StatementResponse{
		Language: value.Language, Name: value.Name, Legend: value.Legend,
		InputFormat: value.InputFormat, OutputFormat: value.OutputFormat,
		Notes: value.Notes, Tutorial: value.Tutorial, Scoring: value.Scoring,
		UpdatedAt: value.UpdatedAt,
	}
}

func FromStatements(values []authoringdomain.Statement) []StatementResponse {
	result := make([]StatementResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromStatement(value))
	}
	return result
}

func (request StatementUpsertRequest) Domain(problemID, language string) authoringdomain.Statement {
	return authoringdomain.Statement{
		ProblemID: problemID, Language: language, Name: request.Name,
		Legend: request.Legend, InputFormat: request.InputFormat,
		OutputFormat: request.OutputFormat, Notes: request.Notes,
		Tutorial: request.Tutorial, Scoring: request.Scoring,
	}
}

// ---------- files ----------

type FileResponse struct {
	ID              int64     `json:"id"`
	Kind            string    `json:"kind" enums:"checker,validator,generator,solution,interactor"`
	Name            string    `json:"name"`
	Language        string    `json:"language"`
	SourceCode      string    `json:"sourceCode"`
	ExpectedVerdict string    `json:"expectedVerdict"`
	IsActive        bool      `json:"isActive"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type FileUpsertRequest struct {
	Kind            string `json:"kind" binding:"required"`
	Name            string `json:"name" binding:"required"`
	Language        string `json:"language" binding:"required"`
	SourceCode      string `json:"sourceCode" binding:"required"`
	ExpectedVerdict string `json:"expectedVerdict,omitempty"`
	IsActive        bool   `json:"isActive,omitempty"`
}

func FromFile(value authoringdomain.File) FileResponse {
	return FileResponse{
		ID: value.ID, Kind: value.Kind, Name: value.Name, Language: value.Language,
		SourceCode: value.SourceCode, ExpectedVerdict: value.ExpectedVerdict,
		IsActive: value.IsActive, UpdatedAt: value.UpdatedAt,
	}
}

func FromFiles(values []authoringdomain.File) []FileResponse {
	result := make([]FileResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromFile(value))
	}
	return result
}

func (request FileUpsertRequest) Domain(problemID string) authoringdomain.File {
	return authoringdomain.File{
		ProblemID: problemID, Kind: request.Kind, Name: request.Name,
		Language: request.Language, SourceCode: request.SourceCode,
		ExpectedVerdict: request.ExpectedVerdict, IsActive: request.IsActive,
	}
}

// ---------- tests ----------

type TestResponse struct {
	ID          int64  `json:"id"`
	Index       int    `json:"index"`
	Group       string `json:"group"`
	Source      string `json:"source" enums:"manual,generator"`
	InputData   string `json:"inputData"`
	GenerateCmd string `json:"generateCmd"`
	IsSample    bool   `json:"isSample"`
	Points      int    `json:"points"`
	Description string `json:"description"`
}

type TestUpsertRequest struct {
	Source      string `json:"source" binding:"required"`
	Group       string `json:"group,omitempty"`
	InputData   string `json:"inputData,omitempty"`
	GenerateCmd string `json:"generateCmd,omitempty"`
	IsSample    bool   `json:"isSample,omitempty"`
	Points      int    `json:"points,omitempty"`
	Description string `json:"description,omitempty"`
}

type TestMoveRequest struct {
	Position int `json:"position" binding:"required"`
}

func FromTest(value authoringdomain.Test) TestResponse {
	return TestResponse{
		ID: value.ID, Index: value.Index, Group: value.Group, Source: value.Source,
		InputData: value.InputData, GenerateCmd: value.GenerateCmd,
		IsSample: value.IsSample, Points: value.Points, Description: value.Description,
	}
}

func FromTests(values []authoringdomain.Test) []TestResponse {
	result := make([]TestResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromTest(value))
	}
	return result
}

func (request TestUpsertRequest) Domain(problemID string, id int64) authoringdomain.Test {
	return authoringdomain.Test{
		ID: id, ProblemID: problemID, Group: request.Group, Source: request.Source,
		InputData: request.InputData, GenerateCmd: request.GenerateCmd,
		IsSample: request.IsSample, Points: request.Points, Description: request.Description,
	}
}

// ---------- workspace ----------

type PackageMetaResponse struct {
	CanEdit                  bool       `json:"canEdit"`
	CanPublish               bool       `json:"canPublish"`
	ProblemPublicID          string     `json:"problemPublicId"`
	ProblemID                string     `json:"problemId"`
	Title                    string     `json:"title"`
	Visibility               string     `json:"visibility"`
	JudgeType                string     `json:"judgeType"`
	StatementLanguage        string     `json:"statementLanguage"`
	TimeLimitMs              int        `json:"timeLimitMs"`
	MemoryLimitKB            int        `json:"memoryLimitKb"`
	PackageRevision          int        `json:"packageRevision"`
	DataRevision             int        `json:"dataRevision"`
	PublishedVersion         int        `json:"publishedVersion"`
	PublishedRevision        int        `json:"publishedRevision"`
	PublishedArtifactVersion int        `json:"publishedArtifactVersion"`
	UnpublishedChanges       bool       `json:"unpublishedChanges"`
	BuiltRevision            int        `json:"builtRevision"`
	LastBuiltAt              *time.Time `json:"lastBuiltAt,omitempty"`
	TestdataCases            int        `json:"testdataCases"`
	TestdataChecker          string     `json:"testdataChecker"`
	TestdataVersion          int        `json:"testdataVersion"`
	TestdataSHA256           string     `json:"testdataSha256"`
	// Stale is true when the package changed after the last successful build.
	Stale bool `json:"stale"`
}

type WorkspaceResponse struct {
	Meta        PackageMetaResponse `json:"meta"`
	Statements  []StatementResponse `json:"statements"`
	Files       []FileResponse      `json:"files"`
	Tests       []TestResponse      `json:"tests"`
	LatestBuild *BuildResponse      `json:"latestBuild,omitempty"`
	Issues      []string            `json:"issues"`
}

func FromWorkspace(value authoringdomain.Workspace) WorkspaceResponse {
	response := WorkspaceResponse{
		Meta:       fromMeta(value.Meta),
		Statements: FromStatements(value.Statements),
		Files:      FromFiles(value.Files),
		Tests:      FromTests(value.Tests),
		Issues:     value.Issues,
	}
	if response.Issues == nil {
		response.Issues = []string{}
	}
	if value.LatestBuild != nil {
		build := FromBuild(*value.LatestBuild)
		response.LatestBuild = &build
	}
	return response
}

func fromMeta(value authoringdomain.PackageMeta) PackageMetaResponse {
	return PackageMetaResponse{
		CanEdit: value.CanEdit, CanPublish: value.CanPublish,
		ProblemPublicID: value.ProblemPublicID,
		ProblemID:       value.ProblemID, Title: value.Title, Visibility: value.Visibility,
		JudgeType: value.JudgeType, StatementLanguage: value.StatementLanguage,
		TimeLimitMs: value.TimeLimitMs, MemoryLimitKB: value.MemoryLimitKB,
		PackageRevision: value.PackageRevision, BuiltRevision: value.BuiltRevision,
		DataRevision: value.DataRevision, PublishedVersion: value.PublishedVersion, PublishedRevision: value.PublishedRevision, PublishedArtifactVersion: value.PublishedArtifactVersion,
		UnpublishedChanges: value.PackageRevision != value.PublishedRevision || value.TestdataVersion != value.PublishedArtifactVersion,
		LastBuiltAt:        value.LastBuiltAt, TestdataCases: value.TestdataCases,
		TestdataChecker: value.TestdataChecker, TestdataVersion: value.TestdataVersion,
		TestdataSHA256: value.TestdataSHA256,
		Stale:          value.TestdataCases == 0 || value.DataRevision != value.BuiltRevision,
	}
}

// TemplateResponse is one starter source offered by the workspace.
type TemplateResponse struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Language    string `json:"language"`
	Title       string `json:"title"`
	Description string `json:"description"`
	SourceCode  string `json:"sourceCode"`
}

func FromTemplates(values []authoringdomain.Template) []TemplateResponse {
	result := make([]TemplateResponse, 0, len(values))
	for _, value := range values {
		result = append(result, TemplateResponse{
			Kind: value.Kind, Name: value.Name, Language: value.Language,
			Title: value.Title, Description: value.Description, SourceCode: value.SourceCode,
		})
	}
	return result
}
