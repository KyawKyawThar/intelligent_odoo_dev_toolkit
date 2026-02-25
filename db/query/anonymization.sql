-- name: CreateAnonProfile :one
INSERT INTO anon_profiles (
    tenant_id,
    created_by,
    name,
    source_env,
    target_env,
    field_rules,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetAnonProfile :one
SELECT * FROM anon_profiles
WHERE id = $1 AND tenant_id = $2
LIMIT 1;

-- name: GetAnonProfileByID :one
SELECT * FROM anon_profiles
WHERE id = $1
LIMIT 1;

-- name: ListAnonProfiles :many
SELECT * FROM anon_profiles
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: UpdateAnonProfile :one
UPDATE anon_profiles
SET
    name = $2,
    source_env = $3,
    target_env = $4,
    field_rules = $5,
    updated_at = now()
WHERE id = $1 AND tenant_id = $6
RETURNING *;

-- name: UpdateAnonProfileStatus :one
UPDATE anon_profiles
SET
    status = $2,
    last_run = CASE WHEN $2 = 'completed' THEN now() ELSE last_run END,
    last_run_by = $3,
    audit_ref = $4,
    updated_at = now()
WHERE id = $1 AND tenant_id = $5
RETURNING *;

-- name: DeleteAnonProfile :exec
DELETE FROM anon_profiles
WHERE id = $1 AND tenant_id = $2;