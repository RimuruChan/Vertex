package problem_test

import (
	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var integrationDB *database.DB
var releaseIntegrationDB = func() {}

var _ = BeforeSuite(func(ctx SpecContext) {
	var err error
	integrationDB, releaseIntegrationDB, err = dbtest.Shared(ctx)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	releaseIntegrationDB()
})

var _ = Describe("Problem store against PostgreSQL", func() {
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		_, err := integrationDB.Pool.ExecContext(ctx,
			`TRUNCATE problem_tags, tags, problems, users RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
	})

	It("keeps statement markdown out of the list projection", func(ctx SpecContext) {
		const statement = "# Large statement\n\nThis body belongs only in the detail query."
		var problemID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, statement_md, visibility)
			 VALUES ('A + B', $1, 'public') RETURNING id`, statement).Scan(&problemID)).To(Succeed())

		store := problemdomain.NewProblemStore(integrationDB)
		items, total, err := store.List(ctx, problemdomain.Filters{Visibility: "public", Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].StatementMD).To(BeEmpty())

		detail, err := store.Get(ctx, problemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(detail.StatementMD).To(Equal(statement))
	})

	It("keeps contest verdicts out of public practice progress", func(ctx SpecContext) {
		var userID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash)
			 VALUES ('practice-user', 'practice@t.local', 'x') RETURNING id`).Scan(&userID)).To(Succeed())

		problemIDs := make([]string, 3)
		for index, title := range []string{"Contest only", "Practice attempt", "Practice solved"} {
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO problems (title, visibility) VALUES ($1, 'public') RETURNING id`, title).
				Scan(&problemIDs[index])).To(Succeed())
		}
		var contestID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, begin_at, end_at)
			 VALUES ('Hidden feedback', now() - interval '1 hour', now() + interval '1 hour')
			 RETURNING id`).Scan(&contestID)).To(Succeed())
		_, err := integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions
			   (user_id, problem_id, language, source_code, status, contest_id, judged_at)
			 VALUES
			   ($1, $2, 'cpp', 'x', 'Accepted', $5, now()),
			   ($1, $3, 'cpp', 'x', 'Wrong Answer', NULL, now()),
			   ($1, $3, 'cpp', 'x', 'Accepted', $5, now()),
			   ($1, $4, 'cpp', 'x', 'Accepted', NULL, now())`,
			userID, problemIDs[0], problemIDs[1], problemIDs[2], contestID)
		Expect(err).NotTo(HaveOccurred())

		store := problemdomain.NewProblemStore(integrationDB)
		statuses, err := store.UserStatuses(ctx, userID, problemIDs)
		Expect(err).NotTo(HaveOccurred())
		Expect(statuses).NotTo(HaveKey(problemIDs[0]))
		Expect(statuses).To(HaveKeyWithValue(problemIDs[1], problemdomain.UserStatusAttempted))
		Expect(statuses).To(HaveKeyWithValue(problemIDs[2], problemdomain.UserStatusSolved))

		expected := map[string]string{
			problemdomain.UserStatusSolved:    problemIDs[2],
			problemdomain.UserStatusAttempted: problemIDs[1],
			problemdomain.UserStatusNone:      problemIDs[0],
		}
		for status, expectedID := range expected {
			items, total, err := store.List(ctx, problemdomain.Filters{
				Visibility: "public", ViewerID: userID, Status: status, Limit: 20,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(total).To(Equal(1))
			Expect(items).To(HaveLen(1))
			Expect(items[0].ID).To(Equal(expectedID))
		}
	})
})
