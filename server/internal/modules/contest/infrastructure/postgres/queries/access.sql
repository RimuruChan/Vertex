-- name: GetContestAccess :one
SELECT id,owner_id,visibility,admission,begin_at,end_at,password_hash,allow_self_registration,allow_late_registration
FROM contests WHERE id=sqlc.arg(contest_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: LockContestAccess :one
SELECT id,owner_id,visibility,admission,begin_at,end_at,password_hash,allow_self_registration,allow_late_registration
FROM contests WHERE id=sqlc.arg(contest_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid FOR UPDATE;

-- name: GetContestGrantFlags :one
SELECT COALESCE(bool_or(a.role='editor'),false)::boolean AS editor,
       COALESCE(bool_or(a.role='jury'),false)::boolean AS jury,
       COALESCE(bool_or(a.role='observer'),false)::boolean AS observer,
       COALESCE(bool_or(a.role='participant'),false)::boolean AS participant
FROM contest_access a WHERE a.domain_id=sqlc.arg(domain_id)::uuid AND a.contest_id=sqlc.arg(contest_id)::uuid
AND (a.user_id=sqlc.arg(user_id)::uuid OR a.group_id IN (
 SELECT group_id FROM domain_group_members WHERE domain_id=sqlc.arg(domain_id)::uuid AND user_id=sqlc.arg(user_id)::uuid));

-- name: HasRegistration :one
SELECT EXISTS(SELECT 1 FROM contest_participants WHERE contest_id=sqlc.arg(contest_id)::uuid AND user_id=sqlc.arg(user_id)::uuid);

-- name: GrantContestUser :exec
INSERT INTO contest_access(domain_id,contest_id,user_id,role,granted_by)
VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(contest_id)::uuid,sqlc.arg(subject_id)::uuid,sqlc.arg(role)::text,sqlc.arg(actor_id)::uuid)
ON CONFLICT(contest_id,user_id,role) WHERE user_id IS NOT NULL
DO UPDATE SET role=EXCLUDED.role,granted_by=EXCLUDED.granted_by;

-- name: GrantContestGroup :exec
INSERT INTO contest_access(domain_id,contest_id,group_id,role,granted_by)
VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(contest_id)::uuid,sqlc.arg(subject_id)::uuid,sqlc.arg(role)::text,sqlc.arg(actor_id)::uuid)
ON CONFLICT(contest_id,group_id,role) WHERE group_id IS NOT NULL
DO UPDATE SET role=EXCLUDED.role,granted_by=EXCLUDED.granted_by;
