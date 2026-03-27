-- name: CreatePerfBudget :one
INSERT INTO perf_budgets (
    env_id,
    module,
    endpoint,
    threshold_pct
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: GetPerfBudget :one
SELECT * FROM perf_budgets
WHERE id = $1 AND env_id = $2
LIMIT 1;

-- name: ListPerfBudgets :many
SELECT * FROM perf_budgets
WHERE env_id = $1 AND is_active = true
ORDER BY module, endpoint;

-- name: ListAllPerfBudgetsByEnv :many
SELECT * FROM perf_budgets
WHERE env_id = $1
ORDER BY module, endpoint;

-- name: UpdatePerfBudget :one
UPDATE perf_budgets
SET
    threshold_pct = $2,
    is_active = $3
WHERE id = $1 AND env_id = $4
RETURNING *;

-- name: DeletePerfBudget :exec
DELETE FROM perf_budgets
WHERE id = $1 AND env_id = $2;

-- ════════════════════════════════════════════
--  Budget Samples (from ingest worker)
-- ════════════════════════════════════════════

-- name: InsertBudgetSample :one
INSERT INTO perf_budget_samples (
    budget_id,
    overhead_pct,
    total_ms,
    module_ms,
    breakdown
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: ListBudgetSamples :many
SELECT * FROM perf_budget_samples
WHERE budget_id = $1
ORDER BY sampled_at DESC
LIMIT $2;

-- name: ListBudgetSamplesBetween :many
SELECT * FROM perf_budget_samples
WHERE budget_id = $1
  AND sampled_at >= $2
  AND sampled_at <= $3
ORDER BY sampled_at ASC;

-- name: GetLatestBudgetSample :one
SELECT * FROM perf_budget_samples
WHERE budget_id = $1
ORDER BY sampled_at DESC
LIMIT 1;

-- name: GetBudgetAverage7d :one
SELECT
    CASE WHEN count(overhead_pct) > 0 THEN avg(overhead_pct)::numeric(5,2) ELSE 0.00 END AS avg_overhead,
    CASE WHEN count(overhead_pct) > 0 THEN max(overhead_pct) ELSE 0 END AS max_overhead,
    count(overhead_pct) AS sample_count
FROM perf_budget_samples
WHERE budget_id = $1
  AND sampled_at > now() - interval '7 days';

-- name: GetBudgetSampleByID :one
SELECT * FROM perf_budget_samples
WHERE id = $1 AND budget_id = $2
LIMIT 1;

-- name: DeleteOldBudgetSamples :execresult
DELETE FROM perf_budget_samples
WHERE budget_id IN (
    SELECT pb.id FROM perf_budgets pb
    JOIN environments e ON pb.env_id = e.id
    WHERE e.tenant_id = $1
)
AND sampled_at < $2;