package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vertex-oj/web/internal/auth"
	"github.com/vertex-oj/web/internal/model"
	"github.com/vertex-oj/web/internal/store"
)

var (
	usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
	emailRe    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// AuthHandler 处理注册/登录/当前用户。
type AuthHandler struct {
	users *store.UserStore
}

func NewAuthHandler(users *store.UserStore) *AuthHandler {
	return &AuthHandler{users: users}
}

type registerReq struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type authResp struct {
	Token string      `json:"token"`
	User  *userPublic `json:"user"`
}

type userPublic struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// Register 处理 POST /api/auth/register。
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username, email and password are required"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	if !usernameRe.MatchString(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username must be 3-32 chars of letters, digits or underscore"})
		return
	}
	if !emailRe.MatchString(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email"})
		return
	}
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 6 chars"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	u, err := h.users.Create(c.Request.Context(), req.Username, req.Email, hash)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrUsernameTaken):
			c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
		case errors.Is(err, store.ErrEmailTaken):
			c.JSON(http.StatusConflict, gin.H{"error": "email already taken"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		}
		return
	}

	token, err := auth.GenerateToken(u.ID, u.Username, u.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	c.JSON(http.StatusCreated, authResp{
		Token: token,
		User:  toPublic(u),
	})
}

// Login 处理 POST /api/auth/login。
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	u, err := h.users.ByUsername(c.Request.Context(), req.Username)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	token, err := auth.GenerateToken(u.ID, u.Username, u.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	c.JSON(http.StatusOK, authResp{
		Token: token,
		User:  toPublic(u),
	})
}

// Me 处理 GET /api/auth/me(需登录)。
func (h *AuthHandler) Me(c *gin.Context) {
	u, err := h.users.ByID(c.Request.Context(), currentUserID(c))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, toPublic(u))
}

func toPublic(u *model.User) *userPublic {
	return &userPublic{ID: u.ID, Username: u.Username, Email: u.Email, Role: u.Role}
}
