package publicid_test

import (
	"context"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPublicID(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Public ID Suite") }

var _ = Describe("Public IDs", func() {
	It("recognizes decimal references without mistaking UUIDs or labels for numbers", func() {
		Expect(publicid.IsNumber("1000")).To(BeTrue())
		for _, ref := range []string{"", "A", "-1", "1.2", "00000000-0000-4000-8000-000000000001"} {
			Expect(publicid.IsNumber(ref)).To(BeFalse())
		}
	})
	It("keeps numeric references stable across updates and never reuses deleted numbers", func() {
		ctx := context.Background()
		db, release, err := dbtest.Shared(ctx)
		Expect(err).NotTo(HaveOccurred())
		if db == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		defer func() { release(); db.Close() }()
		var firstID, firstRef, nextRef string
		Expect(db.Pool.QueryRowContext(ctx, "INSERT INTO problems(title) VALUES ('public-id check') RETURNING id, public_id").Scan(&firstID, &firstRef)).To(Succeed())
		store := publicid.NewStore(db)
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
		Expect(db.Pool.QueryRowContext(ctx, "INSERT INTO problems(title) VALUES ('next public-id check') RETURNING public_id").Scan(&nextRef)).To(Succeed())
		Expect(nextRef).NotTo(Equal(firstRef))
		_, err = store.Resolve(ctx, "problems", firstRef)
		Expect(err).To(MatchError(publicid.ErrNotFound))
		_, err = store.Resolve(ctx, "users; DROP TABLE problems", nextRef)
		Expect(err).To(MatchError(publicid.ErrNotFound))
		_, err = db.Pool.ExecContext(ctx, "DELETE FROM problems WHERE public_id = $1", nextRef)
		Expect(err).NotTo(HaveOccurred())
	})
})
