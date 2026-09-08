package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	identitydomain "github.com/RimuruChan/Vertex/server/internal/modules/identity/domain"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
)

func identityDatabase(t *testing.T) *database.DB {
	t.Helper()
	db, release, err := dbtest.Shared(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if db == nil {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	t.Cleanup(release)
	if err := dbtest.Reset(context.Background(), db, "TRUNCATE auth_sessions, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestUserRegistrationAndCurrentAccountState(t *testing.T) {
	db := identityDatabase(t)
	ctx := context.Background()
	users := identitypg.NewUserRepository(db)
	user, err := users.Create(ctx, "alice", "Alice@EXAMPLE.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "alice@example.test" {
		t.Fatalf("email was not normalized: %q", user.Email)
	}
	var joined bool
	err = db.Pool.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM domain_members m JOIN domains d ON d.id=m.domain_id WHERE m.user_id=$1 AND d.is_official AND m.status='active')`, user.ID).Scan(&joined)
	if err != nil || !joined {
		t.Fatalf("initial membership: joined=%v err=%v", joined, err)
	}
	if _, err := users.Create(ctx, "alice", "other@example.test", "hash"); !errors.Is(err, identitydomain.ErrUsernameTaken) {
		t.Fatalf("username conflict: %v", err)
	}
	if _, err := users.Create(ctx, "other", "ALICE@example.test", "hash"); !errors.Is(err, identitydomain.ErrEmailTaken) {
		t.Fatalf("email conflict: %v", err)
	}
	if _, err := db.Pool.ExecContext(ctx, "UPDATE users SET disabled_at=now(),disabled_reason='spam',role='admin' WHERE id=$1", user.ID); err != nil {
		t.Fatal(err)
	}
	current, err := users.ByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.Disabled() || current.DisabledReason != "spam" || current.Role != "admin" {
		t.Fatalf("stale account state: %+v", current)
	}
}

func TestRegistrationRollsBackWithoutInitialMembership(t *testing.T) {
	db := identityDatabase(t)
	ctx := context.Background()
	if _, err := db.Pool.ExecContext(ctx, "DELETE FROM domains WHERE is_official"); err != nil {
		t.Fatal(err)
	}
	users := identitypg.NewUserRepository(db)
	if _, err := users.Create(ctx, "orphan", "orphan@example.test", "hash"); err == nil {
		t.Fatal("registration succeeded without the official domain")
	}
	if _, err := users.ByUsername(ctx, "orphan"); !errors.Is(err, identitydomain.ErrUserNotFound) {
		t.Fatalf("account escaped rollback: %v", err)
	}
}
