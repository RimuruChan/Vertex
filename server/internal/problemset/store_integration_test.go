package problemset_test

import (
	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	setapp "github.com/RimuruChan/Vertex/server/internal/problemset"
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

// The problem set store builds its WHERE clause dynamically and computes the
// viewer's progress in SQL; both only really hold up against a real database.
var _ = Describe("Problem set store against PostgreSQL", func() {
	var store *setapp.SetStore
	var curator, reader, solved, unsolved, draft string

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `
			TRUNCATE problem_set_problems, problem_sets, submissions,
				problem_tags, tags, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = setapp.NewSetStore(integrationDB)

		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('curator', 'c@t.local', 'x')
			 RETURNING id`).Scan(&curator)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('reader', 'r@t.local', 'x')
			 RETURNING id`).Scan(&reader)).To(Succeed())
		for _, item := range []struct {
			target     *string
			title      string
			visibility string
		}{
			{&solved, "Solved", "public"},
			{&unsolved, "Unsolved", "public"},
			{&draft, "Draft", "draft"},
		} {
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO problems (title, visibility) VALUES ($1, $2) RETURNING id`,
				item.title, item.visibility).Scan(item.target)).To(Succeed())
		}
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions (user_id, problem_id, language, source_code, status)
			 VALUES ($1, $2, 'cpp', 'x', 'Accepted')`, reader, solved)
		Expect(err).NotTo(HaveOccurred())
		var contestID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, begin_at, end_at)
			 VALUES ('Hidden feedback', now() - interval '1 hour', now() + interval '1 hour')
			 RETURNING id`).Scan(&contestID)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions (user_id, problem_id, language, source_code, status, contest_id)
			 VALUES ($1, $2, 'cpp', 'x', 'Accepted', $3)`, reader, unsolved, contestID)
		Expect(err).NotTo(HaveOccurred())
	})

	It("computes the viewer's progress and per-problem status", func(ctx SpecContext) {
		created, err := store.Create(ctx, curator, setapp.UpsertInput{
			Title: "入门", Visibility: setapp.VisibilityPublic,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetItems(ctx, created.ID, curator, false, []setapp.ItemInput{
			{ProblemID: solved, Note: "热身"},
			{ProblemID: unsolved},
		})).To(Succeed())

		// The reader solved exactly one of the two problems.
		seen, err := store.Get(ctx, created.ID, reader, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.ProblemCount).To(Equal(2))
		Expect(seen.SolvedCount).To(Equal(1))
		Expect(seen.Items[0].ProblemID).To(Equal(solved))
		Expect(seen.Items[0].Note).To(Equal("热身"))
		Expect(seen.Items[0].UserStatus).To(Equal("solved"))
		Expect(seen.Items[1].UserStatus).To(Equal("none"))

		// An anonymous viewer never triggers the submissions lookup.
		anonymous, err := store.Get(ctx, created.ID, "", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(anonymous.SolvedCount).To(Equal(0))
		Expect(anonymous.Items[0].UserStatus).To(Equal("none"))
	})

	It("keeps the curator's order across a rewrite", func(ctx SpecContext) {
		created, err := store.Create(ctx, curator, setapp.UpsertInput{Title: "顺序"})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetItems(ctx, created.ID, curator, false, []setapp.ItemInput{
			{ProblemID: unsolved}, {ProblemID: solved},
		})).To(Succeed())
		seen, err := store.Get(ctx, created.ID, "", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.Items[0].ProblemID).To(Equal(unsolved))

		Expect(store.SetItems(ctx, created.ID, curator, false, []setapp.ItemInput{
			{ProblemID: solved}, {ProblemID: unsolved},
		})).To(Succeed())
		seen, err = store.Get(ctx, created.ID, "", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.Items[0].ProblemID).To(Equal(solved))
		Expect(seen.Items).To(HaveLen(2))
	})

	It("filters private sets out of another viewer's listing", func(ctx SpecContext) {
		_, err := store.Create(ctx, curator, setapp.UpsertInput{
			Title: "公开", Visibility: setapp.VisibilityPublic,
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Create(ctx, curator, setapp.UpsertInput{
			Title: "私有", Visibility: setapp.VisibilityPrivate,
		})
		Expect(err).NotTo(HaveOccurred())

		// The count and the page must agree, which is why the filter is in SQL.
		items, total, err := store.List(ctx, setapp.Filters{ViewerID: reader, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].Title).To(Equal("公开"))

		own, total, err := store.List(ctx, setapp.Filters{ViewerID: curator, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(2))
		Expect(own).To(HaveLen(2))

		everything, total, err := store.List(ctx, setapp.Filters{Admin: true, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(2))
		Expect(everything).To(HaveLen(2))
	})

	It("searches titles and descriptions", func(ctx SpecContext) {
		_, err := store.Create(ctx, curator, setapp.UpsertInput{
			Title: "动态规划入门", Description: "从背包开始",
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Create(ctx, curator, setapp.UpsertInput{Title: "图论"})
		Expect(err).NotTo(HaveOccurred())

		items, total, err := store.List(ctx, setapp.Filters{
			Keyword: "背包", ViewerID: curator, Limit: 20,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].Title).To(Equal("动态规划入门"))
	})

	It("rejects inaccessible problems while replacing items", func(ctx SpecContext) {
		created, err := store.Create(ctx, curator, setapp.UpsertInput{Title: "边界"})
		Expect(err).NotTo(HaveOccurred())

		Expect(store.SetItems(ctx, created.ID, reader, false, []setapp.ItemInput{{ProblemID: solved}})).
			To(MatchError(setapp.ErrForbidden))
		Expect(store.SetItems(ctx, created.ID, curator, false, []setapp.ItemInput{{ProblemID: draft}})).
			To(MatchError(setapp.ErrInvalidInput))

		Expect(store.SetItems(ctx, created.ID, reader, true, []setapp.ItemInput{{ProblemID: draft}})).
			To(Succeed())
		curatorView, err := store.Get(ctx, created.ID, curator, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(curatorView.Items).To(BeEmpty())
		Expect(curatorView.ProblemCount).To(BeZero())

		adminView, err := store.Get(ctx, created.ID, reader, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(adminView.Items).To(HaveLen(1))

		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET author_id = $1 WHERE id = $2`, curator, draft)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetItems(ctx, created.ID, curator, false, []setapp.ItemInput{{ProblemID: draft}})).
			To(Succeed())
		ownProblemView, err := store.Get(ctx, created.ID, curator, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(ownProblemView.Items).To(HaveLen(1))
	})

	It("hides an item after another author makes its problem private", func(ctx SpecContext) {
		created, err := store.Create(ctx, curator, setapp.UpsertInput{Title: "会过期的题单"})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetItems(ctx, created.ID, curator, false,
			[]setapp.ItemInput{{ProblemID: unsolved}})).To(Succeed())

		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET visibility = 'private', author_id = $1 WHERE id = $2`, reader, unsolved)
		Expect(err).NotTo(HaveOccurred())

		curatorView, err := store.Get(ctx, created.ID, curator, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(curatorView.Items).To(BeEmpty())
		Expect(curatorView.ProblemCount).To(BeZero())

		authorView, err := store.Get(ctx, created.ID, reader, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(authorView.Items).To(HaveLen(1))

		listed, _, err := store.List(ctx, setapp.Filters{ViewerID: curator, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(listed).To(HaveLen(1))
		Expect(listed[0].ProblemCount).To(BeZero())
	})

	It("reports a missing set rather than an empty one", func(ctx SpecContext) {
		_, err := store.Get(ctx, "00000000-0000-0000-0000-000000000000", "", false)
		Expect(err).To(MatchError(setapp.ErrNotFound))
		Expect(store.Delete(ctx, "00000000-0000-0000-0000-000000000000")).
			To(MatchError(setapp.ErrNotFound))
	})
})
