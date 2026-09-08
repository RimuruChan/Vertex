-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, role)
VALUES ($1, $2, $3, 'user')
RETURNING *;

-- name: JoinOfficialDomain :execrows
INSERT INTO domain_members(domain_id, user_id, role_key, status)
SELECT id, $1, 'member', 'active' FROM domains WHERE is_official;

-- name: UserByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: UserByID :one
SELECT * FROM users WHERE id = $1;

-- name: BootstrapAdmin :exec
INSERT INTO users(username, email, password_hash, role)
VALUES ($1, $2, $3, 'admin')
ON CONFLICT(username) DO NOTHING;
