package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
	submissionstore "github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
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
	var store *submissionstore.Repository
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
		store = submissionstore.NewRepository(integrationDB)

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
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: userID})
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

		progress, err := store.Progress(ctx, id, submissiondomain.Viewer{UserID: userID})
		Expect(err).NotTo(HaveOccurred())
		Expect(progress.ID).To(Equal(id))
		Expect(progress.UserID).To(Equal(userID))
		Expect(progress.Status).To(Equal(submissiondomain.StatusJudging))
		Expect(progress.JudgedCases).To(Equal(1))
		Expect(progress.TotalCases).To(Equal(4))
		Expect(progress.CaseResults).To(HaveLen(1))
		Expect(progress.CaseResults[0].MemoryKb).To(Equal(2048))
	})

	It("refuses an unrestricted batch", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: userID})
		judged(ctx, problemID, "Accepted")
		_, err := store.CreateRejudging(ctx, submissiondomain.RejudgeSelector{}, userID)
		Expect(err).To(MatchError(submissiondomain.ErrRejudgeEmpty))
	})

	It("reports an empty match instead of creating a batch", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: userID})
		_, err := store.CreateRejudging(ctx, submissiondomain.RejudgeSelector{ProblemID: problemID}, userID)
		Expect(err).To(MatchError(submissiondomain.ErrRejudgeEmpty))
	})

	It("expands a selector, queues jobs and tracks progress", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: userID})
		first := judged(ctx, problemID, "Accepted")
		second := judged(ctx, problemID, "Wrong Answer")
		// A submission for another problem must stay out of the batch.
		untouched := judged(ctx, otherProblemID, "Accepted")
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE problems SET submission_count = 2, accepted_count = 1, solved_user_count = 1
			 WHERE id = $1`, problemID)
		Expect(err).NotTo(HaveOccurred())

		batch, err := store.CreateRejudging(ctx, submissiondomain.RejudgeSelector{
			ProblemID: problemID, Reason: "fixed the checker",
		}, userID)
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.TotalCount).To(Equal(2))
		Expect(batch.State).To(Equal(submissiondomain.RejudgingRunning))

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
		Expect(progress.State).To(Equal(submissiondomain.RejudgingRunning))

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
		Expect(settled.State).To(Equal(submissiondomain.RejudgingFinished))
		Expect(settled.FinishedAt).NotTo(BeNil())

		changes, err := store.RejudgingChanges(ctx, batch.ID, 100)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		Expect(changes[0].PriorStatus).To(Equal("Accepted"))
		Expect(changes[0].Status).To(Equal("Time Limit Exceeded"))
		Expect(changes[0].Judged).To(BeTrue())
	})

	It("intersects batch filters without requeueing unfinished or excluded submissions", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: userID})
		var otherUser string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username,email,password_hash) VALUES ('other','other@t.local','x') RETURNING id`).Scan(&otherUser)).To(Succeed())
		fixtures := []struct {
			problem, user, language, status string
			finished, include               bool
		}{
			{problemID, userID, "cpp", "Accepted", true, true},
			{otherProblemID, userID, "cpp", "Accepted", true, true},
			{problemID, otherUser, "cpp", "Accepted", true, true},
			{problemID, userID, "python", "Accepted", true, true},
			{problemID, userID, "cpp", "Wrong Answer", true, true},
			{problemID, userID, "cpp", "Accepted", false, true},
			{problemID, userID, "cpp", "Accepted", true, false},
		}
		selected := []string{}
		var expected string
		for i, fixture := range fixtures {
			var id string
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO submissions (problem_id,user_id,language,status,source_code,judged_at)
     VALUES ($1,$2,$3,$4,'x',CASE WHEN $5 THEN now() ELSE NULL END) RETURNING id`,
				fixture.problem, fixture.user, fixture.language, fixture.status, fixture.finished).Scan(&id)).To(Succeed())
			if fixture.include {
				selected = append(selected, id)
			}
			if i == 0 {
				expected = id
			}
		}
		batch, err := store.CreateRejudging(ctx, submissiondomain.RejudgeSelector{
			ProblemID: problemID, UserID: userID, Language: "cpp", Status: "Accepted", SubmissionIDs: selected,
		}, userID)
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.TotalCount).To(Equal(1))
		var requeued []string
		Expect(integrationDB.Pool.SelectContext(ctx, &requeued,
			`SELECT submission_id FROM judge_jobs WHERE state='queued'`)).To(Succeed())
		Expect(requeued).To(ConsistOf(expected))
	})

	It("stops counting a member that a later batch took over", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: userID})
		target := judged(ctx, problemID, "Accepted")
		first, err := store.CreateRejudging(ctx, submissiondomain.RejudgeSelector{SubmissionIDs: []string{target}}, userID)
		Expect(err).NotTo(HaveOccurred())

		// A later single rejudge may take over work that this batch has not
		// finished yet; generation fencing makes the first batch stop waiting.
		Expect(store.Rejudge(ctx, target)).To(Succeed())

		progress, err := store.Rejudging(ctx, first.ID)
		Expect(err).NotTo(HaveOccurred())
		// The first batch is no longer responsible for it, so it is complete.
		Expect(progress.DoneCount).To(Equal(1))
		Expect(progress.State).To(Equal(submissiondomain.RejudgingFinished))
	})

	It("cancels queued work by restoring the complete prior result", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: userID})
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

		batch, err := store.CreateRejudging(ctx, submissiondomain.RejudgeSelector{ProblemID: problemID}, userID)
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
		Expect(settled.State).To(Equal(submissiondomain.RejudgingCancelled))
		// A cancelled batch cannot be cancelled twice.
		Expect(store.CancelRejudging(ctx, batch.ID)).To(MatchError(submissiondomain.ErrRejudgeClosed))
	})

	It("lists batches scoped to a contest", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: userID})
		judged(ctx, problemID, "Accepted")
		_, err := store.CreateRejudging(ctx, submissiondomain.RejudgeSelector{ProblemID: problemID}, userID)
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
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: userID})
		_, err := store.Rejudging(ctx, "00000000-0000-0000-0000-000000000000")
		Expect(err).To(MatchError(submissiondomain.ErrRejudgeNotFound))
	})
})

func TestSubmissionRepository(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Submission PostgreSQL")
}
