package handler

import "github.com/gin-gonic/gin"

func (h *AuthHandler) RegisterRoutes(api *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	auth := api.Group("/auth")
	auth.POST("/register", h.Register)
	auth.POST("/login", h.Login)
	auth.POST("/refresh", h.Refresh)
	auth.GET("/me", requireAuth, h.Me)
	auth.POST("/logout", requireAuth, h.Logout)
	auth.POST("/logout-all", requireAuth, h.LogoutAll)
}
