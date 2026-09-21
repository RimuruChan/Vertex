package dto

import (
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// ---------- author-facing build reports ----------

type TestOutcomeResponse struct {
	Index          int     `json:"index"`
	Group          string  `json:"group,omitempty"`
	Source         string  `json:"source"`
	Command        string  `json:"command,omitempty"`
	InputBytes     int64   `json:"inputBytes"`
	AnswerBytes    int64   `json:"answerBytes"`
	TimeMs         int     `json:"timeMs"`
	MemoryKB       int     `json:"memoryKb"`
	IsSample       bool    `json:"isSample"`
	Points         float64 `json:"points"`
	HeadsTruncated bool    `json:"headsTruncated,omitempty"`
	Status         string  `json:"status"`
	Message        string  `json:"message,omitempty"`
	InputHead      string  `json:"inputHead,omitempty"`
	AnswerHead     string  `json:"answerHead,omitempty"`
}

type SolutionOutcomeResponse struct {
	Cases           []authoringdomain.SolutionCaseOutcome `json:"cases,omitempty"`
	Name            string                                `json:"name"`
	Language        string                                `json:"language"`
	ExpectedVerdict string                                `json:"expectedVerdict,omitempty"`
	ActualVerdict   string                                `json:"actualVerdict"`
	FailedTest      int                                   `json:"failedTest,omitempty"`
	MaxTimeMs       int                                   `json:"maxTimeMs"`
	MaxMemoryKB     int                                   `json:"maxMemoryKb"`
	Matched         bool                                  `json:"matched"`
	Message         string                                `json:"message,omitempty"`
}

// ---------- internal build worker protocol ----------

type BuildClaimRequest struct {
	CheckProtocol string `json:"checkProtocol" binding:"required"`
	WorkerID      string `json:"workerId" binding:"required"`
	WaitSeconds   int    `json:"waitSeconds,omitempty"`
}

// BuildJobResponse carries a frozen snapshot and lease-authorized blob references.
type BuildJobResponse struct {
	Check          *authoringdomain.CheckSnapshot `json:"check"`
	DomainID       string                         `json:"domainId"`
	BuildID        string                         `json:"buildId"`
	ProblemID      string                         `json:"problemId"`
	Attempt        int                            `json:"attempt"`
	LeaseToken     string                         `json:"leaseToken"`
	LeaseExpiresAt time.Time                      `json:"leaseExpiresAt"`
	Limits         BuildLimitsPayload             `json:"limits"`
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

// BuildJobFromDomain exposes references, never inline source or test data.
func BuildJobFromDomain(build authoringdomain.Build, input authoringdomain.CheckInput, limits BuildLimitsPayload) BuildJobResponse {
	return BuildJobResponse{Check: input.Check, DomainID: input.DomainID, BuildID: build.ID, ProblemID: build.ProblemID, Attempt: build.Attempt, LeaseToken: build.LeaseToken, LeaseExpiresAt: build.LeaseExpires, Limits: limits}
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
	Validation   []authoringdomain.ValidationOutcome `json:"validation,omitempty"`
	ToolchainKey string                              `json:"toolchainKey,omitempty"`
	WorkerID     string                              `json:"workerId" binding:"required"`
	LeaseToken   string                              `json:"leaseToken" binding:"required"`
	Success      bool                                `json:"success"`
	Stage        string                              `json:"stage,omitempty"`
	Checker      string                              `json:"checker,omitempty"`
	Log          string                              `json:"log,omitempty"`
	ErrorMessage string                              `json:"errorMessage,omitempty"`
	Tests        []TestOutcomeResponse               `json:"tests,omitempty"`
	Solutions    []SolutionOutcomeResponse           `json:"solutions,omitempty"`
}

func (request BuildResultRequest) Domain(buildID string) authoringdomain.BuildResult {
	result := authoringdomain.BuildResult{
		Validation:   request.Validation,
		ToolchainKey: request.ToolchainKey,
		BuildID:      buildID, WorkerID: request.WorkerID, LeaseToken: request.LeaseToken,
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
