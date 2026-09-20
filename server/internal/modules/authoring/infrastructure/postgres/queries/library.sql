-- name: ListAuthoringLibrary :many
WITH visible AS (
 SELECT p.public_id,p.owner_id,p.visibility,COALESCE(u.username,'')::text AS owner_name,
 COALESCE(NULLIF(t.summary->>'title',''),p.title)::text AS title,
 COALESCE(t.summary->>'source',p.source)::text AS source,
 grants.grant_rank,
 (w.actor_id IS NOT NULL)::boolean AS has_copy,
 COALESCE(w.tree_hash IS DISTINCT FROM COALESCE(base.tree_hash,h.initial_tree_hash) AND w.actor_id IS NOT NULL,false)::boolean AS has_changes,
 EXISTS(SELECT 1 FROM problem_merge_sessions m WHERE m.problem_id=p.id AND m.actor_id=NULLIF(sqlc.arg(actor_id)::text,'')::uuid)::boolean AS has_conflict,
 COALESCE(w.base_revision,0)::bigint AS base_revision,COALESCE(h.revision,0)::bigint AS head_revision,
 COALESCE(p.published_version,0)::integer AS published_version,COALESCE(v.source_revision,0)::bigint AS published_revision,
 COALESCE(check_run.id::text,'')::text AS check_id,COALESCE(check_run.state,'')::text AS check_state,
 COALESCE(check_run.source_tree_hash=t.tree_hash,false)::boolean AS check_matches,
 GREATEST(p.updated_at,w.updated_at,head_commit.created_at)::timestamptz AS updated_at
 FROM problems p
 LEFT JOIN users u ON u.id=p.owner_id
 LEFT JOIN problem_authoring_heads h ON h.problem_id=p.id
 LEFT JOIN problem_commits head_commit ON head_commit.problem_id=p.id AND head_commit.revision=h.revision
 LEFT JOIN problem_working_copies w ON w.problem_id=p.id AND w.actor_id=NULLIF(sqlc.arg(actor_id)::text,'')::uuid
 LEFT JOIN problem_commits base ON base.problem_id=p.id AND base.revision=w.base_revision
 LEFT JOIN problem_content_trees t ON t.problem_id=p.id AND t.tree_hash=COALESCE(w.tree_hash,h.tree_hash,h.initial_tree_hash)
 LEFT JOIN problem_versions v ON v.problem_id=p.id AND v.version_no=p.published_version
 LEFT JOIN LATERAL (SELECT COALESCE(max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END),0)::integer AS grant_rank
   FROM problem_access a WHERE a.problem_id=p.id AND a.domain_id=p.domain_id
   AND (a.user_id=NULLIF(sqlc.arg(actor_id)::text,'')::uuid OR a.group_id IN (
     SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(actor_id)::text,'')::uuid))) grants ON true
 LEFT JOIN LATERAL (SELECT b.id,b.state,b.source_tree_hash FROM problem_build_jobs b
   WHERE b.problem_id=p.id AND b.source_tree_hash IS NOT NULL
   AND (b.created_by=NULLIF(sqlc.arg(actor_id)::text,'')::uuid OR b.source_revision IS NOT NULL OR EXISTS(
     SELECT 1 FROM problem_commits c WHERE c.problem_id=b.problem_id AND c.tree_hash=b.source_tree_hash))
   ORDER BY b.created_at DESC,b.id DESC LIMIT 1) check_run ON true
 WHERE p.domain_id=sqlc.arg(domain_id)::uuid
 AND (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (p.owner_id=NULLIF(sqlc.arg(actor_id)::text,'')::uuid OR grants.grant_rank>0)))
)
SELECT visible.*,count(*) OVER()::bigint AS total FROM visible
WHERE (sqlc.arg(visibility)::text='' OR visible.visibility=sqlc.arg(visibility)::text)
AND (sqlc.arg(keyword)::text='' OR title ILIKE '%'||sqlc.arg(keyword)::text||'%' OR source ILIKE '%'||sqlc.arg(keyword)::text||'%' OR public_id::text=sqlc.arg(keyword)::text)
AND (sqlc.arg(status)::text='' OR (sqlc.arg(status)::text='changes' AND has_changes) OR (sqlc.arg(status)::text='conflicts' AND has_conflict) OR (sqlc.arg(status)::text='unpublished' AND published_version=0))
ORDER BY updated_at DESC,public_id DESC LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;
