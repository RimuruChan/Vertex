package handler

import "github.com/gin-gonic/gin"

func RegisterRoutes(
	api *gin.RouterGroup,
	public *ProblemHandler,
	admin *AdminProblemHandler,
	optionalAuth gin.HandlerFunc,
	requireAuth gin.HandlerFunc,
	requireAdmin gin.HandlerFunc,
	resolveIDs ...gin.HandlerFunc,
) {
	// optionalAuth lets the public list annotate per-viewer progress without
	// making the endpoint itself authenticated.
	publicRoutes := api.Group("/problems", optionalAuth)
	publicRoutes.Use(resolveIDs...)
	publicRoutes.GET("", public.List)
	publicRoutes.GET("/:id", public.Get)
	api.GET("/tags", public.Tags)

	adminRoutes := api.Group("/admin/problems")
	adminRoutes.Use(requireAuth, requireAdmin)
	adminRoutes.Use(resolveIDs...)
	adminRoutes.GET("", admin.List)
	adminRoutes.POST("", admin.Create)
	adminRoutes.GET("/:id", admin.Get)
	adminRoutes.PUT("/:id", admin.Update)
	adminRoutes.DELETE("/:id", admin.Delete)
	adminRoutes.POST("/:id/testdata", admin.UploadTestdata)
}
