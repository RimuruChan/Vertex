package domain

import "time"

type Submission struct {
	FrozenResult    bool
	ProblemVersion  int
	CanReadSource   bool
	PublicID        string
	ProblemPublicID string
	ContestPublicID *string
	ID              string
	UserID          string
	ProblemID       string
	Language        string
	SourceCode      string
	Status          string
	Score           int
	TotalTimeMs     int
	PeakMemoryKb    int
	CompileResult   string
	CaseResults     []CaseResult
	// JudgedCases / TotalCases 驱动前端的判题进度条,判完后等于测试点总数。
	JudgedCases  int
	TotalCases   int
	ContestID    *string
	SubmittedAt  time.Time
	JudgedAt     *time.Time
	Username     string
	ProblemTitle string
}

type CaseResult struct {
	CaseIndex     int
	Verdict       string
	TimeMs        int
	MemoryKb      int
	ExitStatus    string
	CheckerOutput string
}

// SubmissionProgress is the polling read model. UserID and ContestID are
// internal policy inputs and are intentionally absent from its HTTP DTO.
type SubmissionProgress struct {
	ID            string
	UserID        string
	ContestID     *string
	Status        string
	Score         int
	TotalTimeMs   int
	PeakMemoryKb  int
	CompileResult string
	CaseResults   []CaseResult
	JudgedCases   int
	TotalCases    int
}
