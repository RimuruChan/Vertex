-- name: ListenJudgeJobs :exec
LISTEN vertex_judge_jobs;

-- name: ClaimJudgeJob :one
WITH candidate AS (
		   SELECT id FROM judge_jobs
		   WHERE (state = 'queued' AND available_at <= now())
		      OR (state = 'running' AND lease_expires_at < now() AND judge_jobs.attempt < sqlc.arg(max_attempts)::integer)
		   ORDER BY priority DESC, available_at, created_at
		   FOR UPDATE SKIP LOCKED
		   LIMIT 1
		 ), claimed AS (
		   UPDATE judge_jobs AS job
		   SET state = 'running', attempt = job.attempt + 1, worker_id = sqlc.arg(worker_id)::text,
		       lease_token = gen_random_uuid(),
		       lease_expires_at = now() + (sqlc.arg(lease_millis)::bigint * interval '1 millisecond'),
		       started_at = COALESCE(job.started_at, now()), last_error = ''
		   FROM candidate
		   WHERE job.id = candidate.id
		   RETURNING job.*
		 ), marked AS (
		   -- total_cases is known the moment the job is dispatched, so the UI can
		   -- render "0 / N" instead of an unbounded spinner.
		   UPDATE judgements AS sub
		   SET status = 'Judging', judged_cases = 0,
		       total_cases = (SELECT v.case_count FROM problem_versions v WHERE v.problem_id=sub.problem_id AND v.version_no=sub.problem_version)
		   FROM claimed
		   WHERE sub.submission_id = claimed.submission_id
		     AND sub.generation = claimed.generation
		 )
		 SELECT claimed.id, claimed.submission_id, claimed.generation, claimed.attempt,
		        claimed.worker_id, claimed.lease_token, claimed.lease_expires_at,
		        sub.user_id, sub.problem_id, sub.contest_id, sub.language, sub.source_code,
		        version.time_limit_ms, version.memory_limit_kb,
		        version.testdata_path,version.artifact_version,version.sha256,version.case_count,version.checker,
		        sub.domain_id,evaluation.problem_version
		 FROM claimed
		 JOIN submissions AS sub ON sub.id = claimed.submission_id
		 JOIN judgements evaluation ON evaluation.submission_id=claimed.submission_id AND evaluation.generation=claimed.generation
		 JOIN problem_versions version ON version.problem_id=evaluation.problem_id AND version.version_no=evaluation.problem_version;

-- name: RenewJudgeLease :one
WITH renewed AS (
		   UPDATE judge_jobs
		   SET lease_expires_at = now() + (sqlc.arg(lease_millis)::bigint * interval '1 millisecond')
		   WHERE judge_jobs.id = sqlc.arg(job_id)::uuid AND generation = sqlc.arg(generation)::integer AND lease_token = sqlc.arg(lease_token)::uuid
		     AND worker_id = sqlc.arg(worker_id)::text AND state = 'running' AND lease_expires_at >= now()
		   RETURNING submission_id, generation
		 ), progress AS (
		   UPDATE judgements AS sub
		   SET judged_cases = GREATEST(sub.judged_cases, sqlc.arg(judged_cases)::integer)
		   FROM renewed
		   WHERE sub.submission_id = renewed.submission_id AND sub.generation = renewed.generation
		 )
		 SELECT count(*)::int FROM renewed;

-- name: LockJudgeResultTarget :one
SELECT job.state, job.submission_id, sub.problem_id, sub.user_id, sub.contest_id,
		        COALESCE(job.worker_id, '') AS worker_id, COALESCE(job.lease_token::text, '')::text AS lease_token
		 FROM judge_jobs AS job
		 JOIN submissions AS sub ON sub.id = job.submission_id
		 WHERE job.id = sqlc.arg(job_id)::uuid AND job.generation = sqlc.arg(generation)::integer
		 FOR UPDATE OF job, sub;

-- name: CompleteJudgeJob :execrows
UPDATE judge_jobs SET state = 'completed', finished_at = now(), lease_expires_at = NULL
		 WHERE judge_jobs.id = sqlc.arg(job_id)::uuid AND generation = sqlc.arg(generation)::integer AND lease_token = sqlc.arg(lease_token)::uuid
		   AND worker_id = sqlc.arg(worker_id)::text AND state = 'running' AND lease_expires_at >= now();

-- name: SaveSubmissionResult :execrows
UPDATE judgements SET status = sqlc.arg(status)::text, score = sqlc.arg(score)::integer, total_time_ms = sqlc.arg(total_time_ms)::bigint,
		        peak_memory_kb = sqlc.arg(peak_memory_kb)::integer, compile_result = sqlc.arg(compile_result)::text, case_results = sqlc.arg(case_results)::jsonb,
		        judged_cases = sqlc.arg(judged_cases)::integer, total_cases = GREATEST(total_cases, sqlc.arg(judged_cases)::integer),
		        judged_at = now()
		 WHERE submission_id = sqlc.arg(submission_id)::uuid AND generation = sqlc.arg(generation)::integer
 AND EXISTS(SELECT 1 FROM submissions s WHERE s.id=judgements.submission_id AND s.judge_generation=judgements.generation AND s.result_generation=judgements.generation);

-- name: FailExhaustedJudgeJobs :many
UPDATE judge_jobs SET state = 'dead', finished_at = now(), last_error = 'worker lease expired too many times'
		 WHERE state = 'running' AND lease_expires_at < now() AND attempt >= sqlc.arg(max_attempts)::integer
		 RETURNING submission_id, generation;

-- name: FailSubmissionForExpiredLease :one
WITH failed AS (
 UPDATE judgements SET status='System Error',score=0,total_time_ms=0,peak_memory_kb=0,
        case_results='[]'::jsonb,compile_result=sqlc.arg(compile_result)::text,judged_at=now()
 WHERE submission_id=sqlc.arg(submission_id)::uuid AND generation=sqlc.arg(generation)::integer
 AND EXISTS(SELECT 1 FROM submissions s WHERE s.id=judgements.submission_id AND s.judge_generation=judgements.generation AND s.result_generation=judgements.generation)
 RETURNING submission_id
)
SELECT s.problem_id,s.user_id,s.contest_id FROM submissions s JOIN failed f ON f.submission_id=s.id;
