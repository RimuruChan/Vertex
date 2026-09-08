package httpapi

import "github.com/gin-gonic/gin"

// RegisterRoutes wires curated problem sets. Reading uses optional
// authentication so a signed-in visitor sees their own progress and drafts,
// while an anonymous one still gets the public catalogue.
func RegisterRoutes(api *gin.RouterGroup, sets *SetHandler, optionalAuth, requireAuth gin.HandlerFunc, resolveIDs ...gin.HandlerFunc) {
	public := api.Group("/problem-sets")
	public.Use(optionalAuth)
	public.Use(resolveIDs...)
	public.GET("", sets.List)
	public.GET("/:id", sets.Get)

	authed := api.Group("/problem-sets")
	authed.Use(requireAuth)
	authed.Use(resolveIDs...)
	authed.POST("", sets.Create)
	authed.PUT("/:id", sets.Update)
	authed.DELETE("/:id", sets.Delete)
	authed.PUT("/:id/items", sets.SetItems)
	authed.GET("/:id/access", sets.Grants)
	authed.PUT("/:id/access", sets.SetGrant)
	authed.DELETE("/:id/access/:grantId", sets.RemoveGrant)
	authed.PUT("/:id/owner", sets.Transfer)
}
