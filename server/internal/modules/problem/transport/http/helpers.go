package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
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
	case errors.Is(err, tenancydomain.ErrUnauthenticated):
		writeAPIError(c, http.StatusUnauthorized, "auth.invalid_token", "authentication required")
	case errors.Is(err, tenancydomain.ErrForbidden):
		writeAPIError(c, http.StatusForbidden, "problem.forbidden", "insufficient problem permissions")
	case errors.Is(err, problemdomain.ErrNotFound), errors.Is(err, tenancydomain.ErrNotFound):
		writeAPIError(c, http.StatusNotFound, "problem.not_found", "problem not found")
	case errors.Is(err, problemdomain.ErrInvalidInput):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", err.Error())
	case errors.Is(err, problemdomain.ErrReferenced):
		writeAPIError(c, http.StatusConflict, "problem.referenced", err.Error())
	default:
		_ = c.Error(err)
		writeAPIError(c, http.StatusInternalServerError, "problem.failed", "problem request failed")
	}
}
