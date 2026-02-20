-- Section 6: Schema Snapshots

CREATE TABLE schema_snapshots (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    env_id       UUID        NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    captured_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    models       JSONB       NOT NULL,
    acl_rules    JSONB       NOT NULL,
    record_rules JSONB       NOT NULL,
    model_count  INT,
    field_count  INT,
    diff_ref     TEXT
);
CREATE INDEX idx_schema_env_time ON schema_snapshots(env_id, captured_at DESC);