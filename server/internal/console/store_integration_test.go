package console_test

import (
	consoleapp "github.com/RimuruChan/Vertex/server/internal/console"
	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
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

// The console reads across every domain, so its aggregate SQL only proves out
// against a database that actually has all those tables.
var _ = Describe("Console store against PostgreSQL", func() {
	var store *consoleapp.ConsoleStore
	var admin, member string

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `
			TRUNCATE announcements, editorial_votes, editorials, problem_set_problems,
				problem_sets, judge_jobs, submissions, problem_tags, tags,
				auth_sessions, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = consoleapp.NewConsoleStore(integrationDB)

		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash, role)
			 VALUES ('root', 'root@t.local', 'x', 'admin') RETURNING id`).Scan(&admin)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('member', 'm@t.local', 'x')
			 RETURNING id`).Scan(&member)).To(Succeed())
	})

	It("aggregates the dashboard across domains", func(ctx SpecContext) {
		var problemID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, owner_id) VALUES ('Sum', 'public', $1) RETURNING id`, admin).
			Scan(&problemID)).To(Succeed())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, problemID)).To(Succeed())
		_, err := integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions (user_id, problem_id, language, source_code, status)
			 VALUES ($1, $2, 'cpp', 'x', 'Accepted')`, member, problemID)
		Expect(err).NotTo(HaveOccurred())

		stats, err := store.Stats(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(stats.Users).To(Equal(2))
		Expect(stats.UsersToday).To(Equal(2))
		Expect(stats.Problems).To(Equal(1))
		Expect(stats.PublicProblems).To(Equal(1))
		Expect(stats.Submissions).To(Equal(1))
		Expect(stats.VerdictBreakdown).To(HaveLen(1))
		Expect(stats.VerdictBreakdown[0].Verdict).To(Equal("Accepted"))
		// An empty queue reports no oldest entry rather than a zero time.
		Expect(stats.QueuedJobs).To(Equal(0))
		Expect(stats.OldestQueued).To(BeNil())
	})

	It("searches accounts and keeps the count aligned with the page", func(ctx SpecContext) {
		items, total, err := store.ListAccounts(ctx, consoleapp.AccountFilters{
			Keyword: "mem", Limit: 20,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].Username).To(Equal("member"))

		items, total, err = store.ListAccounts(ctx, consoleapp.AccountFilters{
			Role: "admin", Limit: 20,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].Username).To(Equal("root"))
	})

	It("blocks an account and revokes its sessions in one step", func(ctx SpecContext) {
		_, err := integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO auth_sessions (user_id, refresh_token_hash, expires_at)
			 VALUES ($1, '\x00'::bytea, now() + interval '1 day')`, member)
		Expect(err).NotTo(HaveOccurred())

		disabled := true
		updated, err := store.UpdateAccount(ctx, member, consoleapp.AccountUpdate{
			Disabled: &disabled, Reason: "spam",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Disabled()).To(BeTrue())
		Expect(updated.DisabledReason).To(Equal("spam"))

		var live int
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT count(*)::int FROM auth_sessions WHERE user_id = $1 AND revoked_at IS NULL`,
			member).Scan(&live)).To(Succeed())
		Expect(live).To(Equal(0))

		// Unblocking clears both the timestamp and the reason.
		disabled = false
		updated, err = store.UpdateAccount(ctx, member, consoleapp.AccountUpdate{Disabled: &disabled})
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Disabled()).To(BeFalse())
		Expect(updated.DisabledReason).To(BeEmpty())
	})

	It("changes only the fields the caller set", func(ctx SpecContext) {
		role := "admin"
		_, err := store.UpdateAccount(ctx, member, consoleapp.AccountUpdate{Role: &role})
		Expect(err).NotTo(HaveOccurred())

		rating := 1500
		updated, err := store.UpdateAccount(ctx, member, consoleapp.AccountUpdate{Rating: &rating})
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Rating).To(Equal(1500))
		// The role set by the previous call survived a rating-only update.
		Expect(updated.Role).To(Equal("admin"))
	})

	It("reports a missing account rather than silently succeeding", func(ctx SpecContext) {
		rating := 10
		_, err := store.UpdateAccount(ctx, "00000000-0000-0000-0000-000000000000",
			consoleapp.AccountUpdate{Rating: &rating})
		Expect(err).To(MatchError(consoleapp.ErrNotFound))
	})

	It("merges tags and moves their problems", func(ctx SpecContext) {
		var problemA, problemB string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, owner_id) VALUES ('A', $1) RETURNING id`, admin).Scan(&problemA)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, owner_id) VALUES ('B', $1) RETURNING id`, admin).Scan(&problemB)).To(Succeed())

		var lower, upper int64
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO tags (name) VALUES ('dp') RETURNING id`).Scan(&lower)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO tags (name) VALUES ('DP') RETURNING id`).Scan(&upper)).To(Succeed())
		_, err := integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO problem_tags (problem_id, tag_id) VALUES ($1, $2), ($3, $4), ($1, $4)`,
			problemA, lower, problemB, upper)
		Expect(err).NotTo(HaveOccurred())

		merged, err := store.MergeTags(ctx, lower, upper)
		Expect(err).NotTo(HaveOccurred())
		Expect(merged.Name).To(Equal("DP"))
		// Both problems end up on the target, and the one that already had it
		// keeps a single link.
		Expect(merged.ProblemCount).To(Equal(2))

		tags, err := store.ListTags(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(tags).To(HaveLen(1))
	})

	It("treats a rename onto an existing name as a merge", func(ctx SpecContext) {
		var lower, upper int64
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO tags (name) VALUES ('dp') RETURNING id`).Scan(&lower)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO tags (name) VALUES ('DP') RETURNING id`).Scan(&upper)).To(Succeed())

		renamed, err := store.RenameTag(ctx, lower, "DP")
		Expect(err).NotTo(HaveOccurred())
		Expect(renamed.ID).To(Equal(upper))

		tags, err := store.ListTags(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(tags).To(HaveLen(1))
		Expect(tags[0].Name).To(Equal("DP"))
	})

	It("hides unpublished announcements from readers and pins the rest", func(ctx SpecContext) {
		_, err := store.CreateAnnouncement(ctx, admin, consoleapp.AnnouncementInput{
			Title: "普通", ContentMD: "x", Published: true,
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateAnnouncement(ctx, admin, consoleapp.AnnouncementInput{
			Title: "置顶", ContentMD: "x", Published: true, Pinned: true,
		})
		Expect(err).NotTo(HaveOccurred())
		draft, err := store.CreateAnnouncement(ctx, admin, consoleapp.AnnouncementInput{
			Title: "草稿", ContentMD: "x",
		})
		Expect(err).NotTo(HaveOccurred())

		public, err := store.ListAnnouncements(ctx, true, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(public).To(HaveLen(2))
		Expect(public[0].Title).To(Equal("置顶"))
		Expect(public[0].AuthorName).To(Equal("root"))

		everything, err := store.ListAnnouncements(ctx, false, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(everything).To(HaveLen(3))

		Expect(store.DeleteAnnouncement(ctx, draft.ID)).To(Succeed())
		Expect(store.DeleteAnnouncement(ctx, draft.ID)).To(MatchError(consoleapp.ErrNotFound))
	})
})
