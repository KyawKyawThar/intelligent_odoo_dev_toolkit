-- name: CreateAuditLog :one
INSERT INTO audit_logs (
    tenant_id,
    user_id,
    ip_address,
    action,
    resource,
    resource_id,
    before,
    after,
    metadata
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAuditLogsByUser :many
SELECT * FROM audit_logs
WHERE tenant_id = $1 AND user_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListAuditLogsByAction :many
SELECT * FROM audit_logs
WHERE tenant_id = $1 AND action = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListAuditLogsByResource :many
SELECT * FROM audit_logs
WHERE tenant_id = $1 AND resource = $2 AND resource_id = $3
ORDER BY created_at DESC
LIMIT $4;

-- name: ListAuditLogsBetween :many
SELECT * FROM audit_logs
WHERE tenant_id = $1
  AND created_at >= $2
  AND created_at <= $3
ORDER BY created_at DESC
LIMIT $4 OFFSET $5;

-- name: CountAuditLogs :one
SELECT count(*) FROM audit_logs
WHERE tenant_id = $1;

-- name: GetAuditLogsByTenant :many
-- Used in tests and admin views — filter by tenant + action without pagination
SELECT * FROM audit_logs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND action = sqlc.arg(action)
ORDER BY created_at DESC;

-- name: DeleteOldAuditLogs :execresult
DELETE FROM audit_logs
WHERE tenant_id = $1
  AND created_at < $2;