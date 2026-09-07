package handler

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the administration console. Announcements have a public
// read route because every visitor should see site notices; everything else
// requires an administrator.
func RegisterPublicRoutes(api *gin.RouterGroup, console *ConsoleHandler, optionalAuth gin.HandlerFunc, scope ...gin.HandlerFunc) {
	public := api.Group("/announcements")
	public.Use(optionalAuth)
	public.Use(scope...)
	public.GET("", console.ListAnnouncements)
}

func RegisterRoutes(api *gin.RouterGroup, console *ConsoleHandler, optionalAuth, requireAuth, requireAdmin gin.HandlerFunc, scope ...gin.HandlerFunc) {
	RegisterPublicRoutes(api, console, optionalAuth, scope...)

	admin := api.Group("/admin")
	admin.Use(requireAuth, requireAdmin)
	admin.GET("/stats", console.Stats)

	admin.GET("/users", console.ListAccounts)
	admin.PATCH("/users/:id", console.UpdateAccount)

	// Account governance and system-wide queue stats intentionally stay
	// outside the domain scope. Tags and notices belong to one domain.
	admin = admin.Group("", scope...)
	admin.GET("/tags", console.ListTags)
	admin.PUT("/tags/:id", console.RenameTag)
	admin.POST("/tags/:id/merge", console.MergeTag)
	admin.DELETE("/tags/:id", console.DeleteTag)

	admin.POST("/announcements", console.CreateAnnouncement)
	admin.PUT("/announcements/:id", console.UpdateAnnouncement)
	admin.DELETE("/announcements/:id", console.DeleteAnnouncement)
}
