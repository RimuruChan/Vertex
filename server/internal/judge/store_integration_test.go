package judge

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/submission"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Judge job persistence", Ordered, func() {
	var store *JudgeJobStore

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		_, err := integrationDB.Pool.ExecContext(ctx, `
			TRUNCATE discussion_posts, editorials, problem_set_problems, problem_sets,
				submission_cases, judge_jobs, submissions, problem_versions, problem_testdata,
				problem_tags, tags, contest_submission_cells, contest_participants,
				contest_problems, contests, auth_sessions, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = NewJudgeJobStore(integrationDB)
	})

	It("leases one queued job to only one concurrent worker", func(ctx SpecContext) {
		seed := seedJudgeJob(ctx)
		const workers = 8
		jobs := make(chan *Job, workers)
		errorsFound := make(chan error, workers)
		var wait sync.WaitGroup
		for worker := 0; worker < workers; worker++ {
			wait.Add(1)
			go func(worker int) {
				defer wait.Done()
				job, err := store.Claim(context.Background(), "worker-"+string(rune('a'+worker)), time.Minute)
				errorsFound <- err
				jobs <- job
			}(worker)
		}
		wait.Wait()
		close(errorsFound)
		close(jobs)
		for err := range errorsFound {
			Expect(err).NotTo(HaveOccurred())
		}
		claimed := make([]*Job, 0, 1)
		for job := range jobs {
			if job != nil {
				claimed = append(claimed, job)
			}
		}
		Expect(claimed).To(HaveLen(1))
		Expect(claimed[0].SubmissionID).To(Equal(seed.submissionID))
		Expect(claimed[0].Testdata).To(Equal(Testdata{
			StoragePath: "problem-data", DataVersion: 3, SHA256: "fixture-sha256", CaseCount: 2, Checker: "diff",
		}))
		var status string
		Expect(integrationDB.Pool.GetContext(ctx, &status,
			`SELECT status FROM submissions WHERE id = $1`, seed.submissionID)).To(Succeed())
		Expect(status).To(Equal("Judging"))
	})

	It("reclaims an expired lease with a new token and incremented attempt", func(ctx SpecContext) {
		seedJudgeJob(ctx)
		first, err := store.Claim(ctx, "worker-1", 20*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Attempt).To(Equal(1))
		Eventually(func() bool { return time.Now().After(first.LeaseExpiresAt) }).Should(BeTrue())
		second, err := store.Claim(ctx, "worker-2", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(second).NotTo(BeNil())
		Expect(second.Attempt).To(Equal(2))
		Expect(second.LeaseToken).NotTo(Equal(first.LeaseToken))
	})

	It("renews only the matching live lease", func(ctx SpecContext) {
		seedJudgeJob(ctx)
		job, err := store.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		originalExpiry := job.LeaseExpiresAt
		Expect(store.Heartbeat(ctx, job.ID, job.Generation, job.LeaseToken, job.WorkerID, 3, 2*time.Minute)).To(Succeed())
		var renewedExpiry time.Time
		Expect(integrationDB.Pool.GetContext(ctx, &renewedExpiry,
			`SELECT lease_expires_at FROM judge_jobs WHERE id = $1`, job.ID)).To(Succeed())
		Expect(renewedExpiry).To(BeTemporally(">", originalExpiry))
		Expect(store.Heartbeat(ctx, job.ID, job.Generation, job.LeaseToken, "worker-2", 4, time.Minute)).To(MatchError(ErrStaleLease))
	})

	It("publishes case progress through the heartbeat and never rewinds it", func(ctx SpecContext) {
		seed := seedJudgeJob(ctx)
		job, err := store.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())

		progress := func() (int, int) {
			var judged, total int
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`SELECT judged_cases, total_cases FROM submissions WHERE id = $1`,
				seed.submissionID).Scan(&judged, &total)).To(Succeed())
			return judged, total
		}

		judged, total := progress()
		Expect(judged).To(Equal(0))
		Expect(total).To(Equal(seededCaseCount))

		Expect(store.Heartbeat(ctx, job.ID, job.Generation, job.LeaseToken, job.WorkerID, 2, time.Minute)).To(Succeed())
		judged, _ = progress()
		Expect(judged).To(Equal(2))

		// A heartbeat that arrives out of order must not move the counter back.
		Expect(store.Heartbeat(ctx, job.ID, job.Generation, job.LeaseToken, job.WorkerID, 1, time.Minute)).To(Succeed())
		judged, _ = progress()
		Expect(judged).To(Equal(2))
	})

	It("marks a repeatedly expired lease dead with the existing system-error verdict", func(ctx SpecContext) {
		seed := seedJudgeJob(ctx)
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE judge_jobs SET state = 'running', attempt = $2, worker_id = 'gone',
			 lease_token = gen_random_uuid(), lease_expires_at = now() - interval '1 second'
			 WHERE submission_id = $1`, seed.submissionID, maxJudgeAttempts)
		Expect(err).NotTo(HaveOccurred())

		job, err := store.Claim(ctx, "worker-2", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(job).To(BeNil())
		var state, status string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT job.state, sub.status FROM judge_jobs AS job
			 JOIN submissions AS sub ON sub.id = job.submission_id
			 WHERE job.submission_id = $1`, seed.submissionID).Scan(&state, &status)).To(Succeed())
		Expect(state).To(Equal("dead"))
		Expect(status).To(Equal("System Error"))
	})

	It("rejects a stale result after another worker reclaims the job", func(ctx SpecContext) {
		seed := seedJudgeJob(ctx)
		first, err := store.Claim(ctx, "worker-1", 20*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() bool { return time.Now().After(first.LeaseExpiresAt) }).Should(BeTrue())
		second, err := store.Claim(ctx, "worker-2", time.Minute)
		Expect(err).NotTo(HaveOccurred())

		stale := acceptedResult(first, seed.submissionID)
		Expect(store.Complete(ctx, stale)).To(MatchError(ErrStaleLease))
		Expect(store.Complete(ctx, acceptedResult(second, seed.submissionID))).To(Succeed())
		var status string
		Expect(integrationDB.Pool.GetContext(ctx, &status,
			`SELECT status FROM submissions WHERE id = $1`, seed.submissionID)).To(Succeed())
		Expect(status).To(Equal("Accepted"))
	})

	It("accepts an identical completed result idempotently", func(ctx SpecContext) {
		seed := seedJudgeJob(ctx)
		job, err := store.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		result := acceptedResult(job, seed.submissionID)
		Expect(store.Complete(ctx, result)).To(Succeed())
		Expect(store.Complete(ctx, result)).To(Succeed())
		var cases int
		Expect(integrationDB.Pool.GetContext(ctx, &cases,
			`SELECT count(*) FROM submission_cases WHERE submission_id = $1`, seed.submissionID)).To(Succeed())
		Expect(cases).To(Equal(1))
	})

	It("rebuilds the contest cell when a contest submission completes", func(ctx SpecContext) {
		seed := seedJudgeJob(ctx)
		var contestID string
		Expect(integrationDB.Pool.GetContext(ctx, &contestID,
			`INSERT INTO contests (title, begin_at, end_at)
			 VALUES ('Judge contest', now() - interval '1 hour', now() + interval '1 hour')
			 RETURNING id::text`)).To(Succeed())
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE submissions SET contest_id = $2 WHERE id = $1`, seed.submissionID, contestID)
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_problems (contest_id, problem_id) VALUES ($1, $2)`, contestID, seed.problemID)
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_participants (contest_id, user_id) VALUES ($1, $2)`, contestID, seed.userID)
		Expect(err).NotTo(HaveOccurred())

		job, err := store.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.Complete(ctx, acceptedResult(job, seed.submissionID))).To(Succeed())

		var attempts, penalty int
		var solvedAt time.Time
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT attempts, penalty_sec, solved_at
			 FROM contest_submission_cells
			 WHERE contest_id = $1 AND user_id = $2 AND problem_id = $3`,
			contestID, seed.userID, seed.problemID,
		).Scan(&attempts, &penalty, &solvedAt)).To(Succeed())
		Expect(attempts).To(Equal(1))
		Expect(penalty).To(BeNumerically(">=", 0))
		Expect(solvedAt).NotTo(BeZero())

		board, err := contest.NewContestStore(integrationDB).Rankboard(ctx, contestID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(board.Rows).To(HaveLen(1))
		Expect(board.Rows[0].Solved).To(Equal(1))
	})

	It("fences an old generation after rejudge", func(ctx SpecContext) {
		seed := seedJudgeJob(ctx)
		oldJob, err := store.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(submission.NewSubmissionStore(integrationDB).Rejudge(ctx, seed.submissionID)).To(Succeed())
		Expect(store.Complete(ctx, acceptedResult(oldJob, seed.submissionID))).To(MatchError(ErrStaleLease))

		newJob, err := store.Claim(ctx, "worker-2", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(newJob.Generation).To(Equal(2))
		Expect(newJob.SubmissionID).To(Equal(seed.submissionID))
		var oldState string
		Expect(integrationDB.Pool.GetContext(ctx, &oldState,
			`SELECT state FROM judge_jobs WHERE submission_id = $1 AND generation = 1`, seed.submissionID)).To(Succeed())
		Expect(oldState).To(Equal("cancelled"))
	})

	It("removes completed results from counters while rejudge is pending", func(ctx SpecContext) {
		seed := seedJudgeJob(ctx)
		job, err := store.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.Complete(ctx, acceptedResult(job, seed.submissionID))).To(Succeed())
		var accepted int
		Expect(integrationDB.Pool.GetContext(ctx, &accepted,
			`SELECT problem.accepted_count FROM problems AS problem
			 JOIN submissions AS sub ON sub.problem_id = problem.id WHERE sub.id = $1`, seed.submissionID)).To(Succeed())
		Expect(accepted).To(Equal(1))

		Expect(submission.NewSubmissionStore(integrationDB).Rejudge(ctx, seed.submissionID)).To(Succeed())
		var status string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT sub.status, problem.accepted_count FROM submissions AS sub
			 JOIN problems AS problem ON problem.id = sub.problem_id WHERE sub.id = $1`, seed.submissionID,
		).Scan(&status, &accepted)).To(Succeed())
		Expect(status).To(Equal("Pending"))
		Expect(accepted).To(BeZero())
	})

	It("rolls back all result writes when a case insert fails", func(ctx SpecContext) {
		seed := seedJudgeJob(ctx)
		job, err := store.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		result := acceptedResult(job, seed.submissionID)
		result.Cases = append(result.Cases, result.Cases[0])
		Expect(store.Complete(ctx, result)).NotTo(Succeed())

		var jobState, submissionStatus string
		Expect(integrationDB.Pool.GetContext(ctx, &jobState,
			`SELECT state FROM judge_jobs WHERE id = $1`, job.ID)).To(Succeed())
		Expect(integrationDB.Pool.GetContext(ctx, &submissionStatus,
			`SELECT status FROM submissions WHERE id = $1`, seed.submissionID)).To(Succeed())
		Expect(jobState).To(Equal("running"))
		Expect(submissionStatus).To(Equal("Judging"))
		var cases int
		Expect(integrationDB.Pool.GetContext(ctx, &cases,
			`SELECT count(*) FROM submission_cases WHERE submission_id = $1`, seed.submissionID)).To(Succeed())
		Expect(cases).To(BeZero())
	})

	It("requires the completed lease identity for idempotent retries", func(ctx SpecContext) {
		seed := seedJudgeJob(ctx)
		job, err := store.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		result := acceptedResult(job, seed.submissionID)
		Expect(store.Complete(ctx, result)).To(Succeed())
		result.WorkerID = "worker-2"
		Expect(errors.Is(store.Complete(ctx, result), ErrStaleLease)).To(BeTrue())
	})
})

