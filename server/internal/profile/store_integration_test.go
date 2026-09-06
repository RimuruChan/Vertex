package profile_test

import (
	profileapp "github.com/RimuruChan/Vertex/server/internal/profile"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Profile store against PostgreSQL", func() {
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		_, err := integrationDB.Pool.ExecContext(ctx,
			`TRUNCATE submissions, contests, problems, users RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
	})

	It("reports standalone practice without exposing contest activity", func(ctx SpecContext) {
		var userID, solvedProblem, attemptedProblem, contestID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash)
			 VALUES ('alice', 'alice@t.local', 'x') RETURNING id`).Scan(&userID)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, difficulty)
			 VALUES ('Solved', 'public', 3) RETURNING id`).Scan(&solvedProblem)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, difficulty)
			 VALUES ('Attempted', 'public', 3) RETURNING id`).Scan(&attemptedProblem)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, begin_at, end_at)
			 VALUES ('Hidden feedback', now() - interval '1 hour', now() + interval '1 hour')
			 RETURNING id`).Scan(&contestID)).To(Succeed())

		_, err := integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions
			   (user_id, problem_id, language, source_code, status, contest_id, judged_at)
			 VALUES
			   ($1, $2, 'cpp', 'x', 'Accepted', NULL, now()),
			   ($1, $3, 'cpp', 'x', 'Wrong Answer', NULL, now()),
			   ($1, $3, 'cpp', 'x', 'Accepted', $4, now())`,
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
