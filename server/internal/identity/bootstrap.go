package identity

import (
	"context"
	"errors"
	"fmt"

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

	users := NewUserStore(db)
	existing, err := users.ByUsername(ctx, username)
	if err == nil && existing != nil {
		if existing.Role != "admin" {
			return fmt.Errorf("bootstrap username %q already belongs to a non-admin user", username)
		}
		return nil
	}
	if err != nil && !errors.Is(err, ErrUserNotFound) {
		return fmt.Errorf("lookup bootstrap administrator: %w", err)
	}

	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap administrator password: %w", err)
	}

	_, err = db.Pool.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash, role)
		 VALUES ($1, $2, $3, 'admin')
		 ON CONFLICT (username) DO NOTHING`,
		username, email, hash,
	)
	if err != nil {
		return fmt.Errorf("insert bootstrap administrator: %w", err)
	}
	return nil
}
