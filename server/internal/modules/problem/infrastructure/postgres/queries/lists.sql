-- name: CountPublicProblems :one
SELECT count(*)
FROM problems p JOIN problem_workspaces w ON w.problem_id=p.id
LEFT JOIN LATERAL (SELECT COALESCE(max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END),0)::integer AS grant_rank
 FROM problem_access a WHERE a.problem_id=p.id AND a.domain_id=p.domain_id
 AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (
 SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) grants ON true
WHERE p.domain_id=sqlc.arg(domain_id)::uuid AND p.published_version IS NOT NULL AND (p.visibility='public' OR (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (p.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.grant_rank>0))))
AND (sqlc.arg(visibility)::text='' OR p.visibility=sqlc.arg(visibility)::text)
AND (sqlc.arg(difficulty)::integer<=0 OR p.difficulty=sqlc.arg(difficulty)::integer)
AND (sqlc.arg(keyword)::text='' OR p.title ILIKE '%'||sqlc.arg(keyword)::text||'%' OR p.source ILIKE '%'||sqlc.arg(keyword)::text||'%')
AND (sqlc.arg(tag)::text='' OR EXISTS(SELECT 1 FROM problem_tags pt JOIN tags t ON t.id=pt.tag_id WHERE pt.problem_id=p.id AND t.name=sqlc.arg(tag)::text))
AND (sqlc.arg(viewer_id)::text='' OR sqlc.arg(status)::text='' OR
 (sqlc.arg(status)::text='solved' AND EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL AND sub.status='Accepted')) OR
 (sqlc.arg(status)::text='attempted' AND EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL) AND NOT EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL AND sub.status='Accepted')) OR
 (sqlc.arg(status)::text='none' AND NOT EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL)));

-- name: ListPublicProblems :many
SELECT p.id,p.public_id,p.title,''::text AS statement_md,p.difficulty,p.source,
 p.time_limit_ms,p.memory_limit_kb,p.visibility,p.author_id,p.submission_count,p.accepted_count,p.solved_user_count,
 p.judge_type,p.created_at,p.updated_at,p.owner_id,p.domain_id,COALESCE(p.published_version,0)::integer AS published_version,
 COALESCE((SELECT jsonb_agg(t.name ORDER BY t.name) FROM problem_tags pt JOIN tags t ON t.id=pt.tag_id WHERE pt.problem_id=p.id),'[]'::jsonb)::jsonb AS tags,COALESCE((SELECT u.username FROM users u WHERE u.id=p.owner_id),'')::text AS owner_name,grants.grant_rank AS grant_rank
FROM problems p JOIN problem_workspaces w ON w.problem_id=p.id
LEFT JOIN LATERAL (SELECT COALESCE(max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END),0)::integer AS grant_rank
 FROM problem_access a WHERE a.problem_id=p.id AND a.domain_id=p.domain_id
 AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (
 SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) grants ON true
WHERE p.domain_id=sqlc.arg(domain_id)::uuid AND p.published_version IS NOT NULL AND (p.visibility='public' OR (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (p.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.grant_rank>0))))
AND (sqlc.arg(visibility)::text='' OR p.visibility=sqlc.arg(visibility)::text)
AND (sqlc.arg(difficulty)::integer<=0 OR p.difficulty=sqlc.arg(difficulty)::integer)
AND (sqlc.arg(keyword)::text='' OR p.title ILIKE '%'||sqlc.arg(keyword)::text||'%' OR p.source ILIKE '%'||sqlc.arg(keyword)::text||'%')
AND (sqlc.arg(tag)::text='' OR EXISTS(SELECT 1 FROM problem_tags pt JOIN tags t ON t.id=pt.tag_id WHERE pt.problem_id=p.id AND t.name=sqlc.arg(tag)::text))
AND (sqlc.arg(viewer_id)::text='' OR sqlc.arg(status)::text='' OR
 (sqlc.arg(status)::text='solved' AND EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL AND sub.status='Accepted')) OR
 (sqlc.arg(status)::text='attempted' AND EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL) AND NOT EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL AND sub.status='Accepted')) OR
 (sqlc.arg(status)::text='none' AND NOT EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL)))
