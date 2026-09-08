package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	publiciddomain "github.com/RimuruChan/Vertex/server/internal/publicid/domain"
	"github.com/gin-gonic/gin"
)

type PublicIDResolver interface {
	Resolve(context.Context, string, string) (string, error)
}

// PublicIDs runs after route authentication and before the domain handler.
// Existing UUID paths and request bodies are kept intact for clients/workers.
func PublicIDs(resolver PublicIDResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if resolver == nil {
			c.Next()
			return
		}
		resolve := func(kind, value string) (string, bool) {
			if !publiciddomain.IsNumber(value) {
				return value, true
			}
			id, err := resolver.Resolve(c.Request.Context(), kind, value)
			if err != nil {
				publicIDError(c, err)
				return "", false
			}
			return id, true
		}
		parts := strings.Split(strings.Trim(c.FullPath(), "/"), "/")
		if len(parts) >= 4 && parts[1] == "domains" {
			parts = append([]string{parts[0]}, parts[3:]...)
		}
		if len(parts) < 2 {
			c.Next()
			return
		}
		kind := parts[1]
		if kind == "admin" && len(parts) > 2 {
			kind = parts[2]
		}
		switch kind {
		case "problems", "contests", "submissions", "editorials", "problem-sets", "announcements":
			for i := range c.Params {
				if c.Params[i].Key != "id" {
					continue
				}
				id, ok := resolve(kind, c.Params[i].Value)
				if !ok {
					return
				}
				c.Params[i].Value = id
			}
		}
		// Contest problem labels are resolved by the contest store only after
		// the service has checked start time, registration and staff access.
		query := c.Request.URL.Query()
		for field, resource := range map[string]string{"problem": "problems", "contest": "contests"} {
			if value := query.Get(field); value != "" {
				id, ok := resolve(resource, value)
				if !ok {
					return
				}
				query.Set(field, id)
			}
		}
		c.Request.URL.RawQuery = query.Encode()
		c.Next()
	}
}

func publicIDError(c *gin.Context, err error) {
	if errors.Is(err, publiciddomain.ErrNotFound) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": "resource.not_found", "error": "资源不存在"})
		return
	}
	_ = c.Error(err)
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": "internal.error", "error": "服务器内部错误"})
}
