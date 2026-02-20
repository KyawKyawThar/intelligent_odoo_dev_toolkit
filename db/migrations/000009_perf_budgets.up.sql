-- Section 9: Performance Budgets
CREATE TABLE perf_budgets (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    env_id        UUID        NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    module        TEXT        NOT NULL,
    endpoint      TEXT        NOT NULL,
    threshold_pct INT         NOT NULL DEFAULT 15,
    is_active     BOOLEAN     NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(env_id, module, endpoint)
);

CREATE TABLE perf_budget_samples (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    budget_id    UUID         NOT NULL REFERENCES perf_budgets(id) ON DELETE CASCADE,
    overhead_pct NUMERIC(5,2) NOT NULL,
    total_ms     INT,
    module_ms    INT,
    breakdown    JSONB,
    sampled_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_budget_time ON perf_budget_samples(budget_id, sampled_at DESC);
