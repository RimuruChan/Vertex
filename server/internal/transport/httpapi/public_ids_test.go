package httpapi

import (
	"context"
	"fmt"
	"net/http/httptest"

	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type referenceStub struct{ calls []string }

func (s *referenceStub) Resolve(_ context.Context, kind, ref string) (string, error) {
	s.calls = append(s.calls, kind+":"+ref)
	if ref == "404" {
		return "", publicid.ErrNotFound
	}
	return kind + "-uuid", nil
}

var _ = Describe("Public URL references", func() {
	It("resolves contest numbers and filters while leaving scoped labels to contest policy", func() {
		resolver := &referenceStub{}
		router := gin.New()
		router.GET("/api/contests/:id/problems/:problemId", PublicIDs(resolver), func(c *gin.Context) {
			c.String(200, "%s/%s/%s", c.Param("id"), c.Param("problemId"), c.Query("problem"))
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/api/contests/42/problems/A-1?problem=1000", nil))
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(Equal("contests-uuid/A-1/problems-uuid"))
		Expect(resolver.calls).To(Equal([]string{"contests:42", "problems:1000"}))
	})
	It("retains UUID compatibility without a lookup", func() {
		resolver := &referenceStub{}
		router := gin.New()
		router.GET("/api/problems/:id", PublicIDs(resolver), func(c *gin.Context) { c.String(200, "%s", c.Param("id")) })
		id := "00000000-0000-4000-8000-000000000001"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/api/problems/"+id, nil))
		Expect(response.Body.String()).To(Equal(id))
		Expect(resolver.calls).To(BeEmpty())
	})
	It("does not resolve protected resources before authentication succeeds", func() {
		resolver := &referenceStub{}
		router := gin.New()
		router.GET("/api/admin/problems/:id", func(c *gin.Context) { c.AbortWithStatus(401) }, PublicIDs(resolver), func(c *gin.Context) { c.Status(200) })
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/api/admin/problems/1000", nil))
		Expect(response.Code).To(Equal(401))
		Expect(resolver.calls).To(BeEmpty())
	})
	It("returns a generic not-found response for an absent public number", func() {
		router := gin.New()
		router.GET("/api/problems/:id", PublicIDs(&referenceStub{}), func(c *gin.Context) { Fail(fmt.Sprintf("handler reached for %s", c.Param("id"))) })
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/api/problems/404", nil))
		Expect(response.Code).To(Equal(404))
		Expect(response.Body.String()).To(ContainSubstring("resource.not_found"))
	})
})
