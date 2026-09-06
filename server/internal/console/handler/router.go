package handler

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the administration console. Announcements have a public
// read route because every visitor should see site notices; everything else
// requires an administrator.
func RegisterRoutes(api *gin.RouterGroup, console *ConsoleHandler, optionalAuth, requireAuth, requireAdmin gin.HandlerFunc) {
	public := api.Group("/announcements")
	public.Use(optionalAuth)
	public.GET("", console.ListAnnouncements)

	admin := api.Group("/admin")
	admin.Use(requireAuth, requireAdmin)
	admin.GET("/stats", console.Stats)

	admin.GET("/users", console.ListAccounts)
	admin.PATCH("/users/:id", console.UpdateAccount)

	admin.GET("/tags", console.ListTags)
	admin.PUT("/tags/:id", console.RenameTag)
	admin.POST("/tags/:id/merge", console.MergeTag)
	admin.DELETE("/tags/:id", console.DeleteTag)

	admin.POST("/announcements", console.CreateAnnouncement)
	admin.PUT("/announcements/:id", console.UpdateAnnouncement)
	admin.DELETE("/announcements/:id", console.DeleteAnnouncement)
}
