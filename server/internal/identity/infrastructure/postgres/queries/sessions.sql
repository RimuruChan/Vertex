-- name: CreateSession :one
INSERT INTO auth_sessions(user_id, refresh_token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, user_id, expires_at;

-- name: RotateSession :one
UPDATE auth_sessions AS session
SET refresh_token_hash = sqlc.arg(new_hash), expires_at = sqlc.arg(expires_at), last_used_at = now()
FROM users AS usr
WHERE session.refresh_token_hash = sqlc.arg(old_hash)
  AND session.user_id = usr.id
  AND session.revoked_at IS NULL
  AND session.expires_at > now()
RETURNING session.id AS session_id, session.user_id, session.expires_at,
          usr.id, usr.username, usr.email, usr.password_hash, usr.role, usr.rating,
          usr.created_at, usr.disabled_at, usr.disabled_reason;

-- name: ActiveSessionUser :one
SELECT usr.*
FROM auth_sessions AS session JOIN users AS usr ON usr.id = session.user_id
WHERE session.id = $1 AND session.user_id = $2
  AND session.revoked_at IS NULL AND session.expires_at > now();

-- name: RevokeSession :execrows
UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now())
WHERE id = $1 AND user_id = $2;

-- name: RevokeUserSessions :exec
UPDATE auth_sessions SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;
