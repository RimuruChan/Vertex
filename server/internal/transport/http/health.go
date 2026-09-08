package httpapi

import (
	"context"
	"net/http"

	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	readiness func(context.Context) error
}

func NewHealthHandler(readiness func(context.Context) error) *HealthHandler {
	return &HealthHandler{readiness: readiness}
}

// Live reports whether the HTTP process is serving requests.
//
//	@Summary	Check process liveness
//	@Tags		system
//	@Produce	json
//	@Success	200	{object}	httpx.HealthResponse
//	@Router		/api/health/live [get]
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, httpx.HealthResponse{Status: "ok"})
}

// Ready checks whether dependencies required to serve traffic are available.
//
//	@Summary	Check service readiness
//	@Tags		system
//	@Produce	json
//	@Success	200	{object}	httpx.HealthResponse
//	@Failure	503	{object}	httpx.HealthResponse
//	@Router		/api/health [get]
//	@Router		/api/health/ready [get]
func (h *HealthHandler) Ready(c *gin.Context) {
	if h.readiness != nil && h.readiness(c.Request.Context()) != nil {
		c.JSON(http.StatusServiceUnavailable, httpx.HealthResponse{Status: "unavailable"})
		return
	}
	c.JSON(http.StatusOK, httpx.HealthResponse{Status: "ok"})
}
