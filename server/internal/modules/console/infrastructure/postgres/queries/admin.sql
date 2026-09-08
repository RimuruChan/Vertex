-- name: RevokeAccountSessions :exec
UPDATE auth_sessions SET revoked_at = now()
			 WHERE user_id = sqlc.arg(user_id)::uuid AND revoked_at IS NULL;

-- name: GetAccountSummary :one
SELECT u.id, u.username, u.email, u.role, u.rating, u.created_at,
	u.disabled_at, u.disabled_reason,
	(SELECT count(*) FROM submissions AS s WHERE s.user_id = u.id)::int AS submission_count,
	(SELECT count(DISTINCT s.problem_id) FROM submissions AS s
	   WHERE s.user_id = u.id AND s.status = 'Accepted')::int AS solved_count FROM users AS u WHERE u.id = sqlc.arg(user_id)::uuid;

-- name: CountAnnouncements :one
SELECT count(*) FROM announcements a
WHERE a.domain_id=sqlc.arg(domain_id)::uuid
  AND (NOT sqlc.arg(published_only)::boolean OR a.published)
  AND (sqlc.arg(keyword)::text='' OR strpos(lower(a.title),lower(sqlc.arg(keyword)::text))>0 OR a.public_id::text=sqlc.arg(keyword)::text)
  AND (sqlc.narg(active_pinned)::boolean IS NULL OR
       (a.published AND a.pinned AND (a.pinned_until IS NULL OR a.pinned_until > now())) = sqlc.narg(active_pinned)::boolean);

-- name: ListAnnouncements :many
SELECT a.id,a.public_id::text AS public_id,a.title,a.content_md,a.pinned,a.pinned_until,a.published,a.published_at,COALESCE(u.username,'') AS author_name,a.created_at,a.updated_at
FROM announcements a LEFT JOIN users u ON u.id=a.created_by
WHERE a.domain_id=sqlc.arg(domain_id)::uuid
  AND (NOT sqlc.arg(published_only)::boolean OR a.published)
  AND (sqlc.arg(keyword)::text='' OR strpos(lower(a.title),lower(sqlc.arg(keyword)::text))>0 OR a.public_id::text=sqlc.arg(keyword)::text)
  AND (sqlc.narg(active_pinned)::boolean IS NULL OR
       (a.published AND a.pinned AND (a.pinned_until IS NULL OR a.pinned_until > now())) = sqlc.narg(active_pinned)::boolean)
ORDER BY (a.published AND a.pinned AND (a.pinned_until IS NULL OR a.pinned_until > now())) DESC,
         COALESCE(a.published_at,a.created_at) DESC,a.id DESC
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: GetAnnouncement :one
SELECT a.id,a.public_id::text AS public_id,a.title,a.content_md,a.pinned,a.pinned_until,a.published,a.published_at,COALESCE(u.username,'') AS author_name,a.created_at,a.updated_at FROM announcements a LEFT JOIN users u ON u.id=a.created_by WHERE a.id=sqlc.arg(announcement_id)::uuid AND a.domain_id=sqlc.arg(domain_id)::uuid AND (NOT sqlc.arg(published_only)::boolean OR a.published);

-- name: CreateAnnouncement :one
INSERT INTO announcements(domain_id,created_by,title,content_md,pinned,pinned_until,published,published_at)
VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(user_id)::uuid,sqlc.arg(title)::text,sqlc.arg(body)::text,
       sqlc.arg(pinned)::boolean,CASE WHEN sqlc.arg(pinned)::boolean THEN sqlc.narg(pinned_until)::timestamptz END,
       sqlc.arg(published)::boolean,CASE WHEN sqlc.arg(published)::boolean THEN now() END) RETURNING id;

-- name: UpdateAnnouncement :execrows
UPDATE announcements SET title=sqlc.arg(title)::text,content_md=sqlc.arg(body)::text,
    pinned=sqlc.arg(pinned)::boolean,
    pinned_until=CASE WHEN sqlc.arg(pinned)::boolean THEN sqlc.narg(pinned_until)::timestamptz END,
    published=sqlc.arg(published)::boolean,
    published_at=CASE WHEN sqlc.arg(published)::boolean THEN COALESCE(published_at,now()) ELSE published_at END,
    updated_at=now()