ORDER BY p.created_at DESC,p.id DESC LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: CountWorkspaceProblems :one
SELECT count(*)
FROM problems p JOIN problem_workspaces w ON w.problem_id=p.id
LEFT JOIN LATERAL (SELECT COALESCE(max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END),0)::integer AS grant_rank
 FROM problem_access a WHERE a.problem_id=p.id AND a.domain_id=p.domain_id
 AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (
 SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) grants ON true
WHERE p.domain_id=sqlc.arg(domain_id)::uuid AND (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (p.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.grant_rank>0)))
AND (sqlc.arg(visibility)::text='' OR p.visibility=sqlc.arg(visibility)::text)
AND (sqlc.arg(difficulty)::integer<=0 OR w.difficulty=sqlc.arg(difficulty)::integer)
AND (sqlc.arg(keyword)::text='' OR w.title ILIKE '%'||sqlc.arg(keyword)::text||'%' OR w.source ILIKE '%'||sqlc.arg(keyword)::text||'%')
AND (sqlc.arg(tag)::text='' OR w.tags_json @> jsonb_build_array(sqlc.arg(tag)::text))
AND (sqlc.arg(viewer_id)::text='' OR sqlc.arg(status)::text='' OR
 (sqlc.arg(status)::text='solved' AND EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL AND sub.status='Accepted')) OR
 (sqlc.arg(status)::text='attempted' AND EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL) AND NOT EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL AND sub.status='Accepted')) OR
 (sqlc.arg(status)::text='none' AND NOT EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL)));

-- name: ListWorkspaceProblems :many
SELECT p.id,p.public_id,w.title,''::text AS statement_md,w.difficulty,w.source,
 w.time_limit_ms,w.memory_limit_kb,p.visibility,p.author_id,p.submission_count,p.accepted_count,p.solved_user_count,
 w.judge_type,p.created_at,w.updated_at,p.owner_id,p.domain_id,COALESCE(p.published_version,0)::integer AS published_version,
 w.tags_json AS tags,COALESCE((SELECT u.username FROM users u WHERE u.id=p.owner_id),'')::text AS owner_name,grants.grant_rank AS grant_rank
FROM problems p JOIN problem_workspaces w ON w.problem_id=p.id
LEFT JOIN LATERAL (SELECT COALESCE(max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END),0)::integer AS grant_rank
 FROM problem_access a WHERE a.problem_id=p.id AND a.domain_id=p.domain_id
 AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (
 SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) grants ON true
WHERE p.domain_id=sqlc.arg(domain_id)::uuid AND (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (p.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.grant_rank>0)))
AND (sqlc.arg(visibility)::text='' OR p.visibility=sqlc.arg(visibility)::text)
AND (sqlc.arg(difficulty)::integer<=0 OR w.difficulty=sqlc.arg(difficulty)::integer)
AND (sqlc.arg(keyword)::text='' OR w.title ILIKE '%'||sqlc.arg(keyword)::text||'%' OR w.source ILIKE '%'||sqlc.arg(keyword)::text||'%')
AND (sqlc.arg(tag)::text='' OR w.tags_json @> jsonb_build_array(sqlc.arg(tag)::text))
AND (sqlc.arg(viewer_id)::text='' OR sqlc.arg(status)::text='' OR
 (sqlc.arg(status)::text='solved' AND EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL AND sub.status='Accepted')) OR
 (sqlc.arg(status)::text='attempted' AND EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL) AND NOT EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL AND sub.status='Accepted')) OR
 (sqlc.arg(status)::text='none' AND NOT EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL)))
ORDER BY p.created_at DESC,p.id DESC LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;
