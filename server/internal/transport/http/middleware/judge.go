package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func RequireJudgeService(expectedToken string) gin.HandlerFunc {
	expectedHash := sha256.Sum256([]byte(expectedToken))
	return func(c *gin.Context) {
		raw, ok := bearerToken(c.GetHeader("Authorization"))
		actualHash := sha256.Sum256([]byte(raw))
		if !ok || expectedToken == "" || subtle.ConstantTimeCompare(actualHash[:], expectedHash[:]) != 1 {
			writeAPIError(c, http.StatusUnauthorized, "judge.unauthorized", "judge service authentication failed")
			c.Abort()
			return
		}
		if strings.ContainsAny(raw, "\r\n") {
			writeAPIError(c, http.StatusUnauthorized, "judge.unauthorized", "judge service authentication failed")
			c.Abort()
			return
		}
		c.Next()
	}
}
