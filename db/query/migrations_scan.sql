-- name: CreateMigrationScan :one
INSERT INTO migration_scans (
    env_id,
    triggered_by,
    from_version,
    to_version,
    issues,
    breaking_count,
    warning_count,
    minor_count,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetMigrationScan :one
SELECT * FROM migration_scans
WHERE id = $1 AND env_id = $2
LIMIT 1;

-- name: ListMigrationScans :many
SELECT * FROM migration_scans
WHERE env_id = $1
ORDER BY scanned_at DESC
LIMIT $2 OFFSET $3;

-- name: GetLatestMigrationScan :one
SELECT * FROM migration_scans
WHERE env_id = $1
ORDER BY scanned_at DESC
LIMIT 1;

-- name: DeleteMigrationScan :exec
DELETE FROM migration_scans
WHERE id = $1 AND env_id = $2;