-- Section 14: Audit Log (Enterprise compliance)

CREATE TABLE audit_logs (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     UUID        REFERENCES users(id) ON DELETE SET NULL,
    ip_address  INET,
    action      TEXT        NOT NULL,
    resource    TEXT,
    resource_id TEXT,
    before      JSONB,
    after       JSONB,
    metadata    JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_tenant_time ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX idx_audit_user        ON audit_logs(user_id);
CREATE INDEX idx_audit_action      ON audit_logs(tenant_id, action);