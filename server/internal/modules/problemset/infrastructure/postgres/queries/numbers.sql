-- name: ResolveProblemSetNumber :one
SELECT id FROM problem_sets WHERE domain_id=sqlc.arg(domain_id)::uuid AND public_id=sqlc.arg(public_id)::bigint;
