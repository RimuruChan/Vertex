-- name: ListContestAccessGrants :many
SELECT a.id,a.user_id,u.username,a.group_id,g.name AS group_name,a.role
	 FROM contest_access a LEFT JOIN users u ON u.id=a.user_id LEFT JOIN domain_groups g ON g.id=a.group_id
	 WHERE a.contest_id=sqlc.arg(contest_id)::uuid AND a.domain_id=sqlc.arg(domain_id)::uuid ORDER BY a.id;

-- name: ResolveContestGrantGroup :one
SELECT id FROM domain_groups WHERE domain_id=sqlc.arg(domain_id)::uuid AND (id::text=sqlc.arg(group_ref)::text OR public_id::text=sqlc.arg(group_ref)::text);

-- name: DeleteContestGrant :execrows
DELETE FROM contest_access WHERE contest_id=sqlc.arg(contest_id)::uuid AND id=sqlc.arg(grant_id)::bigint AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: TransferContestOwner :exec
UPDATE contests SET owner_id=sqlc.arg(owner_id)::uuid WHERE contests.id =sqlc.arg(contest_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: DeleteContestOwnerGrants :exec
DELETE FROM contest_access WHERE contest_id=sqlc.arg(contest_id)::uuid AND user_id=sqlc.arg(user_id)::uuid;

-- name: HasContestReferences :one
SELECT COALESCE(EXISTS(SELECT 1 FROM contest_participants WHERE contest_participants.contest_id=sqlc.arg(contest_id)::uuid)
	 OR EXISTS(SELECT 1 FROM submissions WHERE submissions.contest_id=sqlc.arg(contest_id)::uuid)
	 OR EXISTS(SELECT 1 FROM clarifications WHERE clarifications.contest_id=sqlc.arg(contest_id)::uuid)
	 OR EXISTS(SELECT 1 FROM rejudgings WHERE rejudgings.contest_id=sqlc.arg(contest_id)::uuid),false)::boolean AS referenced;

-- name: DeleteContest :exec
DELETE FROM contests WHERE contests.id =sqlc.arg(contest_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: GetContestProblemVersion :one
SELECT problem_version FROM contest_problems WHERE contest_id=sqlc.arg(contest_id)::uuid AND problem_id=sqlc.arg(problem_id)::uuid FOR UPDATE;

-- name: GetProblemVersionLimits :one
SELECT EXISTS(SELECT 1 FROM problem_versions WHERE problem_id=sqlc.arg(problem_id)::uuid AND version_no=sqlc.arg(version)::integer);

-- name: SetContestProblemVersion :exec
UPDATE contest_problems SET problem_version=sqlc.arg(version)::integer WHERE contest_id=sqlc.arg(contest_id)::uuid AND problem_id=sqlc.arg(problem_id)::uuid;

-- name: RecordContestVersionAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(actor_id)::uuid,'contest.problem.version',sqlc.arg(target)::text);
