-- name: GetProblem :one
SELECT p.id,p.public_id,p.title,p.statement_md,p.difficulty,p.source,
 p.time_limit_ms,p.memory_limit_kb,p.visibility,p.author_id,p.submission_count,p.accepted_count,p.solved_user_count,
 p.judge_type,p.created_at,p.updated_at,p.owner_id,p.domain_id,COALESCE(p.published_version,0)::integer AS published_version,
 COALESCE((SELECT jsonb_agg(t.name ORDER BY t.name) FROM problem_tags pt JOIN tags t ON t.id=pt.tag_id WHERE pt.problem_id=p.id),'[]'::jsonb)::jsonb AS tags,COALESCE((SELECT u.username FROM users u WHERE u.id=p.owner_id),'')::text AS owner_name,0::integer AS grant_rank
FROM problems p WHERE p.id=sqlc.arg(problem_id)::uuid AND p.domain_id=sqlc.arg(domain_id)::uuid;

-- name: GetProblemWorkspace :one
SELECT p.id,p.public_id,p.title,p.statement_md,p.difficulty,p.source,
 p.time_limit_ms,p.memory_limit_kb,p.visibility,p.author_id,p.submission_count,p.accepted_count,p.solved_user_count,
 p.judge_type,p.created_at,p.updated_at,p.owner_id,p.domain_id,COALESCE(p.published_version,0)::integer AS published_version,
 COALESCE((SELECT jsonb_agg(t.name ORDER BY t.name) FROM problem_tags pt JOIN tags t ON t.id=pt.tag_id WHERE pt.problem_id=p.id),'[]'::jsonb)::jsonb AS tags,COALESCE((SELECT u.username FROM users u WHERE u.id=p.owner_id),'')::text AS owner_name,0::integer AS grant_rank
FROM problems p WHERE p.id=sqlc.arg(problem_id)::uuid AND p.domain_id=sqlc.arg(domain_id)::uuid;

-- name: LockPracticeCounters :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text, 0));

-- name: RebuildPracticeCounters :exec
UPDATE problems SET
		   submission_count = (SELECT count(*) FROM submission_results
		     WHERE submission_results.problem_id = sqlc.arg(problem_id)::uuid AND contest_id IS NULL AND judged_at IS NOT NULL),
		   accepted_count = (SELECT count(*) FROM submission_results
		     WHERE submission_results.problem_id = sqlc.arg(problem_id)::uuid AND contest_id IS NULL AND status = 'Accepted'),
		   solved_user_count = (SELECT count(DISTINCT user_id) FROM submission_results
		     WHERE submission_results.problem_id = sqlc.arg(problem_id)::uuid AND contest_id IS NULL AND status = 'Accepted')
		 WHERE id = sqlc.arg(problem_id)::uuid;

-- name: GetPublishedTestdata :one
SELECT v.problem_id,v.testdata_path,v.sha256,v.case_count,v.checker,v.spj_source,v.config_json
		 FROM problem_versions v JOIN problems p ON p.id=v.problem_id AND p.published_version=v.version_no
		 WHERE p.id=sqlc.arg(problem_id)::uuid AND p.domain_id=sqlc.arg(domain_id)::uuid;

-- name: ListUserProblemStatuses :many
SELECT problem_id, bool_or(status = 'Accepted') AS solved
		 FROM submission_results
		 WHERE user_id = sqlc.arg(viewer_id)::uuid AND problem_id = ANY(sqlc.arg(problem_ids)::uuid[]) AND contest_id IS NULL AND domain_id = sqlc.arg(domain_id)::uuid
		 GROUP BY problem_id;

-- name: ListPublicProblemTags :many
SELECT t.name, count(*)::int AS problem_count
		 FROM tags t
		 JOIN problem_tags pt ON pt.tag_id = t.id
		 JOIN problems p ON p.id = pt.problem_id AND p.visibility = 'public' AND p.published_version IS NOT NULL
		 WHERE p.domain_id = sqlc.arg(domain_id)::uuid
		 GROUP BY t.name
		 ORDER BY problem_count DESC, t.name ASC;

-- name: GetProblemGrantRank :one
SELECT COALESCE(max(CASE WHEN role='editor' THEN 2 ELSE 1 END),0)::integer
		 FROM problem_access a WHERE a.domain_id=sqlc.arg(domain_id)::uuid AND a.problem_id=sqlc.arg(problem_id)::uuid
		 AND (a.user_id=sqlc.arg(viewer_id)::uuid OR a.group_id IN (
		   SELECT group_id FROM domain_group_members WHERE domain_id=sqlc.arg(domain_id)::uuid AND user_id=sqlc.arg(viewer_id)::uuid));

