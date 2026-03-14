-- =============================================================================
-- Dev-only seed: create a local-dev tenant, user, environment, and API key.
-- Idempotent — safe to re-run after every migrate_up / table drop.
-- Runs automatically via `make dev`.
--
-- Dev credentials (local only — never use in production):
--   Email:    dev@local.dev
--   Password: admin
-- =============================================================================

DO $$
DECLARE
    -- Fixed UUIDs make the seed fully idempotent across DB resets.
    v_tenant_id uuid := '00000000-0000-0000-0000-000000000001';
    v_user_id   uuid := '00000000-0000-0000-0000-000000000002';
    v_api_key   text := 'odt_dev_localagent_key';
    -- bcrypt hash of 'admin' (cost=10, generated with golang.org/x/crypto/bcrypt)
    v_pw_hash   text := '$2a$10$2AZB5NTZJimYtcmaU9S/uezRVUkDXn/hNaOpw.c5nXk8ImAuphZVm';
BEGIN
    -- ── 1. Tenant ────────────────────────────────────────────────────────────
    INSERT INTO tenants (id, name, slug, plan)
    VALUES (v_tenant_id, 'Local Dev Tenant', 'local-dev', 'cloud')
    ON CONFLICT (id) DO NOTHING;

    -- ── 2. User ──────────────────────────────────────────────────────────────
    INSERT INTO users (id, tenant_id, email, password_hash, full_name, email_verified)
    VALUES (v_user_id, v_tenant_id, 'dev@local.dev', v_pw_hash, 'Dev User', true)
    ON CONFLICT (id) DO NOTHING;

    -- ── 3. Environment ───────────────────────────────────────────────────────
    -- Never change the existing UUID — child tables (schema_snapshots etc.)
    -- hold FK references to it.
    INSERT INTO environments (tenant_id, name, odoo_url, db_name, odoo_version, env_type, status, agent_id)
    VALUES (v_tenant_id, 'Local Dev', 'http://localhost:8069', 'odoo', '17.0', 'development', 'connected', 'local-dev-agent')
    ON CONFLICT (agent_id) DO NOTHING;

    -- ── 4. API key ───────────────────────────────────────────────────────────
    -- key_hash stores the raw key in dev because the middleware skips hashing
    -- (see TODO in api_key_auth.go). Replace with a digest when hashing is added.
    INSERT INTO api_keys (tenant_id, key_hash, key_prefix, name, scopes)
    VALUES (v_tenant_id, v_api_key, 'odt_dev', 'Local Dev Agent Key', ARRAY['agent:write'])
    ON CONFLICT (key_hash) DO NOTHING;
END $$;
