package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	identitydomain "github.com/RimuruChan/Vertex/server/internal/identity/domain"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

const (
	issuer   = "vertex-server"
	audience = "vertex-ui"
)

// Claims is the access-token identity. sid binds the JWT to a revocable
// server-side session; role is informational and is refreshed from storage.
type Claims struct {
	SessionID string `json:"sid"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	TokenType string `json:"type"`
	jwt.RegisteredClaims
}

// Manager creates short-lived access tokens and opaque refresh credentials.
type Manager struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

func NewManager(secret string, accessTTL time.Duration) (*Manager, error) {
	if secret == "" {
		return nil, fmt.Errorf("JWT secret must not be empty")
	}
	if accessTTL <= 0 {
		return nil, fmt.Errorf("access token TTL must be positive")
	}
	return &Manager{secret: []byte(secret), accessTTL: accessTTL, now: time.Now}, nil
}

func (m *Manager) AccessTTL() time.Duration { return m.accessTTL }

func (m *Manager) IssueAccess(userID, sessionID, username, role string) (string, error) {
	now := m.now()
	jti, err := randomString(16)
	if err != nil {
		return "", err
	}
	claims := Claims{
		SessionID: sessionID,
		Username:  username,
		Role:      role,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			Subject:   userID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *Manager) ParseAccess(raw string) (*identitydomain.AccessClaims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		return m.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	if !token.Valid || claims.TokenType != "access" || claims.Subject == "" || claims.SessionID == "" || claims.ID == "" {
		return nil, ErrInvalidToken
	}
	return &identitydomain.AccessClaims{
		UserID: claims.Subject, SessionID: claims.SessionID, TokenID: claims.ID,
	}, nil
}

func (m *Manager) NewRefreshToken() (string, []byte, error) {
	raw, err := randomString(32)
	if err != nil {
		return "", nil, err
	}
	return raw, HashRefreshToken(raw), nil
}

func (m *Manager) HashRefreshToken(raw string) []byte           { return HashRefreshToken(raw) }
func (m *Manager) HashPassword(password string) (string, error) { return HashPassword(password) }
func (m *Manager) CheckPassword(hash, password string) bool     { return CheckPassword(hash, password) }

func HashRefreshToken(raw string) []byte {
	hash := sha256.Sum256([]byte(raw))
	return hash[:]
}

func randomString(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read cryptographic random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
