-- name: GetClarificationParent :one
SELECT contest_id, author_id, problem_id FROM clarifications WHERE id = sqlc.arg(parent_id)::bigint;

-- name: FindClarificationProblem :one
SELECT problem_id FROM contest_problems
			 WHERE contest_id = sqlc.arg(contest_id)::uuid AND problem_id = sqlc.arg(problem_id)::uuid
			 FOR KEY SHARE;

-- name: CreateClarification :one
INSERT INTO clarifications
		   (contest_id, problem_id, parent_id, author_id, recipient_id, from_jury, subject, body, domain_id)
		 VALUES (sqlc.arg(contest_id)::uuid, sqlc.narg(problem_id)::uuid, sqlc.narg(parent_id)::bigint, sqlc.narg(author_id)::uuid, sqlc.narg(recipient_id)::uuid, sqlc.arg(from_jury)::boolean, sqlc.arg(subject)::text, sqlc.arg(body)::text, sqlc.arg(domain_id)::uuid)
		 RETURNING id;

-- name: MarkClarificationAnswered :exec
UPDATE clarifications SET answered=true WHERE contest_id=sqlc.arg(contest_id)::uuid AND id=sqlc.arg(parent_id)::bigint;

-- name: GetClarification :one
SELECT c.id, c.contest_id, c.problem_id, c.parent_id, c.author_id,
	COALESCE(author.username, '') AS author_name, c.recipient_id, c.from_jury, c.subject, c.body,
	c.answered, c.created_at, COALESCE(p.title, '') AS problem_name
		 FROM clarifications AS c
		 LEFT JOIN users AS author ON author.id = c.author_id
		 LEFT JOIN problems AS p ON p.id = c.problem_id
		 WHERE c.contest_id = sqlc.arg(contest_id)::uuid AND c.id = sqlc.arg(clarification_id)::bigint
		 AND EXISTS (SELECT 1 FROM contests WHERE contests.id = sqlc.arg(contest_id)::uuid AND contests.domain_id = sqlc.arg(domain_id)::uuid);

-- name: ListVisibleClarifications :many
SELECT c.id,c.contest_id,c.problem_id,c.parent_id,c.author_id,COALESCE(author.username,'') AS author_name,c.recipient_id,c.from_jury,c.subject,c.body,c.answered,c.created_at,COALESCE(p.title,'') AS problem_name
FROM clarifications c LEFT JOIN users author ON author.id=c.author_id LEFT JOIN problems p ON p.id=c.problem_id
WHERE c.contest_id=sqlc.arg(contest_id)::uuid AND EXISTS(SELECT 1 FROM contests parent WHERE parent.id=c.contest_id AND parent.domain_id=sqlc.arg(domain_id)::uuid)
AND (sqlc.arg(is_staff)::boolean OR (c.from_jury AND c.recipient_id IS NULL)
 OR c.author_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR c.recipient_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
 OR c.parent_id IN(SELECT id FROM clarifications WHERE author_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))
ORDER BY COALESCE(c.parent_id,c.id) DESC,c.id;
