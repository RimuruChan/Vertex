package submission_test

import (
	"context"
	"time"

	contestapp "github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	submissionapp "github.com/RimuruChan/Vertex/server/internal/submission"
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

// Rejudging derives its progress from the submissions themselves, so the
// generation fencing only proves out against real rows.
var _ = Describe("Rejudging against PostgreSQL", func() {
	var store *submissionapp.SubmissionStore
	var userID, problemID, otherProblemID string

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `
			TRUNCATE rejudging_submissions, rejudgings, submission_cases, judge_jobs,
				submissions, problem_testdata, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = submissionapp.NewSubmissionStore(integrationDB)

		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash,role) VALUES ('u', 'u@t.local', 'x','admin')
			 RETURNING id`).Scan(&userID)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, owner_id) VALUES ('A', 'public', $1) RETURNING id`, userID).
			Scan(&problemID)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility, owner_id) VALUES ('B', 'public', $1) RETURNING id`, userID).
			Scan(&otherProblemID)).To(Succeed())
		Expect(dbtest.PublishedProblems(ctx, integrationDB)).To(Succeed())
	})

	judged := func(ctx context.Context, problem, status string) string {
		var id string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO submissions (user_id, problem_id, language, source_code, status, judged_at)
			 VALUES ($1, $2, 'cpp', 'x', $3, now()) RETURNING id`,
			userID, problem, status).Scan(&id)).To(Succeed())
		return id
	}

	It("loads the dedicated progress projection", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: userID})
		var id string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO submissions
			   (user_id, problem_id, language, source_code, status, score,
			    total_time_ms, peak_memory_kb, compile_result, case_results,
			    judged_cases, total_cases)
			 VALUES ($1, $2, 'cpp', 'secret source', 'Judging', 25,
			         17, 2048, 'compile output',
			         '[{"caseIndex":1,"verdict":"Accepted","timeMs":17,"memoryKb":2048}]'::jsonb,
			         1, 4)
			 RETURNING id`, userID, problemID).Scan(&id)).To(Succeed())

		progress, err := store.Progress(ctx, id, submissionapp.Viewer{UserID: userID})
		Expect(err).NotTo(HaveOccurred())
		Expect(progress.ID).To(Equal(id))
		Expect(progress.UserID).To(Equal(userID))
		Expect(progress.Status).To(Equal(submissionapp.StatusJudging))
		Expect(progress.JudgedCases).To(Equal(1))
		Expect(progress.TotalCases).To(Equal(4))
		Expect(progress.CaseResults).To(HaveLen(1))
		Expect(progress.CaseResults[0].MemoryKb).To(Equal(2048))
	})

	It("refuses an unrestricted batch", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: userID})
		judged(ctx, problemID, "Accepted")
		_, err := store.CreateRejudging(ctx, submissionapp.RejudgeSelector{}, userID)
		Expect(err).To(MatchError(submissionapp.ErrRejudgeEmpty))
	})

	It("reports an empty match instead of creating a batch", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: userID})
		_, err := store.CreateRejudging(ctx,
			submissionapp.RejudgeSelector{ProblemID: problemID}, userID)
		Expect(err).To(MatchError(submissionapp.ErrRejudgeEmpty))
	})

	It("expands a selector, queues jobs and tracks progress", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: userID})
		first := judged(ctx, problemID, "Accepted")
		second := judged(ctx, problemID, "Wrong Answer")
		// A submission for another problem must stay out of the batch.
		untouched := judged(ctx, otherProblemID, "Accepted")
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET submission_count = 2, accepted_count = 1, solved_user_count = 1
			 WHERE id = $1`, problemID)
		Expect(err).NotTo(HaveOccurred())

		batch, err := store.CreateRejudging(ctx, submissionapp.RejudgeSelector{
			ProblemID: problemID, Reason: "fixed the checker",
		}, userID)
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.TotalCount).To(Equal(2))
		Expect(batch.State).To(Equal(submissionapp.RejudgingRunning))

		// Both members were reset and re-queued; the outsider was not.
		var pending int
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT count(*)::int FROM submissions WHERE status = 'Pending'`).Scan(&pending)).To(Succeed())
		Expect(pending).To(Equal(2))
		var submissionCount, acceptedCount, solvedCount int
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT submission_count, accepted_count, solved_user_count
			 FROM problems WHERE id = $1`, problemID).
			Scan(&submissionCount, &acceptedCount, &solvedCount)).To(Succeed())
		Expect(submissionCount).To(BeZero())
		Expect(acceptedCount).To(BeZero())
		Expect(solvedCount).To(BeZero())
		var outsiderStatus string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT status FROM submissions WHERE id = $1`, untouched).Scan(&outsiderStatus)).To(Succeed())
		Expect(outsiderStatus).To(Equal("Accepted"))

		var queued int
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT count(*)::int FROM judge_jobs WHERE state = 'queued'`).Scan(&queued)).To(Succeed())
		Expect(queued).To(Equal(2))

		// Nothing has been re-judged yet.
		progress, err := store.Rejudging(ctx, batch.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(progress.DoneCount).To(Equal(0))
		Expect(progress.State).To(Equal(submissionapp.RejudgingRunning))

		// One member comes back with a different verdict.
		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE submissions SET status = 'Time Limit Exceeded', judged_at = now() WHERE id = $1`, first)
		Expect(err).NotTo(HaveOccurred())
		progress, err = store.Rejudging(ctx, batch.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(progress.DoneCount).To(Equal(1))
		Expect(progress.ChangedCount).To(Equal(1))

		// The second keeps its old verdict, which counts as done but unchanged.
		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE submissions SET status = 'Wrong Answer', judged_at = now() WHERE id = $1`, second)
		Expect(err).NotTo(HaveOccurred())
		settled, err := store.Rejudging(ctx, batch.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(settled.DoneCount).To(Equal(2))
		Expect(settled.ChangedCount).To(Equal(1))
		// Reaching the total settles the batch without a worker reporting it.
		Expect(settled.State).To(Equal(submissionapp.RejudgingFinished))
		Expect(settled.FinishedAt).NotTo(BeNil())

		changes, err := store.RejudgingChanges(ctx, batch.ID, 100)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		Expect(changes[0].PriorStatus).To(Equal("Accepted"))
		Expect(changes[0].Status).To(Equal("Time Limit Exceeded"))
		Expect(changes[0].Judged).To(BeTrue())
	})

	It("stops counting a member that a later batch took over", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: userID})
		target := judged(ctx, problemID, "Accepted")
		first, err := store.CreateRejudging(ctx,
			submissionapp.RejudgeSelector{SubmissionIDs: []string{target}}, userID)
		Expect(err).NotTo(HaveOccurred())

		// A later single rejudge may take over work that this batch has not
		// finished yet; generation fencing makes the first batch stop waiting.
		Expect(store.Rejudge(ctx, target)).To(Succeed())

		progress, err := store.Rejudging(ctx, first.ID)
		Expect(err).NotTo(HaveOccurred())
		// The first batch is no longer responsible for it, so it is complete.
		Expect(progress.DoneCount).To(Equal(1))
		Expect(progress.State).To(Equal(submissionapp.RejudgingFinished))
	})

	It("cancels queued work by restoring the complete prior result", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: userID})
		target := judged(ctx, problemID, "Accepted")
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE submissions
			 SET score = 100, total_time_ms = 17, peak_memory_kb = 2048,
			     compile_result = 'old compile output',
			     case_results = '[{"caseIndex":1,"verdict":"Accepted","timeMs":17,"memoryKb":2048,"checkerOutput":"ok"}]'::jsonb,
			     judged_cases = 1, total_cases = 1
			 WHERE id = $1`, target)
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO submission_cases
			   (submission_id, case_index, verdict, time_ms, memory_kb, checker_output)
			 VALUES ($1, 1, 'Accepted', 17, 2048, 'ok')`, target)
		Expect(err).NotTo(HaveOccurred())

		batch, err := store.CreateRejudging(ctx,
			submissionapp.RejudgeSelector{ProblemID: problemID}, userID)
		Expect(err).NotTo(HaveOccurred())

		Expect(store.CancelRejudging(ctx, batch.ID)).To(Succeed())
		var cancelled int
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT count(*)::int FROM judge_jobs WHERE state = 'cancelled'`).Scan(&cancelled)).To(Succeed())
		Expect(cancelled).To(Equal(1))
		var (
			status, compileResult, caseResults string
			score, totalTime, peakMemory       int
			judgedCases, totalCases            int
			judgedAt                           *time.Time
		)
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT status, score, total_time_ms, peak_memory_kb, compile_result,
			        case_results::text, judged_cases, total_cases, judged_at
			 FROM submissions WHERE id = $1`, target).Scan(
			&status, &score, &totalTime, &peakMemory, &compileResult,
			&caseResults, &judgedCases, &totalCases, &judgedAt,
		)).To(Succeed())
		Expect(status).To(Equal("Accepted"))
		Expect(score).To(Equal(100))
		Expect(totalTime).To(Equal(17))
		Expect(peakMemory).To(Equal(2048))
		Expect(compileResult).To(Equal("old compile output"))
		Expect(caseResults).To(ContainSubstring(`"checkerOutput": "ok"`))
		Expect(judgedCases).To(Equal(1))
		Expect(totalCases).To(Equal(1))
		Expect(judgedAt).NotTo(BeNil())

		var restoredCases int
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT count(*)::int FROM submission_cases
			 WHERE submission_id = $1 AND verdict = 'Accepted'
			   AND time_ms = 17 AND memory_kb = 2048 AND checker_output = 'ok'`, target).
			Scan(&restoredCases)).To(Succeed())
		Expect(restoredCases).To(Equal(1))
		var submissionCount, acceptedCount, solvedCount int
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT submission_count, accepted_count, solved_user_count
			 FROM problems WHERE id = $1`, problemID).Scan(
			&submissionCount, &acceptedCount, &solvedCount,
		)).To(Succeed())
		Expect(submissionCount).To(Equal(1))
		Expect(acceptedCount).To(Equal(1))
		Expect(solvedCount).To(Equal(1))

		var orphanedPending int
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT count(*)::int FROM submissions AS sub
			 WHERE sub.status IN ('Pending', 'Judging')
			   AND NOT EXISTS (
			     SELECT 1 FROM judge_jobs AS job
			     WHERE job.submission_id = sub.id
			       AND job.generation = sub.judge_generation
			       AND job.state IN ('queued', 'running'))`).Scan(&orphanedPending)).To(Succeed())
		Expect(orphanedPending).To(BeZero())

		settled, err := store.Rejudging(ctx, batch.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(settled.State).To(Equal(submissionapp.RejudgingCancelled))
		// A cancelled batch cannot be cancelled twice.
		Expect(store.CancelRejudging(ctx, batch.ID)).To(MatchError(submissionapp.ErrRejudgeClosed))
	})

	It("lists batches scoped to a contest", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: userID})
		judged(ctx, problemID, "Accepted")
		_, err := store.CreateRejudging(ctx,
			submissionapp.RejudgeSelector{ProblemID: problemID}, userID)
		Expect(err).NotTo(HaveOccurred())

		all, err := store.ListRejudgings(ctx, "", 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(all).To(HaveLen(1))

		// A contest filter that matches nothing returns an empty list, not an error.
		scoped, err := store.ListRejudgings(ctx, "00000000-0000-0000-0000-000000000000", 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(scoped).To(BeEmpty())
	})

	It("reports a missing batch", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: userID})
		_, err := store.Rejudging(ctx, "00000000-0000-0000-0000-000000000000")
		Expect(err).To(MatchError(submissionapp.ErrRejudgeNotFound))
	})
})

