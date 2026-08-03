package handler

import (
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/gin-gonic/gin"
)

func pagination(c *gin.Context) (int, int) {
	page := parseIntDefault(c.Query("page"), 1)
	size := parseIntDefault(c.Query("size"), 20)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

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
