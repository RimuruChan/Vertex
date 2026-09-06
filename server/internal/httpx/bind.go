package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BindJSON decodes a size-limited JSON request and writes the public error
// response when decoding fails.
func BindJSON(c *gin.Context, destination any, maxBytes int64, invalidMessage string) bool {
	if c.Request.ContentLength > maxBytes {
		writeRequestTooLarge(c)
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	if err := c.ShouldBindJSON(destination); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeRequestTooLarge(c)
			return false
		}
		WriteError(c, http.StatusBadRequest, "request.invalid", invalidMessage)
		return false
	}
	return true
}

func writeRequestTooLarge(c *gin.Context) {
	WriteError(c, http.StatusRequestEntityTooLarge, "request.too_large", "request body is too large")
}
