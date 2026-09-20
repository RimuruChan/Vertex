package postgres_test

import (
	"encoding/json"
	authoringapp "github.com/RimuruChan/Vertex/server/internal/modules/authoring/application"
	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	authoringfiles "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

var integrationDB *database.DB
var releaseIntegrationDB = func() {}

var _ = BeforeSuite(func(spec SpecContext) {
	ctx := dbtest.Context(spec)
	var err error
	integrationDB, releaseIntegrationDB, err = dbtest.Shared(ctx)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	releaseIntegrationDB()
})

var _ = Describe("Problem queries against PostgreSQL", func() {
	var fixtureOwner string
	BeforeEach(func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `TRUNCATE problem_tags, tags, problems, users RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		Expect(integrationDB.Pool.QueryRowContext(ctx, "INSERT INTO users(username,email,password_hash) VALUES('setter','setter@example.test','fixture') RETURNING id").Scan(&fixtureOwner)).To(Succeed())
	})

	It("keeps statement markdown out of the list projection", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		const statement = "# Large statement\n\nThis body belongs only in the detail query."
		var problemID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems(domain_id,title, statement_md, visibility, owner_id) VALUES ('00000000-0000-4000-8000-000000000001'::uuid,'A + B', $1, 'public', $2)RETURNING id`, statement, fixtureOwner).Scan(&problemID)).To(Succeed())
		_, err := integrationDB.Pool.ExecContext(ctx, `WITH changed AS (UPDATE problems SET difficulty=3 WHERE id=$1 RETURNING domain_id), added AS (INSERT INTO tags(domain_id,name) SELECT domain_id,'math' FROM changed RETURNING id,domain_id) INSERT INTO problem_tags(domain_id,problem_id,tag_id) SELECT domain_id,$1,id FROM added`, problemID)
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
		ownerContext := tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: fixtureOwner})
		blobs, err := authoringfiles.NewBlobStore(GinkgoT().TempDir(), 1<<20)
		Expect(err).NotTo(HaveOccurred())
		repo := authoringpg.NewRevisionRepository(integrationDB, blobs)
		workbench := authoringapp.NewWorkbench(repo)
		copy, err := repo.Open(ownerContext, problemID)
		Expect(err).NotTo(HaveOccurred())
		material, err := workbench.Material(ownerContext, problemID, "problem", 0)
		Expect(err).NotTo(HaveOccurred())
		material.Metadata.Title = "Working title"
		material.Metadata.Difficulty = 9
		material.Metadata.Tags = []string{"draft"}
		encoded, err := json.Marshal(material.Metadata)
		Expect(err).NotTo(HaveOccurred())
		text := string(encoded)
		_, err = workbench.SaveEntry(ownerContext, problemID, copy.ETag, material.Entry, &text)
		Expect(err).NotTo(HaveOccurred())
		filters.Offset = 0
		items, total, err = store.List(ctx, filters)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].Title).To(Equal("A + B"))
		library, err := repo.Library(ownerContext, authoringdomain.LibraryQuery{Keyword: "Working", Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(library.Total).To(Equal(1))
		Expect(library.Items).To(HaveLen(1))
		Expect(library.Items[0].Title).To(Equal("Working title"))

	})

	It("keeps contest verdicts out of public practice progress", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		var userID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash)
			 VALUES ('practice-user', 'practice@t.local', 'x') RETURNING id`).Scan(&userID)).To(Succeed())

		problemIDs := make([]string, 3)
		for index, title := range []string{"Contest only", "Practice attempt", "Practice solved"} {
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO problems(domain_id,title, visibility, owner_id) VALUES ('00000000-0000-4000-8000-000000000001'::uuid,$1, 'public', $2)RETURNING id`, title, fixtureOwner).
				Scan(&problemIDs[index])).To(Succeed())
		}
		var contestID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests(domain_id,title, begin_at, end_at,owner_id) VALUES ('00000000-0000-4000-8000-000000000001'::uuid,'Hidden feedback', now() - interval '1 hour', now() + interval '1 hour',$1)RETURNING id`, fixtureOwner).Scan(&contestID)).To(Succeed())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, problemIDs...)).To(Succeed())
		_, err := integrationDB.Pool.ExecContext(ctx,
			`WITH fixture_input(domain_id,user_id,problem_id,language,source_code,status,contest_id,judged_at,problem_version) AS (VALUES (('00000000-0000-4000-8000-000000000001'::uuid)::uuid,($1)::uuid,($2)::uuid,('cpp')::text,('x')::text,('Accepted')::text,($5)::uuid,(now())::timestamptz,(1)::integer),(('00000000-0000-4000-8000-000000000001'::uuid)::uuid,($1)::uuid,($3)::uuid,('cpp')::text,('x')::text,('Wrong Answer')::text,(NULL)::uuid,(now())::timestamptz,(1)::integer),(('00000000-0000-4000-8000-000000000001'::uuid)::uuid,($1)::uuid,($3)::uuid,('cpp')::text,('x')::text,('Accepted')::text,($5)::uuid,(now())::timestamptz,(1)::integer),(('00000000-0000-4000-8000-000000000001'::uuid)::uuid,($1)::uuid,($4)::uuid,('cpp')::text,('x')::text,('Accepted')::text,(NULL)::uuid,(now())::timestamptz,(1)::integer)),
fixture AS (SELECT gen_random_uuid() AS fixture_id,* FROM fixture_input),
entries AS (INSERT INTO submissions(id,domain_id,user_id,problem_id,initial_problem_version,contest_id,language,source_code,submitted_at) SELECT f.fixture_id,f.domain_id,f.user_id,f.problem_id,f.problem_version,f.contest_id,f.language,f.source_code,now() FROM fixture f JOIN problems p ON p.id=f.problem_id LEFT JOIN contest_problems cp ON cp.problem_id=p.id AND cp.contest_id=f.contest_id RETURNING *),
evaluations AS (INSERT INTO judgements(submission_id,generation,problem_id,problem_version ,status,judged_at) SELECT e.id,1,e.problem_id,e.initial_problem_version,f.status,f.judged_at FROM entries e JOIN fixture f ON f.fixture_id=e.id RETURNING *)
SELECT count(*) FROM evaluations`,
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
