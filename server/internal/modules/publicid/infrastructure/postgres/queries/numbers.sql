-- name: ResolveProblemNumber :one
SELECT id FROM problems
WHERE domain_id = sqlc.arg(domain_id)::uuid AND public_id = sqlc.arg(public_id)::bigint;

-- name: ResolveContestNumber :one
SELECT id FROM contests
WHERE domain_id = sqlc.arg(domain_id)::uuid AND public_id = sqlc.arg(public_id)::bigint;

-- name: ResolveSubmissionNumber :one
SELECT id FROM submissions
WHERE domain_id = sqlc.arg(domain_id)::uuid AND public_id = sqlc.arg(public_id)::bigint;

-- name: ResolveEditorialNumber :one
SELECT id FROM editorials
WHERE domain_id = sqlc.arg(domain_id)::uuid AND public_id = sqlc.arg(public_id)::bigint;

-- name: ResolveProblemSetNumber :one
SELECT id FROM problem_sets
WHERE domain_id = sqlc.arg(domain_id)::uuid AND public_id = sqlc.arg(public_id)::bigint;

-- name: ResolveAnnouncementNumber :one
SELECT id FROM announcements
WHERE domain_id = sqlc.arg(domain_id)::uuid AND public_id = sqlc.arg(public_id)::bigint;
