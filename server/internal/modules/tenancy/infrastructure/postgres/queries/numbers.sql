-- name: ResolveGroupNumber :one
SELECT id FROM domain_groups WHERE domain_id=sqlc.arg(domain_id)::uuid AND public_id=sqlc.arg(public_id)::bigint;
