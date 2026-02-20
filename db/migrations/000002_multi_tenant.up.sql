-- Section 2: Multi-Tenant Core
CREATE TABLE tenants (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT        NOT NULL,
    slug             TEXT        UNIQUE NOT NULL,
    plan             TEXT        NOT NULL DEFAULT 'cloud',
    plan_status      TEXT        NOT NULL DEFAULT 'active',
    trial_ends_at    TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    settings         JSONB       NOT NULL DEFAULT '{}',
    retention_config JSONB       NOT NULL DEFAULT '{
        "error_traces_days":          7,
        "profiler_recordings_days":   7,
        "budget_samples_days":        30,
        "schema_snapshots_keep":      10,
        "raw_logs_days":              3
    }'
);

CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    full_name     TEXT,
    role          TEXT        NOT NULL DEFAULT 'member',
    is_active     BOOLEAN     NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, email)
);
CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_users_email  ON users(email);