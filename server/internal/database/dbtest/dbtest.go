// Package dbtest wires integration tests to the shared TEST_DATABASE_URL
// instance.
//
// `go test ./...` runs packages in parallel, and every integration suite here
// truncates the same tables. Without coordination one package deletes another's
// fixtures mid-run, which shows up as spurious foreign key violations and
// deadlocks. Each suite therefore holds a session-level advisory lock for its
// whole run, so the suites take turns instead of interleaving.
package dbtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

// suiteLockKey is an arbitrary constant shared by every integration suite.
// Any value works as long as they all agree on it.
const suiteLockKey = 8_140_2026

// releaseTimeout bounds the unlock so a suite teardown cannot hang.
const releaseTimeout = 5 * time.Second

// Shared opens the integration database, applies the migrations and takes the
// suite lock. It returns a nil database when TEST_DATABASE_URL is unset, which
// is the signal for a suite to skip its integration specs.
//
// The returned release function must run at the end of the suite.
func Shared(ctx context.Context) (*database.DB, func(), error) {
	noop := func() {}
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		return nil, noop, nil
	}

	migrations, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		return nil, noop, fmt.Errorf("resolve migrations: %w", err)
	}
	if err := database.RunMigrations(databaseURL, migrations); err != nil {
		return nil, noop, fmt.Errorf("apply migrations: %w", err)
	}

	db, err := database.NewDB(ctx, databaseURL)
	if err != nil {
		return nil, noop, err
	}

	// The lock lives on one reserved connection: a pooled statement could be
	// released back and lose it while the suite is still running.
	conn, err := db.Pool.Connx(ctx)
	if err != nil {
		db.Close()
		return nil, noop, fmt.Errorf("reserve suite connection: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, suiteLockKey); err != nil {
		conn.Close()
		db.Close()
		return nil, noop, fmt.Errorf("take suite lock: %w", err)
	}

	return db, func() {
		release, cancel := context.WithTimeout(context.Background(), releaseTimeout)
		defer cancel()
		_, _ = conn.ExecContext(release, `SELECT pg_advisory_unlock($1)`, suiteLockKey)
		conn.Close()
		db.Close()
	}, nil
}
