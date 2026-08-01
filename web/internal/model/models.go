package model

import (
	"time"
)

// User 对应 users 表。
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"` // user | admin
	Rating       int       `json:"rating"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Problem 对应 problems 表。
type Problem struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	StatementMD      string    `json:"statementMd"`
	Difficulty       int       `json:"difficulty"`
	Source           string    `json:"source"`
	TimeLimitMs      int       `json:"timeLimitMs"`
	MemoryLimitKb    int       `json:"memoryLimitKb"`
	Visibility       string    `json:"visibility"` // draft | private | public
	AuthorID         *string   `json:"authorId,omitempty"`
	SubmissionCount  int       `json:"submissionCount"`
	AcceptedCount    int       `json:"acceptedCount"`
	SolvedUserCount  int       `json:"solvedUserCount"`
	JudgeType        string    `json:"judgeType"` // normal | interactive
	Tags             []string  `json:"tags"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// TestdataInfo 对应 problem_testdata 表(元信息,不含文件内容)。
type TestdataInfo struct {
	ProblemID   string         `json:"problemId"`
	DataVersion int            `json:"dataVersion"`
	StoragePath string         `json:"storagePath"`
	SHA256      string         `json:"sha256"`
	CaseCount   int            `json:"caseCount"`
	Checker     string         `json:"checker"` // diff | spj | interactive
	SPJSource   string         `json:"spjSource"`
	Config      map[string]any `json:"config"` // 每测试点限覆盖 / batched 依赖
}

// Submission 对应 submissions 表。
type Submission struct {
	ID            string         `json:"id"`
	UserID        string         `json:"userId"`
	ProblemID     string         `json:"problemId"`
	Language      string         `json:"language"`
	SourceCode    string         `json:"sourceCode,omitempty"` // 列表接口省略
	Status        string         `json:"status"`
	Score         int            `json:"score"`
	TotalTimeMs   int            `json:"totalTimeMs"`
	PeakMemoryKb  int            `json:"peakMemoryKb"`
	CompileResult string         `json:"compileResult,omitempty"`
	CaseResults   []CaseResult   `json:"caseResults,omitempty"`
	ContestID     *string        `json:"contestId,omitempty"`
	SubmittedAt   time.Time      `json:"submittedAt"`
	JudgedAt      *time.Time     `json:"judgedAt,omitempty"`
	Username      string         `json:"username,omitempty"` // join 填充
	ProblemTitle  string         `json:"problemTitle,omitempty"`
}

// CaseResult 单个测试点的判定快照(也存于 submission_cases 表)。
type CaseResult struct {
	CaseIndex     int    `json:"caseIndex"`
	Verdict       string `json:"verdict"`
	TimeMs        int    `json:"timeMs"`
	MemoryKb      int    `json:"memoryKb"`
	ExitStatus    string `json:"exitStatus,omitempty"`
	CheckerOutput string `json:"checkerOutput,omitempty"`
}

// Contest 对应 contests 表。
type Contest struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Rule             string    `json:"rule"` // acm | ioi
	BeginAt          time.Time `json:"beginAt"`
	EndAt            time.Time `json:"endAt"`
	FreezeAt         *time.Time `json:"freezeAt,omitempty"`
	Visibility       string    `json:"visibility"` // public | private | password
	PasswordHash     string    `json:"-"`
	RankboardVisible bool      `json:"rankboardVisible"`
	CreatedBy        *string   `json:"createdBy,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}

// ContestProblem 比赛题目关联。
type ContestProblem struct {
	ContestID string `json:"contestId"`
	ProblemID string `json:"problemId"`
	SortOrder int    `json:"sortOrder"`
}

// Editorial 对应 editorials 表(题解)。
type Editorial struct {
	ID         string    `json:"id"`
	ProblemID  string    `json:"problemId"`
	AuthorID   *string   `json:"authorId,omitempty"`
	AuthorName string    `json:"authorName,omitempty"`
	Title      string    `json:"title"`
	ContentMD  string    `json:"contentMd"`
	Visibility string    `json:"visibility"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// DiscussionPost 对应 discussion_posts 表(评论)。
type DiscussionPost struct {
	ID          int64      `json:"id"`
	ProblemID   *string    `json:"problemId,omitempty"`
	EditorialID *string    `json:"editorialId,omitempty"`
	ContestID   *string    `json:"contestId,omitempty"`
	AuthorID    *string    `json:"authorId,omitempty"`
	AuthorName  string     `json:"authorName,omitempty"`
	ContentMD   string     `json:"contentMd"`
	ParentID    *int64     `json:"parentId,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}
