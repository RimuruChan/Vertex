package profile_test

import (
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	profileapp "github.com/RimuruChan/Vertex/server/internal/profile"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
			   (user_id, problem_id, language, source_code, status, contest_id, judged_at,problem_version)
			 VALUES
			   ($1, $2, 'cpp', 'x', 'Accepted', NULL, now(),1),
			   ($1, $3, 'cpp', 'x', 'Wrong Answer', NULL, now(),1),
			   ($1, $3, 'cpp', 'x', 'Accepted', $4, now(),1)`,
			userID, solvedProblem, attemptedProblem, contestID)
		Expect(err).NotTo(HaveOccurred())

		profile, err := profileapp.NewProfileStore(integrationDB).ByUsername(ctx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(profile.SolvedCount).To(Equal(1))
		Expect(profile.AttemptedCount).To(Equal(2))
		Expect(profile.SubmissionCount).To(Equal(2))
		Expect(profile.AcceptedCount).To(Equal(1))
		Expect(profile.ByDifficulty).To(ConsistOf(profileapp.DifficultyProgress{
			Difficulty: 3, Solved: 1, Total: 2,
		}))
		Expect(profile.Activity).To(HaveLen(1))
		Expect(profile.Activity[0].Count).To(Equal(2))
	})

	It("counts only published public practice in the routed domain and rechecks domain access", func(ctx SpecContext) {
		user, err := identity.NewUserStore(integrationDB).Create(ctx, "alice", "alice@example.test", "fixture")
		Expect(err).NotTo(HaveOccurred())
		space, err := domain.NewService(domain.NewStore(integrationDB)).Create(ctx, user.ID, domain.CreateInput{Slug: "private-profile", Name: "Private profile"})
		Expect(err).NotTo(HaveOccurred())
		scoped := domain.WithScope(ctx, space)
		writer := problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		public, err := writer.Create(scoped, user.ID, &problem.CreateInput{Title: "Published", Visibility: "public", Difficulty: 3})
		Expect(err).NotTo(HaveOccurred())
		private, err := writer.Create(scoped, user.ID, &problem.CreateInput{Title: "Private", Visibility: "private", Difficulty: 8})
		Expect(err).NotTo(HaveOccurred())
		_, err = writer.Create(scoped, user.ID, &problem.CreateInput{Title: "Unreleased", Visibility: "public", Difficulty: 10})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, public.ID, private.ID)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, `INSERT INTO submissions(domain_id,user_id,problem_id,language,source_code,status,judged_at) VALUES($1,$2,$3,'cpp','fixture','Accepted',now()),($1,$2,$4,'cpp','fixture','Accepted',now())`, space.Domain.ID, user.ID, public.ID, private.ID)
		Expect(err).NotTo(HaveOccurred())
		store := profileapp.NewProfileStore(integrationDB)
		result, err := store.ByUsername(scoped, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SolvedCount).To(Equal(1))
		Expect(result.SubmissionCount).To(Equal(1))
		Expect(result.ByDifficulty).To(Equal([]profileapp.DifficultyProgress{{Difficulty: 3, Solved: 1, Total: 1}}))
		Expect(result.Activity[0].Count).To(Equal(1))
		result, err = store.ByUsername(ctx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SubmissionCount).To(BeZero())
		Expect(result.ByDifficulty).To(BeEmpty())
		_, err = store.ByUsername(domain.WithScope(ctx, domain.Scope{Domain: space.Domain}), "alice")
		Expect(err).To(MatchError(domain.ErrNotFound))
	})
})
