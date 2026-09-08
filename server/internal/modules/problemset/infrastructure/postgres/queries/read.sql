-- name: CountVisibleSets :one
SELECT count(*) FROM problem_sets s
LEFT JOIN LATERAL (SELECT COALESCE(max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END),0)::integer AS grant_rank
 FROM problem_set_access a WHERE a.set_id=s.id AND a.domain_id=s.domain_id
 AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (SELECT group_id FROM domain_group_members WHERE domain_id=s.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) grants ON true
WHERE s.domain_id=sqlc.arg(domain_id)::uuid AND (s.visibility='public' OR sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (s.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.grant_rank>0)))
AND (sqlc.arg(author_id)::text='' OR s.author_id=NULLIF(sqlc.arg(author_id)::text,'')::uuid)
AND (sqlc.arg(keyword)::text='' OR s.title ILIKE '%'||sqlc.arg(keyword)::text||'%' OR s.description ILIKE '%'||sqlc.arg(keyword)::text||'%');

-- name: ListVisibleSets :many
SELECT s.id,s.public_id,s.title,s.description,s.author_id,COALESCE(author.username,'') AS author_name,
 s.visibility,s.created_at,s.updated_at,s.domain_id,s.owner_id,owner.username AS owner_name,
 grants.grant_rank,COALESCE(item_stats.all_items_visible,true)::boolean AS all_items_visible,
 COALESCE(item_stats.problem_count,0)::integer AS problem_count,COALESCE(item_stats.solved_count,0)::integer AS solved_count
FROM problem_sets s LEFT JOIN users author ON author.id=s.author_id JOIN users owner ON owner.id=s.owner_id
LEFT JOIN LATERAL (SELECT COALESCE(max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END),0)::integer AS grant_rank
 FROM problem_set_access a WHERE a.set_id=s.id AND a.domain_id=s.domain_id
 AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (SELECT group_id FROM domain_group_members WHERE domain_id=s.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) grants ON true
LEFT JOIN LATERAL (
 SELECT count(*) FILTER(WHERE visible)::integer AS problem_count,
 count(*) FILTER(WHERE visible AND solved)::integer AS solved_count,
 COALESCE(bool_and(visible),true)::boolean AS all_items_visible
 FROM (SELECT ((p.visibility='public' AND p.published_version IS NOT NULL) OR sqlc.arg(is_manager)::boolean OR
 (sqlc.arg(active_member)::boolean AND (p.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR EXISTS(
 SELECT 1 FROM problem_access pa WHERE pa.problem_id=p.id AND pa.domain_id=p.domain_id
 AND (pa.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR pa.group_id IN (SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)))))) AS visible,EXISTS(SELECT 1 FROM submissions sub WHERE sub.problem_id=p.id AND sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.contest_id IS NULL AND sub.status='Accepted') AS solved
 FROM problem_set_problems item JOIN problems p ON p.id=item.problem_id
 WHERE item.set_id=s.id AND item.domain_id=s.domain_id) item_access) item_stats ON true
WHERE s.domain_id=sqlc.arg(domain_id)::uuid AND (s.visibility='public' OR sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (s.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.grant_rank>0)))
AND (sqlc.arg(author_id)::text='' OR s.author_id=NULLIF(sqlc.arg(author_id)::text,'')::uuid)
AND (sqlc.arg(keyword)::text='' OR s.title ILIKE '%'||sqlc.arg(keyword)::text||'%' OR s.description ILIKE '%'||sqlc.arg(keyword)::text||'%')
ORDER BY s.created_at DESC,s.id DESC LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: GetVisibleSet :one
SELECT s.id,s.public_id,s.title,s.description,s.author_id,COALESCE(author.username,'') AS author_name,
 s.visibility,s.created_at,s.updated_at,s.domain_id,s.owner_id,owner.username AS owner_name,
 grants.grant_rank,COALESCE(item_stats.all_items_visible,true)::boolean AS all_items_visible,
 COALESCE(item_stats.problem_count,0)::integer AS problem_count,COALESCE(item_stats.solved_count,0)::integer AS solved_count
