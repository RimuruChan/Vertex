package middleware

import (
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RequireJudgeService", func() {
	BeforeEach(func() { gin.SetMode(gin.TestMode) })

	It("allows only the configured bearer credential", func() {
		router := gin.New()
		router.Use(RequireJudgeService("service-secret"))
		router.POST("/claim", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		unauthorized := httptest.NewRecorder()
		router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/claim", nil))
		Expect(unauthorized.Code).To(Equal(http.StatusUnauthorized))

		authorized := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/claim", nil)
		request.Header.Set("Authorization", "Bearer service-secret")
		router.ServeHTTP(authorized, request)
		Expect(authorized.Code).To(Equal(http.StatusNoContent))
	})
})
