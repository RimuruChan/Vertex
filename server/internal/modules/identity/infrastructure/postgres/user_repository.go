package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	identitydomain "github.com/RimuruChan/Vertex/server/internal/modules/identity/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres/internal/dbgen"

	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/jackc/pgx/v5/pgconn"
)

// UserRepository implements account persistence and atomic registration.
type UserRepository struct {
	db      *sql.DB
	queries *dbgen.Queries
}

var _ identitydomain.UserRepository = (*UserRepository)(nil)

func NewUserRepository(db *database.DB) *UserRepository {
	return &UserRepository{db: db.Pool.DB, queries: dbgen.New(db.Pool.DB)}
}

// Create 创建用户;username/email 冲突返回对应错误。
func (s *UserRepository) Create(ctx context.Context, username, email, passwordHash string) (*identitydomain.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	row, err := queries.CreateUser(ctx, dbgen.CreateUserParams{Username: username, Email: strings.ToLower(email), PasswordHash: passwordHash})
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) {
			switch postgresError.ConstraintName {
			case "users_username_key":
				return nil, identitydomain.ErrUsernameTaken
			case "users_email_key":
				return nil, identitydomain.ErrEmailTaken
			}
		}
		return nil, err
	}
	count, err := queries.JoinOfficialDomain(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, fmt.Errorf("official domain is not initialized")
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return userFromRow(row), nil
}

func (s *UserRepository) ByUsername(ctx context.Context, username string) (*identitydomain.User, error) {
	row, err := s.queries.UserByUsername(ctx, username)
	return userResult(row, err)
}
func (s *UserRepository) ByID(ctx context.Context, id string) (*identitydomain.User, error) {
	row, err := s.queries.UserByID(ctx, id)
	return userResult(row, err)
}
func userResult(row dbgen.User, err error) (*identitydomain.User, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return nil, identitydomain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return userFromRow(row), nil
}
func userFromRow(row dbgen.User) *identitydomain.User {
	return &identitydomain.User{ID: row.ID, Username: row.Username, Email: row.Email, PasswordHash: row.PasswordHash,
		Role: row.Role, Rating: row.Rating, CreatedAt: row.CreatedAt, DisabledAt: row.DisabledAt, DisabledReason: row.DisabledReason}
}
