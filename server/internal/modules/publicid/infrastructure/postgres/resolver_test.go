package postgres_test

import (
	"context"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	publiciddomain "github.com/RimuruChan/Vertex/server/internal/modules/publicid/domain"
	publicidpg "github.com/RimuruChan/Vertex/server/internal/modules/publicid/infrastructure/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPublicID(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Public ID Suite") }

var _ = Describe("Public IDs", func() {

	It("keeps numeric references stable across updates and never reuses deleted numbers", func() {
		ctx := context.Background()
		db, release, err := dbtest.Shared(ctx)
		Expect(err).NotTo(HaveOccurred())
		if db == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		defer func() { release(); db.Close() }()
		Expect(dbtest.Reset(ctx, db, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		var owner string
		Expect(db.Pool.QueryRowContext(ctx, "INSERT INTO users(username,email,password_hash) VALUES('setter','setter@example.test','fixture') RETURNING id").Scan(&owner)).To(Succeed())
		var firstID, firstRef, nextRef string
		Expect(db.Pool.QueryRowContext(ctx, "INSERT INTO problems(title,owner_id) VALUES ('public-id check',$1) RETURNING id, public_id", owner).Scan(&firstID, &firstRef)).To(Succeed())
		store := publicidpg.NewResolver(db)
		resolved, err := store.Resolve(ctx, "problems", firstRef)
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(firstID))
		_, err = db.Pool.ExecContext(ctx, "UPDATE problems SET title = 'renamed' WHERE id = $1", firstID)
		Expect(err).NotTo(HaveOccurred())
		resolved, err = store.Resolve(ctx, "problems", firstRef)
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(firstID))
		_, err = db.Pool.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", firstID)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Pool.QueryRowContext(ctx, "INSERT INTO problems(title,owner_id) VALUES ('next public-id check',$1) RETURNING public_id", owner).Scan(&nextRef)).To(Succeed())
		Expect(nextRef).NotTo(Equal(firstRef))
		_, err = store.Resolve(ctx, "problems", firstRef)
		Expect(err).To(MatchError(publiciddomain.ErrNotFound))
		_, err = store.Resolve(ctx, "users; DROP TABLE problems", nextRef)
		Expect(err).To(MatchError(publiciddomain.ErrNotFound))
		_, err = db.Pool.ExecContext(ctx, "DELETE FROM problems WHERE public_id = $1", nextRef)
		Expect(err).NotTo(HaveOccurred())
	})
})
