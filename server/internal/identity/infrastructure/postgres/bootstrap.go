package postgres

import (
	"context"
	"errors"
	"fmt"

	identitydomain "github.com/RimuruChan/Vertex/server/internal/identity/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity/infrastructure/postgres/internal/dbgen"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

// BootstrapAdmin idempotently creates the explicitly configured initial
// administrator without reading process configuration inside the adapter.
func BootstrapAdmin(
	ctx context.Context,
	db *database.DB,
	username, password, email string,
	hashPassword func(string) (string, error),
) error {
	if username == "" {
		return nil
	}
	if hashPassword == nil {
		return fmt.Errorf("bootstrap password hasher is required")
	}

	users := NewUserRepository(db)
	existing, err := users.ByUsername(ctx, username)
	if err == nil && existing != nil {
		if existing.Role != "admin" {
			return fmt.Errorf("bootstrap username %q already belongs to a non-admin user", username)
		}
		return nil
	}
	if err != nil && !errors.Is(err, identitydomain.ErrUserNotFound) {
		return fmt.Errorf("lookup bootstrap administrator: %w", err)
	}

	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap administrator password: %w", err)
	}

	err = dbgen.New(db.Pool).BootstrapAdmin(ctx, dbgen.BootstrapAdminParams{Username: username, Email: email, PasswordHash: hash})
	if err != nil {
		return fmt.Errorf("insert bootstrap administrator: %w", err)
	}
	return nil
}
