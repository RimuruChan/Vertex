package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Submission request body limit", func() {
	BeforeEach(func() { gin.SetMode(gin.TestMode) })

	It("rejects an oversized submission before reaching the service", func() {
		handler := &SubmissionHandler{}
		router := gin.New()
		router.POST("/submissions", handler.Submit)
		body := `{"problemId":"p1","language":"cpp","sourceCode":"` +
			strings.Repeat("x", maxSubmissionBody) + `"}`
		request := httptest.NewRequest(http.MethodPost, "/submissions", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusRequestEntityTooLarge))
		Expect(response.Body.String()).To(MatchJSON(
			`{"code":"request.too_large","error":"request body is too large"}`,
		))
	})
})
