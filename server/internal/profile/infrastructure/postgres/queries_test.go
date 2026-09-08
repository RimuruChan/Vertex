package postgres_test

import (
	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	identitypg "github.com/RimuruChan/Vertex/server/internal/identity/infrastructure/postgres"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	problemfiles "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/filesystem"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	profiledomain "github.com/RimuruChan/Vertex/server/internal/profile/domain"
	profilepg "github.com/RimuruChan/Vertex/server/internal/profile/infrastructure/postgres"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

var _ = Describe("Profile store against PostgreSQL", func() {
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `TRUNCATE submissions, contests, problems, users RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
	})

	It("reports standalone practice without exposing contest activity", func(ctx SpecContext) {
		var userID, solvedProblem, attemptedProblem, contestID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash)
			 VALUES ('alice', 'alice@t.local', 'x') RETURNING id`).Scan(&userID)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, difficulty, owner_id)
			 VALUES ('Solved', 'public', 3, $1) RETURNING id`, userID).Scan(&solvedProblem)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, difficulty, owner_id)
			 VALUES ('Attempted', 'public', 3, $1) RETURNING id`, userID).Scan(&attemptedProblem)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, begin_at, end_at,owner_id)
			 VALUES ('Hidden feedback', now() - interval '1 hour', now() + interval '1 hour',$1)
			 RETURNING id`, userID).Scan(&contestID)).To(Succeed())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, solvedProblem, attemptedProblem)).To(Succeed())

		_, err := integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions
			   (user_id, problem_id, language, source_code, status, contest_id, judged_at,problem_version,submitted_at)
			 VALUES
			   ($1, $2, 'cpp', 'x', 'Accepted', NULL, now(),1,now()),
			   ($1, $3, 'cpp', 'x', 'Wrong Answer', NULL, now(),1,now()),
			   ($1, $3, 'cpp', 'x', 'Accepted', $4, now(),1,now()),
            ($1, $2, 'cpp', 'x', 'Accepted', NULL, now()-interval '91 days',1,now()-interval '91 days')`,
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

	It("counts only published public practice in the routed domain and rechecks domain access", func(ctx SpecContext) {
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
		_, err = integrationDB.Pool.ExecContext(ctx, `INSERT INTO submissions(domain_id,user_id,problem_id,language,source_code,status,judged_at) VALUES($1,$2,$3,'cpp','fixture','Accepted',now()),($1,$2,$4,'cpp','fixture','Accepted',now())`, space.Domain.ID, user.ID, public.ID, private.ID)
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

var _ = BeforeSuite(func(ctx SpecContext) {
	var err error
	integrationDB, releaseSuite, err = dbtest.Shared(ctx)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	releaseSuite()
})
