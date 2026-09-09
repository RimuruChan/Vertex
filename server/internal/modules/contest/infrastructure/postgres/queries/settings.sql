-- name: CreateContest :one
INSERT INTO contests AS c (title, description, rule, begin_at, end_at, freeze_at, unfreeze_at,
		                      penalty_minutes, penalize_compile_error, feedback,
		                      visibility, password_hash, rankboard_visible, show_problem_metadata, created_by, domain_id,owner_id,admission,allow_self_registration,allow_late_registration)
		 VALUES (sqlc.arg(title)::text, sqlc.arg(description)::text, sqlc.arg(rule)::text, sqlc.arg(begin_at)::timestamptz, sqlc.arg(end_at)::timestamptz, sqlc.narg(freeze_at)::timestamptz, sqlc.narg(unfreeze_at)::timestamptz, sqlc.arg(penalty_minutes)::integer, sqlc.arg(penalize_compile_error)::boolean, sqlc.arg(feedback)::text, sqlc.arg(visibility)::text, sqlc.arg(password_hash)::text, sqlc.arg(rankboard_visible)::boolean, sqlc.arg(show_problem_metadata)::boolean, sqlc.arg(creator_id)::uuid, sqlc.arg(domain_id)::uuid,sqlc.arg(creator_id)::uuid,sqlc.arg(admission)::text,sqlc.arg(allow_self_registration)::boolean,sqlc.arg(allow_late_registration)::boolean)
		 RETURNING c.id,c.public_id,c.title,c.description,c.rule,c.begin_at,c.end_at,c.freeze_at,c.unfreeze_at,
 c.penalty_minutes,c.penalize_compile_error,c.feedback,c.visibility,c.password_hash,c.rankboard_visible,c.show_problem_metadata,c.created_by,c.created_at,
 c.owner_id,c.domain_id,c.admission,COALESCE((SELECT u.username FROM users u WHERE u.id=c.owner_id),'')::text AS owner_name,c.allow_self_registration,c.allow_late_registration,false AS editor,false AS jury,false AS observer,false AS participant,false AS registered;

-- name: UpdateContest :one
UPDATE contests AS c SET title = sqlc.arg(title)::text, description = sqlc.arg(description)::text, rule = sqlc.arg(rule)::text, begin_at = sqlc.arg(begin_at)::timestamptz,
		        end_at = sqlc.arg(end_at)::timestamptz, freeze_at = sqlc.narg(freeze_at)::timestamptz, unfreeze_at = sqlc.narg(unfreeze_at)::timestamptz,
		        penalty_minutes = sqlc.arg(penalty_minutes)::integer, penalize_compile_error = sqlc.arg(penalize_compile_error)::boolean, feedback = sqlc.arg(feedback)::text,
		        visibility = sqlc.arg(visibility)::text,
		        password_hash = CASE
		          WHEN sqlc.arg(password_hash)::text <> '' THEN sqlc.arg(password_hash)::text
		          WHEN sqlc.arg(visibility)::text = 'password' THEN password_hash
		          ELSE ''
		        END,
		        rankboard_visible = sqlc.arg(rankboard_visible)::boolean, show_problem_metadata = sqlc.arg(show_problem_metadata)::boolean, admission=sqlc.arg(admission)::text, allow_self_registration=sqlc.arg(allow_self_registration)::boolean, allow_late_registration=sqlc.arg(allow_late_registration)::boolean
		 WHERE c.id = sqlc.arg(contest_id)::uuid AND domain_id = sqlc.arg(domain_id)::uuid
		 RETURNING c.id,c.public_id,c.title,c.description,c.rule,c.begin_at,c.end_at,c.freeze_at,c.unfreeze_at,
 c.penalty_minutes,c.penalize_compile_error,c.feedback,c.visibility,c.password_hash,c.rankboard_visible,c.show_problem_metadata,c.created_by,c.created_at,
 c.owner_id,c.domain_id,c.admission,COALESCE((SELECT u.username FROM users u WHERE u.id=c.owner_id),'')::text AS owner_name,c.allow_self_registration,c.allow_late_registration,false AS editor,false AS jury,false AS observer,false AS participant,false AS registered;
