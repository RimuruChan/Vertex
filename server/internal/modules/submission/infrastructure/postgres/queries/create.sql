-- name: CreateSubmission :one
INSERT INTO submissions (user_id, problem_id, language, source_code, status, contest_id, domain_id)
		 VALUES (sqlc.arg(user_id)::uuid, sqlc.arg(problem_id)::uuid, sqlc.arg(language)::text, sqlc.arg(source_code)::text, 'Pending', sqlc.narg(contest_id)::uuid, sqlc.arg(domain_id)::uuid)
		 RETURNING id, public_id, user_id, problem_id, language, source_code, status, score,
		           total_time_ms, peak_memory_kb, compile_result, contest_id, submitted_at, judged_at,problem_version;

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
