package handler

import "github.com/gin-gonic/gin"

func (h *ProfileHandler) RegisterRoutes(api *gin.RouterGroup, scope ...gin.HandlerFunc) {
	profiles := api.Group("/users", scope...)
	profiles.GET("/:username", h.Get)
}
