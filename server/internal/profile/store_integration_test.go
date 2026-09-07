package profile_test

import (
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
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
})
