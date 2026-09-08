package domain

// 判题状态常量(与 migrations 中的 CHECK 约束一致)。
const (
	StatusPending     = "Pending"
	StatusJudging     = "Judging"
	StatusAccepted    = "Accepted"
	StatusWrongAnswer = "Wrong Answer"
	StatusTLE         = "Time Limit Exceeded"
	StatusMLE         = "Memory Limit Exceeded"
	StatusRE          = "Runtime Error"
	StatusCE          = "Compile Error"
	StatusOLE         = "Output Limit Exceeded"
	StatusSE          = "System Error"
	StatusSkipped     = "Skipped"
)
