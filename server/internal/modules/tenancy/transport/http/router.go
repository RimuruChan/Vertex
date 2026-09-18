package httpapi

import (
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

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
	scoped.GET("/groups/:group", httpx.RequireNumberParam("group"), h.Group)
	scoped.PUT("/groups/:group", httpx.RequireNumberParam("group"), h.UpdateGroup)
	scoped.DELETE("/groups/:group", httpx.RequireNumberParam("group"), h.DeleteGroup)
	scoped.PUT("/groups/:group/owner", httpx.RequireNumberParam("group"), h.TransferGroup)
	scoped.GET("/groups/:group/members", httpx.RequireNumberParam("group"), h.GroupMembers)
	scoped.PUT("/groups/:group/members/:username", httpx.RequireNumberParam("group"), h.SetGroupMember)
	scoped.DELETE("/groups/:group/members/:username", httpx.RequireNumberParam("group"), h.RemoveGroupMember)
}
