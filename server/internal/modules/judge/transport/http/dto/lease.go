package dto

type LeaseRequest struct {
	WorkerID   string `json:"workerId" binding:"required"`
	Generation int    `json:"generation" binding:"required"`
	LeaseToken string `json:"leaseToken" binding:"required"`
	// JudgedCases 是该 job 已判完的测试点数,仅用于前端进度显示。
	// 老 worker 不带该字段时按 0 处理,不影响续租。
	JudgedCases int `json:"judgedCases,omitempty"`
}
