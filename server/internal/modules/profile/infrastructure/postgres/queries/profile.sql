-- name: GetProfileAccount :one
SELECT id, username, role, rating, created_at
FROM users WHERE username = sqlc.arg(username)::text;

-- name: GetProfileSubmissionStats :one
SELECT count(DISTINCT s.problem_id) FILTER (WHERE s.status = 'Accepted')::integer AS solved_count,
       count(DISTINCT s.problem_id)::integer AS attempted_count,
       count(*)::integer AS submission_count,
       count(*) FILTER (WHERE s.status = 'Accepted')::integer AS accepted_count
FROM submissions s
WHERE s.user_id = sqlc.arg(user_id)::uuid AND s.contest_id IS NULL
  AND s.domain_id = sqlc.arg(domain_id)::uuid
  AND EXISTS (SELECT 1 FROM problems p WHERE p.id = s.problem_id
              AND p.visibility = 'public' AND p.published_version IS NOT NULL);

-- name: ListProfileDifficultyStats :many
SELECT p.difficulty,
       count(*) FILTER (WHERE solved.problem_id IS NOT NULL)::integer AS solved,
       count(*)::integer AS total
FROM problems p
LEFT JOIN (
    SELECT DISTINCT s.problem_id FROM submissions s
    WHERE s.user_id = sqlc.arg(user_id)::uuid AND s.contest_id IS NULL AND s.status = 'Accepted'
) solved ON solved.problem_id = p.id
WHERE p.visibility = 'public' AND p.published_version IS NOT NULL AND p.domain_id = sqlc.arg(domain_id)::uuid
GROUP BY p.difficulty ORDER BY p.difficulty;

-- name: ListProfileActivity :many
SELECT to_char(s.submitted_at AT TIME ZONE 'UTC', 'YYYY-MM-DD')::text AS date, count(*)::integer AS count
FROM submissions s
WHERE s.user_id = sqlc.arg(user_id)::uuid AND s.contest_id IS NULL
  AND s.domain_id = sqlc.arg(domain_id)::uuid
  AND EXISTS (SELECT 1 FROM problems p WHERE p.id = s.problem_id
              AND p.visibility = 'public' AND p.published_version IS NOT NULL)
  AND s.submitted_at >= now() - sqlc.arg(window_days)::integer * interval '1 day'
GROUP BY date ORDER BY date;
