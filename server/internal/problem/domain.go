package problem

import "time"

type Problem struct {
	ID              string
	Title           string
	StatementMD     string
	Difficulty      int
	Source          string
	TimeLimitMs     int
	MemoryLimitKb   int
	Visibility      string
	AuthorID        *string
	SubmissionCount int
	AcceptedCount   int
	SolvedUserCount int
	JudgeType       string
	Tags            []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type TestdataInfo struct {
	ProblemID   string
	DataVersion int
	StoragePath string
	SHA256      string
	CaseCount   int
	Checker     string
	SPJSource   string
	Config      map[string]any
}
