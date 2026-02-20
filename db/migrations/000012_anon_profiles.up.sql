-- Section 12: Anonymization

CREATE TABLE anon_profiles (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_by  UUID        REFERENCES users(id) ON DELETE SET NULL,
    name        TEXT        NOT NULL,
    source_env  UUID        REFERENCES environments(id) ON DELETE SET NULL,
    target_env  UUID        REFERENCES environments(id) ON DELETE SET NULL,
    field_rules JSONB       NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'draft',
    last_run    TIMESTAMPTZ,
    last_run_by UUID        REFERENCES users(id) ON DELETE SET NULL,
    audit_ref   TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_anon_tenant ON anon_profiles(tenant_id);