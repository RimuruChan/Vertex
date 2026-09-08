package dto

type ProblemVersionRequest struct {
	Version         int `json:"version" binding:"required,gt=0"`
	ExpectedVersion int `json:"expectedVersion" binding:"required,gt=0"`
}