-- name: CreateProblem :one
INSERT INTO problems AS p (title, statement_md, difficulty, source,
		                      time_limit_ms, memory_limit_kb, visibility, author_id, owner_id, domain_id)
		 VALUES (sqlc.arg(title)::text, sqlc.arg(statement_md)::text, sqlc.arg(difficulty)::integer, sqlc.arg(source)::text, sqlc.arg(time_limit_ms)::integer, sqlc.arg(memory_limit_kb)::integer, sqlc.arg(visibility)::text, sqlc.arg(author_id)::uuid, sqlc.arg(author_id)::uuid, sqlc.arg(domain_id)::uuid)
		 RETURNING p.id,p.public_id,p.title,p.statement_md,p.difficulty,p.source,
 p.time_limit_ms,p.memory_limit_kb,p.visibility,p.author_id,p.submission_count,p.accepted_count,p.solved_user_count,
 p.judge_type,p.created_at,p.updated_at,p.owner_id,p.domain_id,COALESCE(p.published_version,0)::integer AS published_version,
 COALESCE((SELECT jsonb_agg(t.name ORDER BY t.name) FROM problem_tags pt JOIN tags t ON t.id=pt.tag_id WHERE pt.problem_id=p.id),'[]'::jsonb)::jsonb AS tags,COALESCE((SELECT u.username FROM users u WHERE u.id=p.owner_id),'')::text AS owner_name,0::integer AS grant_rank;

-- name: HasProblemReferences :one
SELECT COALESCE(EXISTS(SELECT 1 FROM submission_results WHERE submission_results.problem_id =sqlc.arg(problem_id)::uuid) OR EXISTS(SELECT 1 FROM contest_problems WHERE contest_problems.problem_id =sqlc.arg(problem_id)::uuid),false)::boolean AS referenced;

-- name: DeleteProblem :execrows
DELETE FROM problems WHERE id = sqlc.arg(problem_id)::uuid AND domain_id = sqlc.arg(domain_id)::uuid;

-- name: ListProblemGrants :many
SELECT a.id,a.user_id,u.username,a.group_id,g.name AS group_name,COALESCE(g.public_id::text,'')::text AS group_number,a.role
	 FROM problem_access a LEFT JOIN users u ON u.id=a.user_id LEFT JOIN domain_groups g ON g.id=a.group_id
	 WHERE a.problem_id=sqlc.arg(problem_id)::uuid AND a.domain_id=sqlc.arg(domain_id)::uuid ORDER BY a.id;

-- name: FindProblemCollaborator :one
SELECT m.user_id FROM domain_members m JOIN users u ON u.id=m.user_id
	 WHERE m.domain_id=sqlc.arg(domain_id)::uuid AND u.username=sqlc.arg(username)::text AND m.status='active' AND u.disabled_at IS NULL
	 FOR SHARE OF u;

-- name: RecordProblemAccessAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(actor_id)::uuid,sqlc.arg(action)::text,sqlc.arg(target)::text);

-- name: ResolveProblemGrantGroup :one
SELECT id FROM domain_groups WHERE domain_id=sqlc.arg(domain_id)::uuid AND (id::text=sqlc.arg(group_ref)::text OR public_id::text=sqlc.arg(group_ref)::text);

-- name: DeleteProblemGrant :execrows
DELETE FROM problem_access WHERE problem_id=sqlc.arg(problem_id)::uuid AND id=sqlc.arg(grant_id)::bigint AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: TransferProblemOwner :exec
UPDATE problems SET owner_id=sqlc.arg(owner_id)::uuid,updated_at=now() WHERE id=sqlc.arg(problem_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: DeleteProblemOwnerGrants :exec
DELETE FROM problem_access WHERE problem_id=sqlc.arg(problem_id)::uuid AND user_id=sqlc.arg(user_id)::uuid;

-- name: GetProblemAccess :one
SELECT id,owner_id,visibility,COALESCE(published_version,0)::integer AS published_version FROM problems WHERE id=sqlc.arg(problem_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: LockProblemAccess :one
SELECT id,owner_id,visibility,COALESCE(published_version,0)::integer AS published_version FROM problems WHERE id=sqlc.arg(problem_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid FOR UPDATE;

-- name: GrantProblemUser :exec
INSERT INTO problem_access(domain_id,problem_id,user_id,role,granted_by)
VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(problem_id)::uuid,sqlc.arg(subject_id)::uuid,sqlc.arg(role)::text,sqlc.arg(actor_id)::uuid)
ON CONFLICT(problem_id,user_id) WHERE user_id IS NOT NULL DO UPDATE SET role=EXCLUDED.role,granted_by=EXCLUDED.granted_by;

-- name: GrantProblemGroup :exec
INSERT INTO problem_access(domain_id,problem_id,group_id,role,granted_by)
VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(problem_id)::uuid,sqlc.arg(subject_id)::uuid,sqlc.arg(role)::text,sqlc.arg(actor_id)::uuid)
ON CONFLICT(problem_id,group_id) WHERE group_id IS NOT NULL DO UPDATE SET role=EXCLUDED.role,granted_by=EXCLUDED.granted_by;

-- name: CreateProblemTags :exec
WITH names AS (SELECT DISTINCT item.name FROM jsonb_array_elements_text(sqlc.arg(tags)::jsonb) AS item(name) WHERE item.name<>''),
added AS (INSERT INTO tags(domain_id,name) SELECT sqlc.arg(domain_id)::uuid,name FROM names ORDER BY name
 ON CONFLICT(domain_id,name) DO UPDATE SET name=EXCLUDED.name RETURNING id)
INSERT INTO problem_tags(domain_id,problem_id,tag_id)
SELECT sqlc.arg(domain_id)::uuid,sqlc.arg(problem_id)::uuid,id FROM added;
