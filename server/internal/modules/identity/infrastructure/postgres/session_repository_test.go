package postgres_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	identitydomain "github.com/RimuruChan/Vertex/server/internal/modules/identity/domain"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
)

func TestRefreshRotationIsAtomicAndRevocable(t *testing.T) {
	db := identityDatabase(t)
	ctx := context.Background()
	user, err := identitypg.NewUserRepository(db).Create(ctx, "alice", "alice@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	sessions := identitypg.NewSessionRepository(db)
	oldHash := sha256.Sum256([]byte("old credential"))
	expires := time.Now().Add(time.Hour)
	initial, err := sessions.Create(ctx, user.ID, oldHash[:], expires)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.ExecContext(ctx, "UPDATE users SET role='admin' WHERE id=$1", user.ID); err != nil {
		t.Fatal(err)
	}
	type rotation struct {
		session *identitydomain.Session
		user    *identitydomain.User
		hash    []byte
		err     error
	}
	results := make(chan rotation, 2)
	start := make(chan struct{})
	for _, raw := range []string{"new credential one", "new credential two"} {
		go func(raw string) {
			<-start
			hash := sha256.Sum256([]byte(raw))
			session, current, err := sessions.Rotate(ctx, oldHash[:], hash[:], expires)
			results <- rotation{session, current, hash[:], err}
		}(raw)
	}
	close(start)
	winners := 0
	var winningHash []byte
	for range 2 {
		result := <-results
		if result.err == nil {
			winners++
			winningHash = result.hash
			if result.session.ID != initial.ID || result.user.ID != user.ID || result.user.Role != "admin" {
				t.Fatalf("rotation returned a different session or stale account: %+v", result)
			}
		} else if !errors.Is(result.err, identitydomain.ErrUnauthorized) {
			t.Fatal(result.err)
		}
	}
	if winners != 1 {
		t.Fatalf("expected one successful use of the old credential, got %d", winners)
	}
	if err := sessions.Revoke(ctx, initial.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.ActiveUser(ctx, initial.ID, user.ID); !errors.Is(err, identitydomain.ErrUnauthorized) {
		t.Fatalf("revoked access accepted: %v", err)
	}
	if _, _, err := sessions.Rotate(ctx, winningHash, oldHash[:], expires); !errors.Is(err, identitydomain.ErrUnauthorized) {
		t.Fatalf("revoked refresh accepted: %v", err)
	}
}

func TestSessionOwnershipAndExpiry(t *testing.T) {
	db := identityDatabase(t)
	ctx := context.Background()
	users := identitypg.NewUserRepository(db)
	alice, err := users.Create(ctx, "alice", "alice@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := users.Create(ctx, "bob", "bob@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	sessions := identitypg.NewSessionRepository(db)
	activeHash, expiredHash := sha256.Sum256([]byte("active")), sha256.Sum256([]byte("expired"))
	active, err := sessions.Create(ctx, alice.ID, activeHash[:], time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	expired, err := sessions.Create(ctx, alice.ID, expiredHash[:], time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Revoke(ctx, active.ID, bob.ID); !errors.Is(err, identitydomain.ErrUnauthorized) {
		t.Fatalf("other account revoked session: %v", err)
	}
	if _, err := sessions.ActiveUser(ctx, active.ID, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.ActiveUser(ctx, expired.ID, alice.ID); !errors.Is(err, identitydomain.ErrUnauthorized) {
		t.Fatalf("expired access accepted: %v", err)
	}
	if _, _, err := sessions.Rotate(ctx, expiredHash[:], activeHash[:], time.Now().Add(time.Hour)); !errors.Is(err, identitydomain.ErrUnauthorized) {
		t.Fatalf("expired refresh accepted: %v", err)
	}
	if err := sessions.RevokeAll(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.ActiveUser(ctx, active.ID, alice.ID); !errors.Is(err, identitydomain.ErrUnauthorized) {
		t.Fatalf("logout-all left access active: %v", err)
	}
}
