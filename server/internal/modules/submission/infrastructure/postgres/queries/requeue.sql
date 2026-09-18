-- name: GetSubmissionRejudgeTarget :one
SELECT contest_id,problem_id FROM submission_results WHERE submission_results.id =sqlc.arg(submission_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: LockRejudgeGeneration :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text,0));

-- name: RecordRejudgeAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(actor_id)::uuid,sqlc.arg(action)::text,sqlc.arg(target)::text);

-- name: CancelActiveJudgeJobs :exec
UPDATE judge_jobs SET state = 'cancelled', finished_at = now()
		 WHERE submission_id = sqlc.arg(submission_id)::uuid AND state IN ('queued', 'running')
		 AND EXISTS (SELECT 1 FROM submission_results WHERE submission_results.id = sqlc.arg(submission_id)::uuid AND submission_results.domain_id = sqlc.arg(domain_id)::uuid);

-- name: ResetSubmissionForRejudge :one
WITH target AS (
 UPDATE submissions SET judge_generation=judge_generation+1,result_generation=judge_generation+1
 WHERE id=sqlc.arg(submission_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid
 RETURNING *
), evaluation AS (
 INSERT INTO judgements(submission_id,generation,problem_id,problem_version)
 SELECT t.id,t.judge_generation,t.problem_id,
        CASE WHEN t.contest_id IS NULL THEN p.published_version ELSE cp.problem_version END
 FROM target t JOIN problems p ON p.id=t.problem_id
 LEFT JOIN contest_problems cp ON cp.contest_id=t.contest_id AND cp.problem_id=t.problem_id
 RETURNING submission_id
)
SELECT t.judge_generation,t.problem_id,t.user_id,t.contest_id FROM target t JOIN evaluation j ON j.submission_id=t.id;

-- name: EnqueueJudgeGeneration :exec
INSERT INTO judge_jobs (submission_id, generation) VALUES (sqlc.arg(submission_id)::uuid, sqlc.arg(generation)::integer);
