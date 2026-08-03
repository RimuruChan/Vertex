package handler

import (
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/gin-gonic/gin"
)

func parseIntDefault(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func writeAPIError(c *gin.Context, status int, code, message string) {
	httpx.WriteError(c, status, code, message)
}
