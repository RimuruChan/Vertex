package httpapi

import (
	"context"
	identityapp "github.com/RimuruChan/Vertex/server/internal/identity/application"
	identitydomain "github.com/RimuruChan/Vertex/server/internal/identity/domain"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/ratelimit"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type registrationAuthenticator struct{}

func (registrationAuthenticator) Authenticate(context.Context, string) (*identityapp.Identity, error) {
	return &identityapp.Identity{User: &identitydomain.User{ID: "user-1", Role: "user"}}, nil
}

var _ = Describe("Contest registration abuse control", func() {
	It("returns the shared 429 response before checking a password", func() {
		gin.SetMode(gin.TestMode)
		limiter := ratelimit.New(8)
		policy := ratelimit.Policy{Limiter: limiter, Limit: 1, Window: time.Hour}
		key := ratelimit.Key("contest-register", "user-1\x00contest-1")
		Expect(policy.Allow(key)).To(BeTrue())

		handler := NewContestHandler(nil, policy)
		auth := middleware.NewAuthMiddleware(registrationAuthenticator{})
		router := gin.New()
		router.POST("/contests/:id/register", auth.Require(), handler.Register)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/contests/contest-1/register", strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer access")
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusTooManyRequests))
		Expect(response.Body.String()).To(MatchJSON(
			`{"code":"request.rate_limited","error":"too many requests, try again later"}`))
	})
})

func TestContestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Contest Handler Suite")
}
