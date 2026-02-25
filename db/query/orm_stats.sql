-- name: InsertORMStat :one
INSERT INTO orm_stats (
    env_id,
    model,
    method,
    call_count,
    total_ms,
    avg_ms,
    max_ms,
    p95_ms,
    n1_detected,
    sample_sql,
    period
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: ListORMStatsByPeriod :many
SELECT * FROM orm_stats
WHERE env_id = $1
  AND period >= $2
  AND period <= $3
ORDER BY total_ms DESC;

-- name: ListTopORMStats :many
SELECT * FROM orm_stats
WHERE env_id = $1
  AND period >= $2
ORDER BY total_ms DESC
LIMIT $3;

-- name: ListN1Detections :many
SELECT * FROM orm_stats
WHERE env_id = $1
  AND n1_detected = true
  AND period >= $2
ORDER BY call_count DESC
LIMIT $3;

-- name: GetORMStatsAggregated :many
SELECT
    model,
    method,
    sum(call_count)::int AS total_calls,
    sum(total_ms)::int AS total_duration_ms,
    max(max_ms) AS peak_ms,
    bool_or(n1_detected) AS has_n1
FROM orm_stats
WHERE env_id = $1
  AND period >= $2
GROUP BY model, method
ORDER BY total_duration_ms DESC
LIMIT $3;

-- name: DeleteOldORMStats :execresult
DELETE FROM orm_stats
WHERE env_id IN (
    SELECT id FROM environments WHERE tenant_id = $1
)
AND period < $2;