-- name: CreateSchemaSnapshot :one
INSERT INTO schema_snapshots (
    env_id,
    models,
    acl_rules,
    record_rules,
    model_count,
    field_count,
    diff_ref
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetLatestSchema :one
SELECT * FROM schema_snapshots
WHERE env_id = $1
ORDER BY captured_at DESC
LIMIT 1;

-- name: GetSchemaByID :one
SELECT * FROM schema_snapshots
WHERE id = $1 AND env_id = $2
LIMIT 1;
-- name: GetSchemaSnapshotByID :one
SELECT * FROM schema_snapshots
WHERE id = $1
LIMIT 1;
-- name: ListSchemaSnapshots :many
SELECT id, env_id, captured_at, model_count, field_count, diff_ref
FROM schema_snapshots
WHERE env_id = $1
ORDER BY captured_at DESC
LIMIT $2;

-- name: GetTwoSchemasForDiff :many
SELECT * FROM schema_snapshots
WHERE id = ANY($1::uuid[])
ORDER BY captured_at ASC;

-- name: CountSchemasByEnv :one
SELECT count(*) FROM schema_snapshots
WHERE env_id = $1;

-- name: DeleteOldSchemaSnapshots :execresult
-- Keep the N most recent per env, delete the rest for a tenant
-- Call from retention worker with the keep count
DELETE FROM schema_snapshots
WHERE id IN (
    SELECT s.id
    FROM schema_snapshots s
    JOIN environments e ON s.env_id = e.id
    WHERE e.tenant_id = $1
    AND s.id NOT IN (
        SELECT s2.id
        FROM schema_snapshots s2
        WHERE s2.env_id = s.env_id
        ORDER BY s2.captured_at DESC
        LIMIT $2
    )
);