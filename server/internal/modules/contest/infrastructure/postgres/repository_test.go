package postgres_test

import (
	"context"
	"fmt"
	contestapp "github.com/RimuruChan/Vertex/server/internal/modules/contest/application"
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	contestpg "github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres"
	contestdto "github.com/RimuruChan/Vertex/server/internal/modules/contest/transport/http/dto"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
	"time"
)

// These specs exercise the scoring SQL against a real PostgreSQL instance:
// the pure ScoreCell tests prove the arithmetic, these prove that the queries
// around it read and write the columns the migrations actually created.
var _ = Describe("Contest scoring against PostgreSQL", Ordered, func() {
	var store *contestpg.Repository

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `
			TRUNCATE contest_submission_cells, contest_participants, contest_access,
				contest_problems, clarifications, contests, submissions, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = contestpg.NewRepository(integrationDB)
	})

	// fixture builds a contest with one problem and two registered users.
	type fixture struct {
		owner     string
		contestID string
		problemID string
		alice     string
		bob       string
		begin     time.Time
	}
	ownerContext := func(ctx context.Context, f fixture) context.Context {
		return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: f.owner})
	}

	build := func(ctx context.Context, rule string, freeze *time.Time) fixture {
		var result fixture
		result.begin = time.Now().Add(-2 * time.Hour).Truncate(time.Second)

		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('alice', 'a@test.local', 'x')
			 RETURNING id`).Scan(&result.alice)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('bob', 'b@test.local', 'x')
			 RETURNING id`).Scan(&result.bob)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx, "INSERT INTO users(username,email,password_hash) VALUES('organizer','organizer@example.test','fixture') RETURNING id").Scan(&result.owner)).To(Succeed())
		Expect(dbtest.OfficialMembers(ctx, integrationDB)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, owner_id) VALUES ('Sum', 'public', $1) RETURNING id`, result.alice).
			Scan(&result.problemID)).To(Succeed())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, result.problemID)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, rule, begin_at, end_at, freeze_at, penalty_minutes,owner_id,created_by)
			 VALUES ('Round', $1, $2, $3, $4, 20,$5,$5) RETURNING id`,
			rule, result.begin, result.begin.Add(5*time.Hour), freeze, result.owner).
			Scan(&result.contestID)).To(Succeed())

		Expect(store.SetProblems(ownerContext(ctx, result), result.contestID, []contestdomain.ProblemEntry{
			{ProblemID: result.problemID, Label: "A", Color: "#ff0000", Points: 100},
		})).To(Succeed())
		for _, userID := range []string{result.alice, result.bob} {
			_, err := integrationDB.Pool.ExecContext(ctx, "INSERT INTO contest_participants(contest_id,user_id) VALUES($1,$2)", result.contestID, userID)
			Expect(err).NotTo(HaveOccurred())
		}
		return result
	}

	// submit inserts a judged submission at the given offset from the start.
	submit := func(ctx context.Context, f fixture, userID, status string, score, minutes int) {
		_, err := integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submissions (user_id, problem_id, language, source_code, status, score,
			                          contest_id, submitted_at, judged_at)
			 VALUES ($1, $2, 'cpp', 'x', $3, $4, $5, $6, now())`,
			userID, f.problemID, status, score, f.contestID,
			f.begin.Add(time.Duration(minutes)*time.Minute))
		Expect(err).NotTo(HaveOccurred())
	}

	rebuild := func(ctx context.Context, f fixture, userID string) {
		tx, err := integrationDB.Pool.BeginTxx(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(contestpg.RebuildCell(ctx, tx, f.contestID, userID, f.problemID)).To(Succeed())
		Expect(tx.Commit()).To(Succeed())
	}

	rowFor := func(board *contestdomain.Rankboard, username string) contestdomain.RankRow {
		for _, row := range board.Rows {
			if row.Username == username {
				return row
			}
		}
		Fail(fmt.Sprintf("username %q is not on the board", username))
		return contestdomain.RankRow{}
	}

	DescribeTable("persists additional scoring formats and rebuilds their standings", func(ctx SpecContext, format string, expected int) {
		f := build(ctx, format, nil)
		submit(ctx, f, f.alice, "Wrong Answer", 0, 10)
		submit(ctx, f, f.alice, "Accepted", 100, 30)
		rebuild(ctx, f, f.alice)
		board, err := store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(board.Format).To(Equal(format))
		Expect(rowFor(board, "alice").Score).To(Equal(expected))
		Expect(rowFor(board, "alice").Solved).To(Equal(1))
	},
		Entry("Leduo discounts the second submission", contestdomain.FormatLeduo, 95),
		Entry("CF applies time decay and wrong submission penalty", contestdomain.FormatCF, 46),
	)

	It("writes ICPC penalty and ranks the faster solver first", func(ctx SpecContext) {
		f := build(ctx, contestdomain.FormatICPC, nil)
		submit(ctx, f, f.alice, "Wrong Answer", 0, 10)
		submit(ctx, f, f.alice, "Accepted", 100, 40)
		submit(ctx, f, f.bob, "Accepted", 100, 30)
		rebuild(ctx, f, f.alice)
		rebuild(ctx, f, f.bob)

		board, err := store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(board.Format).To(Equal(contestdomain.FormatICPC))
		Expect(board.Problems).To(HaveLen(1))
		Expect(board.Problems[0].Label).To(Equal("A"))

		alice := rowFor(board, "alice")
		Expect(alice.Solved).To(Equal(1))
		Expect(alice.Cells[0].Attempts).To(Equal(2))
		// 40 minutes to solve plus one rejected run at 20 minutes.
		Expect(alice.Penalty).To(Equal((40 + 20) * 60))

		bob := rowFor(board, "bob")
		Expect(bob.Penalty).To(Equal(30 * 60))
		Expect(bob.Rank).To(Equal(1))
		Expect(alice.Rank).To(Equal(2))
		// Whoever solved first is highlighted as the first solver.
		Expect(board.FirstSolvers[f.problemID]).To(Equal(f.bob))
	})

	It("keeps the pinned statement when practice visibility and metadata change", func(ctx SpecContext) {
		f := build(ctx, contestdomain.FormatICPC, nil)
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET visibility = 'draft', statement_md = '# Secret',
			        source = 'jury', time_limit_ms = 2500, memory_limit_kb = 131072
			 WHERE id = $1`, f.problemID)
		Expect(err).NotTo(HaveOccurred())

		detail, err := store.Problem(ctx, f.contestID, f.problemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(detail.Visibility).To(Equal("draft"))
		Expect(detail.StatementMD).To(Equal("Fixture statement"))
		Expect(detail.TimeLimitMs).To(Equal(1000))
		Expect(detail.MemoryLimitKB).To(Equal(262144))
		Expect(detail.Version).To(Equal(1))

		_, err = store.Problem(ctx, f.contestID, "00000000-0000-0000-0000-000000000000")
		Expect(err).To(MatchError(contestdomain.ErrProblemNotInContest))
	})

	It("defaults stored contests to icpc and rejects unsupported acm names", func(ctx SpecContext) {
		f := build(ctx, contestdomain.FormatCF, nil)
		var rule string
		Expect(integrationDB.Pool.QueryRowContext(ctx, `UPDATE contests SET rule = DEFAULT WHERE id = $1 RETURNING rule`, f.contestID).Scan(&rule)).To(Succeed())
		Expect(rule).To(Equal(contestdomain.FormatICPC))
		_, err := integrationDB.Pool.ExecContext(ctx, `UPDATE contests SET rule = 'acm' WHERE id = $1`, f.contestID)
		Expect(err).To(HaveOccurred())
	})

	It("keeps the post-freeze solve out of the public board but not the jury one", func(ctx SpecContext) {
		freeze := time.Now().Add(-90 * time.Minute).Truncate(time.Second)
		f := build(ctx, contestdomain.FormatICPC, &freeze)
		// 10 minutes in is before the freeze; 100 minutes in is after it.
		submit(ctx, f, f.alice, "Wrong Answer", 0, 10)
		submit(ctx, f, f.alice, "Accepted", 100, 100)
		rebuild(ctx, f, f.alice)

		public, err := store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		row := rowFor(public, "alice")
		Expect(row.Solved).To(Equal(0))
		Expect(row.Cells[0].PendingCount).To(Equal(1))
		Expect(row.HasPending).To(BeTrue())

		jury, err := store.Rankboard(ctx, f.contestID, true)
		Expect(err).NotTo(HaveOccurred())
		juryRow := rowFor(jury, "alice")
		Expect(juryRow.Solved).To(Equal(1))
		Expect(juryRow.Penalty).To(Equal((100 + 20) * 60))
	})

	It("hides irrelevant pending flags and restores them after a pre-freeze AC is overturned", func(ctx SpecContext) {
		freeze := time.Now().Add(-90 * time.Minute).Truncate(time.Second)
		f := build(ctx, contestdomain.FormatICPC, &freeze)
		submit(ctx, f, f.alice, "Accepted", 100, 10)
		submit(ctx, f, f.alice, "Accepted", 100, 100)
		rebuild(ctx, f, f.alice)
		public, err := store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(rowFor(public, "alice").HasPending).To(BeFalse())
		for _, row := range contestdto.FromRankboard(public).Rows {
			if row.UserID == f.alice {
				Expect(row.Cells[0].PendingCount).To(BeZero())
				Expect(row.Cells[0].SolvedAt).NotTo(BeNil())
			}
		}
		_, err = integrationDB.Pool.ExecContext(ctx, `UPDATE submissions SET status = 'Wrong Answer', score = 0 WHERE contest_id = $1 AND submitted_at < $2`, f.contestID, freeze)
		Expect(err).NotTo(HaveOccurred())
		rebuild(ctx, f, f.alice)
		public, err = store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(rowFor(public, "alice").HasPending).To(BeTrue())
		for _, row := range contestdto.FromRankboard(public).Rows {
			if row.UserID == f.alice {
				Expect(row.Cells[0].PendingCount).To(Equal(1))
				Expect(row.Cells[0].SolvedAt).To(BeNil())
				Expect(row.Cells[0].Score).To(BeZero())
			}
		}
	})

	It("stores the IOI best score and the OI final score", func(ctx SpecContext) {
		f := build(ctx, contestdomain.FormatIOI, nil)
		submit(ctx, f, f.alice, "Wrong Answer", 80, 10)
		submit(ctx, f, f.alice, "Wrong Answer", 30, 20)
		rebuild(ctx, f, f.alice)

		board, err := store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(rowFor(board, "alice").Score).To(Equal(80))

		// Changing the rule re-scores the whole board in the same transaction.
		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE contests SET rule = 'oi' WHERE id = $1`, f.contestID)
		Expect(err).NotTo(HaveOccurred())
		rebuild(ctx, f, f.alice)

		board, err = store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(board.Format).To(Equal(contestdomain.FormatOI))
		Expect(rowFor(board, "alice").Score).To(Equal(30))
	})

	It("converges on the same cell when a verdict is replayed or changed", func(ctx SpecContext) {
		f := build(ctx, contestdomain.FormatICPC, nil)
		submit(ctx, f, f.alice, "Wrong Answer", 0, 10)
		submit(ctx, f, f.alice, "Accepted", 100, 40)

		// Replaying the rebuild must not double-count anything.
		for i := 0; i < 3; i++ {
			rebuild(ctx, f, f.alice)
		}
		board, err := store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(rowFor(board, "alice").Penalty).To(Equal((40 + 20) * 60))

		// A rejudge that overturns the accepted run must take the solve away.
		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE submissions SET status = 'Wrong Answer', score = 0
			 WHERE contest_id = $1 AND status = 'Accepted'`, f.contestID)
		Expect(err).NotTo(HaveOccurred())
		rebuild(ctx, f, f.alice)

		board, err = store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		row := rowFor(board, "alice")
		Expect(row.Solved).To(Equal(0))
		Expect(row.Penalty).To(Equal(0))
		Expect(row.Cells[0].Attempts).To(Equal(2))
	})

	It("keeps problem progress and last submissions scoped to the individual contestant", func(ctx SpecContext) {
		f := build(ctx, contestdomain.FormatICPC, nil)
		before, err := store.ProblemStatuses(ctx, f.contestID, f.alice)
		Expect(err).NotTo(HaveOccurred())
		Expect(before).To(BeEmpty())
		submit(ctx, f, f.alice, "Wrong Answer", 0, 10)
		submit(ctx, f, f.bob, "Accepted", 100, 12)
		alice, err := store.ProblemStatuses(ctx, f.contestID, f.alice)
		Expect(err).NotTo(HaveOccurred())
		bob, err := store.ProblemStatuses(ctx, f.contestID, f.bob)
		Expect(err).NotTo(HaveOccurred())
		Expect(alice[f.problemID].UserStatus).To(Equal("attempted"))
		Expect(bob[f.problemID].UserStatus).To(Equal("solved"))
		Expect(alice[f.problemID].LastSubmissionID).NotTo(BeEmpty())
		Expect(alice[f.problemID].LastSubmissionID).NotTo(Equal(bob[f.problemID].LastSubmissionID))
		submit(ctx, f, f.alice, "Accepted", 100, 20)
		after, err := store.ProblemStatuses(ctx, f.contestID, f.alice)
		Expect(err).NotTo(HaveOccurred())
		Expect(after[f.problemID].UserStatus).To(Equal("solved"))
		Expect(after[f.problemID].LastSubmissionID).NotTo(Equal(alice[f.problemID].LastSubmissionID))
	})

	It("recomputes the whole board when the penalty setting changes", func(ctx SpecContext) {
		f := build(ctx, contestdomain.FormatICPC, nil)
		submit(ctx, f, f.alice, "Wrong Answer", 0, 10)
		submit(ctx, f, f.alice, "Accepted", 100, 40)
		rebuild(ctx, f, f.alice)

		current, err := store.Get(ctx, f.contestID)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Update(ownerContext(ctx, f), f.contestID, &contestdomain.PersistInput{
			Title: current.Title, Rule: contestdomain.FormatICPC,
			BeginAt: current.BeginAt, EndAt: current.EndAt,
			PenaltyMinutes: 5, PenalizeCompileError: true,
			Feedback: contestdomain.FeedbackFull, Visibility: "public", RankboardVisible: true,
		})
		Expect(err).NotTo(HaveOccurred())

		board, err := store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		// The stored cell was rebuilt with the new 5 minute penalty.
		Expect(rowFor(board, "alice").Penalty).To(Equal((40 + 5) * 60))
	})

	It("round-trips contest staff and the clarification channel", func(ctx SpecContext) {
		f := build(ctx, contestdomain.FormatICPC, nil)

		added, err := store.AddStaff(ownerContext(ctx, f), f.contestID, "bob", contestdomain.StaffJury)
		Expect(err).NotTo(HaveOccurred())
		Expect(added.UserID).To(Equal(f.bob))
		role, err := store.StaffRole(ctx, f.contestID, f.bob)
		Expect(err).NotTo(HaveOccurred())
		Expect(role).To(Equal(contestdomain.StaffJury))

		_, err = store.AddStaff(ownerContext(ctx, f), f.contestID, "nobody", contestdomain.StaffJury)
		Expect(err).To(MatchError(contestdomain.ErrInvalidInput))

		question, err := store.CreateClarification(ctx, contestdomain.ClarificationInput{
			ContestID: f.contestID, ProblemID: &f.problemID, AuthorID: f.alice,
			Body: "样例有保证吗?",
		})
		Expect(err).NotTo(HaveOccurred())

		// A jury reply with no recipient inherits the asker rather than
		// becoming a broadcast.
		reply, err := store.CreateClarification(ctx, contestdomain.ClarificationInput{
			ContestID: f.contestID, ParentID: &question.ID, AuthorID: f.bob,
			FromJury: true, Body: "没有保证。",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(reply.RecipientID).NotTo(BeNil())
		Expect(*reply.RecipientID).To(Equal(f.alice))
		Expect(reply.IsAnnouncement()).To(BeFalse())

		// The asker sees their thread with the answer nested under it.
		threads, err := store.ListClarifications(ctx, f.contestID, contestdomain.Viewer{UserID: f.alice})
		Expect(err).NotTo(HaveOccurred())
		Expect(threads).To(HaveLen(1))
		Expect(threads[0].Answered).To(BeTrue())
		Expect(threads[0].Replies).To(HaveLen(1))

		// An unrelated contestant sees nothing until an announcement goes out.
		outsider, err := store.ListClarifications(ctx, f.contestID, contestdomain.Viewer{UserID: "00000000-0000-0000-0000-000000000000"})
		Expect(err).NotTo(HaveOccurred())
		Expect(outsider).To(BeEmpty())

		_, err = store.CreateClarification(ctx, contestdomain.ClarificationInput{
			ContestID: f.contestID, AuthorID: f.bob, FromJury: true,
			Subject: "公告", Body: "数据已更新。",
		})
		Expect(err).NotTo(HaveOccurred())
		outsider, err = store.ListClarifications(ctx, f.contestID, contestdomain.Viewer{UserID: "00000000-0000-0000-0000-000000000000"})
		Expect(err).NotTo(HaveOccurred())
		Expect(outsider).To(HaveLen(1))
		Expect(outsider[0].IsAnnouncement()).To(BeTrue())
	})

	It("rejects clarification problem IDs outside the contest", func(ctx SpecContext) {
		f := build(ctx, contestdomain.FormatICPC, nil)
		_, err := store.AddStaff(ownerContext(ctx, f), f.contestID, "bob", contestdomain.StaffJury)
		Expect(err).NotTo(HaveOccurred())
		var outsideProblemID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, owner_id) VALUES ('Outside', 'private', $1) RETURNING id`, f.alice).
			Scan(&outsideProblemID)).To(Succeed())

		service := contestapp.NewService(store, nil)
		_, err = service.Ask(ctx, contestdomain.ClarificationInput{
			ContestID: f.contestID, ProblemID: &outsideProblemID,
			AuthorID: f.alice, Body: "Can I use this problem?",
		})
		Expect(err).To(MatchError(contestdomain.ErrInvalidInput))

		_, err = service.Reply(ctx, contestdomain.ClarificationInput{
			ContestID: f.contestID, ProblemID: &outsideProblemID,
			AuthorID: f.bob, FromJury: true, Body: "No.",
		})
		Expect(err).To(MatchError(contestdomain.ErrInvalidInput))

		var count int
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT count(*)::int FROM clarifications WHERE contest_id = $1`, f.contestID).
			Scan(&count)).To(Succeed())
		Expect(count).To(BeZero())
	})
})

// integrationDB is non-nil only when TEST_DATABASE_URL is configured. The
// scoring specs that need real SQL skip without it; the pure ones always run.
func TestContest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Contest Suite")
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
