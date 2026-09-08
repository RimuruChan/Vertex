package dto

import (
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
)

// ---------- author-facing build reports ----------

type TestOutcomeResponse struct {
	Index       int    `json:"index"`
	Group       string `json:"group,omitempty"`
	Source      string `json:"source"`
	Command     string `json:"command,omitempty"`
	InputBytes  int64  `json:"inputBytes"`
	AnswerBytes int64  `json:"answerBytes"`
	TimeMs      int    `json:"timeMs"`
	MemoryKB    int    `json:"memoryKb"`
	IsSample    bool   `json:"isSample"`
	Points      int    `json:"points"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
	InputHead   string `json:"inputHead,omitempty"`
	AnswerHead  string `json:"answerHead,omitempty"`
}

type SolutionOutcomeResponse struct {
	Name            string `json:"name"`
	Language        string `json:"language"`
	ExpectedVerdict string `json:"expectedVerdict,omitempty"`
	ActualVerdict   string `json:"actualVerdict"`
	FailedTest      int    `json:"failedTest,omitempty"`
	MaxTimeMs       int    `json:"maxTimeMs"`
	MaxMemoryKB     int    `json:"maxMemoryKb"`
	Matched         bool   `json:"matched"`
	Message         string `json:"message,omitempty"`
}

type BuildResponse struct {
	DataRevision  int                       `json:"dataRevision"`
	ID            string                    `json:"id"`
	ProblemID     string                    `json:"problemId"`
	Revision      int                       `json:"revision"`
	State         string                    `json:"state" enums:"queued,running,succeeded,failed,cancelled,dead"`
	Stage         string                    `json:"stage"`
	Attempt       int                       `json:"attempt"`
	ProgressDone  int                       `json:"progressDone"`
	ProgressTotal int                       `json:"progressTotal"`
	Log           string                    `json:"log"`
	ErrorMessage  string                    `json:"errorMessage,omitempty"`
	Tests         []TestOutcomeResponse     `json:"tests"`
	Solutions     []SolutionOutcomeResponse `json:"solutions"`
	PackageCases  int                       `json:"packageCases"`
	CreatedAt     time.Time                 `json:"createdAt"`
	StartedAt     *time.Time                `json:"startedAt,omitempty"`
	FinishedAt    *time.Time                `json:"finishedAt,omitempty"`
}

func FromBuild(value authoringdomain.Build) BuildResponse {
	response := BuildResponse{
		DataRevision: value.DataRevision,
		ID:           value.ID, ProblemID: value.ProblemID, Revision: value.Revision,
		State: value.State, Stage: value.Stage, Attempt: value.Attempt,
		ProgressDone: value.ProgressDone, ProgressTotal: value.ProgressTotal,
		Log: value.Log, ErrorMessage: value.ErrorMessage,
		PackageCases: value.PackageCases, CreatedAt: value.CreatedAt,
		StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
		Tests:     make([]TestOutcomeResponse, 0, len(value.Tests)),
		Solutions: make([]SolutionOutcomeResponse, 0, len(value.Solutions)),
	}
	for _, item := range value.Tests {
		response.Tests = append(response.Tests, TestOutcomeResponse(item))
	}
	for _, item := range value.Solutions {
		response.Solutions = append(response.Solutions, SolutionOutcomeResponse(item))
	}
	return response
}

func FromBuilds(values []authoringdomain.Build) []BuildResponse {
	result := make([]BuildResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromBuild(value))
	}
	return result
}

// ---------- internal build worker protocol ----------

type BuildClaimRequest struct {
	WorkerID    string `json:"workerId" binding:"required"`
	WaitSeconds int    `json:"waitSeconds,omitempty"`
}

// BuildJobResponse is the complete build input. The worker receives sources
// and test definitions inline so it needs no database access of its own.
type BuildJobResponse struct {
	DomainID       string             `json:"domainId"`
	DataRevision   int                `json:"dataRevision"`
	BuildID        string             `json:"buildId"`
	ProblemID      string             `json:"problemId"`
	Revision       int                `json:"revision"`
	Attempt        int                `json:"attempt"`
	LeaseToken     string             `json:"leaseToken"`
	LeaseExpiresAt time.Time          `json:"leaseExpiresAt"`
	TimeLimitMs    int                `json:"timeLimitMs"`
	MemoryLimitKB  int                `json:"memoryLimitKb"`
	JudgeType      string             `json:"judgeType"`
	Checker        *BuildFile         `json:"checker,omitempty"`
	Validator      *BuildFile         `json:"validator,omitempty"`
	Interactor     *BuildFile         `json:"interactor,omitempty"`
	Generators     []BuildFile        `json:"generators"`
	Solutions      []BuildSolution    `json:"solutions"`
	Tests          []BuildTest        `json:"tests"`
	Limits         BuildLimitsPayload `json:"limits"`
}

// BuildLimitsPayload lets the server, not the worker, own the resource budget
// of trusted-but-unvetted author code.
type BuildLimitsPayload struct {
	GeneratorTimeMs int `json:"generatorTimeMs"`
	ValidatorTimeMs int `json:"validatorTimeMs"`
	SolutionTimeMs  int `json:"solutionTimeMs"`
	CheckerTimeMs   int `json:"checkerTimeMs"`
	MemoryLimitKB   int `json:"memoryLimitKb"`
}

type BuildFile struct {
	Name       string `json:"name"`
	Language   string `json:"language"`
	SourceCode string `json:"sourceCode"`
}

type BuildSolution struct {
	Name            string `json:"name"`
	Language        string `json:"language"`
	SourceCode      string `json:"sourceCode"`
	ExpectedVerdict string `json:"expectedVerdict,omitempty"`
	IsMain          bool   `json:"isMain"`
}

type BuildTest struct {
	Index       int    `json:"index"`
	Group       string `json:"group,omitempty"`
	Source      string `json:"source"`
	InputData   string `json:"inputData,omitempty"`
	GenerateCmd string `json:"generateCmd,omitempty"`
	IsSample    bool   `json:"isSample"`
	Points      int    `json:"points"`
}

// BuildJobFromDomain flattens the package snapshot into the wire job.
func BuildJobFromDomain(build authoringdomain.Build, pkg authoringdomain.Package, limits BuildLimitsPayload) BuildJobResponse {
	job := BuildJobResponse{
		DomainID: pkg.DomainID, DataRevision: pkg.DataRevision,
		BuildID: build.ID, ProblemID: build.ProblemID, Revision: build.Revision,
		Attempt: build.Attempt, LeaseToken: build.LeaseToken,
		LeaseExpiresAt: build.LeaseExpires, TimeLimitMs: pkg.TimeLimitMs,
		MemoryLimitKB: pkg.MemoryLimitKB, JudgeType: pkg.JudgeType,
		Generators: make([]BuildFile, 0, len(pkg.Generators)),
		Solutions:  make([]BuildSolution, 0, len(pkg.Solutions)),
		Tests:      make([]BuildTest, 0, len(pkg.Tests)),
		Limits:     limits,
	}
	job.Checker = buildFile(pkg.Checker)
	job.Validator = buildFile(pkg.Validator)
	job.Interactor = buildFile(pkg.Interactor)
	for _, generator := range pkg.Generators {
		job.Generators = append(job.Generators, BuildFile{
			Name: generator.Name, Language: generator.Language, SourceCode: generator.SourceCode,
		})
	}
	for _, solution := range pkg.Solutions {
		job.Solutions = append(job.Solutions, BuildSolution{
			Name: solution.Name, Language: solution.Language, SourceCode: solution.SourceCode,
			ExpectedVerdict: solution.ExpectedVerdict, IsMain: solution.IsActive,
		})
	}
	for _, test := range pkg.Tests {
		job.Tests = append(job.Tests, BuildTest{
			Index: test.Index, Group: test.Group, Source: test.Source,
			InputData: test.InputData, GenerateCmd: test.GenerateCmd,
			IsSample: test.IsSample, Points: test.Points,
		})
	}
	return job
}

func buildFile(file *authoringdomain.File) *BuildFile {
	if file == nil {
		return nil
	}
	return &BuildFile{Name: file.Name, Language: file.Language, SourceCode: file.SourceCode}
}

type BuildProgressRequest struct {
	WorkerID   string `json:"workerId" binding:"required"`
	LeaseToken string `json:"leaseToken" binding:"required"`
	Stage      string `json:"stage,omitempty"`
	Done       int    `json:"done,omitempty"`
	Total      int    `json:"total,omitempty"`
	Log        string `json:"log,omitempty"`
}

func (request BuildProgressRequest) Domain(buildID string) authoringdomain.Progress {
	return authoringdomain.Progress{
		BuildID: buildID, WorkerID: request.WorkerID, LeaseToken: request.LeaseToken,
		Stage: request.Stage, Done: request.Done, Total: request.Total, Log: request.Log,
	}
}

type BuildResultRequest struct {
	WorkerID     string                    `json:"workerId" binding:"required"`
	LeaseToken   string                    `json:"leaseToken" binding:"required"`
	Success      bool                      `json:"success"`
	Stage        string                    `json:"stage,omitempty"`
	Checker      string                    `json:"checker,omitempty"`
	Log          string                    `json:"log,omitempty"`
	ErrorMessage string                    `json:"errorMessage,omitempty"`
	Tests        []TestOutcomeResponse     `json:"tests,omitempty"`
	Solutions    []SolutionOutcomeResponse `json:"solutions,omitempty"`
}

func (request BuildResultRequest) Domain(buildID string) authoringdomain.BuildResult {
	result := authoringdomain.BuildResult{
		BuildID: buildID, WorkerID: request.WorkerID, LeaseToken: request.LeaseToken,
		Success: request.Success, Stage: request.Stage, Log: request.Log,
		ErrorMessage: request.ErrorMessage,
		Tests:        make([]authoringdomain.TestOutcome, 0, len(request.Tests)),
		Solutions:    make([]authoringdomain.SolutionOutcome, 0, len(request.Solutions)),
	}
	for _, item := range request.Tests {
		result.Tests = append(result.Tests, authoringdomain.TestOutcome(item))
	}
	for _, item := range request.Solutions {
		result.Solutions = append(result.Solutions, authoringdomain.SolutionOutcome(item))
	}
	return result
}

// BuildPackageResponse acknowledges a materialized artifact.
type BuildPackageResponse struct {
	StoragePath string `json:"storagePath"`
	SHA256      string `json:"sha256"`
	CaseCount   int    `json:"caseCount"`
	Checker     string `json:"checker"`
}
