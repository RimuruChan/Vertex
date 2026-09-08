-- name: LockProblemSetAccess :one
SELECT id,owner_id,visibility FROM problem_sets WHERE id=sqlc.arg(set_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid FOR UPDATE;

-- name: RecordProblemSetAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(actor_id)::uuid,sqlc.arg(action)::text,sqlc.arg(target)::text);

-- name: ListProblemSetGrants :many
SELECT a.id,a.user_id,u.username,a.group_id,g.name AS group_name,a.role
	 FROM problem_set_access a LEFT JOIN users u ON u.id=a.user_id LEFT JOIN domain_groups g ON g.id=a.group_id
	 WHERE a.domain_id=sqlc.arg(domain_id)::uuid AND a.set_id=sqlc.arg(set_id)::uuid ORDER BY a.id;

-- name: FindProblemSetCollaborator :one
SELECT m.user_id FROM domain_members m JOIN users u ON u.id=m.user_id
	 WHERE m.domain_id=sqlc.arg(domain_id)::uuid AND u.username=sqlc.arg(username)::text AND m.status='active' AND u.disabled_at IS NULL FOR SHARE OF u;

-- name: ResolveProblemSetGrantGroup :one
SELECT id FROM domain_groups WHERE domain_id=sqlc.arg(domain_id)::uuid AND (id::text=sqlc.arg(group_ref)::text OR public_id::text=sqlc.arg(group_ref)::text);

-- name: DeleteProblemSetGrant :execrows
DELETE FROM problem_set_access WHERE set_id=sqlc.arg(set_id)::uuid AND id=sqlc.arg(grant_id)::bigint AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: TransferProblemSetOwner :exec
UPDATE problem_sets SET owner_id=sqlc.arg(owner_id)::uuid,updated_at=now() WHERE id=sqlc.arg(set_id)::uuid;

-- name: DeleteProblemSetOwnerGrants :exec
DELETE FROM problem_set_access WHERE set_id=sqlc.arg(set_id)::uuid AND user_id=sqlc.arg(user_id)::uuid;

-- name: CreateProblemSet :one
INSERT INTO problem_sets(title,description,visibility,author_id,owner_id,domain_id)
	 VALUES(sqlc.arg(title)::text,sqlc.arg(description)::text,sqlc.arg(visibility)::text,sqlc.arg(author_id)::uuid,sqlc.arg(author_id)::uuid,sqlc.arg(domain_id)::uuid) RETURNING id;

-- name: UpdateProblemSet :exec
UPDATE problem_sets SET title=sqlc.arg(title)::text,description=sqlc.arg(description)::text,visibility=sqlc.arg(visibility)::text,updated_at=now() WHERE id=sqlc.arg(set_id)::uuid;

-- name: DeleteProblemSet :exec
DELETE FROM problem_sets WHERE id=sqlc.arg(set_id)::uuid;

-- name: ListProblemSetProblemIDs :many
SELECT problem_id FROM problem_set_problems WHERE set_id=sqlc.arg(set_id)::uuid;

-- name: DeleteProblemSetItems :exec
DELETE FROM problem_set_problems WHERE set_id=sqlc.arg(set_id)::uuid;

-- name: InsertProblemSetItem :exec
INSERT INTO problem_set_problems(domain_id,set_id,problem_id,sort_order,note) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(set_id)::uuid,sqlc.arg(problem_id)::uuid,sqlc.arg(sort_order)::integer,sqlc.arg(note)::text);

-- name: TouchProblemSet :exec
UPDATE problem_sets SET updated_at=now() WHERE id=sqlc.arg(set_id)::uuid;

-- name: GrantSetUser :exec
INSERT INTO problem_set_access(domain_id,set_id,user_id,role,granted_by)
VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(set_id)::uuid,sqlc.arg(subject_id)::uuid,sqlc.arg(role)::text,sqlc.arg(actor_id)::uuid)
ON CONFLICT(set_id,user_id) WHERE user_id IS NOT NULL DO UPDATE SET role=EXCLUDED.role,granted_by=EXCLUDED.granted_by;

-- name: GrantSetGroup :exec
INSERT INTO problem_set_access(domain_id,set_id,group_id,role,granted_by)
VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(set_id)::uuid,sqlc.arg(subject_id)::uuid,sqlc.arg(role)::text,sqlc.arg(actor_id)::uuid)
ON CONFLICT(set_id,group_id) WHERE group_id IS NOT NULL DO UPDATE SET role=EXCLUDED.role,granted_by=EXCLUDED.granted_by;
