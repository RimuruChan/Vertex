package httpx

import (
	"bytes"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var _ = Describe("BindJSON", func() {
	BeforeEach(func() { gin.SetMode(gin.TestMode) })

	It("binds a request within the limit", func() {
		var body struct {
			Name string `json:"name" binding:"required"`
		}
		response := bindRequest(`{"name":"vertex"}`, 64, &body)

		Expect(response.Code).To(Equal(http.StatusNoContent))
		Expect(body.Name).To(Equal("vertex"))
	})

	It("maps malformed JSON to request.invalid", func() {
		response := bindRequest(`{"name":`, 64, &struct{}{})

		Expect(response.Code).To(Equal(http.StatusBadRequest))
		Expect(response.Body.String()).To(MatchJSON(
			`{"code":"request.invalid","error":"invalid test request"}`,
		))
	})

	It("maps MaxBytesReader failures to request.too_large", func() {
		response := bindRequestWithLength(`{"name":"`+strings.Repeat("x", 64)+`"}`, 32, -1, &struct {
			Name string `json:"name"`
		}{})

		Expect(response.Code).To(Equal(http.StatusRequestEntityTooLarge))
		Expect(response.Body.String()).To(MatchJSON(
			`{"code":"request.too_large","error":"request body is too large"}`,
		))
	})
})

func bindRequest(raw string, limit int64, destination any) *httptest.ResponseRecorder {
	return bindRequestWithLength(raw, limit, int64(len(raw)), destination)
}

func bindRequestWithLength(raw string, limit, contentLength int64, destination any) *httptest.ResponseRecorder {
	router := gin.New()
	router.POST("/", func(c *gin.Context) {
		if !BindJSON(c, destination, limit, "invalid test request") {
			return
		}
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(raw))
	request.ContentLength = contentLength
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestHTTPX(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTPX Suite")
}
