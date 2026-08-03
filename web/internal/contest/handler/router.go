package handler

import "github.com/gin-gonic/gin"

func (h *ContestHandler) RegisterRoutes(
	api *gin.RouterGroup,
	optionalAuth gin.HandlerFunc,
	requireAuth gin.HandlerFunc,
	requireAdmin gin.HandlerFunc,
) {
	public := api.Group("/contests")
	public.Use(optionalAuth)
	public.GET("", h.List)
	public.GET("/:id", h.Get)
	public.GET("/:id/rankboard", h.Rankboard)

	authed := api.Group("/contests")
	authed.Use(requireAuth)
	authed.GET("/:id/registration", h.Registration)
	authed.POST("/:id/register", h.Register)

	admin := api.Group("/admin/contests")
	admin.Use(requireAuth, requireAdmin)
	admin.POST("", h.Create)
	admin.GET("", h.ListAdmin)
	admin.GET("/:id", h.GetAdmin)
	admin.PUT("/:id", h.Update)
	admin.PUT("/:id/problems", h.SetProblems)
}
