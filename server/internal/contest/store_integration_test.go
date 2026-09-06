package contest_test

import (
	"context"
	"fmt"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"time"

	contestapp "github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These specs exercise the scoring SQL against a real PostgreSQL instance:
// the pure ScoreCell tests prove the arithmetic, these prove that the queries
// around it read and write the columns the migrations actually created.
var _ = Describe("Contest scoring against PostgreSQL", Ordered, func() {
	var store *contestapp.ContestStore

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `
			TRUNCATE contest_submission_cells, contest_participants, contest_access,
				contest_problems, clarifications, contests, submissions, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = contestapp.NewContestStore(integrationDB)
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
		return domain.WithScope(ctx, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: f.owner})
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
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO contests (title, rule, begin_at, end_at, freeze_at, penalty_minutes,owner_id,created_by)
			 VALUES ('Round', $1, $2, $3, $4, 20,$5,$5) RETURNING id`,
			rule, result.begin, result.begin.Add(5*time.Hour), freeze, result.owner).
			Scan(&result.contestID)).To(Succeed())

		Expect(store.SetProblems(ownerContext(ctx, result), result.contestID, []contestapp.ProblemEntry{
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
		Expect(contestapp.RebuildCell(ctx, tx, f.contestID, userID, f.problemID)).To(Succeed())
		Expect(tx.Commit()).To(Succeed())
	}

	rowFor := func(board *contestapp.Rankboard, username string) contestapp.RankRow {
		for _, row := range board.Rows {
			if row.Username == username {
				return row
			}
		}
		Fail(fmt.Sprintf("username %q is not on the board", username))
		return contestapp.RankRow{}
	}

	It("writes ICPC penalty and ranks the faster solver first", func(ctx SpecContext) {
		f := build(ctx, contestapp.FormatICPC, nil)
		submit(ctx, f, f.alice, "Wrong Answer", 0, 10)
		submit(ctx, f, f.alice, "Accepted", 100, 40)
		submit(ctx, f, f.bob, "Accepted", 100, 30)
		rebuild(ctx, f, f.alice)
		rebuild(ctx, f, f.bob)

		board, err := store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(board.Format).To(Equal(contestapp.FormatICPC))
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

	It("loads a full unpublished problem only through its contest relation", func(ctx SpecContext) {
		f := build(ctx, contestapp.FormatICPC, nil)
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET visibility = 'draft', statement_md = '# Secret',
			        source = 'jury', time_limit_ms = 2500, memory_limit_kb = 131072
			 WHERE id = $1`, f.problemID)
		Expect(err).NotTo(HaveOccurred())

		detail, err := store.Problem(ctx, f.contestID, f.problemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(detail.Visibility).To(Equal("draft"))
		Expect(detail.StatementMD).To(Equal("# Secret"))
		Expect(detail.TimeLimitMs).To(Equal(2500))
		Expect(detail.MemoryLimitKB).To(Equal(131072))

		_, err = store.Problem(ctx, f.contestID, "00000000-0000-0000-0000-000000000000")
		Expect(err).To(MatchError(contestapp.ErrProblemNotInContest))
	})

	It("keeps the post-freeze solve out of the public board but not the jury one", func(ctx SpecContext) {
		freeze := time.Now().Add(-90 * time.Minute).Truncate(time.Second)
		f := build(ctx, contestapp.FormatICPC, &freeze)
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

	It("stores the IOI best score and the OI final score", func(ctx SpecContext) {
		f := build(ctx, contestapp.FormatIOI, nil)
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
		Expect(board.Format).To(Equal(contestapp.FormatOI))
		Expect(rowFor(board, "alice").Score).To(Equal(30))
	})

	It("converges on the same cell when a verdict is replayed or changed", func(ctx SpecContext) {
		f := build(ctx, contestapp.FormatICPC, nil)
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

	It("recomputes the whole board when the penalty setting changes", func(ctx SpecContext) {
		f := build(ctx, contestapp.FormatICPC, nil)
		submit(ctx, f, f.alice, "Wrong Answer", 0, 10)
		submit(ctx, f, f.alice, "Accepted", 100, 40)
		rebuild(ctx, f, f.alice)

		current, err := store.Get(ctx, f.contestID)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Update(ownerContext(ctx, f), f.contestID, &contestapp.PersistInput{
			Title: current.Title, Rule: contestapp.FormatICPC,
			BeginAt: current.BeginAt, EndAt: current.EndAt,
			PenaltyMinutes: 5, PenalizeCompileError: true,
			Feedback: contestapp.FeedbackFull, Visibility: "public", RankboardVisible: true,
		})
		Expect(err).NotTo(HaveOccurred())

		board, err := store.Rankboard(ctx, f.contestID, false)
		Expect(err).NotTo(HaveOccurred())
		// The stored cell was rebuilt with the new 5 minute penalty.
		Expect(rowFor(board, "alice").Penalty).To(Equal((40 + 5) * 60))
	})

	It("round-trips contest staff and the clarification channel", func(ctx SpecContext) {
		f := build(ctx, contestapp.FormatICPC, nil)

		added, err := store.AddStaff(ownerContext(ctx, f), f.contestID, "bob", contestapp.StaffJury)
		Expect(err).NotTo(HaveOccurred())
		Expect(added.UserID).To(Equal(f.bob))
		role, err := store.StaffRole(ctx, f.contestID, f.bob)
		Expect(err).NotTo(HaveOccurred())
		Expect(role).To(Equal(contestapp.StaffJury))

		_, err = store.AddStaff(ownerContext(ctx, f), f.contestID, "nobody", contestapp.StaffJury)
		Expect(err).To(MatchError(contestapp.ErrInvalidInput))

		question, err := store.CreateClarification(ctx, contestapp.ClarificationInput{
			ContestID: f.contestID, ProblemID: &f.problemID, AuthorID: f.alice,
			Body: "样例有保证吗?",
		})
		Expect(err).NotTo(HaveOccurred())

		// A jury reply with no recipient inherits the asker rather than
		// becoming a broadcast.
		reply, err := store.CreateClarification(ctx, contestapp.ClarificationInput{
			ContestID: f.contestID, ParentID: &question.ID, AuthorID: f.bob,
			FromJury: true, Body: "没有保证。",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(reply.RecipientID).NotTo(BeNil())
		Expect(*reply.RecipientID).To(Equal(f.alice))
		Expect(reply.IsAnnouncement()).To(BeFalse())

		// The asker sees their thread with the answer nested under it.
		threads, err := store.ListClarifications(ctx, f.contestID,
			contestapp.Viewer{UserID: f.alice})
		Expect(err).NotTo(HaveOccurred())
		Expect(threads).To(HaveLen(1))
		Expect(threads[0].Answered).To(BeTrue())
		Expect(threads[0].Replies).To(HaveLen(1))

		// An unrelated contestant sees nothing until an announcement goes out.
		outsider, err := store.ListClarifications(ctx, f.contestID,
			contestapp.Viewer{UserID: "00000000-0000-0000-0000-000000000000"})
		Expect(err).NotTo(HaveOccurred())
		Expect(outsider).To(BeEmpty())

		_, err = store.CreateClarification(ctx, contestapp.ClarificationInput{
			ContestID: f.contestID, AuthorID: f.bob, FromJury: true,
			Subject: "公告", Body: "数据已更新。",
		})
		Expect(err).NotTo(HaveOccurred())
		outsider, err = store.ListClarifications(ctx, f.contestID,
			contestapp.Viewer{UserID: "00000000-0000-0000-0000-000000000000"})
		Expect(err).NotTo(HaveOccurred())
		Expect(outsider).To(HaveLen(1))
		Expect(outsider[0].IsAnnouncement()).To(BeTrue())
	})

	It("rejects clarification problem IDs outside the contest", func(ctx SpecContext) {
		f := build(ctx, contestapp.FormatICPC, nil)
		_, err := store.AddStaff(ownerContext(ctx, f), f.contestID, "bob", contestapp.StaffJury)
		Expect(err).NotTo(HaveOccurred())
		var outsideProblemID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, owner_id) VALUES ('Outside', 'private', $1) RETURNING id`, f.alice).
			Scan(&outsideProblemID)).To(Succeed())

		service := contestapp.NewService(store, nil)
		_, err = service.Ask(ctx, contestapp.ClarificationInput{
			ContestID: f.contestID, ProblemID: &outsideProblemID,
			AuthorID: f.alice, Body: "Can I use this problem?",
		})
		Expect(err).To(MatchError(contestapp.ErrInvalidInput))

		_, err = service.Reply(ctx, contestapp.ClarificationInput{
			ContestID: f.contestID, ProblemID: &outsideProblemID,
			AuthorID: f.bob, FromJury: true, Body: "No.",
		})
		Expect(err).To(MatchError(contestapp.ErrInvalidInput))

		var count int
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT count(*)::int FROM clarifications WHERE contest_id = $1`, f.contestID).
			Scan(&count)).To(Succeed())
		Expect(count).To(BeZero())
	})
})
