package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/identity/dto"
	"github.com/RimuruChan/Vertex/server/internal/ratelimit"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AuthHandler", func() {
	BeforeEach(func() { gin.SetMode(gin.TestMode) })

	It("returns the compatibility token and a protected refresh cookie", func() {
		service := &fakeAuthService{loginResult: authResult("access-1", "refresh-1")}
		handler := NewAuthHandler(service, AuthCookieConfig{Secure: true, Lifetime: 30 * 24 * time.Hour}, AuthRateLimits{})
		router := gin.New()
		router.POST("/login", handler.Login)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"username":"alice","password":"secret"}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusOK))
		var body dto.AuthResponse
		Expect(json.Unmarshal(response.Body.Bytes(), &body)).To(Succeed())
		Expect(body.AccessToken).To(Equal("access-1"))
		Expect(body.Token).To(Equal("access-1"))
		cookies := response.Result().Cookies()
		Expect(cookies).To(HaveLen(1))
		Expect(cookies[0].Name).To(Equal("vertex_refresh"))
		Expect(cookies[0].Value).To(Equal("refresh-1"))
		Expect(cookies[0].HttpOnly).To(BeTrue())
		Expect(cookies[0].Secure).To(BeTrue())
		Expect(cookies[0].SameSite).To(Equal(http.SameSiteLaxMode))
		Expect(cookies[0].Path).To(Equal("/api/auth"))
	})

	It("rotates the refresh cookie", func() {
		service := &fakeAuthService{refreshResult: authResult("access-2", "refresh-2")}
		handler := NewAuthHandler(service, AuthCookieConfig{Lifetime: time.Hour}, AuthRateLimits{})
		router := gin.New()
		router.POST("/refresh", handler.Refresh)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/refresh", nil)
		request.AddCookie(&http.Cookie{Name: "vertex_refresh", Value: "refresh-1"})
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(service.refreshedToken).To(Equal("refresh-1"))
		Expect(response.Result().Cookies()[0].Value).To(Equal("refresh-2"))
	})

	It("does not erase a concurrently rotated cookie after a refresh rejection", func() {
		service := &fakeAuthService{refreshErr: identity.ErrUnauthorized}
		handler := NewAuthHandler(service, AuthCookieConfig{Lifetime: time.Hour}, AuthRateLimits{})
		router := gin.New()
		router.POST("/refresh", handler.Refresh)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/refresh", nil)
		request.AddCookie(&http.Cookie{Name: "vertex_refresh", Value: "stale-refresh"})
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusUnauthorized))
		Expect(response.Result().Cookies()).To(BeEmpty())
	})

	It("maps invalid credentials without leaking persistence errors", func() {
		service := &fakeAuthService{loginErr: identity.ErrInvalidCredentials}
		handler := NewAuthHandler(service, AuthCookieConfig{}, AuthRateLimits{})
		router := gin.New()
		router.POST("/login", handler.Login)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"username":"alice","password":"wrong"}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusUnauthorized))
		Expect(response.Body.String()).To(MatchJSON(`{"code":"auth.invalid_credentials","error":"invalid username or password"}`))
	})

	It("rejects oversized credential bodies before calling the service", func() {
		service := &fakeAuthService{}
		handler := NewAuthHandler(service, AuthCookieConfig{}, AuthRateLimits{})
		router := gin.New()
		router.POST("/login", handler.Login)
		response := httptest.NewRecorder()
		body := `{"username":"alice","password":"` + strings.Repeat("x", maxAuthBody) + `"}`
		request := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusRequestEntityTooLarge))
		Expect(response.Body.String()).To(MatchJSON(`{"code":"request.too_large","error":"request body is too large"}`))
		Expect(service.loginCalls).To(Equal(0))
	})

	It("limits login attempts by normalized account", func() {
		service := &fakeAuthService{loginErr: identity.ErrInvalidCredentials}
		limiter := ratelimit.New(16)
		handler := NewAuthHandler(service, AuthCookieConfig{}, AuthRateLimits{
			Login: ratelimit.Policy{Limiter: limiter, Limit: 1, Window: time.Hour},
		})
		router := gin.New()
		router.POST("/login", handler.Login)

		for index, username := range []string{"Alice", "  ALICE  "} {
			response := httptest.NewRecorder()
			body := `{"username":` + strconv.Quote(username) + `,"password":"wrong"}`
			request := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			if index == 0 {
				Expect(response.Code).To(Equal(http.StatusUnauthorized))
			} else {
				Expect(response.Code).To(Equal(http.StatusTooManyRequests))
				Expect(response.Body.String()).To(MatchJSON(`{"code":"request.rate_limited","error":"too many requests, try again later"}`))
			}
		}
		Expect(service.loginCalls).To(Equal(1))
	})

	It("isolates registration limits by the TCP client", func() {
		service := &fakeAuthService{registerErr: identity.ErrUsernameTaken}
		limiter := ratelimit.New(16)
		handler := NewAuthHandler(service, AuthCookieConfig{}, AuthRateLimits{
			Register: ratelimit.Policy{Limiter: limiter, Limit: 1, Window: time.Hour},
		})
		router := gin.New()
		router.POST("/register", handler.Register)

		requestFrom := func(remote string) *httptest.ResponseRecorder {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(
				`{"username":"alice","email":"alice@example.com","password":"secret"}`))
			request.RemoteAddr = remote
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			return response
		}

		Expect(requestFrom("192.0.2.1:1000").Code).To(Equal(http.StatusConflict))
		Expect(requestFrom("192.0.2.1:2000").Code).To(Equal(http.StatusTooManyRequests))
		Expect(requestFrom("192.0.2.2:1000").Code).To(Equal(http.StatusConflict))
		Expect(service.registerCalls).To(Equal(2))
	})

	It("bounds random login names by the TCP client", func() {
		service := &fakeAuthService{loginErr: identity.ErrInvalidCredentials}
		limiter := ratelimit.New(32)
		handler := NewAuthHandler(service, AuthCookieConfig{}, AuthRateLimits{
			Login:       ratelimit.Policy{Limiter: limiter, Limit: 10, Window: time.Hour},
			LoginClient: ratelimit.Policy{Limiter: limiter, Limit: 2, Window: time.Hour},
		})
		router := gin.New()
		router.POST("/login", handler.Login)

		requestAs := func(username, remote string) *httptest.ResponseRecorder {
			response := httptest.NewRecorder()
			body := `{"username":` + strconv.Quote(username) + `,"password":"wrong"}`
			request := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(body))
			request.RemoteAddr = remote
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			return response
		}

		Expect(requestAs("random-1", "192.0.2.1:1000").Code).To(Equal(http.StatusUnauthorized))
		Expect(requestAs("random-2", "192.0.2.1:2000").Code).To(Equal(http.StatusUnauthorized))
		Expect(requestAs("random-3", "192.0.2.1:3000").Code).To(Equal(http.StatusTooManyRequests))
		Expect(requestAs("random-3", "192.0.2.2:1000").Code).To(Equal(http.StatusUnauthorized))
		Expect(service.loginCalls).To(Equal(3))
	})
})

type fakeAuthService struct {
	registerErr    error
	registerCalls  int
	loginResult    *identity.Result
	loginErr       error
	refreshResult  *identity.Result
	refreshErr     error
	refreshedToken string
	loginCalls     int
}

func (f *fakeAuthService) Register(context.Context, string, string, string) (*identity.Result, error) {
	f.registerCalls++
	return nil, f.registerErr
}

func (f *fakeAuthService) Login(context.Context, string, string) (*identity.Result, error) {
	f.loginCalls++
	return f.loginResult, f.loginErr
}

func (f *fakeAuthService) Refresh(_ context.Context, token string) (*identity.Result, error) {
	f.refreshedToken = token
	return f.refreshResult, f.refreshErr
}

func (f *fakeAuthService) Logout(context.Context, *identity.Identity) error { return nil }

func (f *fakeAuthService) LogoutAll(context.Context, *identity.Identity) error { return nil }

func authResult(access, refresh string) *identity.Result {
	return &identity.Result{
		AccessToken: access, RefreshToken: refresh, ExpiresIn: 900,
		User: &identity.User{ID: "user-1", Username: "alice", Email: "alice@example.com", Role: "user"},
	}
}
