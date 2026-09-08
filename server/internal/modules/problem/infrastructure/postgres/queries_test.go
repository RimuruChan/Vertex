package postgres_test

import (
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
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

var _ = Describe("Problem queries against PostgreSQL", func() {
	var fixtureOwner string
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `TRUNCATE problem_tags, tags, problems, users RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		Expect(integrationDB.Pool.QueryRowContext(ctx, "INSERT INTO users(username,email,password_hash) VALUES('setter','setter@example.test','fixture') RETURNING id").Scan(&fixtureOwner)).To(Succeed())
	})

	It("keeps statement markdown out of the list projection", func(ctx SpecContext) {
		const statement = "# Large statement\n\nThis body belongs only in the detail query."
		var problemID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, statement_md, visibility, owner_id)
			 VALUES ('A + B', $1, 'public', $2) RETURNING id`, statement, fixtureOwner).Scan(&problemID)).To(Succeed())
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE problem_workspaces SET difficulty=3,tags_json='[\"math\"]' WHERE problem_id=$1", problemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, problemID)).To(Succeed())

		store := problempg.NewQueries(integrationDB)
		items, total, err := store.List(ctx, problemdomain.Filters{Visibility: "public", Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].StatementMD).To(BeEmpty())

		detail, err := store.Get(ctx, problemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(detail.StatementMD).To(Equal(statement))
		filters := problemdomain.Filters{Visibility: "public", Tag: "math", Difficulty: 3, Keyword: "A +", Limit: 1}
		items, total, err = store.List(ctx, filters)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(total).To(Equal(1))
		filters.Offset = 1
		items, total, err = store.List(ctx, filters)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		Expect(total).To(Equal(1))
		Expect(dbtest.OfficialMembers(ctx, integrationDB)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problem_workspaces SET title='Working title',difficulty=9,tags_json='[\"draft\"]' WHERE problem_id=$1", problemID)
		Expect(err).NotTo(HaveOccurred())
		filters.Offset = 0
		items, total, err = store.List(ctx, filters)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].Title).To(Equal("A + B"))
		items, total, err = store.List(ctx, problemdomain.Filters{Workspace: true, ViewerID: fixtureOwner, Tag: "draft", Difficulty: 9, Keyword: "Working", Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].Title).To(Equal("Working title"))

	})

	It("keeps contest verdicts out of public practice progress", func(ctx SpecContext) {
		var userID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash)
			 VALUES ('practice-user', 'practice@t.local', 'x') RETURNING id`).Scan(&userID)).To(Succeed())

		problemIDs := make([]string, 3)
		for index, title := range []string{"Contest only", "Practice attempt", "Practice solved"} {
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO problems (title, visibility, owner_id) VALUES ($1, 'public', $2) RETURNING id`, title, fixtureOwner).
				Scan(&problemIDs[index])).To(Succeed())
		}
		var contestID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, begin_at, end_at,owner_id)
			 VALUES ('Hidden feedback', now() - interval '1 hour', now() + interval '1 hour',$1)
			 RETURNING id`, fixtureOwner).Scan(&contestID)).To(Succeed())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, problemIDs...)).To(Succeed())
		_, err := integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions
			   (user_id, problem_id, language, source_code, status, contest_id, judged_at,problem_version)
			 VALUES
			   ($1, $2, 'cpp', 'x', 'Accepted', $5, now(),1),
			   ($1, $3, 'cpp', 'x', 'Wrong Answer', NULL, now(),1),
			   ($1, $3, 'cpp', 'x', 'Accepted', $5, now(),1),
			   ($1, $4, 'cpp', 'x', 'Accepted', NULL, now(),1)`,
			userID, problemIDs[0], problemIDs[1], problemIDs[2], contestID)
		Expect(err).NotTo(HaveOccurred())

		store := problempg.NewQueries(integrationDB)
		statuses, err := store.UserStatuses(ctx, userID, problemIDs)
		Expect(err).NotTo(HaveOccurred())
		Expect(statuses).NotTo(HaveKey(problemIDs[0]))
		Expect(statuses).To(HaveKeyWithValue(problemIDs[1], problemdomain.UserStatusAttempted))
		Expect(statuses).To(HaveKeyWithValue(problemIDs[2], problemdomain.UserStatusSolved))

		expected := map[string]string{problemdomain.UserStatusSolved: problemIDs[2], problemdomain.UserStatusAttempted: problemIDs[1], problemdomain.UserStatusNone: problemIDs[0]}
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

func TestProblem(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Problem Suite")
}
