package httpapi

import (
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(
	api *gin.RouterGroup,
	public *ProblemHandler,
	admin *AdminProblemHandler,
	optionalAuth gin.HandlerFunc,
	requireAuth gin.HandlerFunc,
	resolveIDs ...gin.HandlerFunc,
) {
	// optionalAuth lets the public list annotate per-viewer progress without
	// making the endpoint itself authenticated.
	publicRoutes := api.Group("/problems", optionalAuth)
	publicRoutes.Use(resolveIDs...)
	publicRoutes.Use(httpx.NumberParam("problems", "id"))
	publicRoutes.GET("", public.List)
	publicRoutes.GET("/:id", public.Get)
	publicRoutes.GET("/:id/files/:fileId", public.GetFile)
	tags := api.Group("/tags", optionalAuth)
	tags.Use(resolveIDs...)
	tags.GET("", public.Tags)

	adminRoutes := api.Group("/admin/problems")
	adminRoutes.Use(requireAuth)
	adminRoutes.Use(resolveIDs...)
	adminRoutes.Use(httpx.NumberParam("problems", "id"))
	adminRoutes.GET("", admin.List)
	adminRoutes.POST("", admin.Create)
	adminRoutes.GET("/:id", admin.Get)
	adminRoutes.DELETE("/:id", admin.Delete)
	adminRoutes.GET("/:id/access", admin.Grants)
	adminRoutes.PUT("/:id/access", admin.SetGrant)
	adminRoutes.DELETE("/:id/access/:grant", admin.RemoveGrant)
	adminRoutes.PUT("/:id/owner", admin.TransferOwner)
}