WHERE id=sqlc.arg(announcement_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: DeleteAnnouncement :execrows
DELETE FROM announcements WHERE id=sqlc.arg(announcement_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: ListTagCatalog :many
SELECT t.id,t.name,(SELECT count(*) FROM problem_tags pt WHERE pt.tag_id=t.id AND pt.domain_id=t.domain_id)::int AS problem_count FROM tags t WHERE t.domain_id=sqlc.arg(domain_id)::uuid ORDER BY t.name;

-- name: UpsertTag :one
INSERT INTO tags(domain_id,name) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(name)::text) ON CONFLICT(domain_id,name) DO UPDATE SET name=excluded.name RETURNING id;

-- name: GetTagCatalogEntry :one
SELECT t.id,t.name,(SELECT count(*) FROM problem_tags pt WHERE pt.tag_id=t.id AND pt.domain_id=t.domain_id)::int AS problem_count FROM tags t WHERE t.id=sqlc.arg(tag_id)::bigint AND t.domain_id=sqlc.arg(domain_id)::uuid;

-- name: FindTagByName :one
SELECT id FROM tags WHERE domain_id=sqlc.arg(domain_id)::uuid AND name=sqlc.arg(name)::text;

-- name: MergeProblemTags :exec
INSERT INTO problem_tags(domain_id,problem_id,tag_id) SELECT domain_id,problem_id,sqlc.arg(target_id)::bigint FROM problem_tags WHERE problem_tags.tag_id=sqlc.arg(source_id)::bigint AND problem_tags.domain_id=sqlc.arg(domain_id)::uuid ON CONFLICT(problem_id,tag_id) DO NOTHING;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id=sqlc.arg(tag_id)::bigint AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: RenameTag :exec
UPDATE tags SET name=sqlc.arg(name)::text WHERE id=sqlc.arg(tag_id)::bigint AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: RecordResourceAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(user_id)::uuid,sqlc.arg(action)::text,sqlc.arg(target)::text);

-- name: CountAccounts :one
SELECT count(*)::integer FROM users u WHERE (sqlc.arg(keyword)::text='' OR u.username ILIKE '%'||sqlc.arg(keyword)::text||'%' OR u.email ILIKE '%'||sqlc.arg(keyword)::text||'%') AND (sqlc.arg(role)::text='' OR u.role=sqlc.arg(role)::text) AND (NOT sqlc.arg(only_disabled)::boolean OR u.disabled_at IS NOT NULL);

-- name: ListAccounts :many
SELECT u.id, u.username, u.email, u.role, u.rating, u.created_at,
	u.disabled_at, u.disabled_reason,
	(SELECT count(*) FROM submissions AS s WHERE s.user_id = u.id)::int AS submission_count,
	(SELECT count(DISTINCT s.problem_id) FROM submissions AS s
	   WHERE s.user_id = u.id AND s.status = 'Accepted')::int AS solved_count FROM users u WHERE (sqlc.arg(keyword)::text='' OR u.username ILIKE '%'||sqlc.arg(keyword)::text||'%' OR u.email ILIKE '%'||sqlc.arg(keyword)::text||'%') AND (sqlc.arg(role)::text='' OR u.role=sqlc.arg(role)::text) AND (NOT sqlc.arg(only_disabled)::boolean OR u.disabled_at IS NOT NULL) ORDER BY u.created_at DESC LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: UpdateAccount :execrows
UPDATE users SET role=CASE WHEN sqlc.arg(set_role)::boolean THEN sqlc.arg(role)::text ELSE role END,
 rating=CASE WHEN sqlc.arg(set_rating)::boolean THEN sqlc.arg(rating)::integer ELSE rating END,
 disabled_at=CASE WHEN NOT sqlc.arg(set_disabled)::boolean THEN disabled_at WHEN sqlc.arg(disabled)::boolean THEN now() ELSE NULL END,
 disabled_reason=CASE WHEN NOT sqlc.arg(set_disabled)::boolean THEN disabled_reason WHEN sqlc.arg(disabled)::boolean THEN sqlc.arg(reason)::text ELSE '' END
WHERE id=sqlc.arg(user_id)::uuid;

-- name: LockTaggedWorkspaces :many
SELECT w.problem_id,w.tags_json FROM problem_workspaces w JOIN problems p ON p.id=w.problem_id
WHERE p.domain_id=sqlc.arg(domain_id)::uuid AND w.tags_json ? sqlc.arg(old_name)::text ORDER BY w.problem_id FOR UPDATE OF w;

-- name: UpdateWorkspaceTags :exec
WITH changed AS (UPDATE problem_workspaces SET tags_json=sqlc.arg(tags)::jsonb,updated_at=now()
 WHERE problem_id=sqlc.arg(problem_id)::uuid RETURNING problem_id)
UPDATE problems SET package_revision=package_revision+1,updated_at=now() WHERE id IN(SELECT problem_id FROM changed);
