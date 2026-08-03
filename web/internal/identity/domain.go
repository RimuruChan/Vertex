package identity

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized  = errors.New("unauthorized")
	ErrUserNotFound  = errors.New("user not found")
	ErrUsernameTaken = errors.New("username already taken")
	ErrEmailTaken    = errors.New("email already taken")
)

type User struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	Role         string
	Rating       int
	CreatedAt    time.Time
}

type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
}

// AccessClaims contains the protocol-independent identity resolved from an
// access credential. JWT-specific registered claims stay in the adapter.
type AccessClaims struct {
	UserID    string
	SessionID string
	TokenID   string
}
