-- name: GetSubmissionRejudgeTarget :one
SELECT contest_id,problem_id FROM submissions WHERE submissions.id =sqlc.arg(submission_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: LockRejudgeGeneration :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text,0));

-- name: RecordRejudgeAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(actor_id)::uuid,sqlc.arg(action)::text,sqlc.arg(target)::text);

-- name: CancelActiveJudgeJobs :exec
UPDATE judge_jobs SET state = 'cancelled', finished_at = now()
		 WHERE submission_id = sqlc.arg(submission_id)::uuid AND state IN ('queued', 'running')
		 AND EXISTS (SELECT 1 FROM submissions WHERE submissions.id = sqlc.arg(submission_id)::uuid AND submissions.domain_id = sqlc.arg(domain_id)::uuid);

-- name: ResetSubmissionForRejudge :one
UPDATE submissions SET status = 'Pending', judged_at = NULL, score = 0,
		                        problem_version=CASE WHEN contest_id IS NULL THEN (SELECT published_version FROM problems WHERE problems.id =submissions.problem_id)
		                          ELSE COALESCE((SELECT cp.problem_version FROM contest_problems cp WHERE cp.contest_id=submissions.contest_id AND cp.problem_id=submissions.problem_id),problem_version) END,
		                        total_time_ms = 0, peak_memory_kb = 0,
		                        compile_result = '', case_results = '[]'::jsonb,
		                        judged_cases = 0, total_cases = 0,
		                        judge_generation = judge_generation + 1
		 WHERE submissions.id = sqlc.arg(submission_id)::uuid AND submissions.domain_id = sqlc.arg(domain_id)::uuid
		 RETURNING judge_generation, problem_id, user_id, contest_id;

-- name: EnqueueJudgeGeneration :exec
INSERT INTO judge_jobs (submission_id, generation) VALUES (sqlc.arg(submission_id)::uuid, sqlc.arg(generation)::integer);

-- name: DeleteSubmissionCases :exec
DELETE FROM submission_cases WHERE submission_id = sqlc.arg(submission_id)::uuid;
