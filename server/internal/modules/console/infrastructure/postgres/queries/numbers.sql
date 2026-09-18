-- name: ResolveAnnouncementNumber :one
SELECT id FROM announcements WHERE domain_id=sqlc.arg(domain_id)::uuid AND public_id=sqlc.arg(public_id)::bigint;
