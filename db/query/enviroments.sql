-- name: CreateEnvironment :one
INSERT INTO environments (
    tenant_id,
    name,
    odoo_url,
    db_name,
    odoo_version,
    env_type,
    status,
    feature_flags
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetEnvironmentByID :one
SELECT * FROM environments
WHERE id = $1 AND tenant_id = $2
LIMIT 1;

-- name: GetEnvironmentByAgentID :one
SELECT * FROM environments
WHERE agent_id = $1
LIMIT 1;

-- name: ListEnvironmentsByTenant :many
SELECT * FROM environments
WHERE tenant_id = $1
ORDER BY created_at;

-- name: CountEnvironmentsByTenant :one
SELECT count(*) FROM environments
WHERE tenant_id = $1;

-- name: UpdateEnvironment :one
UPDATE environments
SET
    name = $2,
    odoo_url = $3,
    db_name = $4,
    odoo_version = $5,
    env_type = $6,
    updated_at = now()
WHERE id = $1 AND tenant_id = $7
RETURNING *;

-- name: UpdateEnvironmentStatus :exec
UPDATE environments
SET
    status = $2,
    last_sync = now(),
    updated_at = now()
WHERE id = $1;

-- name: RegisterAgent :one
UPDATE environments
SET
    agent_id = $2,
    status = 'connected',
    last_sync = now(),
    updated_at = now()
WHERE id = $1 AND tenant_id = $3
RETURNING *;

-- name: DisconnectAgent :exec
UPDATE environments
SET
    status = 'disconnected',
    updated_at = now()
WHERE agent_id = $1;

-- name: UpdateFeatureFlags :one
UPDATE environments
SET
    feature_flags = $2,
    updated_at = now()
WHERE id = $1 AND tenant_id = $3
RETURNING *;

-- name: GetFeatureFlags :one
SELECT feature_flags FROM environments
WHERE id = $1
LIMIT 1;

-- name: DeleteEnvironment :execresult
DELETE FROM environments
WHERE id = $1 AND tenant_id = $2;

-- ════════════════════════════════════════════
--  Agent Heartbeats
-- ════════════════════════════════════════════

-- name: InsertHeartbeat :one
INSERT INTO agent_heartbeats (
    env_id,
    agent_id,
    agent_version,
    status,
    metadata
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetLatestHeartbeat :one
SELECT * FROM agent_heartbeats
WHERE env_id = $1
ORDER BY received_at DESC
LIMIT 1;

-- name: ListHeartbeats :many
SELECT * FROM agent_heartbeats
WHERE env_id = $1
ORDER BY received_at DESC
LIMIT $2;

-- name: ListStaleAgents :many
SELECT e.id AS env_id, e.name, e.agent_id, e.tenant_id,
       COALESCE(h.received_at, '1970-01-01'::timestamptz) AS last_heartbeat
FROM environments e
LEFT JOIN LATERAL (
    SELECT received_at FROM agent_heartbeats
    WHERE env_id = e.id
    ORDER BY received_at DESC LIMIT 1
) h ON true
WHERE e.status = 'connected'
  AND (h.received_at IS NULL OR h.received_at < now() - $1::interval);

-- name: DeleteOldHeartbeats :execresult
DELETE FROM agent_heartbeats
WHERE env_id IN (
    SELECT id FROM environments WHERE tenant_id = $1
)
AND received_at < $2;