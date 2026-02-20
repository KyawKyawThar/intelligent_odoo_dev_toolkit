-- Section 10: ORM Stats (aggregated per 30s window)
-- TimescaleDB hypertable candidate at scale

CREATE TABLE orm_stats (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    env_id      UUID         NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    model       TEXT         NOT NULL,
    method      TEXT         NOT NULL,
    call_count  INT          NOT NULL,
    total_ms    INT          NOT NULL,
    avg_ms      NUMERIC(8,2),
    max_ms      INT,
    p95_ms      INT,
    n1_detected BOOLEAN      NOT NULL DEFAULT false,
    sample_sql  TEXT,
    period      TIMESTAMPTZ  NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_orm_stats_env_period ON orm_stats(env_id, period DESC);
CREATE INDEX idx_orm_stats_model      ON orm_stats(env_id, model, method);