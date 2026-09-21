package handler

import "github.com/gin-gonic/gin"

// RegisterInternalRoutes wires the build worker protocol next to the judge
// worker protocol so both share one credential and one base URL.
func (h *BuildHandler) RegisterInternalRoutes(internal *gin.RouterGroup, requireJudge gin.HandlerFunc) {
	builds := internal.Group("/judge/v1/builds")
	builds.Use(requireJudge)
	builds.POST("/claim", h.Claim)
	builds.POST("/:buildId/progress", h.Progress)
	builds.POST("/:buildId/package", h.UploadPackage)
	builds.PUT("/:buildId/result", h.Complete)
}
