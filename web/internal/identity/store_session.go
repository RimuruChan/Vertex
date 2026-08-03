package identity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/RimuruChan/Vertex/web/internal/database"
)

// SessionStore persists revocable refresh sessions. Only refresh-token hashes
// are stored, so a database leak does not expose reusable credentials.
type SessionStore struct{ db *database.DB }

func NewSessionStore(db *database.DB) *SessionStore { return &SessionStore{db: db} }

func (s *SessionStore) Create(ctx context.Context, userID string, refreshHash []byte, expiresAt time.Time) (*Session, error) {
	var session Session
	err := s.db.Pool.QueryRowContext(ctx,
		`INSERT INTO auth_sessions (user_id, refresh_token_hash, expires_at)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, expires_at`,
		userID, refreshHash, expiresAt,
	).Scan(&session.ID, &session.UserID, &session.ExpiresAt)
	return &session, err
}

func (s *SessionStore) Rotate(ctx context.Context, oldHash, newHash []byte, expiresAt time.Time) (*Session, *User, error) {
	var session Session
	var user User
	err := s.db.Pool.QueryRowContext(ctx,
		`UPDATE auth_sessions AS session
		 SET refresh_token_hash = $2, expires_at = $3, last_used_at = now()
		 FROM users AS usr
		 WHERE session.refresh_token_hash = $1
		   AND session.user_id = usr.id
		   AND session.revoked_at IS NULL
		   AND session.expires_at > now()
		 RETURNING session.id, session.user_id, session.expires_at,
		           usr.id, usr.username, usr.email, usr.password_hash, usr.role, usr.rating, usr.created_at`,
		oldHash, newHash, expiresAt,
	).Scan(&session.ID, &session.UserID, &session.ExpiresAt,
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role, &user.Rating, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrUnauthorized
	}
	if err != nil {
		return nil, nil, err
	}
	return &session, &user, nil
}

func (s *SessionStore) ActiveUser(ctx context.Context, sessionID, userID string) (*User, error) {
	var user User
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT usr.id, usr.username, usr.email, usr.password_hash, usr.role, usr.rating, usr.created_at
		 FROM auth_sessions AS session
		 JOIN users AS usr ON usr.id = session.user_id
		 WHERE session.id = $1 AND session.user_id = $2
		   AND session.revoked_at IS NULL AND session.expires_at > now()`,
		sessionID, userID,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role, &user.Rating, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthorized
	}
	return &user, err
}

func (s *SessionStore) Revoke(ctx context.Context, sessionID, userID string) error {
	command, err := s.db.Pool.ExecContext(ctx,
		`UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now())
		 WHERE id = $1 AND user_id = $2`, sessionID, userID)
	if err != nil {
		return err
	}
	affected, err := command.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrUnauthorized
	}
	return nil
}

func (s *SessionStore) RevokeAll(ctx context.Context, userID string) error {
	_, err := s.db.Pool.ExecContext(ctx,
		`UPDATE auth_sessions SET revoked_at = now()
		 WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}
