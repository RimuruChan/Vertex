-- name: CreateSubmission :one
WITH entry AS (
 INSERT INTO submissions(user_id,problem_id,language,source_code,contest_id,domain_id,initial_problem_version)
 SELECT sqlc.arg(user_id)::uuid,p.id,sqlc.arg(language)::text,sqlc.arg(source_code)::text,
        sqlc.narg(contest_id)::uuid,sqlc.arg(domain_id)::uuid,
        CASE WHEN sqlc.narg(contest_id)::uuid IS NULL THEN p.published_version ELSE cp.problem_version END
 FROM problems p LEFT JOIN contest_problems cp ON cp.problem_id=p.id AND cp.contest_id=sqlc.narg(contest_id)::uuid
 WHERE p.id=sqlc.arg(problem_id)::uuid AND p.domain_id=sqlc.arg(domain_id)::uuid
 RETURNING *
), evaluation AS (
 INSERT INTO judgements(submission_id,generation,problem_id,problem_version)
 SELECT id,1,problem_id,initial_problem_version FROM entry RETURNING *
)
SELECT e.id,e.public_id,e.user_id,e.problem_id,e.language,e.source_code,j.status,j.score,
       j.total_time_ms,j.peak_memory_kb,j.compile_result,e.contest_id,e.submitted_at,j.judged_at,j.problem_version,
 (SELECT public_id::text FROM problems WHERE id=e.problem_id)::text AS problem_public_id,
 COALESCE((SELECT public_id::text FROM contests WHERE id=e.contest_id),'')::text AS contest_public_id
FROM entry e JOIN evaluation j ON j.submission_id=e.id;

-- name: EnqueueInitialJudgeJob :exec
INSERT INTO judge_jobs (submission_id, generation) VALUES (sqlc.arg(submission_id)::uuid, 1);

-- name: LockSubmissionProblem :one
SELECT 1 FROM problems WHERE id = sqlc.arg(problem_id)::uuid AND domain_id = sqlc.arg(domain_id)::uuid FOR SHARE;

-- name: LockSubmissionContestProblem :one
SELECT 1 FROM contest_problems
		 WHERE contest_id = sqlc.arg(contest_id)::uuid AND problem_id = sqlc.arg(problem_id)::uuid
		 FOR SHARE;

-- name: NotifyJudgeJob :exec
SELECT pg_notify('vertex_judge_jobs', sqlc.arg(submission_id)::text);
