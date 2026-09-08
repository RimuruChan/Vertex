-- name: GetEditorialProblem :one
SELECT problem_id FROM editorials WHERE id=sqlc.arg(editorial_id)::uuid AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: RecordContentAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(user_id)::uuid,sqlc.arg(action)::text,sqlc.arg(target)::text);

-- name: UpdateDiscussionPost :exec
UPDATE discussion_posts SET content_md=sqlc.arg(body)::text,updated_at=now() WHERE id=sqlc.arg(post_id)::bigint;

-- name: DeleteDiscussionPost :exec
DELETE FROM discussion_posts WHERE id=sqlc.arg(post_id)::bigint;

-- name: GetDiscussionTarget :one
SELECT problem_id,editorial_id FROM discussion_posts WHERE id=sqlc.arg(post_id)::bigint AND domain_id=sqlc.arg(domain_id)::uuid;

-- name: HasSolvedProblem :one
SELECT EXISTS(SELECT 1 FROM submissions WHERE domain_id=sqlc.arg(domain_id)::uuid AND problem_id=sqlc.arg(problem_id)::uuid AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid AND contest_id IS NULL AND status='Accepted');

-- name: CreateEditorial :one
INSERT INTO editorials(domain_id,problem_id,author_id,title,content_md,visibility,status,solved_only)
	 VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(problem_id)::uuid,sqlc.arg(user_id)::uuid,sqlc.arg(title)::text,sqlc.arg(body)::text,sqlc.arg(visibility)::text,sqlc.arg(status)::text,sqlc.arg(solved_only)::boolean) RETURNING id;

-- name: UpdateEditorial :exec
UPDATE editorials SET title=sqlc.arg(title)::text,content_md=sqlc.arg(body)::text,visibility=sqlc.arg(visibility)::text,status=sqlc.arg(status)::text,solved_only=sqlc.arg(solved_only)::boolean,updated_at=now() WHERE id=sqlc.arg(editorial_id)::uuid;

-- name: DeleteEditorial :exec
DELETE FROM editorials WHERE id=sqlc.arg(editorial_id)::uuid;

-- name: AddEditorialVote :exec
INSERT INTO editorial_votes(editorial_id,user_id) VALUES(sqlc.arg(editorial_id)::uuid,sqlc.arg(user_id)::uuid) ON CONFLICT(editorial_id,user_id) DO NOTHING;

-- name: RemoveEditorialVote :exec
DELETE FROM editorial_votes WHERE editorial_id=sqlc.arg(editorial_id)::uuid AND user_id=sqlc.arg(user_id)::uuid;

-- name: RecountEditorialVotes :one
UPDATE editorials SET vote_count=(SELECT count(*) FROM editorial_votes WHERE editorial_id=sqlc.arg(editorial_id)::uuid) WHERE id=sqlc.arg(editorial_id)::uuid RETURNING vote_count;
