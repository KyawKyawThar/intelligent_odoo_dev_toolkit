-- name: CreateTenant :one
INSERT INTO tenants (
    name,
    slug,
    plan,
    plan_status,
    trial_ends_at,
    settings,
    retention_config
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetTenantByID :one
SELECT * FROM tenants
WHERE id = $1 LIMIT 1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants
WHERE slug = $1 LIMIT 1;

-- name: ListTenants :many
SELECT * FROM tenants
ORDER BY created_at DESC;

-- name: UpdateTenantSettings :one
UPDATE tenants
SET
    name = $2,
    settings = $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateTenantPlan :one
UPDATE tenants
SET
    plan = $2,
    plan_status = $3,
    trial_ends_at = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateTenantRetention :one
UPDATE tenants
SET
    retention_config = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTenant :exec
DELETE FROM tenants
WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (
    tenant_id,
    email,
    password_hash,
    full_name,
    email_verified,
    is_active
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND tenant_id = $2
LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 AND tenant_id = $2
LIMIT 1;

-- name: ListUsersByTenant :many
SELECT * FROM users
WHERE tenant_id = $1
ORDER BY created_at;

-- name: UpdateUser :one
UPDATE users
SET
    full_name = $2,
    email_verified = $3,
    is_active = $4,
    updated_at = now()
WHERE id = $1 AND tenant_id = $5
RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE users
SET
    password_hash = $2,
    updated_at = now()
WHERE id = $1 AND tenant_id = $3;

-- name: UpdateUserProfile :one
UPDATE users
SET
    full_name = COALESCE(sqlc.narg('full_name'), full_name),
    email = COALESCE(sqlc.narg('email'), email),
    email_verified = CASE 
        WHEN sqlc.narg('email') IS NOT NULL AND sqlc.narg('email') != email THEN false 
        ELSE email_verified 
    END,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: UpdateUserLastLogin :exec
UPDATE users
SET
    last_login_at = now(),
    updated_at = now()
WHERE id = $1;

-- name: GetUserByEmailGlobal :one
-- Login flow: user provides email only, we resolve tenant from the result
SELECT u.*, t.slug AS tenant_slug, t.plan AS tenant_plan
FROM users u
JOIN tenants t ON u.tenant_id = t.id
WHERE u.email = $1 AND u.is_active = true
LIMIT 1;

-- name: GetUserByIDGlobal :one
-- Fetch user by id across tenants (used by password reset flows)
SELECT u.*, t.slug AS tenant_slug, t.plan AS tenant_plan
FROM users u
JOIN tenants t ON u.tenant_id = t.id
WHERE u.id = $1
LIMIT 1;

-- name: CountUsersByTenant :one
SELECT count(*) FROM users
WHERE tenant_id = $1;

-- name: DeleteUser :exec
DELETE FROM users
WHERE id = $1 AND tenant_id = $2;


-- name: DeactivateUser :exec
UPDATE users
SET
    is_active = false,
    updated_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: VerifyUserEmail :exec
UPDATE users
SET
    email_verified = true,
    updated_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: UnverifyUserEmail :exec
UPDATE users
SET
    email_verified = false,
    updated_at = now()
WHERE id = $1 AND tenant_id = $2;