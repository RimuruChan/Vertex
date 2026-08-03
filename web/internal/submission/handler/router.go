package handler

import "github.com/gin-gonic/gin"

func (h *SubmissionHandler) RegisterRoutes(
	api *gin.RouterGroup,
	requireAuth gin.HandlerFunc,
	requireAdmin gin.HandlerFunc,
) {
	submissions := api.Group("/submissions")
	submissions.Use(requireAuth)
	submissions.POST("", h.Submit)
	submissions.GET("", h.List)
	submissions.GET("/:id", h.Get)

	admin := api.Group("/admin/submissions")
	admin.Use(requireAuth, requireAdmin)
	admin.POST("/:id/rejudge", h.Rejudge)
}
