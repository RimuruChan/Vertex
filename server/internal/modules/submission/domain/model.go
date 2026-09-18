package domain

import "time"

type SubmissionRecord struct {
	AsOf time.Time
	Submission
	FrozenResult bool

	CanReadSource bool

	ProblemPublicID string
	ContestPublicID *string

	// JudgedCases / TotalCases 驱动前端的判题进度条,判完后等于测试点总数。

	Username     string
	ProblemTitle string
	Judgement
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
	AsOf          time.Time
	PublicID      string
	FrozenResult  bool
	CanReadSource bool
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
type Submission struct {
	PublicID string

	ID         string
	UserID     string
	ProblemID  string
	Language   string
	SourceCode string

	ContestID   *string
	SubmittedAt time.Time
}
type Judgement struct {
	ProblemVersion int

	Status        string
	Score         int
	TotalTimeMs   int
	PeakMemoryKb  int
	CompileResult string
	CaseResults   []CaseResult

	JudgedCases int
	TotalCases  int

	JudgedAt *time.Time
}
