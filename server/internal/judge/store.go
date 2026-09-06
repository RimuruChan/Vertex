package judge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/problem"
)

const maxJudgeAttempts = 5

// JudgeJobStore owns queue, lease and fenced result persistence in Web.
type JudgeJobStore struct{ db *database.DB }

func NewJudgeJobStore(db *database.DB) *JudgeJobStore { return &JudgeJobStore{db: db} }

func (s *JudgeJobStore) Claim(ctx context.Context, workerID string, leaseTTL time.Duration) (*Job, error) {
	if err := s.failExhausted(ctx); err != nil {
		return nil, err
	}
	leaseMillis := leaseTTL.Milliseconds()
	var job Job
	err := s.db.Pool.QueryRowContext(ctx,
		`WITH candidate AS (
		   SELECT id FROM judge_jobs
		   WHERE (state = 'queued' AND available_at <= now())
		      OR (state = 'running' AND lease_expires_at < now() AND attempt < $3)
		   ORDER BY priority DESC, available_at, created_at
		   FOR UPDATE SKIP LOCKED
		   LIMIT 1
		 ), claimed AS (
		   UPDATE judge_jobs AS job
		   SET state = 'running', attempt = job.attempt + 1, worker_id = $1,
		       lease_token = gen_random_uuid(),
		       lease_expires_at = now() + ($2::bigint * interval '1 millisecond'),
		       started_at = COALESCE(job.started_at, now()), last_error = ''
		   FROM candidate
		   WHERE job.id = candidate.id
		   RETURNING job.*
		 ), marked AS (
		   -- total_cases is known the moment the job is dispatched, so the UI can
		   -- render "0 / N" instead of an unbounded spinner.
		   UPDATE submissions AS sub
		   SET status = 'Judging', judged_cases = 0,
		       total_cases = COALESCE(
		         (SELECT td.case_count FROM problem_testdata AS td WHERE td.problem_id = sub.problem_id), 0)
		   FROM claimed
		   WHERE sub.id = claimed.submission_id
		     AND sub.judge_generation = claimed.generation
		 )
		 SELECT claimed.id, claimed.submission_id, claimed.generation, claimed.attempt,
		        claimed.worker_id, claimed.lease_token, claimed.lease_expires_at,
		        sub.user_id, sub.problem_id, sub.contest_id, sub.language, sub.source_code,
		        problem.time_limit_ms, problem.memory_limit_kb,
		        COALESCE(testdata.storage_path, ''), COALESCE(testdata.data_version, 0),
		        COALESCE(testdata.sha256, ''), COALESCE(testdata.case_count, 0),
		        COALESCE(testdata.checker, 'diff')
		 FROM claimed
		 JOIN submissions AS sub ON sub.id = claimed.submission_id
		 JOIN problems AS problem ON problem.id = sub.problem_id
		 LEFT JOIN problem_testdata AS testdata ON testdata.problem_id = sub.problem_id`,
		workerID, leaseMillis, maxJudgeAttempts,
	).Scan(
		&job.ID, &job.SubmissionID, &job.Generation, &job.Attempt,
		&job.WorkerID, &job.LeaseToken, &job.LeaseExpiresAt,
		&job.UserID, &job.ProblemID, &job.ContestID, &job.Language, &job.SourceCode,
		&job.TimeLimitMs, &job.MemoryLimitKB,
		&job.Testdata.StoragePath, &job.Testdata.DataVersion, &job.Testdata.SHA256,
		&job.Testdata.CaseCount, &job.Testdata.Checker,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// Heartbeat renews the fenced lease and, in the same statement, publishes the
// worker's case progress. Progress only moves forward so a heartbeat that
// arrives out of order cannot rewind the number the user is watching.
func (s *JudgeJobStore) Heartbeat(
	ctx context.Context, jobID string, generation int, leaseToken, workerID string,
	judgedCases int, leaseTTL time.Duration,
) error {
	var renewed int
	err := s.db.Pool.QueryRowContext(ctx,
		`WITH renewed AS (
		   UPDATE judge_jobs
		   SET lease_expires_at = now() + ($5::bigint * interval '1 millisecond')
		   WHERE id = $1 AND generation = $2 AND lease_token = $3::uuid
		     AND worker_id = $4 AND state = 'running' AND lease_expires_at >= now()
		   RETURNING submission_id, generation
		 ), progress AS (
		   UPDATE submissions AS sub
		   SET judged_cases = GREATEST(sub.judged_cases, $6)
		   FROM renewed
		   WHERE sub.id = renewed.submission_id AND sub.judge_generation = renewed.generation
		 )
		 SELECT count(*)::int FROM renewed`,
		jobID, generation, leaseToken, workerID, leaseTTL.Milliseconds(), judgedCases,
	).Scan(&renewed)
	if err != nil {
		return err
	}
	if renewed != 1 {
		return ErrStaleLease
	}
	return nil
}

func (s *JudgeJobStore) Complete(ctx context.Context, result Result) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var state, submissionID, problemID, userID, assignedWorkerID, assignedLeaseToken string
	var contestID *string
	err = tx.QueryRowContext(ctx,
		`SELECT job.state, job.submission_id, sub.problem_id, sub.user_id, sub.contest_id,
		        COALESCE(job.worker_id, ''), COALESCE(job.lease_token::text, '')
		 FROM judge_jobs AS job
		 JOIN submissions AS sub ON sub.id = job.submission_id
		 WHERE job.id = $1 AND job.generation = $2
		 FOR UPDATE OF job, sub`, result.JobID, result.Generation,
	).Scan(&state, &submissionID, &problemID, &userID, &contestID, &assignedWorkerID, &assignedLeaseToken)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaleLease
	}
	if err != nil {
		return err
	}
	if submissionID != result.SubmissionID {
		return ErrStaleLease
	}
	if state == "completed" {
		if assignedWorkerID != result.WorkerID || assignedLeaseToken != result.LeaseToken {
			return ErrStaleLease
		}
		return tx.Commit()
	}

	command, err := tx.ExecContext(ctx,
		`UPDATE judge_jobs SET state = 'completed', finished_at = now(), lease_expires_at = NULL
		 WHERE id = $1 AND generation = $2 AND lease_token = $3::uuid
		   AND worker_id = $4 AND state = 'running' AND lease_expires_at >= now()`,
		result.JobID, result.Generation, result.LeaseToken, result.WorkerID)
	if err != nil {
		return err
	}
	affected, err := command.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrStaleLease
	}

	caseJSON, err := json.Marshal(caseResultSnapshot(result.Cases))
	if err != nil {
		return err
	}
	command, err = tx.ExecContext(ctx,
		`UPDATE submissions SET status = $3, score = $4, total_time_ms = $5,
		        peak_memory_kb = $6, compile_result = $7, case_results = $8,
		        judged_cases = $9, total_cases = GREATEST(total_cases, $9),
		        judged_at = now()
		 WHERE id = $1 AND judge_generation = $2`,
		result.SubmissionID, result.Generation, result.Status, result.Score,
		result.TotalTimeMs, result.PeakMemoryKB, result.CompileResult, caseJSON,
		len(result.Cases))
	if err != nil {
		return err
	}
	affected, err = command.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrStaleLease
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM submission_cases WHERE submission_id = $1`, result.SubmissionID); err != nil {
		return err
	}
	for _, item := range result.Cases {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO submission_cases
			 (submission_id, case_index, verdict, time_ms, memory_kb, exit_status, checker_output)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			result.SubmissionID, item.CaseIndex, item.Verdict, item.TimeMs,
			item.MemoryKB, item.ExitStatus, item.CheckerOutput); err != nil {
			return err
		}
	}

	if contestID != nil && *contestID != "" {
		if err := contest.RebuildCell(ctx, tx, *contestID, userID, problemID); err != nil {
			return err
		}
	} else if err := problem.RebuildPracticeCounters(ctx, tx, problemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *JudgeJobStore) failExhausted(ctx context.Context) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx,
		`UPDATE judge_jobs SET state = 'dead', finished_at = now(), last_error = 'worker lease expired too many times'
		 WHERE state = 'running' AND lease_expires_at < now() AND attempt >= $1
		 RETURNING submission_id, generation`, maxJudgeAttempts)
	if err != nil {
		return err
	}
	type deadJob struct {
		submissionID string
		generation   int
	}
	var dead []deadJob
	for rows.Next() {
		var item deadJob
		if err := rows.Scan(&item.submissionID, &item.generation); err != nil {
			rows.Close()
			return err
		}
		dead = append(dead, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range dead {
		var problemID, userID string
		var contestID *string
		err := tx.QueryRowContext(ctx,
			`UPDATE submissions SET status = 'System Error', score = 0,
			        total_time_ms = 0, peak_memory_kb = 0, case_results = '[]'::jsonb,
			        compile_result = $3, judged_at = now()
			 WHERE id = $1 AND judge_generation = $2
			 RETURNING problem_id, user_id, contest_id`,
			item.submissionID, item.generation, "judge worker lease expired too many times").Scan(
			&problemID, &userID, &contestID,
		)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM submission_cases WHERE submission_id = $1`, item.submissionID); err != nil {
			return err
		}
		if contestID != nil && *contestID != "" {
			if err := contest.RebuildCell(ctx, tx, *contestID, userID, problemID); err != nil {
				return err
			}
		} else if err := problem.RebuildPracticeCounters(ctx, tx, problemID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type persistedCaseResult struct {
	CaseIndex     int    `json:"caseIndex"`
	Verdict       string `json:"verdict"`
	TimeMs        int    `json:"timeMs"`
	MemoryKB      int    `json:"memoryKb"`
	ExitStatus    string `json:"exitStatus,omitempty"`
	CheckerOutput string `json:"checkerOutput,omitempty"`
}

func caseResultSnapshot(cases []CaseResult) []persistedCaseResult {
	snapshot := make([]persistedCaseResult, 0, len(cases))
	for _, item := range cases {
		snapshot = append(snapshot, persistedCaseResult{
			CaseIndex: item.CaseIndex, Verdict: item.Verdict, TimeMs: item.TimeMs,
			MemoryKB: item.MemoryKB, ExitStatus: item.ExitStatus, CheckerOutput: item.CheckerOutput,
		})
	}
	return snapshot
}
