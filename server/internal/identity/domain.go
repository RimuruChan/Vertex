package identity

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	// ErrAccountDisabled is returned instead of ErrInvalidCredentials so a
	// blocked user is told why they cannot sign in rather than being left to
	// retry a password that is in fact correct.
	ErrAccountDisabled = errors.New("account is disabled")
	ErrUserNotFound    = errors.New("user not found")
	ErrUsernameTaken   = errors.New("username already taken")
	ErrEmailTaken      = errors.New("email already taken")
)

type User struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	Role         string
	Rating       int
	CreatedAt    time.Time
	// DisabledAt is set when an administrator blocked the account.
	DisabledAt     *time.Time
	DisabledReason string
}

// Disabled reports whether the account may no longer authenticate.
func (u *User) Disabled() bool { return u != nil && u.DisabledAt != nil }

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
