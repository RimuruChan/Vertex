package domain

import (
	"context"
	"time"
)

// UserRepository persists accounts. Create atomically establishes the account
// and its initial official-domain membership; neither may be committed alone.
type UserRepository interface {
	Create(ctx context.Context, username, email, passwordHash string) (*User, error)
	ByUsername(ctx context.Context, username string) (*User, error)
	ByID(ctx context.Context, id string) (*User, error)
}

// SessionRepository owns revocation and one-time refresh rotation. Rotate
// consumes the old hash atomically and returns the current account state.
type SessionRepository interface {
	Create(ctx context.Context, userID string, refreshHash []byte, expiresAt time.Time) (*Session, error)
	Rotate(ctx context.Context, oldHash, newHash []byte, expiresAt time.Time) (*Session, *User, error)
	ActiveUser(ctx context.Context, sessionID, userID string) (*User, error)
	Revoke(ctx context.Context, sessionID, userID string) error
	RevokeAll(ctx context.Context, userID string) error
}
