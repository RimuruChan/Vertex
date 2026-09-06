package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/jackc/pgx/v5/pgconn"
)

// UserStore 负责 users 表的读写。
type UserStore struct{ db *database.DB }

func NewUserStore(db *database.DB) *UserStore { return &UserStore{db: db} }

// Create 创建用户;username/email 冲突返回对应错误。
func (s *UserStore) Create(ctx context.Context, username, email, passwordHash string) (*User, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var u User
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (username, email, password_hash, role)
		 VALUES ($1, $2, $3, 'user')
		 RETURNING id, username, email, password_hash, role, rating, created_at,
		           disabled_at, disabled_reason`,
		username, strings.ToLower(email), passwordHash,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Rating, &u.CreatedAt,
		&u.DisabledAt, &u.DisabledReason)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) {
			switch postgresError.ConstraintName {
			case "users_username_key":
				return nil, ErrUsernameTaken
			case "users_email_key":
				return nil, ErrEmailTaken
			}
		}
		return nil, err
	}
	joined, err := tx.ExecContext(ctx, `INSERT INTO domain_members(domain_id,user_id,role_key,status)
		SELECT id,$1,'member','active' FROM domains WHERE is_official`, u.ID)
	if err != nil {
		return nil, err
	}
	count, err := joined.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, fmt.Errorf("official domain is not initialized")
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &u, nil
}

// ByUsername returns the exact registered username used for login.
func (s *UserStore) ByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, role, rating, created_at,
		        disabled_at, disabled_reason
		 FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Rating, &u.CreatedAt,
		&u.DisabledAt, &u.DisabledReason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ByID 按 ID 取用户。
func (s *UserStore) ByID(ctx context.Context, id string) (*User, error) {
	var u User
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, role, rating, created_at,
		        disabled_at, disabled_reason
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Rating, &u.CreatedAt,
		&u.DisabledAt, &u.DisabledReason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
