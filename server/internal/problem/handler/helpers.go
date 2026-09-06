package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/problem"
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

func writeProblemError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrUnauthenticated):
		writeAPIError(c, http.StatusUnauthorized, "auth.invalid_token", "authentication required")
	case errors.Is(err, domain.ErrForbidden):
		writeAPIError(c, http.StatusForbidden, "problem.forbidden", "insufficient problem permissions")
	case errors.Is(err, problem.ErrNotFound), errors.Is(err, domain.ErrNotFound):
		writeAPIError(c, http.StatusNotFound, "problem.not_found", "problem not found")
	case errors.Is(err, problem.ErrInvalidInput):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", err.Error())
	default:
		_ = c.Error(err)
		writeAPIError(c, http.StatusInternalServerError, "problem.failed", "problem request failed")
	}
}
