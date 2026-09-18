-- name: ResolveSubmissionNumber :one
SELECT id FROM submissions WHERE domain_id=sqlc.arg(domain_id)::uuid AND public_id=sqlc.arg(public_id)::bigint;
