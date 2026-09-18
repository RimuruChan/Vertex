package postgres_test

import (
	setdomain "github.com/RimuruChan/Vertex/server/internal/modules/problemset/domain"
	setpg "github.com/RimuruChan/Vertex/server/internal/modules/problemset/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

var integrationDB *database.DB
var releaseSuite = func() {}

var _ = BeforeSuite(func(spec SpecContext) {
	ctx := dbtest.Context(spec)
	var err error
	integrationDB, releaseSuite, err = dbtest.Shared(ctx)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	releaseSuite()
})

// Visibility, progress and atomic item replacement are verified against PostgreSQL.
var _ = Describe("Problem set store against PostgreSQL", func() {
	var store *setpg.Repository
	var curator, reader, setter, solved, unsolved, draft string

	BeforeEach(func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `
			TRUNCATE problem_set_problems, problem_sets, submissions,
				problem_tags, tags, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = setpg.NewRepository(integrationDB)

		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('curator', 'c@t.local', 'x')
			 RETURNING id`).Scan(&curator)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('reader', 'r@t.local', 'x')
			 RETURNING id`).Scan(&reader)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx, "INSERT INTO users(username,email,password_hash) VALUES('setter','setter@example.test','fixture') RETURNING id").Scan(&setter)).To(Succeed())
		Expect(dbtest.OfficialMembers(ctx, integrationDB)).To(Succeed())
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
				`INSERT INTO problems(domain_id,title, visibility, owner_id) VALUES ('00000000-0000-4000-8000-000000000001'::uuid,$1, $2, $3)RETURNING id`,
				item.title, item.visibility, setter).Scan(item.target)).To(Succeed())
		}
		Expect(dbtest.PublishedProblems(ctx, integrationDB)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`WITH fixture_input(domain_id,user_id,problem_id,language,source_code,status) AS (VALUES (('00000000-0000-4000-8000-000000000001'::uuid)::uuid,($1)::uuid,($2)::uuid,('cpp')::text,('x')::text,('Accepted')::text)),
fixture AS (SELECT gen_random_uuid() AS fixture_id,* FROM fixture_input),
entries AS (INSERT INTO submissions(id,domain_id,user_id,problem_id,initial_problem_version,contest_id,language,source_code,submitted_at) SELECT f.fixture_id,f.domain_id,f.user_id,f.problem_id,CASE WHEN NULL::uuid IS NULL THEN p.published_version ELSE cp.problem_version END,NULL::uuid,f.language,f.source_code,now() FROM fixture f JOIN problems p ON p.id=f.problem_id LEFT JOIN contest_problems cp ON cp.problem_id=p.id AND cp.contest_id=NULL::uuid RETURNING *),
evaluations AS (INSERT INTO judgements(submission_id,generation,problem_id,problem_version ,status) SELECT e.id,1,e.problem_id,e.initial_problem_version,f.status FROM entries e JOIN fixture f ON f.fixture_id=e.id RETURNING *)
SELECT count(*) FROM evaluations`, reader, solved)
		Expect(err).NotTo(HaveOccurred())
		var contestID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests(domain_id,title, begin_at, end_at,owner_id) VALUES ('00000000-0000-4000-8000-000000000001'::uuid,'Hidden feedback', now() - interval '1 hour', now() + interval '1 hour',$1)RETURNING id`, curator).Scan(&contestID)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`WITH fixture_input(domain_id,user_id,problem_id,language,source_code,status,contest_id,problem_version) AS (VALUES (('00000000-0000-4000-8000-000000000001'::uuid)::uuid,($1)::uuid,($2)::uuid,('cpp')::text,('x')::text,('Accepted')::text,($3)::uuid,(1)::integer)),
fixture AS (SELECT gen_random_uuid() AS fixture_id,* FROM fixture_input),
entries AS (INSERT INTO submissions(id,domain_id,user_id,problem_id,initial_problem_version,contest_id,language,source_code,submitted_at) SELECT f.fixture_id,f.domain_id,f.user_id,f.problem_id,f.problem_version,f.contest_id,f.language,f.source_code,now() FROM fixture f JOIN problems p ON p.id=f.problem_id LEFT JOIN contest_problems cp ON cp.problem_id=p.id AND cp.contest_id=f.contest_id RETURNING *),
evaluations AS (INSERT INTO judgements(submission_id,generation,problem_id,problem_version ,status) SELECT e.id,1,e.problem_id,e.initial_problem_version,f.status FROM entries e JOIN fixture f ON f.fixture_id=e.id RETURNING *)
SELECT count(*) FROM evaluations`, reader, unsolved, contestID)
		Expect(err).NotTo(HaveOccurred())
	})

	It("computes the viewer's progress and per-problem status", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		created, err := store.Create(ctx, curator, setdomain.UpsertInput{
			Title: "入门", Visibility: setdomain.VisibilityPublic,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetItems(ctx, created.ID, curator, []setdomain.ItemInput{
			{ProblemID: solved, Note: "热身"},
			{ProblemID: unsolved},
		})).To(Succeed())

		// The reader solved exactly one of the two problems.
		seen, err := store.Get(ctx, created.ID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.ProblemCount).To(Equal(2))
		Expect(seen.SolvedCount).To(Equal(1))
		Expect(seen.Items[0].ProblemID).To(Equal(solved))
		Expect(seen.Items[0].Note).To(Equal("热身"))
		Expect(seen.Items[0].UserStatus).To(Equal("solved"))
		Expect(seen.Items[1].UserStatus).To(Equal("none"))

		// An anonymous viewer never triggers the submissions lookup.
		anonymous, err := store.Get(ctx, created.ID, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(anonymous.SolvedCount).To(Equal(0))
		Expect(anonymous.Items[0].UserStatus).To(Equal("none"))
	})

	It("keeps the curator's order across a rewrite", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		created, err := store.Create(ctx, curator, setdomain.UpsertInput{Title: "顺序"})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetItems(ctx, created.ID, curator, []setdomain.ItemInput{
			{ProblemID: unsolved}, {ProblemID: solved},
		})).To(Succeed())
		seen, err := store.Get(ctx, created.ID, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.Items[0].ProblemID).To(Equal(unsolved))

		Expect(store.SetItems(ctx, created.ID, curator, []setdomain.ItemInput{
			{ProblemID: solved}, {ProblemID: unsolved},
		})).To(Succeed())
		seen, err = store.Get(ctx, created.ID, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.Items[0].ProblemID).To(Equal(solved))
		Expect(seen.Items).To(HaveLen(2))
	})

	It("filters private sets out of another viewer's listing", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		_, err := store.Create(ctx, curator, setdomain.UpsertInput{
			Title: "公开", Visibility: setdomain.VisibilityPublic,
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Create(ctx, curator, setdomain.UpsertInput{
			Title: "私有", Visibility: setdomain.VisibilityPrivate,
		})
		Expect(err).NotTo(HaveOccurred())

		// The count and the page must agree, which is why the filter is in SQL.
		items, total, err := store.List(ctx, setdomain.Filters{ViewerID: reader, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].Title).To(Equal("公开"))

		own, total, err := store.List(ctx, setdomain.Filters{ViewerID: curator, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(2))
		Expect(own).To(HaveLen(2))

		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE users SET role='admin' WHERE id=$1", reader)
		Expect(err).NotTo(HaveOccurred())
		everything, total, err := store.List(ctx, setdomain.Filters{ViewerID: reader, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(2))
		Expect(everything).To(HaveLen(2))
	})

	It("searches titles and descriptions", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		_, err := store.Create(ctx, curator, setdomain.UpsertInput{
			Title: "动态规划入门", Description: "从背包开始",
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Create(ctx, curator, setdomain.UpsertInput{Title: "图论"})
		Expect(err).NotTo(HaveOccurred())

		items, total, err := store.List(ctx, setdomain.Filters{
			Keyword: "背包", ViewerID: curator, Limit: 20,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].Title).To(Equal("动态规划入门"))
	})

	It("rejects inaccessible problems while replacing items", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		created, err := store.Create(ctx, curator, setdomain.UpsertInput{Title: "边界"})
		Expect(err).NotTo(HaveOccurred())

		Expect(store.SetItems(ctx, created.ID, reader, []setdomain.ItemInput{{ProblemID: solved}})).
			To(MatchError(setdomain.ErrForbidden))
		Expect(store.SetItems(ctx, created.ID, curator, []setdomain.ItemInput{{ProblemID: draft}})).
			To(MatchError(setdomain.ErrInvalidInput))
		Expect(store.SetItems(ctx, created.ID, reader, []setdomain.ItemInput{{ProblemID: draft}})).To(MatchError(setdomain.ErrForbidden))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE users SET role='admin' WHERE id=$1", reader)
		Expect(err).NotTo(HaveOccurred())

		Expect(store.SetItems(ctx, created.ID, reader, []setdomain.ItemInput{{ProblemID: draft}})).
			To(Succeed())
		curatorView, err := store.Get(ctx, created.ID, curator)
		Expect(err).NotTo(HaveOccurred())
		Expect(curatorView.Items).To(BeEmpty())
		Expect(curatorView.ProblemCount).To(BeZero())

		adminView, err := store.Get(ctx, created.ID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(adminView.Items).To(HaveLen(1))

		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET owner_id = $1 WHERE id = $2`, curator, draft)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetItems(ctx, created.ID, curator, []setdomain.ItemInput{{ProblemID: draft}})).
			To(Succeed())
		ownProblemView, err := store.Get(ctx, created.ID, curator)
		Expect(err).NotTo(HaveOccurred())
		Expect(ownProblemView.Items).To(HaveLen(1))
	})

	It("hides an item after another author makes its problem private", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		created, err := store.Create(ctx, curator, setdomain.UpsertInput{Title: "会过期的题单"})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetItems(ctx, created.ID, curator,
			[]setdomain.ItemInput{{ProblemID: unsolved}})).To(Succeed())

		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET visibility = 'private', owner_id = $1 WHERE id = $2`, reader, unsolved)
		Expect(err).NotTo(HaveOccurred())

		curatorView, err := store.Get(ctx, created.ID, curator)
		Expect(err).NotTo(HaveOccurred())
		Expect(curatorView.Items).To(BeEmpty())
		Expect(curatorView.ProblemCount).To(BeZero())

		authorView, err := store.Get(ctx, created.ID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(authorView.Items).To(HaveLen(1))

		listed, _, err := store.List(ctx, setdomain.Filters{ViewerID: curator, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(listed).To(HaveLen(1))
		Expect(listed[0].ProblemCount).To(BeZero())
	})

	It("reports a missing set rather than an empty one", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		_, err := store.Get(ctx, "00000000-0000-0000-0000-000000000000", "")
		Expect(err).To(MatchError(setdomain.ErrNotFound))
		Expect(store.Delete(ctx, "00000000-0000-0000-0000-000000000000", curator)).
			To(MatchError(setdomain.ErrNotFound))
	})
})

func TestProblemSet(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Problem Set Suite")
}
