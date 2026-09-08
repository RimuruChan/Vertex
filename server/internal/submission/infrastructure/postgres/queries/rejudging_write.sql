-- name: GetRejudgingTarget :one
SELECT contest_id,problem_id FROM rejudgings WHERE id=$1 AND domain_id=$2;

-- name: LockActiveJudgeJobs :many
SELECT id FROM judge_jobs WHERE submission_id=ANY($1::uuid[]) AND state IN ('queued','running') ORDER BY id FOR UPDATE;

-- name: CreateRejudging :one
INSERT INTO rejudgings (contest_id, problem_id, reason, total_count, created_by, domain_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, contest_id, problem_id, reason, state, total_count, created_by, created_at, finished_at;

-- name: SaveRejudgingSnapshot :exec
INSERT INTO rejudging_submissions
			   (rejudging_id, submission_id, generation, prior_status, prior_score,
			    prior_total_time_ms, prior_peak_memory_kb, prior_compile_result,
			    prior_case_results, prior_judged_cases, prior_total_cases, prior_judged_at, domain_id,prior_problem_version)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,$14);

-- name: LockRejudging :one
SELECT state FROM rejudgings WHERE id = $1 AND domain_id = $2 FOR UPDATE;

-- name: RestoreQueuedRejudgingSubmissions :many
WITH cancelled AS (
		   UPDATE judge_jobs AS job
		   SET state = 'cancelled', finished_at = now()
		   FROM rejudging_submissions AS member
		   WHERE member.rejudging_id = $1
		     AND job.submission_id = member.submission_id
		     AND job.generation = member.generation
		     AND job.state = 'queued'
		   RETURNING job.submission_id, job.generation
		 )
		 UPDATE submissions AS sub
		 SET status = member.prior_status,
		     problem_version = member.prior_problem_version,
		     score = member.prior_score,
		     total_time_ms = member.prior_total_time_ms,
		     peak_memory_kb = member.prior_peak_memory_kb,
		     compile_result = member.prior_compile_result,
		     case_results = member.prior_case_results,
		     judged_cases = member.prior_judged_cases,
		     total_cases = member.prior_total_cases,
		     judged_at = member.prior_judged_at
		 FROM rejudging_submissions AS member
		 JOIN cancelled
		   ON cancelled.submission_id = member.submission_id
		  AND cancelled.generation = member.generation
		 WHERE member.rejudging_id = $1
		   AND sub.id = member.submission_id
		   AND sub.judge_generation = member.generation
		 RETURNING sub.id, sub.problem_id, sub.user_id, sub.contest_id;

-- name: DeleteCancelledRejudgingCases :exec
DELETE FROM submission_cases AS result
		 USING rejudging_submissions AS member, judge_jobs AS job, submissions AS sub
		 WHERE member.rejudging_id = $1
		   AND job.submission_id = member.submission_id
		   AND job.generation = member.generation
		   AND job.state = 'cancelled'
		   AND sub.id = member.submission_id
		   AND sub.judge_generation = member.generation
		   AND result.submission_id = member.submission_id;

-- name: RestoreCancelledRejudgingCases :exec
INSERT INTO submission_cases
		   (submission_id, case_index, verdict, time_ms, memory_kb, exit_status, checker_output)
		 SELECT member.submission_id,
		        (item.value->>'caseIndex')::integer,
		        item.value->>'verdict',
		        COALESCE((item.value->>'timeMs')::integer, 0),
		        COALESCE((item.value->>'memoryKb')::integer, 0),
		        COALESCE(item.value->>'exitStatus', ''),
		        COALESCE(item.value->>'checkerOutput', '')
		 FROM rejudging_submissions AS member
		 JOIN judge_jobs AS job
		   ON job.submission_id = member.submission_id
		  AND job.generation = member.generation
		  AND job.state = 'cancelled'
		 JOIN submissions AS sub
		   ON sub.id = member.submission_id
		  AND sub.judge_generation = member.generation
		 CROSS JOIN LATERAL jsonb_array_elements(member.prior_case_results) AS item(value)
		 WHERE member.rejudging_id = $1;

-- name: CancelRejudging :exec
UPDATE rejudgings SET state = 'cancelled', finished_at = now() WHERE id = $1;


-- name: FindRejudgingCandidates :many
SELECT id FROM submissions WHERE domain_id = sqlc.arg(domain_id)::uuid
 AND judged_at IS NOT NULL
 AND (sqlc.arg(contest_filter)::text = '' OR contest_id = NULLIF(sqlc.arg(contest_filter)::text, '')::uuid)
 AND (sqlc.arg(problem_filter)::text = '' OR problem_id = NULLIF(sqlc.arg(problem_filter)::text, '')::uuid)
 AND (sqlc.arg(user_filter)::text = '' OR user_id = NULLIF(sqlc.arg(user_filter)::text, '')::uuid)
 AND (sqlc.arg(language_filter)::text = '' OR language = sqlc.arg(language_filter)::text)
 AND (sqlc.arg(status_filter)::text = '' OR status = sqlc.arg(status_filter)::text)
 AND (NOT sqlc.arg(filter_ids)::boolean OR id = ANY(sqlc.arg(submission_ids)::uuid[]))
 AND (NOT sqlc.arg(practice_only)::boolean OR contest_id IS NULL)
 ORDER BY submitted_at, id LIMIT sqlc.arg(batch_limit)::integer;

-- name: LockRejudgingSnapshots :many
SELECT id, status, score, total_time_ms, peak_memory_kb, problem_version, compile_result, case_results, judged_cases, total_cases, judged_at
 FROM submissions WHERE domain_id = sqlc.arg(domain_id)::uuid
 AND judged_at IS NOT NULL
 AND (sqlc.arg(contest_filter)::text = '' OR contest_id = NULLIF(sqlc.arg(contest_filter)::text, '')::uuid)
 AND (sqlc.arg(problem_filter)::text = '' OR problem_id = NULLIF(sqlc.arg(problem_filter)::text, '')::uuid)
 AND (sqlc.arg(user_filter)::text = '' OR user_id = NULLIF(sqlc.arg(user_filter)::text, '')::uuid)
 AND (sqlc.arg(language_filter)::text = '' OR language = sqlc.arg(language_filter)::text)
 AND (sqlc.arg(status_filter)::text = '' OR status = sqlc.arg(status_filter)::text)
 AND (NOT sqlc.arg(filter_ids)::boolean OR id = ANY(sqlc.arg(submission_ids)::uuid[]))
 AND (NOT sqlc.arg(practice_only)::boolean OR contest_id IS NULL)
 AND id = ANY(sqlc.arg(candidate_ids)::uuid[])
 ORDER BY submitted_at, id LIMIT sqlc.arg(batch_limit)::integer FOR UPDATE;
