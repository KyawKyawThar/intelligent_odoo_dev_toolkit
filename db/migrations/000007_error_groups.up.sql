-- Section 7: Error Tracking

CREATE TABLE error_groups (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    env_id           UUID        NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    signature        TEXT        NOT NULL,
    error_type       TEXT        NOT NULL,
    message          TEXT        NOT NULL,
    module           TEXT,
    model            TEXT,
    first_seen       TIMESTAMPTZ NOT NULL,
    last_seen        TIMESTAMPTZ NOT NULL,
    occurrence_count INT         NOT NULL DEFAULT 1,
    affected_users   INT[]       NOT NULL DEFAULT '{}',
    status           TEXT        NOT NULL DEFAULT 'open',
    resolved_by      UUID        REFERENCES users(id) ON DELETE SET NULL,
    resolved_at      TIMESTAMPTZ,
    raw_trace_ref    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_error_group_sig      ON error_groups(env_id, signature);
CREATE INDEX        idx_error_group_status   ON error_groups(env_id, status);
CREATE INDEX        idx_error_group_lastseen ON error_groups(env_id, last_seen DESC);