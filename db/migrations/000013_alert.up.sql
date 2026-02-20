-- Section 13: Alerts + Notifications

CREATE TABLE alerts (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    env_id          UUID        NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    type            TEXT        NOT NULL,
    severity        TEXT        NOT NULL,
    message         TEXT        NOT NULL,
    metadata        JSONB       NOT NULL DEFAULT '{}',
    acknowledged    BOOLEAN     NOT NULL DEFAULT false,
    acknowledged_by UUID        REFERENCES users(id) ON DELETE SET NULL,
    acknowledged_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_alerts_env_time ON alerts(env_id, created_at DESC);
CREATE INDEX idx_alerts_unacked  ON alerts(env_id, acknowledged) WHERE acknowledged = false;

CREATE TABLE notification_channels (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    type        TEXT        NOT NULL,
    config      JSONB       NOT NULL,
    is_active   BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alert_deliveries (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id    UUID        NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    channel_id  UUID        NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    status      TEXT        NOT NULL DEFAULT 'pending',
    attempt     INT         NOT NULL DEFAULT 1,
    error       TEXT,
    sent_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_deliveries_alert   ON alert_deliveries(alert_id);
CREATE INDEX idx_deliveries_pending ON alert_deliveries(status) WHERE status = 'pending';