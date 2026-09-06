package middleware

import (
	"context"
	"errors"
	"net/http/httptest"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type scopeResolverFunc func(context.Context, string, string) (domain.Scope, error)

func (f scopeResolverFunc) Get(ctx context.Context, slug, userID string) (domain.Scope, error) {
	return f(ctx, slug, userID)
}

var _ = Describe("ResolveDomain", func() {
	It("hides inaccessible domains and does not turn database failures into empty results", func() {
		for _, test := range []struct {
			err    error
			status int
			code   string
		}{
			{domain.ErrNotFound, 404, "domain.not_found"},
			{domain.ErrForbidden, 404, "domain.not_found"},
			{domain.ErrUnauthenticated, 401, "auth.invalid_token"},
			{errors.New("database unavailable"), 500, "domain.resolve_failed"},
		} {
			router := gin.New()
			resolver := scopeResolverFunc(func(_ context.Context, slug, _ string) (domain.Scope, error) {
				Expect(slug).To(Equal("official"))
				return domain.Scope{}, test.err
			})
			router.GET("/problems", ResolveDomain(resolver), func(*gin.Context) { Fail("scope failure reached the handler") })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", "/problems?domain=other", nil))
			Expect(response.Code).To(Equal(test.status))
			Expect(response.Body.String()).To(ContainSubstring(test.code))
			Expect(response.Body.String()).NotTo(ContainSubstring("database unavailable"))
		}
	})
	It("allows archived reads but blocks resource writes even for a site administrator", func() {
		resolver := scopeResolverFunc(func(_ context.Context, slug, _ string) (domain.Scope, error) {
			Expect(slug).To(Equal("archived"))
			return domain.Scope{Domain: domain.Domain{ID: "domain-id", Archived: true}, SiteAdmin: true}, nil
		})
		router := gin.New()
		group := router.Group("/domains/:domain/problems", ResolveDomain(resolver))
		group.GET("", func(c *gin.Context) { c.String(200, domain.ID(c.Request.Context())) })
		group.POST("", func(*gin.Context) { Fail("archived write reached the handler") })
		read, write := httptest.NewRecorder(), httptest.NewRecorder()
		router.ServeHTTP(read, httptest.NewRequest("GET", "/domains/archived/problems", nil))
		router.ServeHTTP(write, httptest.NewRequest("POST", "/domains/archived/problems", nil))
		Expect(read.Code).To(Equal(200))
		Expect(read.Body.String()).To(Equal("domain-id"))
		Expect(write.Code).To(Equal(403))
	})
})
