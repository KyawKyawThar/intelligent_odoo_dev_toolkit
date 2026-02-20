-- Section 11: Migration Scans

CREATE TABLE migration_scans (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    env_id         UUID        NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    triggered_by   UUID        REFERENCES users(id) ON DELETE SET NULL,
    from_version   TEXT        NOT NULL,
    to_version     TEXT        NOT NULL,
    issues         JSONB       NOT NULL,
    breaking_count INT         NOT NULL DEFAULT 0,
    warning_count  INT         NOT NULL DEFAULT 0,
    minor_count    INT         NOT NULL DEFAULT 0,
    status         TEXT        NOT NULL DEFAULT 'completed',
    scanned_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_migration_env_time ON migration_scans(env_id, scanned_at DESC);