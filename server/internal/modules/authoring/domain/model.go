// Package domain describes versioned problem materials and frozen sandbox checks.
// Explicit publication binds an immutable commit and matching successful check
// into the release used by judging.
package domain

import (
	"errors"
	"time"
)

// Build states. queued/running are lease-managed; the rest are terminal.
const (
	BuildQueued    = "queued"
	BuildRunning   = "running"
	BuildSucceeded = "succeeded"
	BuildFailed    = "failed"
	BuildCancelled = "cancelled"
	BuildDead      = "dead"
)

// Build stages are advisory progress labels reported by the worker.
const (
	StageQueued    = "queued"
	StageCompile   = "compile"
	StageGenerate  = "generate"
	StageValidate  = "validate"
	StageAnswer    = "answer"
	StageCheck     = "check"
	StageSolutions = "solutions"
	StagePackage   = "package"
	StageDone      = "done"
)

var (
	ErrNotFound         = errors.New("authoring resource not found")
	ErrInvalidInput     = errors.New("invalid authoring input")
	ErrStaleLease       = errors.New("stale build lease")
	ErrBuildRunning     = errors.New("a build is already in progress")
	ErrNotBuildable     = errors.New("problem package is not buildable")
	ErrNotPublished     = errors.New("problem has no usable checked release")
	ErrRevisionConflict = errors.New("published version changed; refresh before publishing")
	ErrPackageTooBig    = errors.New("build package exceeds the configured limit")
	ErrPackageTarget    = errors.New("build package does not belong to the leased problem")
)

// ValidationError carries a user-facing reason while still unwrapping to
// ErrInvalidInput so handlers keep one error-mapping branch.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

func InvalidInput(message string) error { return &ValidationError{Message: message} }

// CheckInput is the immutable envelope for one frozen authoring check.
// Program sources and tests are fetched by their authorized content references.
type CheckInput struct {
	Check     *CheckSnapshot `json:"check"`
	DomainID  string         `json:"domainId"`
	ProblemID string         `json:"problemId"`
}

// TestOutcome is the per-test build report shown in the authoring console.
type TestOutcome struct {
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

// SolutionOutcome reports one non-main solution's observed verdict against the
// verdict the author declared. This is the invocation/对拍 stage.
type SolutionOutcome struct {
	Cases           []SolutionCaseOutcome `json:"cases,omitempty"`
	Name            string                `json:"name"`
	Language        string                `json:"language"`
	ExpectedVerdict string                `json:"expectedVerdict,omitempty"`
	ActualVerdict   string                `json:"actualVerdict"`
	FailedTest      int                   `json:"failedTest,omitempty"`
	MaxTimeMs       int                   `json:"maxTimeMs"`
	MaxMemoryKB     int                   `json:"maxMemoryKb"`
	Matched         bool                  `json:"matched"`
	Message         string                `json:"message,omitempty"`
}

type SolutionCaseOutcome struct {
	Index    int    `json:"index"`
	Verdict  string `json:"verdict"`
	TimeMs   int    `json:"timeMs"`
	MemoryKB int    `json:"memoryKb"`
}

// Build is one package build attempt with its lease and reported progress.
type Build struct {
	ProblemNumber string
	ID            string
	ProblemID     string
	State         string
	Stage         string
	Attempt       int
	WorkerID      string
	LeaseToken    string
	LeaseExpires  time.Time
	ProgressDone  int
	ProgressTotal int
	Log           string
	ErrorMessage  string
	Tests         []TestOutcome
	Solutions     []SolutionOutcome
	PackagePath   string
	PackageSHA256 string
	PackageCases  int
	CreatedBy     *string
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

// BuildResult is the fenced completion payload posted by a worker.
type BuildResult struct {
	Validation   []ValidationOutcome
	ToolchainKey string
	BuildID      string
	WorkerID     string
	LeaseToken   string
	Success      bool
	Stage        string
	Log          string
	ErrorMessage string
	Tests        []TestOutcome
	Solutions    []SolutionOutcome
}

// Progress is the fenced heartbeat payload posted by a worker.
type Progress struct {
	BuildID    string
	WorkerID   string
	LeaseToken string
	Stage      string
	Done       int
	Total      int
	Log        string
}

// PackageUpload is the materialized artifact recorded before completion.
type PackageUpload struct {
	Artifact    *CheckArtifact
	StoragePath string
	SHA256      string
	CaseCount   int
	Checker     string
	// Created reports whether this upload installed a new artifact. Unreferenced
	// uploads are reclaimed after the GC grace period, never during SQL rollback.
	Created bool
}
