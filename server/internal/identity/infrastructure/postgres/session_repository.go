package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	identitydomain "github.com/RimuruChan/Vertex/server/internal/identity/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity/infrastructure/postgres/internal/dbgen"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

// SessionRepository persists revocable refresh sessions. Only refresh-token hashes
// are stored, so a database leak does not expose reusable credentials.
type SessionRepository struct{ queries *dbgen.Queries }

var _ identitydomain.SessionRepository = (*SessionRepository)(nil)

func NewSessionRepository(db *database.DB) *SessionRepository {
	return &SessionRepository{queries: dbgen.New(db.Pool.DB)}
}

func (s *SessionRepository) Create(ctx context.Context, userID string, refreshHash []byte, expiresAt time.Time) (*identitydomain.Session, error) {
	row, err := s.queries.CreateSession(ctx, dbgen.CreateSessionParams{UserID: userID, RefreshTokenHash: refreshHash, ExpiresAt: expiresAt})
	if err != nil {
		return nil, err
	}
	return &identitydomain.Session{ID: row.ID, UserID: row.UserID, ExpiresAt: row.ExpiresAt}, nil
}
func (s *SessionRepository) Rotate(ctx context.Context, oldHash, newHash []byte, expiresAt time.Time) (*identitydomain.Session, *identitydomain.User, error) {
	row, err := s.queries.RotateSession(ctx, dbgen.RotateSessionParams{OldHash: oldHash, NewHash: newHash, ExpiresAt: expiresAt})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, identitydomain.ErrUnauthorized
	}
	if err != nil {
		return nil, nil, err
	}
	return &identitydomain.Session{ID: row.SessionID, UserID: row.UserID, ExpiresAt: row.ExpiresAt},
		&identitydomain.User{ID: row.ID, Username: row.Username, Email: row.Email, PasswordHash: row.PasswordHash, Role: row.Role,
			Rating: row.Rating, CreatedAt: row.CreatedAt, DisabledAt: row.DisabledAt, DisabledReason: row.DisabledReason}, nil
}
func (s *SessionRepository) ActiveUser(ctx context.Context, sessionID, userID string) (*identitydomain.User, error) {
	row, err := s.queries.ActiveSessionUser(ctx, dbgen.ActiveSessionUserParams{ID: sessionID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, identitydomain.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	return userFromRow(row), nil
}
func (s *SessionRepository) Revoke(ctx context.Context, sessionID, userID string) error {
	affected, err := s.queries.RevokeSession(ctx, dbgen.RevokeSessionParams{ID: sessionID, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return identitydomain.ErrUnauthorized
	}
	return nil
}
func (s *SessionRepository) RevokeAll(ctx context.Context, userID string) error {
	return s.queries.RevokeUserSessions(ctx, userID)
}
