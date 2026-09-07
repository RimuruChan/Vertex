package handler

import "github.com/gin-gonic/gin"

func (h *ContestHandler) RegisterRoutes(
	api *gin.RouterGroup,
	optionalAuth gin.HandlerFunc,
	requireAuth gin.HandlerFunc,
	resolveIDs ...gin.HandlerFunc,
) {
	public := api.Group("/contests")
	public.Use(optionalAuth)
	public.Use(resolveIDs...)
	public.GET("", h.List)
	public.GET("/:id", h.Get)
	public.GET("/:id/rankboard", h.Rankboard)

	authed := api.Group("/contests")
	authed.Use(requireAuth)
	authed.Use(resolveIDs...)
	authed.GET("/:id/problems/:problemId", h.GetProblem)
	authed.PUT("/:id/problems/:problemId/version", h.UseProblemVersion)
	authed.GET("/:id/registration", h.Registration)
	authed.POST("/:id/register", h.Register)
	// Clarifications and staff are contest-scoped: the service checks the
	// caller's jury role rather than a global admin flag, so a contest can be
	// run by people who are not installation administrators.
	authed.GET("/:id/clarifications", h.ListClarifications)
	authed.POST("/:id/clarifications", h.Ask)
	authed.POST("/:id/clarifications/reply", h.Reply)
	authed.GET("/:id/staff", h.ListStaff)
	authed.POST("/:id/staff", h.AddStaff)
	authed.DELETE("/:id/staff/:userId", h.RemoveStaff)
	authed.GET("/:id/access", h.Grants)
	authed.PUT("/:id/access", h.SetGrant)
	authed.DELETE("/:id/access/:grant", h.RemoveGrant)
	authed.PUT("/:id/owner", h.TransferOwner)
	authed.DELETE("/:id", h.Delete)

	admin := api.Group("/admin/contests")
	admin.Use(requireAuth)
	admin.Use(resolveIDs...)
	admin.POST("", h.Create)
	admin.GET("", h.ListAdmin)
	admin.GET("/:id", h.GetAdmin)
	admin.PUT("/:id", h.Update)
	admin.PUT("/:id/problems", h.SetProblems)
}
