-- Section 8: Profiler

CREATE TABLE profiler_recordings (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    env_id        UUID        NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    triggered_by  UUID        REFERENCES users(id) ON DELETE SET NULL,
    name          TEXT        NOT NULL,
    endpoint      TEXT,
    total_ms      INT         NOT NULL,
    sql_count     INT,
    sql_ms        INT,
    python_ms     INT,
    waterfall     JSONB       NOT NULL,
    compute_chain JSONB,
    n1_patterns   JSONB,
    raw_log_ref   TEXT,
    recorded_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_profiler_env_time ON profiler_recordings(env_id, recorded_at DESC);