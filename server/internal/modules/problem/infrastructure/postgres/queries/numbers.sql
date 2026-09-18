-- name: ResolveProblemNumber :one
SELECT id FROM problems WHERE domain_id=sqlc.arg(domain_id)::uuid AND public_id=sqlc.arg(public_id)::bigint;
