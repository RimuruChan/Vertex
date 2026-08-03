package judge

import (
	"errors"
	"time"
)

var (
	ErrStaleLease    = errors.New("stale judge lease")
	ErrInvalidResult = errors.New("invalid judge result")
)

type Job struct {
	ID             string
	SubmissionID   string
	Generation     int
	Attempt        int
	WorkerID       string
	LeaseToken     string
	LeaseExpiresAt time.Time
	UserID         string
	ProblemID      string
	ContestID      *string
	Language       string
	SourceCode     string
	TimeLimitMs    int
	MemoryLimitKB  int
	Testdata       Testdata
}

type Testdata struct {
	StoragePath string
	DataVersion int
	SHA256      string
	CaseCount   int
	Checker     string
}

type CaseResult struct {
	CaseIndex     int
	Verdict       string
	TimeMs        int
	MemoryKB      int
	ExitStatus    string
	CheckerOutput string
}

type Result struct {
	JobID         string
	SubmissionID  string
	Generation    int
	LeaseToken    string
	WorkerID      string
	Status        string
	Score         int
	TotalTimeMs   int64
	PeakMemoryKB  int
	CompileResult string
	Cases         []CaseResult
}
