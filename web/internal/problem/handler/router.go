package handler

import "github.com/gin-gonic/gin"

func RegisterRoutes(
	api *gin.RouterGroup,
	public *ProblemHandler,
	admin *AdminProblemHandler,
	requireAuth gin.HandlerFunc,
	requireAdmin gin.HandlerFunc,
) {
	api.GET("/problems", public.List)
	api.GET("/problems/:id", public.Get)

	adminRoutes := api.Group("/admin/problems")
	adminRoutes.Use(requireAuth, requireAdmin)
	adminRoutes.GET("", admin.List)
	adminRoutes.POST("", admin.Create)
	adminRoutes.GET("/:id", admin.Get)
	adminRoutes.PUT("/:id", admin.Update)
	adminRoutes.DELETE("/:id", admin.Delete)
	adminRoutes.POST("/:id/testdata", admin.UploadTestdata)
}
