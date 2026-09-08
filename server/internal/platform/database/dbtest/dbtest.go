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
	"strings"
	"time"

	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

// suiteLockKey is an arbitrary constant shared by every integration suite.
// Any value works as long as they all agree on it.
const suiteLockKey = 8_140_2026

// releaseTimeout bounds the unlock so a suite teardown cannot hang.
const releaseTimeout = 5 * time.Second

// Reset preserves each suite's original truncate scope and restores the
// bootstrap domain removed by cascading user truncation. Shared must hold the
// suite advisory lock before this helper is used.
func Reset(ctx context.Context, db *database.DB, statement string) error {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(statement)), "TRUNCATE ") {
		return fmt.Errorf("integration reset requires a TRUNCATE statement")
	}
	if _, err := db.Pool.ExecContext(ctx, statement); err != nil {
		return err
	}
	return tenancypg.NewRepository(db).EnsureOfficial(ctx)
}

// OfficialMembers fills membership for raw SQL account fixtures. Production
// accounts receive it through identity.UserStore.Create instead.
func OfficialMembers(ctx context.Context, db *database.DB) error {
	_, err := db.Pool.ExecContext(ctx, `INSERT INTO domain_members(domain_id,user_id,role_key,status)
	 SELECT $1,id,'member','active' FROM users ON CONFLICT(domain_id,user_id) DO NOTHING`, tenancydomain.OfficialID)
	return err
}

// Shared opens the integration database, applies the migrations and takes the
// suite lock. It returns a nil database when TEST_DATABASE_URL is unset, which
// is the signal for a suite to skip its integration specs.
//
// The returned release function must run at the end of the suite. Packages
// outside internal may supply their explicit relative migration directory.
func Shared(ctx context.Context, migrationDirectory ...string) (*database.DB, func(), error) {
	noop := func() {}
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		return nil, noop, nil
	}

	var directory string
	if len(migrationDirectory) > 0 {
		directory = migrationDirectory[0]
	} else {
		var err error
		directory, err = findMigrations()
		if err != nil {
			return nil, noop, err
		}
	}
	migrations, err := filepath.Abs(directory)
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

// Package depth changes as contexts split into domain, application and
// infrastructure. Resolve migrations from the module instead of counting ../.
func findMigrations() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(directory, "migrations")
		if _, err := os.Stat(filepath.Join(candidate, "000001_init.up.sql")); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("cannot locate server migrations from test directory")
		}
		directory = parent
	}
}