// seededCaseCount mirrors the problem_testdata fixture below.
const seededCaseCount = 2

type judgeSeed struct {
	submissionID string
	userID       string
	problemID    string
}

func seedJudgeJob(ctx context.Context) judgeSeed {
	var userID, problemID, submissionID string
	Expect(integrationDB.Pool.GetContext(ctx, &userID,
		`INSERT INTO users (username, email, password_hash) VALUES ('judge-user', 'judge@example.com', 'hash') RETURNING id::text`)).To(Succeed())
	Expect(integrationDB.Pool.GetContext(ctx, &problemID,
		`INSERT INTO problems (title, visibility) VALUES ('Judge fixture', 'public') RETURNING id::text`)).To(Succeed())
	_, err := integrationDB.Pool.ExecContext(ctx,
		`INSERT INTO problem_testdata (problem_id, storage_path, data_version, sha256, case_count, checker)
		 VALUES ($1, 'problem-data', 3, 'fixture-sha256', 2, 'diff')`, problemID)
	Expect(err).NotTo(HaveOccurred())
	Expect(integrationDB.Pool.GetContext(ctx, &submissionID,
		`INSERT INTO submissions (user_id, problem_id, language, source_code)
		 VALUES ($1, $2, 'cpp', 'int main(){}') RETURNING id::text`, userID, problemID)).To(Succeed())
	_, err = integrationDB.Pool.ExecContext(ctx,
		`INSERT INTO judge_jobs (submission_id, generation) VALUES ($1, 1)`, submissionID)
	Expect(err).NotTo(HaveOccurred())
	return judgeSeed{submissionID: submissionID, userID: userID, problemID: problemID}
}

func acceptedResult(job *Job, submissionID string) Result {
	return Result{
		JobID: job.ID, SubmissionID: submissionID, Generation: job.Generation,
		LeaseToken: job.LeaseToken, WorkerID: job.WorkerID,
		Status: "Accepted", Score: 100, TotalTimeMs: 5, PeakMemoryKB: 1024,
		Cases: []CaseResult{{CaseIndex: 1, Verdict: "Accepted", TimeMs: 5, MemoryKB: 1024}},
	}
}
