-- ════════════════════════════════════════════
--  Sessions
-- ════════════════════════════════════════════

-- name: CreateSession :one
INSERT INTO sessions (
    user_id,
    tenant_id,
    refresh_token,
    user_agent,
    ip_address,
    expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetSessionByToken :one
SELECT
    s.*,
    u.is_active AS user_is_active
FROM sessions s
JOIN users u ON s.user_id = u.id
WHERE s.refresh_token = $1
  AND s.expires_at > now()
LIMIT 1;

-- name: GetSession :one
SELECT
    s.*,
    u.is_active AS user_is_active
FROM sessions s
JOIN users u ON s.user_id = u.id
WHERE s.id = $1
LIMIT 1;

-- name: ListSessions :many
SELECT *
FROM sessions
WHERE user_id = $1
  AND expires_at > now()
ORDER BY created_at DESC;

-- name: TouchSession :exec
UPDATE sessions
SET last_used_at = now()
WHERE id = $1;

-- name: UpdateSessionToken :exec
UPDATE sessions
SET refresh_token = $2,
    expires_at = $3,
    last_used_at = now()
WHERE id = $1;


-- name: RevokeSession :exec
DELETE FROM sessions
WHERE id = $1 AND user_id = $2;

-- name: RevokeAllSessions :exec
DELETE FROM sessions
WHERE user_id = $1;

-- name: DeleteExpiredSessions :execresult
DELETE FROM sessions
WHERE expires_at < now();