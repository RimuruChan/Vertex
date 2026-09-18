package httpapi

import (
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// NumberResolver is injected once; each resource router declares how to use it.
type NumberResolver = httpx.NumberResolver

func ResourceReferences(resolver NumberResolver) gin.HandlerFunc {
	return httpx.BindReferences(resolver)
}
