package httpapi

import "github.com/gin-gonic/gin"

// Published notices inherit domain access; management has a separate route.
func RegisterPublicRoutes(api *gin.RouterGroup, console *ConsoleHandler, optionalAuth gin.HandlerFunc, scope ...gin.HandlerFunc) {
	public := api.Group("/announcements")
	public.Use(optionalAuth)
	public.Use(scope...)
	public.GET("", console.ListAnnouncements)
	public.GET("/:id", console.GetAnnouncement)
}

func RegisterRoutes(api *gin.RouterGroup, console *ConsoleHandler, optionalAuth, requireAuth, requireAdmin gin.HandlerFunc, scope ...gin.HandlerFunc) {
	RegisterResourceRoutes(api, console, optionalAuth, requireAuth, scope...)

	admin := api.Group("/admin")
	admin.Use(requireAuth, requireAdmin)
	admin.GET("/stats", console.Stats)

	admin.GET("/users", console.ListAccounts)
	admin.PATCH("/users/:id", console.UpdateAccount)
}

func RegisterResourceRoutes(api *gin.RouterGroup, console *ConsoleHandler, optionalAuth, requireAuth gin.HandlerFunc, scope ...gin.HandlerFunc) {
	RegisterPublicRoutes(api, console, optionalAuth, scope...)
	admin := api.Group("/admin", requireAuth)
	admin.Use(scope...)
	admin.Use(console.RequireResourceManagement)
	admin.GET("/tags", console.ListTags)
	admin.GET("/tags/:id", console.GetTag)
	admin.POST("/tags", console.CreateTag)
	admin.PUT("/tags/:id", console.RenameTag)
	admin.POST("/tags/:id/merge", console.MergeTag)
	admin.DELETE("/tags/:id", console.DeleteTag)

	admin.POST("/announcements", console.CreateAnnouncement)
	admin.GET("/announcements", console.ListManagedAnnouncements)
	admin.GET("/announcements/:id", console.GetManagedAnnouncement)
	admin.PUT("/announcements/:id", console.UpdateAnnouncement)
	admin.DELETE("/announcements/:id", console.DeleteAnnouncement)
}
