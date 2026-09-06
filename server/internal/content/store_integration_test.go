package content

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
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
	var store *EditorialStore
	var discussions *DiscussionStore
	var access *AccessStore
	var author, reader, problemID string

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `
			TRUNCATE discussion_posts, editorial_votes, editorials, submissions,
				contest_staff, contest_participants, contests, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = NewEditorialStore(integrationDB)
		discussions = NewDiscussionStore(integrationDB)
		access = NewAccessStore(integrationDB)

		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('author', 'a@t.local', 'x')
			 RETURNING id`).Scan(&author)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('reader', 'r@t.local', 'x')
			 RETURNING id`).Scan(&reader)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility) VALUES ('Sum', 'public') RETURNING id`).
			Scan(&problemID)).To(Succeed())
	})

	publish := func(ctx context.Context, title, status string) *Editorial {
		created, err := store.Create(ctx, author, EditorialInput{
			ProblemID: problemID, Title: title, ContentMD: "思路",
			Visibility: VisibilityPublic, Status: status,
		})
		Expect(err).NotTo(HaveOccurred())
		return created
	}

	It("lists with a count that matches the page", func(ctx SpecContext) {
		publish(ctx, "已发布", StatusPublished)
		publish(ctx, "草稿", StatusDraft)

		// A reader sees only the published one, and the total agrees.
		items, total, err := store.List(ctx, EditorialFilters{
			ViewerID: reader, Limit: 20,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].Title).To(Equal("已发布"))

		// The author additionally sees their own draft.
		items, total, err = store.List(ctx, EditorialFilters{ViewerID: author, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(2))
		Expect(items).To(HaveLen(2))

		// An anonymous reader still gets a usable list.
		items, total, err = store.List(ctx, EditorialFilters{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].Voted).To(BeFalse())
	})

	It("uses a body-free list projection while detail keeps the content", func(ctx SpecContext) {
		created := publish(ctx, "题解", StatusPublished)

		items, total, err := store.List(ctx, EditorialFilters{ViewerID: reader, Limit: 20})
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
		publish(ctx, "前缀和做法", StatusPublished)
		items, total, err := store.List(ctx, EditorialFilters{
			ProblemID: problemID, Keyword: "前缀", ViewerID: reader, Limit: 20,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].ProblemTitle).To(Equal("Sum"))
	})

	It("keeps the vote count consistent across repeats and withdrawals", func(ctx SpecContext) {
		created := publish(ctx, "题解", StatusPublished)

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
		quiet := publish(ctx, "冷门", StatusPublished)
		popular := publish(ctx, "热门", StatusPublished)
		_, err := store.Vote(ctx, popular.ID, reader, true)
		Expect(err).NotTo(HaveOccurred())
		_ = quiet

		items, _, err := store.List(ctx, EditorialFilters{
			Sort: "votes", ViewerID: reader, Limit: 20,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(items[0].Title).To(Equal("热门"))
	})

	It("does not list a public editorial through an inaccessible problem", func(ctx SpecContext) {
		publish(ctx, "题解", StatusPublished)
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET visibility = 'private' WHERE id = $1`, problemID)
		Expect(err).NotTo(HaveOccurred())

		items, total, err := store.List(ctx, EditorialFilters{ViewerID: reader, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		Expect(total).To(Equal(0))

		// The editorial author retains access to their own work.
		items, total, err = store.List(ctx, EditorialFilters{ViewerID: author, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(total).To(Equal(1))
	})

	It("projects problem and contest visibility without exposing their models", func(ctx SpecContext) {
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET visibility = 'private', author_id = $2 WHERE id = $1`, problemID, author)
		Expect(err).NotTo(HaveOccurred())
		visible, err := access.CanViewProblem(ctx, problemID, reader, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeFalse())
		visible, err = access.CanViewProblem(ctx, problemID, author, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue())
		visible, err = access.CanViewProblem(ctx, problemID, reader, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue())

		var passwordContest, privateContest string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, begin_at, end_at, visibility, created_by)
			 VALUES ('Password', now(), now() + interval '1 hour', 'password', $1)
			 RETURNING id`, author).Scan(&passwordContest)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, begin_at, end_at, visibility, created_by)
			 VALUES ('Private', now(), now() + interval '1 hour', 'private', $1)
			 RETURNING id`, author).Scan(&privateContest)).To(Succeed())

		visible, err = access.CanViewContest(ctx, passwordContest, "", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeFalse())
		visible, err = access.CanViewContest(ctx, passwordContest, reader, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeFalse())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_participants (contest_id, user_id) VALUES ($1, $2)`, passwordContest, reader)
		Expect(err).NotTo(HaveOccurred())
		visible, err = access.CanViewContest(ctx, passwordContest, reader, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue())

		visible, err = access.CanViewContest(ctx, privateContest, reader, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeFalse())
		visible, err = access.CanViewContest(ctx, privateContest, author, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue())
		visible, err = access.CanViewContest(ctx, privateContest, reader, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_staff (contest_id, user_id, role) VALUES ($1, $2, 'observer')`, privateContest, reader)
		Expect(err).NotTo(HaveOccurred())
		visible, err = access.CanViewContest(ctx, privateContest, reader, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue())
	})

	It("reports whether the viewer solved the problem", func(ctx SpecContext) {
		solved, err := store.HasSolved(ctx, problemID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(solved).To(BeFalse())

		var contestID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, begin_at, end_at)
			 VALUES ('Hidden feedback', now() - interval '1 hour', now() + interval '1 hour')
			 RETURNING id`).Scan(&contestID)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions (user_id, problem_id, language, source_code, status, contest_id)
			 VALUES ($1, $2, 'cpp', 'x', 'Accepted', $3)`, reader, problemID, contestID)
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

		updated, err := discussions.Update(ctx, root.ID, "改过的问题")
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.ContentMD).To(Equal("改过的问题"))

		posts, err := discussions.ListByProblem(ctx, problemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(posts).To(HaveLen(2))

		// Removing the root takes its replies with it.
		Expect(discussions.Delete(ctx, root.ID)).To(Succeed())
		posts, err = discussions.ListByProblem(ctx, problemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(posts).To(BeEmpty())
		_, err = discussions.Get(ctx, reply.ID)
		Expect(err).To(MatchError(ErrNotFound))
	})
})
