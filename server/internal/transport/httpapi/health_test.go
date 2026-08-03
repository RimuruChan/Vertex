package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HealthHandler", func() {
	BeforeEach(func() { gin.SetMode(gin.TestMode) })

	It("keeps liveness independent from database readiness", func() {
		handler := NewHealthHandler(func(_ context.Context) error { return errors.New("database unavailable") })
		router := gin.New()
		router.GET("/live", handler.Live)
		router.GET("/ready", handler.Ready)

		live := httptest.NewRecorder()
		router.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/live", nil))
		Expect(live.Code).To(Equal(http.StatusOK))

		ready := httptest.NewRecorder()
		router.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
		Expect(ready.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(ready.Body.String()).To(MatchJSON(`{"status":"unavailable"}`))
	})
})
