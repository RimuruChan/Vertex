package postgres_test

import (
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problemfiles "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/filesystem"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	profiledomain "github.com/RimuruChan/Vertex/server/internal/modules/profile/domain"
	profilepg "github.com/RimuruChan/Vertex/server/internal/modules/profile/infrastructure/postgres"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

var _ = Describe("Profile store against PostgreSQL", func() {
	BeforeEach(func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `TRUNCATE submissions, contests, problems, users RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
	})

	It("reports standalone practice without exposing contest activity", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		var userID, solvedProblem, attemptedProblem, contestID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash)
			 VALUES ('alice', 'alice@t.local', 'x') RETURNING id`).Scan(&userID)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems(domain_id,title, visibility, difficulty, owner_id) VALUES ('00000000-0000-4000-8000-000000000001'::uuid,'Solved', 'public', 3, $1)RETURNING id`, userID).Scan(&solvedProblem)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems(domain_id,title, visibility, difficulty, owner_id) VALUES ('00000000-0000-4000-8000-000000000001'::uuid,'Attempted', 'public', 3, $1)RETURNING id`, userID).Scan(&attemptedProblem)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests(domain_id,title, begin_at, end_at,owner_id) VALUES ('00000000-0000-4000-8000-000000000001'::uuid,'Hidden feedback', now() - interval '1 hour', now() + interval '1 hour',$1)RETURNING id`, userID).Scan(&contestID)).To(Succeed())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, solvedProblem, attemptedProblem)).To(Succeed())

		_, err := integrationDB.Pool.ExecContext(ctx,
			`WITH fixture_input(domain_id,user_id,problem_id,language,source_code,status,contest_id,judged_at,problem_version,submitted_at) AS (VALUES (('00000000-0000-4000-8000-000000000001'::uuid)::uuid,($1)::uuid,($2)::uuid,('cpp')::text,('x')::text,('Accepted')::text,(NULL)::uuid,(now())::timestamptz,(1)::integer,(now())::timestamptz),(('00000000-0000-4000-8000-000000000001'::uuid)::uuid,($1)::uuid,($3)::uuid,('cpp')::text,('x')::text,('Wrong Answer')::text,(NULL)::uuid,(now())::timestamptz,(1)::integer,(now())::timestamptz),(('00000000-0000-4000-8000-000000000001'::uuid)::uuid,($1)::uuid,($3)::uuid,('cpp')::text,('x')::text,('Accepted')::text,($4)::uuid,(now())::timestamptz,(1)::integer,(now())::timestamptz),(('00000000-0000-4000-8000-000000000001'::uuid)::uuid,($1)::uuid,($2)::uuid,('cpp')::text,('x')::text,('Accepted')::text,(NULL)::uuid,(now()-interval '91 days')::timestamptz,(1)::integer,(now()-interval '91 days')::timestamptz)),
fixture AS (SELECT gen_random_uuid() AS fixture_id,* FROM fixture_input),
entries AS (INSERT INTO submissions(id,domain_id,user_id,problem_id,initial_problem_version,contest_id,language,source_code,submitted_at) SELECT f.fixture_id,f.domain_id,f.user_id,f.problem_id,f.problem_version,f.contest_id,f.language,f.source_code,f.submitted_at FROM fixture f JOIN problems p ON p.id=f.problem_id LEFT JOIN contest_problems cp ON cp.problem_id=p.id AND cp.contest_id=f.contest_id RETURNING *),
evaluations AS (INSERT INTO judgements(submission_id,generation,problem_id,problem_version ,status,judged_at) SELECT e.id,1,e.problem_id,e.initial_problem_version,f.status,f.judged_at FROM entries e JOIN fixture f ON f.fixture_id=e.id RETURNING *)
SELECT count(*) FROM evaluations`,
			userID, solvedProblem, attemptedProblem, contestID)
		Expect(err).NotTo(HaveOccurred())

		profile, err := profilepg.NewQueries(integrationDB).ByUsername(ctx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(profile.SolvedCount).To(Equal(1))
		Expect(profile.AttemptedCount).To(Equal(2))
		Expect(profile.SubmissionCount).To(Equal(3))
		Expect(profile.AcceptedCount).To(Equal(2))
		Expect(profile.ByDifficulty).To(ConsistOf(profiledomain.DifficultyProgress{
			Difficulty: 3, Solved: 1, Total: 2,
		}))
		Expect(profile.Activity).To(HaveLen(1))
		Expect(profile.Activity[0].Count).To(Equal(2))
	})

	It("counts only published public practice in the routed domain and rechecks domain access", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		user, err := identitypg.NewUserRepository(integrationDB).Create(ctx, "alice", "alice@example.test", "fixture")
		Expect(err).NotTo(HaveOccurred())
		space, err := tenancyapp.NewService(tenancypg.NewRepository(integrationDB)).Create(ctx, user.ID, tenancydomain.CreateInput{Slug: "private-profile", Name: "Private profile"})
		Expect(err).NotTo(HaveOccurred())
		scoped := tenancydomain.WithScope(ctx, space)
		writer := problempg.NewRepository(integrationDB, problemfiles.NewTestdataStorage(GinkgoT().TempDir()))
		public, err := writer.Create(scoped, user.ID, &problemdomain.CreateInput{Title: "Published", Visibility: "public", Difficulty: 3})
		Expect(err).NotTo(HaveOccurred())
		private, err := writer.Create(scoped, user.ID, &problemdomain.CreateInput{Title: "Private", Visibility: "private", Difficulty: 8})
		Expect(err).NotTo(HaveOccurred())
		_, err = writer.Create(scoped, user.ID, &problemdomain.CreateInput{Title: "Unreleased", Visibility: "public", Difficulty: 10})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, public.ID, private.ID)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, `WITH fixture_input(domain_id,user_id,problem_id,language,source_code,status,judged_at) AS (VALUES (($1)::uuid,($2)::uuid,($3)::uuid,('cpp')::text,('fixture')::text,('Accepted')::text,(now())::timestamptz),(($1)::uuid,($2)::uuid,($4)::uuid,('cpp')::text,('fixture')::text,('Accepted')::text,(now())::timestamptz)),
fixture AS (SELECT gen_random_uuid() AS fixture_id,* FROM fixture_input),
entries AS (INSERT INTO submissions(id,domain_id,user_id,problem_id,initial_problem_version,contest_id,language,source_code,submitted_at) SELECT f.fixture_id,f.domain_id,f.user_id,f.problem_id,CASE WHEN NULL::uuid IS NULL THEN p.published_version ELSE cp.problem_version END,NULL::uuid,f.language,f.source_code,now() FROM fixture f JOIN problems p ON p.id=f.problem_id LEFT JOIN contest_problems cp ON cp.problem_id=p.id AND cp.contest_id=NULL::uuid RETURNING *),
evaluations AS (INSERT INTO judgements(submission_id,generation,problem_id,problem_version ,status,judged_at) SELECT e.id,1,e.problem_id,e.initial_problem_version,f.status,f.judged_at FROM entries e JOIN fixture f ON f.fixture_id=e.id RETURNING *)
SELECT count(*) FROM evaluations`, space.Domain.ID, user.ID, public.ID, private.ID)
		Expect(err).NotTo(HaveOccurred())
		store := profilepg.NewQueries(integrationDB)
		result, err := store.ByUsername(scoped, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SolvedCount).To(Equal(1))
		Expect(result.SubmissionCount).To(Equal(1))
		Expect(result.ByDifficulty).To(Equal([]profiledomain.DifficultyProgress{{Difficulty: 3, Solved: 1, Total: 1}}))
		Expect(result.Activity[0].Count).To(Equal(1))
		result, err = store.ByUsername(ctx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SubmissionCount).To(BeZero())
		Expect(result.ByDifficulty).To(BeEmpty())
		_, err = store.ByUsername(tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: space.Domain}), "alice")
		Expect(err).To(MatchError(tenancydomain.ErrNotFound))
	})
})

func TestProfile(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Profile Suite")
}

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
