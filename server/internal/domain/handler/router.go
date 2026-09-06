package handler

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(api *gin.RouterGroup, optionalAuth, requireAuth gin.HandlerFunc) {
	api.GET("/domain-permissions", h.Permissions)
	api.GET("/domains", optionalAuth, h.List)
	api.POST("/domains", requireAuth, h.Create)
	api.GET("/domains/:domain", optionalAuth, h.Get)
	scoped := api.Group("/domains/:domain", requireAuth)
	scoped.PUT("", h.Update)
	scoped.POST("/membership", h.Join)
	scoped.PUT("/owner", h.Transfer)
	scoped.PUT("/archive", h.Archive)
	scoped.GET("/members", h.Members)
	scoped.PUT("/members/:username", h.SetMember)
	scoped.GET("/roles", h.Roles)
	scoped.PUT("/roles/:role", h.SaveRole)
	scoped.DELETE("/roles/:role", h.DeleteRole)
	scoped.GET("/groups", h.Groups)
	scoped.POST("/groups", h.CreateGroup)
	scoped.GET("/groups/:group", h.Group)
	scoped.PUT("/groups/:group", h.UpdateGroup)
	scoped.DELETE("/groups/:group", h.DeleteGroup)
	scoped.PUT("/groups/:group/owner", h.TransferGroup)
	scoped.GET("/groups/:group/members", h.GroupMembers)
	scoped.PUT("/groups/:group/members/:username", h.SetGroupMember)
	scoped.DELETE("/groups/:group/members/:username", h.RemoveGroupMember)
}
