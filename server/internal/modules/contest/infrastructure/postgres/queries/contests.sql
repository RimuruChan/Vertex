-- name: CountVisibleContests :one
SELECT count(*) FROM contests c LEFT JOIN LATERAL (
 SELECT COALESCE(bool_or(a.role='editor'),false)::boolean AS editor,COALESCE(bool_or(a.role='jury'),false)::boolean AS jury,
 COALESCE(bool_or(a.role='observer'),false)::boolean AS observer,COALESCE(bool_or(a.role='participant'),false)::boolean AS participant
 FROM contest_access a WHERE a.contest_id=c.id AND a.domain_id=c.domain_id
 AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (SELECT group_id FROM domain_group_members WHERE domain_id=c.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) grants ON true
WHERE c.domain_id=sqlc.arg(domain_id)::uuid AND CASE WHEN sqlc.arg(managed_only)::boolean THEN (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (c.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.editor OR grants.jury OR grants.observer))) ELSE
 (c.visibility IN ('public','password') OR (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (c.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.editor OR grants.jury OR grants.observer))) OR (sqlc.arg(active_member)::boolean AND (grants.participant OR (sqlc.arg(can_submit)::boolean AND c.admission='members' AND EXISTS(SELECT 1 FROM contest_participants cp WHERE cp.contest_id=c.id AND cp.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))))) END
 AND (sqlc.arg(keyword)::text='' OR strpos(lower(c.title),lower(sqlc.arg(keyword)::text))>0 OR c.public_id::text=sqlc.arg(keyword)::text);

-- name: ListVisibleContests :many
SELECT c.id,c.public_id,c.title,c.description,c.rule,c.begin_at,c.end_at,c.freeze_at,c.unfreeze_at,
 c.penalty_minutes,c.penalize_compile_error,c.feedback,c.visibility,c.password_hash,c.rankboard_visible,c.created_by,c.created_at,
 c.owner_id,c.domain_id,c.admission,COALESCE((SELECT u.username FROM users u WHERE u.id=c.owner_id),'')::text AS owner_name,c.allow_self_registration,c.allow_late_registration,grants.editor,grants.jury,grants.observer,grants.participant,EXISTS(SELECT 1 FROM contest_participants cp WHERE cp.contest_id=c.id AND cp.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid) AS registered
FROM contests c LEFT JOIN LATERAL (
 SELECT COALESCE(bool_or(a.role='editor'),false)::boolean AS editor,COALESCE(bool_or(a.role='jury'),false)::boolean AS jury,
 COALESCE(bool_or(a.role='observer'),false)::boolean AS observer,COALESCE(bool_or(a.role='participant'),false)::boolean AS participant
 FROM contest_access a WHERE a.contest_id=c.id AND a.domain_id=c.domain_id
 AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (SELECT group_id FROM domain_group_members WHERE domain_id=c.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) grants ON true
WHERE c.domain_id=sqlc.arg(domain_id)::uuid AND CASE WHEN sqlc.arg(managed_only)::boolean THEN (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (c.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.editor OR grants.jury OR grants.observer))) ELSE
 (c.visibility IN ('public','password') OR (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND (c.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR grants.editor OR grants.jury OR grants.observer))) OR (sqlc.arg(active_member)::boolean AND (grants.participant OR (sqlc.arg(can_submit)::boolean AND c.admission='members' AND EXISTS(SELECT 1 FROM contest_participants cp WHERE cp.contest_id=c.id AND cp.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))))) END
 AND (sqlc.arg(keyword)::text='' OR strpos(lower(c.title),lower(sqlc.arg(keyword)::text))>0 OR c.public_id::text=sqlc.arg(keyword)::text)
ORDER BY c.begin_at DESC,c.id DESC LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: GetContest :one
SELECT c.id,c.public_id,c.title,c.description,c.rule,c.begin_at,c.end_at,c.freeze_at,c.unfreeze_at,
 c.penalty_minutes,c.penalize_compile_error,c.feedback,c.visibility,c.password_hash,c.rankboard_visible,c.created_by,c.created_at,
 c.owner_id,c.domain_id,c.admission,COALESCE((SELECT u.username FROM users u WHERE u.id=c.owner_id),'')::text AS owner_name,c.allow_self_registration,c.allow_late_registration,false AS editor,false AS jury,false AS observer,false AS participant,false AS registered FROM contests c WHERE c.id=sqlc.arg(contest_id)::uuid AND c.domain_id=sqlc.arg(domain_id)::uuid;
