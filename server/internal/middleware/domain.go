package middleware

import (
	"context"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/gin-gonic/gin"
)

type DomainResolver interface {
	Get(context.Context, string, string) (domain.Scope, error)
}

func ResolveDomain(resolver DomainResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("domain")
		if name == "" {
			name = domain.OfficialSlug
		}
		scope, err := resolver.Get(c.Request.Context(), name, CurrentUserID(c))
		if err != nil || !scope.CanEnter() {
			status, code, message := 404, "domain.not_found", "域不存在或不可访问"
			if errors.Is(err, domain.ErrUnauthenticated) {
				status, code, message = 401, "auth.invalid_token", "账号不可用，请重新登录"
			} else if err != nil && !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, domain.ErrForbidden) {
				status, code, message = 500, "domain.resolve_failed", "无法解析域"
				_ = c.Error(err)
			}
			httpx.WriteError(c, status, code, message)
			c.Abort()
			return
		}
		if scope.Domain.Archived && c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" {
			httpx.WriteError(c, 403, "domain.archived", "域已归档，只能读取")
			c.Abort()
			return
		}
		c.Request = c.Request.WithContext(domain.WithScope(c.Request.Context(), scope))
		c.Next()
	}
}
