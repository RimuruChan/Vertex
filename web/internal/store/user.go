package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/vertex-oj/web/internal/model"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrUsernameTaken  = errors.New("username already taken")
	ErrEmailTaken     = errors.New("email already taken")
)

// UserStore 负责 users 表的读写。
type UserStore struct{ db *DB }

func NewUserStore(db *DB) *UserStore { return &UserStore{db: db} }

// Create 创建用户;username/email 冲突返回对应错误。
func (s *UserStore) Create(ctx context.Context, username, email, passwordHash string) (*model.User, error) {
	var u model.User
	err := s.db.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, password_hash, role)
		 VALUES ($1, $2, $3, 'user')
		 RETURNING id, username, email, password_hash, role, rating, created_at`,
		username, strings.ToLower(email), passwordHash,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Rating, &u.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "users_username_key") {
			return nil, ErrUsernameTaken
		}
		if strings.Contains(err.Error(), "users_email_key") {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	return &u, nil
}

// ByUsername 按用户名取用户(登录用,支持大小写不敏感匹配)。
func (s *UserStore) ByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, role, rating, created_at
		 FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Rating, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ByID 按 ID 取用户。
func (s *UserStore) ByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, role, rating, created_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Rating, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
