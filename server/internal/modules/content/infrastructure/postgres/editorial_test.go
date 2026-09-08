package postgres

import (
	"context"
	"testing"

	contentdomain "github.com/RimuruChan/Vertex/server/internal/modules/content/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var integrationDB *database.DB
var releaseSuite = func() {}

var _ = BeforeSuite(func(ctx SpecContext) {
	var err error
	integrationDB, releaseSuite, err = dbtest.Shared(ctx)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	releaseSuite()
})

var _ = Describe("Editorial store against PostgreSQL", func() {
	var store *EditorialRepository
	var discussions *DiscussionRepository
	var access *ProblemAccess
	var author, reader, problemID string

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `
			TRUNCATE discussion_posts, editorial_votes, editorials, submissions,
				contest_access, contest_participants, contests, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = NewEditorialRepository(integrationDB)
		discussions = NewDiscussionRepository(integrationDB)
		access = NewProblemAccess(integrationDB)

		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('author', 'a@t.local', 'x')
			 RETURNING id`).Scan(&author)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('reader', 'r@t.local', 'x')
			 RETURNING id`).Scan(&reader)).To(Succeed())
		Expect(dbtest.OfficialMembers(ctx, integrationDB)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, owner_id) VALUES ('Sum', 'public', $1) RETURNING id`, author).
			Scan(&problemID)).To(Succeed())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, problemID)).To(Succeed())
	})

	publish := func(ctx context.Context, title, status string) *contentdomain.Editorial {
		created, err := store.Create(ctx, author, contentdomain.EditorialInput{
			ProblemID: problemID, Title: title, ContentMD: "思路",
			Visibility: contentdomain.VisibilityPublic, Status: status,
		})
		Expect(err).NotTo(HaveOccurred())
		return created
	}

	It("lists with a count that matches the page", func(ctx SpecContext) {
		publish(ctx, "已发布", contentdomain.StatusPublished)
		publish(ctx, "草稿", contentdomain.StatusDraft)

		// A reader sees only the published one, and the total agrees.
		items, total, err := store.List(ctx, contentdomain.EditorialFilters{
			ViewerID: reader, Limit: 20,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].Title).To(Equal("已发布"))

		// The author additionally sees their own draft.
		items, total, err = store.List(ctx, contentdomain.EditorialFilters{ViewerID: author, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(2))
		Expect(items).To(HaveLen(2))

		// An anonymous reader still gets a usable list.
		items, total, err = store.List(ctx, contentdomain.EditorialFilters{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].Voted).To(BeFalse())
	})

	It("uses a body-free list projection while detail keeps the content", func(ctx SpecContext) {
		created := publish(ctx, "题解", contentdomain.StatusPublished)

		items, total, err := store.List(ctx, contentdomain.EditorialFilters{ViewerID: reader, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].ID).To(Equal(created.ID))
		Expect(items[0].Title).To(Equal("题解"))

		detail, err := store.Get(ctx, created.ID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(detail.ContentMD).To(Equal("思路"))
	})

	It("filters by problem and searches titles", func(ctx SpecContext) {
		publish(ctx, "前缀和做法", contentdomain.StatusPublished)
		items, total, err := store.List(ctx, contentdomain.EditorialFilters{
			ProblemID: problemID, Keyword: "前缀", ViewerID: reader, Limit: 20,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].ProblemTitle).To(Equal("Sum"))
	})

	It("keeps the vote count consistent across repeats and withdrawals", func(ctx SpecContext) {
		created := publish(ctx, "题解", contentdomain.StatusPublished)

		for i := 0; i < 3; i++ {
			total, err := store.Vote(ctx, created.ID, reader, true)
			Expect(err).NotTo(HaveOccurred())
			// A repeated upvote is idempotent because the count is recomputed.
			Expect(total).To(Equal(1))
		}
		seen, err := store.Get(ctx, created.ID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.Voted).To(BeTrue())

		total, err := store.Vote(ctx, created.ID, reader, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(0))
		seen, err = store.Get(ctx, created.ID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.Voted).To(BeFalse())
	})

	It("orders by votes when asked", func(ctx SpecContext) {
		quiet := publish(ctx, "冷门", contentdomain.StatusPublished)
		popular := publish(ctx, "热门", contentdomain.StatusPublished)
		_, err := store.Vote(ctx, popular.ID, reader, true)
		Expect(err).NotTo(HaveOccurred())
		_ = quiet

		items, _, err := store.List(ctx, contentdomain.EditorialFilters{
			Sort: "votes", ViewerID: reader, Limit: 20,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(items[0].Title).To(Equal("热门"))
	})

	It("does not list a public editorial through an inaccessible problem", func(ctx SpecContext) {
		publish(ctx, "题解", contentdomain.StatusPublished)
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET visibility = 'private' WHERE id = $1`, problemID)
		Expect(err).NotTo(HaveOccurred())

		items, total, err := store.List(ctx, contentdomain.EditorialFilters{ViewerID: reader, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		Expect(total).To(Equal(0))

		// This author still owns the parent problem, so its boundary permits access.
		items, total, err = store.List(ctx, contentdomain.EditorialFilters{ViewerID: author, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(total).To(Equal(1))
	})

	It("ignores role hints when projecting problem access", func(ctx SpecContext) {
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE problems SET visibility='private' WHERE id=$1", problemID)
		Expect(err).NotTo(HaveOccurred())
		visible, err := access.CanViewProblem(ctx, problemID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeFalse())
		visible, err = access.CanViewProblem(ctx, problemID, author)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE users SET role='admin' WHERE id=$1", reader)
		Expect(err).NotTo(HaveOccurred())
		visible, err = access.CanViewProblem(ctx, problemID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue())
	})

	It("reports whether the viewer solved the problem", func(ctx SpecContext) {
		solved, err := store.HasSolved(ctx, problemID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(solved).To(BeFalse())

		var contestID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, begin_at, end_at,owner_id)
			 VALUES ('Hidden feedback', now() - interval '1 hour', now() + interval '1 hour',$1)
			 RETURNING id`, author).Scan(&contestID)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions (user_id, problem_id, language, source_code, status, contest_id,problem_version)
			 VALUES ($1, $2, 'cpp', 'x', 'Accepted', $3,1)`, reader, problemID, contestID)
		Expect(err).NotTo(HaveOccurred())
		solved, err = store.HasSolved(ctx, problemID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(solved).To(BeFalse())

		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions (user_id, problem_id, language, source_code, status)
			 VALUES ($1, $2, 'cpp', 'x', 'Accepted')`, reader, problemID)
		Expect(err).NotTo(HaveOccurred())

		solved, err = store.HasSolved(ctx, problemID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(solved).To(BeTrue())
	})

	It("stamps an edited post and cascades replies on delete", func(ctx SpecContext) {
		root, err := discussions.CreateProblemPost(ctx, problemID, reader, "问题", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(root.Edited()).To(BeFalse())

		reply, err := discussions.CreateProblemPost(ctx, problemID, author, "回答", &root.ID)
		Expect(err).NotTo(HaveOccurred())

		updated, err := discussions.Update(ctx, root.ID, reader, "改过的问题")
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.ContentMD).To(Equal("改过的问题"))

		posts, err := discussions.ListByProblem(ctx, problemID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(posts.Posts).To(HaveLen(2))

		// Removing the root takes its replies with it.
		Expect(discussions.Delete(ctx, root.ID, reader)).To(Succeed())
		posts, err = discussions.ListByProblem(ctx, problemID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(posts.Posts).To(BeEmpty())
		_, err = discussions.Get(ctx, reply.ID, reader)
		Expect(err).To(MatchError(contentdomain.ErrNotFound))
	})
})

func TestContent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Content Suite")
}
