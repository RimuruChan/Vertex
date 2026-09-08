package domain

import "time"

// 当前查看者对某道题的进度状态。未登录时一律为 UserStatusNone。
const (
	UserStatusNone      = "none"
	UserStatusAttempted = "attempted"
	UserStatusSolved    = "solved"
)

type Problem struct {
	PublishedVersion int
	OwnerID          string
	OwnerName        string
	DomainID         string
	Permissions      Permissions
	PublicID         string
	ID               string
	Title            string
	StatementMD      string
	Difficulty       int
	Source           string
	TimeLimitMs      int
	MemoryLimitKb    int
	Visibility       string
	AuthorID         *string
	SubmissionCount  int
	AcceptedCount    int
	SolvedUserCount  int
	JudgeType        string
	Tags             []string
	UserStatus       string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Tag 是标签目录条目,ProblemCount 只统计 public 题目。
type Tag struct {
	Name         string
	ProblemCount int
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
