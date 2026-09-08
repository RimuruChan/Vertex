-- name: GetContestStaffRole :one
SELECT role FROM contest_staff WHERE contest_id = sqlc.arg(contest_id)::uuid AND user_id = sqlc.arg(user_id)::uuid
		 AND EXISTS (SELECT 1 FROM contests WHERE contests.id = sqlc.arg(contest_id)::uuid AND contests.domain_id = sqlc.arg(domain_id)::uuid);

-- name: ListContestStaff :many
SELECT staff.contest_id, staff.user_id, u.username, staff.role, staff.created_at::timestamptz AS created_at
		 FROM contest_staff AS staff JOIN users u ON u.id = staff.user_id
		 WHERE staff.contest_id = sqlc.arg(contest_id)::uuid AND EXISTS (SELECT 1 FROM contests WHERE contests.id = sqlc.arg(contest_id)::uuid AND domain_id = sqlc.arg(domain_id)::uuid)
		 ORDER BY staff.role, u.username;

-- name: ClearContestStaffRoles :exec
DELETE FROM contest_access WHERE contest_id=sqlc.arg(contest_id)::uuid AND user_id=sqlc.arg(user_id)::uuid AND role IN ('jury','observer');

-- name: GrantContestStaffRole :one
INSERT INTO contest_access(domain_id,contest_id,user_id,role,granted_by)
	 VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(contest_id)::uuid,sqlc.arg(user_id)::uuid,sqlc.arg(role)::text,sqlc.arg(actor_id)::uuid) RETURNING contest_id,user_id,role,created_at;

-- name: RemoveContestStaff :execrows
DELETE FROM contest_access WHERE contest_id = sqlc.arg(contest_id)::uuid AND user_id = sqlc.arg(user_id)::uuid AND role IN ('jury','observer')
		 AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: FindContestCollaborator :one
SELECT m.user_id FROM domain_members m JOIN users u ON u.id=m.user_id
	 WHERE m.domain_id=sqlc.arg(domain_id)::uuid AND u.username=sqlc.arg(username)::text AND m.status='active' AND u.disabled_at IS NULL
	 FOR SHARE OF u;

-- name: RecordContestAccessAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(actor_id)::uuid,sqlc.arg(action)::text,sqlc.arg(target)::text);
