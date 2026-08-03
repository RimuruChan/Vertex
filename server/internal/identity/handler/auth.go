package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/identity/dto"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const refreshCookieName = "vertex_refresh"

type AuthService interface {
	Register(ctx context.Context, username, email, password string) (*identity.Result, error)
	Login(ctx context.Context, username, password string) (*identity.Result, error)
	Refresh(ctx context.Context, refreshToken string) (*identity.Result, error)
	Logout(ctx context.Context, identity *identity.Identity) error
	LogoutAll(ctx context.Context, identity *identity.Identity) error
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
}

func NewAuthHandler(service AuthService, cookie AuthCookieConfig) *AuthHandler {
	return &AuthHandler{service: service, cookie: cookie}
}

// Register creates an account and starts a revocable session.
//
//	@Summary	Register
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		dto.RegisterRequest	true	"Account credentials"
//	@Success	201		{object}	dto.AuthResponse
//	@Failure	400,409	{object}	httpx.ErrorResponse
//	@Router		/api/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var request dto.RegisterRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "username, email and password are required")
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
//	@Param		request	body		dto.LoginRequest	true	"Login credentials"
//	@Success	200		{object}	dto.AuthResponse
//	@Failure	400,401	{object}	httpx.ErrorResponse
//	@Router		/api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var request dto.LoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "username and password are required")
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
	var validation *identity.ValidationError
	switch {
	case errors.As(err, &validation):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", validation.Message)
	case errors.Is(err, identity.ErrUsernameTaken):
		writeAPIError(c, http.StatusConflict, "auth.username_taken", err.Error())
	case errors.Is(err, identity.ErrEmailTaken):
		writeAPIError(c, http.StatusConflict, "auth.email_taken", err.Error())
	case errors.Is(err, identity.ErrInvalidCredentials):
		writeAPIError(c, http.StatusUnauthorized, "auth.invalid_credentials", identity.ErrInvalidCredentials.Error())
	case errors.Is(err, identity.ErrUnauthorized):
		writeAPIError(c, http.StatusUnauthorized, "auth.unauthorized", "authentication required")
	default:
		writeAPIError(c, http.StatusInternalServerError, "internal.error", "authentication service failed")
	}
}

func authResponse(result *identity.Result) dto.AuthResponse {
	return dto.FromResult(result)
}

func userResponse(user *identity.User) dto.UserResponse {
	return dto.FromUser(user)
}

func writeAPIError(c *gin.Context, status int, code, message string) {
	httpx.WriteError(c, status, code, message)
}
