-- name: ResolveContestNumber :one
SELECT id FROM contests WHERE domain_id=sqlc.arg(domain_id)::uuid AND public_id=sqlc.arg(public_id)::bigint;
