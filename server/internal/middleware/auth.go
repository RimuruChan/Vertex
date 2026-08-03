package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/gin-gonic/gin"
)

type contextKey string

const ctxIdentity contextKey = "identity"

type AccessAuthenticator interface {
	Authenticate(ctx context.Context, accessToken string) (*identity.Identity, error)
}

// AuthMiddleware resolves JWTs through the revocable server-side session.
type AuthMiddleware struct{ authenticator AccessAuthenticator }

func NewAuthMiddleware(authenticator AccessAuthenticator) *AuthMiddleware {
	return &AuthMiddleware{authenticator: authenticator}
}

func (m *AuthMiddleware) Require() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			writeAPIError(c, http.StatusUnauthorized, "auth.missing_token", "missing token")
			c.Abort()
			return
		}
		identity, err := m.authenticator.Authenticate(c.Request.Context(), raw)
		if err != nil {
			writeAPIError(c, http.StatusUnauthorized, "auth.invalid_token", "invalid or expired token")
			c.Abort()
			return
		}
		c.Set(string(ctxIdentity), identity)
		c.Next()
	}
}

func (m *AuthMiddleware) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.Next()
			return
		}
		raw, ok := bearerToken(header)
		if !ok {
			writeAPIError(c, http.StatusUnauthorized, "auth.invalid_token", "invalid token")
			c.Abort()
			return
		}
		identity, err := m.authenticator.Authenticate(c.Request.Context(), raw)
		if err != nil {
			writeAPIError(c, http.StatusUnauthorized, "auth.invalid_token", "invalid or expired token")
			c.Abort()
			return
		}
		c.Set(string(ctxIdentity), identity)
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := CurrentIdentity(c)
		if identity == nil || identity.User == nil || identity.User.Role != "admin" {
			writeAPIError(c, http.StatusForbidden, "auth.admin_required", "admin only")
			c.Abort()
			return
		}
		c.Next()
	}
}

func CurrentIdentity(c *gin.Context) *identity.Identity {
	value, ok := c.Get(string(ctxIdentity))
	if !ok {
		return nil
	}
	identity, _ := value.(*identity.Identity)
	return identity
}

func CurrentUserID(c *gin.Context) string {
	identity := CurrentIdentity(c)
	if identity == nil || identity.User == nil {
		return ""
	}
	return identity.User.ID
}

func CurrentRole(c *gin.Context) string {
	identity := CurrentIdentity(c)
	if identity == nil || identity.User == nil {
		return ""
	}
	return identity.User.Role
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return raw, raw != ""
}

func writeAPIError(c *gin.Context, status int, code, message string) {
	httpx.WriteError(c, status, code, message)
}
