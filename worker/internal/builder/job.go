// Package builder turns a problem package into a testdata snapshot. It runs
// the same sandbox as judging: generators, validators, the model solution and
// the checker are all jury-authored, but none of them is trusted with the
// worker's own filesystem or network.
package builder

import (
	"context"
	"io"
	"time"
)

// Stage names mirror the server's build stages so progress reports line up
// with what the authoring console renders.
const (
	StageCompile   = "compile"
	StageGenerate  = "generate"
	StageValidate  = "validate"
	StageAnswer    = "answer"
	StageCheck     = "check"
	StageSolutions = "solutions"
	StagePackage   = "package"
)

// Limits is the per-stage budget handed down by the server.
type Limits struct {
	GeneratorTimeMs int
	ValidatorTimeMs int
	SolutionTimeMs  int
	CheckerTimeMs   int
	MemoryLimitKB   int
}

// Job is one leased package build.
type Job struct {
	Check        *FrozenSnapshot
	FetchContent func(context.Context, BlobRef) (io.ReadCloser, error)
	DomainID     string
	BuildID      string
	ProblemID    string
	Attempt      int
	LeaseToken   string
	LeaseExpires time.Time
	Limits       Limits
}

// TestOutcome is the per-test report the authoring console renders.
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

// SolutionOutcome reports one alternate solution's observed verdict.
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

// Report is the complete build outcome.
type Report struct {
	Validation   []ValidationOutcome
	ToolchainKey string
	Success      bool
	Stage        string
	ErrorMessage string
	Checker      string
	Archive      []byte
	Tests        []TestOutcome
	Solutions    []SolutionOutcome
}

type ValidationOutcome struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Mode    string `json:"mode"`
	Actual  string `json:"actual"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Reporter receives progress from a running build. Implementations renew the
// lease, so a build that stops reporting eventually loses its job.
type Reporter interface {
	Stage(stage string, done, total int)
	Logf(format string, args ...any)
}

// nopReporter lets tests and one-off runs skip progress plumbing.
type nopReporter struct{}

func (nopReporter) Stage(string, int, int) {}
func (nopReporter) Logf(string, ...any)    {}

// NopReporter returns a reporter that discards progress.
func NopReporter() Reporter { return nopReporter{} }
