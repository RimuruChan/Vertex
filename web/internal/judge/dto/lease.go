package dto

type LeaseRequest struct {
	WorkerID   string `json:"workerId" binding:"required"`
	Generation int    `json:"generation" binding:"required"`
	LeaseToken string `json:"leaseToken" binding:"required"`
}
