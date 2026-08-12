package handler

import "github.com/gin-gonic/gin"

func (h *ProfileHandler) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/users/:username", h.Get)
}