FROM problem_sets s LEFT JOIN users author ON author.id=s.author_id JOIN users owner ON owner.id=s.owner_id
LEFT JOIN LATERAL (SELECT COALESCE(max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END),0)::integer AS grant_rank
 FROM problem_set_access a WHERE a.set_id=s.id AND a.domain_id=s.domain_id
 AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (SELECT group_id FROM domain_group_members WHERE domain_id=s.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) grants ON true
LEFT JOIN LATERAL (
 SELECT count(*) FILTER(WHERE visible)::integer AS problem_count,
 count(*) FILTER(WHERE visible AND solved)::integer AS solved_count,
 COALESCE(bool_and(visible),true)::boolean AS all_items_visible
 FROM (SELECT ((p.visibility='public' AND p.published_version IS NOT NULL) OR sqlc.arg(is_manager)::boolean OR
 (sqlc.arg(active_member)::boolean AND (p.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR EXISTS(
 SELECT 1 FROM problem_access pa WHERE pa.problem_id=p.id AND pa.domain_id=p.domain_id
 AND (pa.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR pa.group_id IN (SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)))))) AS visible,EXISTS(SELECT 1 FROM submissions sub WHERE sub.problem_id=p.id AND sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.contest_id IS NULL AND sub.status='Accepted') AS solved
 FROM problem_set_problems item JOIN problems p ON p.id=item.problem_id
 WHERE item.set_id=s.id AND item.domain_id=s.domain_id) item_access) item_stats ON true
WHERE s.domain_id=sqlc.arg(domain_id)::uuid AND s.id=sqlc.arg(set_id)::uuid AND (s.visibility='public' OR sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (s.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.grant_rank>0)));

-- name: ListVisibleItems :many
SELECT item.problem_id,p.public_id,p.owner_id,item.sort_order,item.note,p.title,p.difficulty,p.visibility,p.submission_count,p.accepted_count,
 COALESCE((SELECT jsonb_agg(t.name ORDER BY t.name) FROM problem_tags pt JOIN tags t ON t.id=pt.tag_id WHERE pt.problem_id=p.id),'[]'::jsonb)::jsonb AS tags,
 CASE WHEN sqlc.arg(viewer_id)::text='' THEN 'none' WHEN EXISTS(SELECT 1 FROM submissions sub WHERE sub.problem_id=p.id AND sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.contest_id IS NULL AND sub.status='Accepted') THEN 'solved'
 WHEN EXISTS(SELECT 1 FROM submissions sub WHERE sub.problem_id=p.id AND sub.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND sub.contest_id IS NULL) THEN 'attempted' ELSE 'none' END::text AS user_status
FROM problem_set_problems item JOIN problems p ON p.id=item.problem_id
WHERE item.domain_id=sqlc.arg(domain_id)::uuid AND item.set_id=sqlc.arg(set_id)::uuid AND ((p.visibility='public' AND p.published_version IS NOT NULL) OR sqlc.arg(is_manager)::boolean OR
 (sqlc.arg(active_member)::boolean AND (p.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR EXISTS(
 SELECT 1 FROM problem_access pa WHERE pa.problem_id=p.id AND pa.domain_id=p.domain_id
 AND (pa.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR pa.group_id IN (SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))))))
ORDER BY item.sort_order;

-- name: GetSetGrantRank :one
SELECT grants.grant_rank FROM problem_sets s
LEFT JOIN LATERAL (SELECT COALESCE(max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END),0)::integer AS grant_rank
 FROM problem_set_access a WHERE a.set_id=s.id AND a.domain_id=s.domain_id
 AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (SELECT group_id FROM domain_group_members WHERE domain_id=s.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) grants ON true
WHERE s.id=sqlc.arg(set_id)::uuid AND s.domain_id=sqlc.arg(domain_id)::uuid;
