-- name: CreateAPIKey :one
INSERT INTO api_keys (
    tenant_id,
    created_by,
    key_hash,
    key_prefix,
    name,
    scopes,
    expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys
WHERE key_hash = $1
  AND is_active = true
  AND (expires_at IS NULL OR expires_at > now())
LIMIT 1;

-- name: GetAPIKeyByID :one
SELECT * FROM api_keys
WHERE id = $1 AND tenant_id = $2
LIMIT 1;

-- name: ListAPIKeysByTenant :many
SELECT id, tenant_id, created_by, key_prefix, name, scopes,
       last_used, expires_at, is_active, created_at
FROM api_keys
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: TouchAPIKey :exec
UPDATE api_keys
SET last_used = now()
WHERE id = $1;

-- name: RevokeAPIKey :exec
UPDATE api_keys
SET is_active = false
WHERE id = $1 AND tenant_id = $2;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys
WHERE id = $1 AND tenant_id = $2;