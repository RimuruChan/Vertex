package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	identityapp "github.com/RimuruChan/Vertex/server/internal/modules/identity/application"
	identitydomain "github.com/RimuruChan/Vertex/server/internal/modules/identity/domain"
	dto "github.com/RimuruChan/Vertex/server/internal/modules/identity/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/platform/ratelimit"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

const (
	refreshCookieName = "vertex_refresh"
	maxAuthBody       = 16 << 10
)

type AuthService interface {
	Register(ctx context.Context, username, email, password string) (*identityapp.Result, error)
	Login(ctx context.Context, username, password string) (*identityapp.Result, error)
	Refresh(ctx context.Context, refreshToken string) (*identityapp.Result, error)
	Logout(ctx context.Context, identity *identityapp.Identity) error
	LogoutAll(ctx context.Context, identity *identityapp.Identity) error
}

type AuthCookieConfig struct {
	Secure   bool
	Domain   string
	Lifetime time.Duration
}

// AuthHandler maps the HTTP authentication contract to the Identity service.
type AuthHandler struct {
	service AuthService
	cookie  AuthCookieConfig
	limits  AuthRateLimits
}

type AuthRateLimits struct {
	Login       ratelimit.Policy
	LoginClient ratelimit.Policy
	Register    ratelimit.Policy
}

func NewAuthHandler(service AuthService, cookie AuthCookieConfig, limits AuthRateLimits) *AuthHandler {
	return &AuthHandler{service: service, cookie: cookie, limits: limits}
}

// Register creates an account and starts a revocable session.
//
//	@Summary	Register
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		request			body		dto.RegisterRequest	true	"Account credentials"
//	@Success	201				{object}	dto.AuthResponse
//	@Failure	400,409,413,429	{object}	httpx.ErrorResponse
//	@Router		/api/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var request dto.RegisterRequest
	if !httpx.BindJSON(c, &request, maxAuthBody, "username, email and password are required") {
		return
	}
	client := ratelimit.ClientIdentity(c.Request)
	if !h.limits.Register.Allow(ratelimit.Key("auth-register", client)) {
		httpx.WriteRateLimited(c)
		return
	}
	result, err := h.service.Register(c.Request.Context(), request.Username, request.Email, request.Password)
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	h.setRefreshCookie(c, result.RefreshToken)
	c.JSON(http.StatusCreated, authResponse(result))
}

// Login starts a revocable session.
//
//	@Summary	Login
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		request			body		dto.LoginRequest	true	"Login credentials"
//	@Success	200				{object}	dto.AuthResponse
//	@Failure	400,401,413,429	{object}	httpx.ErrorResponse
//	@Router		/api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var request dto.LoginRequest
	if !httpx.BindJSON(c, &request, maxAuthBody, "username and password are required") {
		return
	}
	client := ratelimit.ClientIdentity(c.Request)
	if !h.limits.LoginClient.Allow(ratelimit.Key("auth-login-client", client)) {
		httpx.WriteRateLimited(c)
		return
	}
	account := strings.ToLower(strings.TrimSpace(request.Username))
	if !h.limits.Login.Allow(ratelimit.Key("auth-login", account)) {
		httpx.WriteRateLimited(c)
		return
	}
	result, err := h.service.Login(c.Request.Context(), request.Username, request.Password)
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	h.setRefreshCookie(c, result.RefreshToken)
	c.JSON(http.StatusOK, authResponse(result))
}

// Refresh rotates the HttpOnly refresh token and returns a new access token.
//
//	@Summary	Refresh access token
//	@Tags		auth
//	@Produce	json
//	@Success	200	{object}	dto.AuthResponse
//	@Failure	401	{object}	httpx.ErrorResponse
//	@Router		/api/auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	raw, err := c.Cookie(refreshCookieName)
	if err != nil {
		writeAPIError(c, http.StatusUnauthorized, "auth.unauthorized", "refresh token is missing or invalid")
		return
	}
	result, err := h.service.Refresh(c.Request.Context(), raw)
	if err != nil {
		// Do not clear the cookie on a failed rotation: another tab may have
		// concurrently succeeded with the same old token and already set the
		// replacement cookie. Logout remains the explicit cookie-clearing path.
		h.writeAuthError(c, err)
		return
	}
	h.setRefreshCookie(c, result.RefreshToken)
	c.JSON(http.StatusOK, authResponse(result))
}

// Logout revokes the current session.
//
//	@Summary	Logout current session
//	@Tags		auth
//	@Security	BearerAuth
//	@Success	204
//	@Failure	401	{object}	httpx.ErrorResponse
//	@Router		/api/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	if err := h.service.Logout(c.Request.Context(), middleware.CurrentIdentity(c)); err != nil {
		h.writeAuthError(c, err)
		return
	}
	h.clearRefreshCookie(c)
	c.Status(http.StatusNoContent)
}

// LogoutAll revokes every session for the current user.
//
//	@Summary	Logout all sessions
//	@Tags		auth
//	@Security	BearerAuth
//	@Success	204
//	@Failure	401	{object}	httpx.ErrorResponse
//	@Router		/api/auth/logout-all [post]
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	if err := h.service.LogoutAll(c.Request.Context(), middleware.CurrentIdentity(c)); err != nil {
		h.writeAuthError(c, err)
		return
	}
	h.clearRefreshCookie(c)
	c.Status(http.StatusNoContent)
}

// Me returns the current database-backed identity.
//
//	@Summary	Current user
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	dto.UserResponse
//	@Failure	401	{object}	httpx.ErrorResponse
//	@Router		/api/auth/me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	identity := middleware.CurrentIdentity(c)
	if identity == nil || identity.User == nil {
		writeAPIError(c, http.StatusUnauthorized, "auth.unauthorized", "authentication required")
		return
	}
	c.JSON(http.StatusOK, userResponse(identity.User))
}

func (h *AuthHandler) setRefreshCookie(c *gin.Context, value string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, value, int(h.cookie.Lifetime/time.Second), "/api/auth", h.cookie.Domain, h.cookie.Secure, true)
}

func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, "", -1, "/api/auth", h.cookie.Domain, h.cookie.Secure, true)
}

func (h *AuthHandler) writeAuthError(c *gin.Context, err error) {
	var validation *identitydomain.ValidationError
	switch {
	case errors.As(err, &validation):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", validation.Message)
	case errors.Is(err, identitydomain.ErrUsernameTaken):
		writeAPIError(c, http.StatusConflict, "auth.username_taken", err.Error())
	case errors.Is(err, identitydomain.ErrEmailTaken):
		writeAPIError(c, http.StatusConflict, "auth.email_taken", err.Error())
	case errors.Is(err, identitydomain.ErrAccountDisabled):
		writeAPIError(c, http.StatusForbidden, "auth.account_disabled", identitydomain.ErrAccountDisabled.Error())
	case errors.Is(err, identitydomain.ErrInvalidCredentials):
		writeAPIError(c, http.StatusUnauthorized, "auth.invalid_credentials", identitydomain.ErrInvalidCredentials.Error())
	case errors.Is(err, identitydomain.ErrUnauthorized):
		writeAPIError(c, http.StatusUnauthorized, "auth.unauthorized", "authentication required")
	default:
		writeAPIError(c, http.StatusInternalServerError, "internal.error", "authentication service failed")
	}
}

func authResponse(result *identityapp.Result) dto.AuthResponse {
	return dto.FromResult(result)
}

func userResponse(user *identitydomain.User) dto.UserResponse {
	return dto.FromUser(user)
}

func writeAPIError(c *gin.Context, status int, code, message string) {
	httpx.WriteError(c, status, code, message)
}
