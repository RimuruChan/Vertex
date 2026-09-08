-- name: ListContestProblems :many
SELECT cp.contest_id,c.public_id AS contest_public_id,cp.problem_id,p.public_id AS problem_public_id,cp.sort_order,cp.label,cp.color,cp.points,v.title,v.difficulty,p.visibility,v.tags_json,cp.problem_version
 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id JOIN problems p ON p.id=cp.problem_id
 JOIN problem_versions v ON v.problem_id=cp.problem_id AND v.version_no=cp.problem_version
 WHERE cp.contest_id=sqlc.arg(contest_id)::uuid AND cp.domain_id=sqlc.arg(domain_id)::uuid ORDER BY cp.sort_order;

-- name: ListContestProblemVersions :many
SELECT problem_id,problem_version FROM contest_problems WHERE contest_id=sqlc.arg(contest_id)::uuid;

-- name: DeleteContestProblems :exec
DELETE FROM contest_problems WHERE contest_id = sqlc.arg(contest_id)::uuid;

-- name: InsertContestProblem :exec
INSERT INTO contest_problems (contest_id, problem_id, sort_order, label, color, points, domain_id,problem_version)
			 VALUES (sqlc.arg(contest_id)::uuid, sqlc.arg(problem_id)::uuid, sqlc.arg(sort_order)::integer, sqlc.arg(label)::text, sqlc.arg(color)::text, sqlc.arg(points)::integer, sqlc.arg(domain_id)::uuid,sqlc.arg(problem_version)::integer);

-- name: DeleteRemovedContestCells :exec
DELETE FROM contest_submission_cells
		 WHERE contest_submission_cells.contest_id = sqlc.arg(contest_id)::uuid AND problem_id NOT IN (
		   SELECT problem_id FROM contest_problems WHERE contest_problems.contest_id = sqlc.arg(contest_id)::uuid);

-- name: IsContestParticipant :one
SELECT EXISTS (SELECT 1 FROM contest_participants AS participant
		 JOIN contests AS contest ON contest.id = participant.contest_id
		 WHERE contest_id = sqlc.arg(contest_id)::uuid AND user_id = sqlc.arg(user_id)::uuid AND contest.domain_id = sqlc.arg(domain_id)::uuid);

-- name: RegisterContestParticipant :exec
INSERT INTO contest_participants (contest_id, user_id)
		 SELECT id, sqlc.arg(user_id)::uuid FROM contests WHERE contests.id = sqlc.arg(contest_id)::uuid AND contests.domain_id = sqlc.arg(domain_id)::uuid
		 ON CONFLICT (contest_id, user_id) DO NOTHING;

-- name: HasContestProblem :one
SELECT EXISTS (
		   SELECT 1 FROM contest_problems WHERE contest_id = sqlc.arg(contest_id)::uuid AND problem_id = sqlc.arg(problem_id)::uuid AND domain_id = sqlc.arg(domain_id)::uuid
		);

-- name: GetContestProblemByID :one
SELECT cp.contest_id,c.public_id AS contest_public_id,cp.problem_id,p.public_id AS problem_public_id,cp.sort_order,cp.label,cp.color,cp.points,
v.title,v.difficulty,p.visibility,v.tags_json,v.statement_md,v.source,v.time_limit_ms,v.memory_limit_kb,v.judge_type,cp.problem_version
FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id JOIN problems p ON p.id=cp.problem_id
JOIN problem_versions v ON v.problem_id=cp.problem_id AND v.version_no=cp.problem_version
WHERE cp.contest_id=sqlc.arg(contest_id)::uuid AND cp.domain_id=sqlc.arg(domain_id)::uuid AND cp.problem_id=sqlc.arg(problem_id)::uuid;

-- name: GetContestProblemByNumber :one
SELECT cp.contest_id,c.public_id AS contest_public_id,cp.problem_id,p.public_id AS problem_public_id,cp.sort_order,cp.label,cp.color,cp.points,
v.title,v.difficulty,p.visibility,v.tags_json,v.statement_md,v.source,v.time_limit_ms,v.memory_limit_kb,v.judge_type,cp.problem_version
FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id JOIN problems p ON p.id=cp.problem_id
JOIN problem_versions v ON v.problem_id=cp.problem_id AND v.version_no=cp.problem_version
WHERE cp.contest_id=sqlc.arg(contest_id)::uuid AND cp.domain_id=sqlc.arg(domain_id)::uuid AND p.public_id=sqlc.arg(public_id)::bigint;

-- name: GetContestProblemByLabel :one
SELECT cp.contest_id,c.public_id AS contest_public_id,cp.problem_id,p.public_id AS problem_public_id,cp.sort_order,cp.label,cp.color,cp.points,
v.title,v.difficulty,p.visibility,v.tags_json,v.statement_md,v.source,v.time_limit_ms,v.memory_limit_kb,v.judge_type,cp.problem_version
FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id JOIN problems p ON p.id=cp.problem_id
JOIN problem_versions v ON v.problem_id=cp.problem_id AND v.version_no=cp.problem_version
WHERE cp.contest_id=sqlc.arg(contest_id)::uuid AND cp.domain_id=sqlc.arg(domain_id)::uuid AND cp.label=sqlc.arg(label)::text;
