// Package authoring owns the Polygon-style problem package: the structured
// statement, the testlib sources (checker, validator, generators, solutions),
// the test plan, and the sandboxed build that turns all of it into the
// immutable testdata snapshot the judge pipeline consumes.
//
// The package layer never writes problem_testdata directly. Only a fenced
// build result can publish, so a half-edited package can never become the
// data that graders are judged against.
package authoring

import (
	"errors"
	"time"
)

// File kinds. Each mirrors one Polygon package role.
const (
	KindChecker    = "checker"
	KindValidator  = "validator"
	KindGenerator  = "generator"
	KindSolution   = "solution"
	KindInteractor = "interactor"
)

// Test input sources.
const (
	TestManual    = "manual"
	TestGenerator = "generator"
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
	ErrNotFound      = errors.New("authoring resource not found")
	ErrInvalidInput  = errors.New("invalid authoring input")
	ErrStaleLease    = errors.New("stale build lease")
	ErrBuildRunning  = errors.New("a build is already in progress")
	ErrNotBuildable  = errors.New("problem package is not buildable")
	ErrNotPublished  = errors.New("problem package has no successful build")
	ErrPackageTooBig = errors.New("build package exceeds the configured limit")
	ErrPackageTarget = errors.New("build package does not belong to the leased problem")
)

// ValidationError carries a user-facing reason while still unwrapping to
// ErrInvalidInput so handlers keep one error-mapping branch.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

func invalid(message string) error { return &ValidationError{Message: message} }

// Statement is one localized statement revision.
type Statement struct {
	ProblemID    string
	Language     string
	Name         string
	Legend       string
	InputFormat  string
	OutputFormat string
	Notes        string
	Tutorial     string
	Scoring      string
	UpdatedAt    time.Time
}

// File is one source file of the package. ExpectedVerdict only applies to
// solutions; IsActive marks the entry checker/validator/interactor and the
// main solution (标程).
type File struct {
	ID              int64
	ProblemID       string
	Kind            string
	Name            string
	Language        string
	SourceCode      string
	ExpectedVerdict string
	IsActive        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Test is one planned test. Answers are never authored by hand: they are
// produced by the main solution during the build.
type Test struct {
	ID          int64
	ProblemID   string
	Index       int
	Group       string
	Source      string
	InputData   string
	GenerateCmd string
	IsSample    bool
	Points      int
	Description string
}

// PackageMeta is the problem-level summary shown in the authoring workspace.
// PackageRevision > BuiltRevision means the published testdata is stale.
type PackageMeta struct {
	ProblemPublicID   string
	ProblemID         string
	Title             string
	Visibility        string
	JudgeType         string
	StatementLanguage string
	TimeLimitMs       int
	MemoryLimitKB     int
	PackageRevision   int
	BuiltRevision     int
	LastBuiltAt       *time.Time
	TestdataCases     int
	TestdataChecker   string
	TestdataVersion   int
	TestdataSHA256    string
}

// Package is the complete build input snapshot handed to a worker.
type Package struct {
	ProblemID     string
	Revision      int
	Title         string
	TimeLimitMs   int
	MemoryLimitKB int
	JudgeType     string
	Checker       *File
	Validator     *File
	Interactor    *File
	Generators    []File
	Solutions     []File
	Tests         []Test
}

// MainSolution returns the 标程 whose output becomes every expected answer.
func (p *Package) MainSolution() *File {
	for i := range p.Solutions {
		if p.Solutions[i].IsActive {
			return &p.Solutions[i]
		}
	}
	return nil
}

// TestOutcome is the per-test build report shown in the authoring console.
type TestOutcome struct {
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

// SolutionOutcome reports one non-main solution's observed verdict against the
// verdict the author declared. This is the invocation/对拍 stage.
type SolutionOutcome struct {
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

// Build is one package build attempt with its lease and reported progress.
type Build struct {
	ID            string
	ProblemID     string
	Revision      int
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
	StoragePath string
	SHA256      string
	CaseCount   int
	Checker     string
	// created is true only when this upload installed a new artifact. The
	// service uses it to avoid deleting a pre-existing content-addressed path
	// while compensating for a later database failure.
	created bool
}
