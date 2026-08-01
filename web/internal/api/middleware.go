package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vertex-oj/web/internal/auth"
)

// contextKey 用于在 gin.Context 中传递当前用户。
type contextKey string

const (
	ctxUserID   contextKey = "uid"
	ctxUsername contextKey = "username"
	ctxRole     contextKey = "role"
)

// RequireAuth 校验 Bearer Token,并把用户身份写入上下文。
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")

		claims, err := auth.ParseToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set(string(ctxUserID), claims.UserID)
		c.Set(string(ctxUsername), claims.Username)
		c.Set(string(ctxRole), claims.Role)
		c.Next()
	}
}

// RequireAdmin 要求 admin 角色,需在 RequireAuth 之后挂载。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if role, _ := c.Get(string(ctxRole)); role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin only"})
			return
		}
		c.Next()
	}
}

// currentUserID 从上下文取当前用户 ID。
func currentUserID(c *gin.Context) string {
	id, _ := c.Get(string(ctxUserID))
	s, _ := id.(string)
	return s
}

// currentRole 从上下文取当前角色。
func currentRole(c *gin.Context) string {
	r, _ := c.Get(string(ctxRole))
	s, _ := r.(string)
	return s
}
