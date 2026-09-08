-- name: LockContestCell :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text, 0));

-- name: DeleteContestCell :exec
DELETE FROM contest_submission_cells
			 WHERE contest_submission_cells.contest_id = sqlc.arg(contest_id)::uuid AND user_id = sqlc.arg(user_id)::uuid AND problem_id = sqlc.arg(problem_id)::uuid;

-- name: UpsertContestCell :exec
INSERT INTO contest_submission_cells
		   (contest_id, user_id, problem_id, attempts, penalty_sec, score, solved_at,
		    public_attempts, public_penalty_sec, public_score, public_solved_at,
		    pending_count, last_submit_at, domain_id)
		 VALUES (sqlc.arg(contest_id)::uuid, sqlc.arg(user_id)::uuid, sqlc.arg(problem_id)::uuid, sqlc.arg(attempts)::integer, sqlc.arg(penalty_sec)::integer, sqlc.arg(score)::integer, sqlc.narg(solved_at)::timestamptz, sqlc.arg(public_attempts)::integer, sqlc.arg(public_penalty_sec)::integer, sqlc.arg(public_score)::integer, sqlc.narg(public_solved_at)::timestamptz, sqlc.arg(pending_count)::integer, sqlc.narg(last_submit_at)::timestamptz,
		         (SELECT domain_id FROM contests WHERE contests.id = sqlc.arg(contest_id)::uuid))
		 ON CONFLICT (contest_id, user_id, problem_id) DO UPDATE SET
		   attempts = EXCLUDED.attempts,
		   penalty_sec = EXCLUDED.penalty_sec,
		   score = EXCLUDED.score,
		   solved_at = EXCLUDED.solved_at,
		   public_attempts = EXCLUDED.public_attempts,
		   public_penalty_sec = EXCLUDED.public_penalty_sec,
		   public_score = EXCLUDED.public_score,
		   public_solved_at = EXCLUDED.public_solved_at,
		   pending_count = EXCLUDED.pending_count,
		   last_submit_at = EXCLUDED.last_submit_at;

-- name: GetContestScoringRules :one
SELECT contest.rule, contest.begin_at, contest.end_at, contest.freeze_at,
		        contest.penalty_minutes, contest.penalize_compile_error,
		        COALESCE(problem.points, 100)::integer AS max_points
		 FROM contests AS contest
		 LEFT JOIN contest_problems AS problem
		   ON problem.contest_id = contest.id AND problem.problem_id = sqlc.arg(problem_id)::uuid
		 WHERE contest.id = sqlc.arg(contest_id)::uuid;

-- name: ListScoredSubmissions :many
SELECT submitted_at, status, score
		 FROM submissions
		 WHERE contest_id = sqlc.arg(contest_id)::uuid AND user_id = sqlc.arg(user_id)::uuid AND problem_id = sqlc.arg(problem_id)::uuid
		 ORDER BY submitted_at;

-- name: ListContestScoringTargets :many
SELECT DISTINCT user_id, problem_id FROM submissions WHERE contest_id = sqlc.arg(contest_id)::uuid;