var _ = Describe("Submission visibility against PostgreSQL", func() {
	type fixture struct {
		users       map[string]string
		problems    map[string]string
		submissions map[string]string
		contests    map[string]string
	}

	var store *submissionapp.SubmissionStore
	var f fixture

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `
			TRUNCATE rejudging_submissions, rejudgings, submission_cases, judge_jobs,
				submissions, contest_participants, contest_access, contest_problems,
				contests, problem_testdata, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = submissionapp.NewSubmissionStore(integrationDB)
		f = fixture{
			users:       map[string]string{},
			problems:    map[string]string{},
			submissions: map[string]string{},
			contests:    map[string]string{},
		}

		for _, name := range []string{"owner", "author", "creator", "participant", "private-participant", "staff", "outsider"} {
			var id string
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO users (username, email, password_hash)
				 VALUES ($1, $1 || '@test.local', 'x') RETURNING id`, name).
				Scan(&id)).To(Succeed())
			f.users[name] = id
		}
		Expect(dbtest.OfficialMembers(ctx, integrationDB)).To(Succeed())

		problems := map[string]string{}
		for _, visibility := range []string{"public", "private", "draft"} {
			var id string
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO problems (title, visibility, author_id, owner_id)
				 VALUES ($1, $1, $2, $2) RETURNING id`, visibility, f.users["author"]).
				Scan(&id)).To(Succeed())
			problems[visibility] = id
		}
		f.problems = problems
		Expect(dbtest.PublishedProblems(ctx, integrationDB)).To(Succeed())

		for _, visibility := range []string{"public", "password", "private"} {
			var id string
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO contests (title, begin_at, end_at, visibility, created_by,owner_id)
				 VALUES ($1, now() - interval '1 hour', now() + interval '1 hour', $1, $2,$2)
				 RETURNING id`, visibility, f.users["creator"]).
				Scan(&id)).To(Succeed())
			f.contests[visibility] = id
		}
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_participants (contest_id, user_id) VALUES ($1, $2), ($3, $4)`,
			f.contests["password"], f.users["participant"],
			f.contests["private"], f.users["private-participant"])
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_access (contest_id, user_id, role) VALUES ($1, $2, 'observer')`,
			f.contests["private"], f.users["staff"])
		Expect(err).NotTo(HaveOccurred())

		insertSubmission := func(name, problemID string, contestID *string) {
			var id string
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO submissions (user_id, problem_id, language, source_code, status, contest_id,problem_version)
				 VALUES ($1, $2, 'cpp', 'secret', 'Accepted', $3,1) RETURNING id`,
				f.users["owner"], problemID, contestID).Scan(&id)).To(Succeed())
			f.submissions[name] = id
		}
		insertSubmission("practice-public", problems["public"], nil)
		insertSubmission("practice-private", problems["private"], nil)
		insertSubmission("practice-draft", problems["draft"], nil)
		publicContest := f.contests["public"]
		passwordContest := f.contests["password"]
		privateContest := f.contests["private"]
		insertSubmission("contest-public", problems["private"], &publicContest)
		insertSubmission("contest-password", problems["private"], &passwordContest)
		insertSubmission("contest-private", problems["private"], &privateContest)
	})

	visibleNames := func(ctx context.Context, viewer submissionapp.Viewer) ([]string, int) {
		items, total, err := store.List(ctx, submissionapp.Filters{Limit: 100}, viewer)
		Expect(err).NotTo(HaveOccurred())
		namesByID := make(map[string]string, len(f.submissions))
		for name, id := range f.submissions {
			namesByID[id] = name
		}
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, namesByID[item.ID])
		}
		return names, total
	}

	It("filters list rows and totals before pagination", func(ctx SpecContext) {
		viewer := submissionapp.Viewer{UserID: f.users["outsider"]}
		names, total := visibleNames(ctx, viewer)
		Expect(names).To(ConsistOf("practice-public"))
		Expect(total).To(Equal(1))

		items, total, err := store.List(ctx, submissionapp.Filters{Limit: 1}, viewer)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(total).To(Equal(1))

		items, total, err = store.List(ctx, submissionapp.Filters{
			ContestID: f.contests["private"], Limit: 20,
		}, viewer)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		Expect(total).To(BeZero())
	})

	It("grants the established owner, author and contest-scoped readers", func(ctx SpecContext) {
		cases := []struct {
			viewer submissionapp.Viewer
			want   []string
		}{
			{submissionapp.Viewer{UserID: f.users["owner"]}, []string{
				"practice-public", "practice-private", "practice-draft", "contest-public", "contest-password", "contest-private",
			}},
			{submissionapp.Viewer{UserID: f.users["author"]}, []string{
				"practice-public", "practice-private", "practice-draft",
			}},
			{submissionapp.Viewer{UserID: f.users["participant"]}, []string{
				"practice-public",
			}},
			{submissionapp.Viewer{UserID: f.users["private-participant"]}, []string{
				"practice-public",
			}},
			{submissionapp.Viewer{UserID: f.users["staff"]}, []string{
				"practice-public", "contest-private",
			}},
			{submissionapp.Viewer{UserID: f.users["creator"]}, []string{
				"practice-public", "contest-public", "contest-password", "contest-private",
			}},
			{submissionapp.Viewer{UserID: f.users["outsider"], Admin: true}, []string{
				"practice-public",
			}},
		}
		for _, tc := range cases {
			names, total := visibleNames(ctx, tc.viewer)
			Expect(names).To(ConsistOf(tc.want))
			Expect(total).To(Equal(len(tc.want)))
		}
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE users SET role='admin' WHERE id=$1", f.users["outsider"])
		Expect(err).NotTo(HaveOccurred())
		names, total := visibleNames(ctx, submissionapp.Viewer{UserID: f.users["outsider"]})
		Expect(total).To(Equal(6))
		Expect(names).To(HaveLen(6))
	})

	It("uses the same not-found boundary for detail and progress", func(ctx SpecContext) {
		outsider := submissionapp.Viewer{UserID: f.users["outsider"]}
		_, err := store.Get(ctx, f.submissions["practice-private"], outsider)
		Expect(err).To(MatchError(submissionapp.ErrNotFound))
		_, err = store.Progress(ctx, f.submissions["practice-private"], outsider)
		Expect(err).To(MatchError(submissionapp.ErrNotFound))

		_, err = store.Get(ctx, f.submissions["contest-public"], outsider)
		Expect(err).To(MatchError(submissionapp.ErrNotFound))
		_, err = store.Progress(ctx, f.submissions["contest-public"], outsider)
		Expect(err).To(MatchError(submissionapp.ErrNotFound))

		privateParticipant := submissionapp.Viewer{UserID: f.users["private-participant"]}
		_, err = store.Get(ctx, f.submissions["contest-private"], privateParticipant)
		Expect(err).To(MatchError(submissionapp.ErrNotFound))
		_, err = store.Get(ctx, f.submissions["contest-private"], submissionapp.Viewer{UserID: f.users["staff"]})
		Expect(err).NotTo(HaveOccurred())
	})

	It("rechecks contest membership and problem scope in the create transaction", func(ctx SpecContext) {
		passwordContest := f.contests["password"]
		input := &submissionapp.Submission{
			UserID: f.users["participant"], ProblemID: f.problems["private"],
			Language: "cpp", SourceCode: "int main() {}", ContestID: &passwordContest,
		}

		_, err := store.Create(ctx, input)
		Expect(err).To(MatchError(contestapp.ErrProblemNotInContest))

		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_problems (contest_id, problem_id, label)
			 VALUES ($1, $2, 'A')`, passwordContest, f.problems["private"])
		Expect(err).NotTo(HaveOccurred())
		created, err := store.Create(ctx, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(created.ContestID).NotTo(BeNil())
		Expect(*created.ContestID).To(Equal(passwordContest))

		privateContest := f.contests["private"]
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_problems (contest_id, problem_id, label)
			 VALUES ($1, $2, 'A')`, privateContest, f.problems["private"])
		Expect(err).NotTo(HaveOccurred())
		input.UserID = f.users["private-participant"]
		input.ContestID = &privateContest
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET admission='restricted' WHERE id=$1", privateContest)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Create(ctx, input)
		Expect(err).To(MatchError(submissionapp.ErrNotFound))

		input.UserID = f.users["staff"]
		_, err = store.Create(ctx, input)
		Expect(err).To(MatchError(contestapp.ErrNotParticipant))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contest_access SET role='jury' WHERE contest_id=$1 AND user_id=$2", privateContest, input.UserID)
		Expect(err).NotTo(HaveOccurred())
		created, err = store.Create(ctx, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(created.UserID).To(Equal(f.users["staff"]))
	})

	It("reveals finished contest submissions only through the public board boundary", func(ctx SpecContext) {
		publicContest := f.contests["public"]
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE contests SET end_at = now() - interval '1 minute' WHERE id = $1`, publicContest)
		Expect(err).NotTo(HaveOccurred())

		// The contest problem is private, so an outsider still cannot learn its
		// title merely because the public contest has ended.
		outsider := submissionapp.Viewer{UserID: f.users["outsider"]}
		_, err = store.Get(ctx, f.submissions["contest-public"], outsider)
		Expect(err).To(MatchError(submissionapp.ErrNotFound))

		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_participants (contest_id, user_id) VALUES ($1, $2)`,
			publicContest, f.users["participant"])
		Expect(err).NotTo(HaveOccurred())
		participant := submissionapp.Viewer{UserID: f.users["participant"]}
		_, err = store.Get(ctx, f.submissions["contest-public"], participant)
		Expect(err).NotTo(HaveOccurred())

		// A freeze remains an information boundary after the scheduled end until
		// the jury explicitly reveals it.
		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE contests
			 SET freeze_at = now() - interval '30 minutes', unfreeze_at = NULL
			 WHERE id = $1`, publicContest)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Get(ctx, f.submissions["contest-public"], participant)
		Expect(err).To(MatchError(submissionapp.ErrNotFound))

		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE contests SET unfreeze_at = now() - interval '1 second' WHERE id = $1`, publicContest)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Get(ctx, f.submissions["contest-public"], participant)
		Expect(err).NotTo(HaveOccurred())
	})
})
