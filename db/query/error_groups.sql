-- name: UpsertErrorGroup :one
INSERT INTO error_groups (
    env_id,
    signature,
    error_type,
    message,
    module,
    model,
    first_seen,
    last_seen,
    occurrence_count,
    affected_users,
    raw_trace_ref
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $7, 1, $8, $9
)
ON CONFLICT (env_id, signature) DO UPDATE SET
    last_seen = EXCLUDED.last_seen,
    occurrence_count = error_groups.occurrence_count + 1,
    raw_trace_ref = EXCLUDED.raw_trace_ref,
    message = EXCLUDED.message
RETURNING *;

-- name: AppendAffectedUsers :exec
-- Merge new user IDs into the existing array (deduplicated)
UPDATE error_groups
SET affected_users = (
    SELECT array_agg(DISTINCT u)
    FROM unnest(affected_users || sqlc.arg(user_ids)::int[]) u
)
WHERE id = sqlc.arg(id);

-- name: GetErrorGroupBySignature :one
SELECT * FROM error_groups
WHERE env_id = $1 AND signature = $2
LIMIT 1;

-- name: GetErrorGroupByID :one
SELECT * FROM error_groups
WHERE id = $1 AND env_id = $2
LIMIT 1;

-- name: ListErrorGroups :many
SELECT * FROM error_groups
WHERE env_id = $1
ORDER BY occurrence_count DESC, last_seen DESC
LIMIT $2 OFFSET $3;

-- name: ListErrorGroupsByStatus :many
SELECT * FROM error_groups
WHERE env_id = $1 AND status = $2
ORDER BY last_seen DESC
LIMIT $3 OFFSET $4;

-- name: ListErrorGroupsByType :many
SELECT * FROM error_groups
WHERE env_id = $1 AND error_type = $2
ORDER BY occurrence_count DESC
LIMIT $3 OFFSET $4;

-- name: SearchErrorGroups :many
SELECT * FROM error_groups
WHERE env_id = $1
  AND (message ILIKE '%' || $2 || '%' OR module ILIKE '%' || $2 || '%' OR model ILIKE '%' || $2 || '%')
ORDER BY last_seen DESC
LIMIT $3 OFFSET $4;

-- name: CountErrorGroupsByEnv :one
SELECT count(*) FROM error_groups
WHERE env_id = $1;

-- name: CountErrorGroupsByStatus :one
SELECT count(*) FROM error_groups
WHERE env_id = $1 AND status = $2;

-- name: ResolveErrorGroup :one
UPDATE error_groups
SET
    status = 'resolved',
    resolved_by = $2,
    resolved_at = now()
WHERE id = $1 AND env_id = $3
RETURNING *;

-- name: AcknowledgeErrorGroup :one
UPDATE error_groups
SET status = 'acknowledged'
WHERE id = $1 AND env_id = $2
RETURNING *;

-- name: ReopenErrorGroup :one
UPDATE error_groups
SET
    status = 'open',
    resolved_by = NULL,
    resolved_at = NULL
WHERE id = $1 AND env_id = $2
RETURNING *;

-- name: DeleteOldErrorGroups :execresult
DELETE FROM error_groups
WHERE env_id IN (
    SELECT id FROM environments WHERE tenant_id = $1
)
AND last_seen < $2
AND status != 'open';