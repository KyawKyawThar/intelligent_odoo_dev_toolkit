-- Section 5: Environments + Agent Heartbeats

CREATE TABLE environments (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    odoo_url      TEXT        NOT NULL,
    db_name       TEXT        NOT NULL,
    odoo_version  TEXT,
    env_type      TEXT        NOT NULL DEFAULT 'production',
    status        TEXT        NOT NULL DEFAULT 'disconnected',
    agent_id      TEXT        UNIQUE,
    feature_flags JSONB       NOT NULL DEFAULT '{}',
    last_sync     TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_environments_tenant ON environments(tenant_id);
CREATE INDEX idx_environments_agent  ON environments(agent_id);

CREATE TABLE agent_heartbeats (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    env_id        UUID        NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    agent_id      TEXT        NOT NULL,
    agent_version TEXT,
    status        TEXT        NOT NULL DEFAULT 'ok',
    metadata      JSONB       NOT NULL DEFAULT '{}',
    received_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_heartbeats_env_time ON agent_heartbeats(env_id, received_at DESC);