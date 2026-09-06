package handler

import "github.com/gin-gonic/gin"

func (h *SubmissionHandler) RegisterRoutes(
	api *gin.RouterGroup,
	requireAuth gin.HandlerFunc,
	requireAdmin gin.HandlerFunc,
	resolveIDs ...gin.HandlerFunc,
) {
	submissions := api.Group("/submissions")
	submissions.Use(requireAuth)
	submissions.Use(resolveIDs...)
	submissions.POST("", h.Submit)
	submissions.GET("", h.List)
	submissions.GET("/:id/progress", h.Progress)
	submissions.GET("/:id", h.Get)

	admin := api.Group("/admin/submissions")
	admin.Use(requireAuth, requireAdmin)
	admin.Use(resolveIDs...)
	admin.POST("/:id/rejudge", h.Rejudge)

	rejudgings := api.Group("/admin/rejudgings")
	rejudgings.Use(requireAuth, requireAdmin)
	rejudgings.Use(resolveIDs...)
	rejudgings.POST("", h.CreateRejudging)
	rejudgings.GET("", h.ListRejudgings)
	rejudgings.GET("/:id", h.GetRejudging)
	rejudgings.GET("/:id/changes", h.RejudgingChanges)
	rejudgings.POST("/:id/cancel", h.CancelRejudging)
}
