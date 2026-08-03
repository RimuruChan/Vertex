package handler

import "github.com/gin-gonic/gin"

func (h *JudgeHandler) RegisterRoutes(internal *gin.RouterGroup, requireJudge gin.HandlerFunc) {
	jobs := internal.Group("/judge/v1/jobs")
	jobs.Use(requireJudge)
	jobs.POST("/claim", h.Claim)
	jobs.POST("/:jobId/heartbeat", h.Heartbeat)
	jobs.PUT("/:jobId/result", h.Complete)
}
