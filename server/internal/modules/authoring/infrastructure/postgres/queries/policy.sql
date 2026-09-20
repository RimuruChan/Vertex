-- name: ChangeAuthoringVisibility :execrows
UPDATE problems SET visibility=sqlc.arg(visibility),updated_at=now()
WHERE id=sqlc.arg(problem_id) AND visibility=sqlc.arg(expected_visibility);
