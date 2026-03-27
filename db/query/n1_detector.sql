-- name: GetN1PatternsSummary :many
-- Aggregates N+1 detections by model:method across time windows.
-- Returns per-pattern stats including occurrence count, total/peak calls,
-- total/peak duration, sample SQL, and first/last seen timestamps.
SELECT
    model,
    method,
    count(*)::int               AS occurrences,
    sum(call_count)::int        AS total_calls,
    max(call_count)::int        AS peak_calls,
    sum(total_ms)::int          AS total_duration_ms,
    max(total_ms)::int          AS peak_ms,
    (sum(call_count)::float / GREATEST(count(*), 1))::float8 AS avg_calls_per_window,
    min(period)::timestamptz    AS first_seen,
    max(period)::timestamptz    AS last_seen,
    -- Pick the sample_sql from the row with the highest call_count.
    (array_agg(sample_sql ORDER BY call_count DESC))[1]::text AS sample_sql
FROM orm_stats
WHERE env_id = $1
  AND n1_detected = true
  AND period >= $2
GROUP BY model, method
ORDER BY total_duration_ms DESC
LIMIT $3;

-- name: CountN1Windows :one
-- Counts distinct time windows that contain at least one N+1 detection.
SELECT count(DISTINCT period)::int AS n1_windows
FROM orm_stats
WHERE env_id = $1
  AND n1_detected = true
  AND period >= $2;

-- name: GetN1Timeline :many
-- Returns per-period N+1 event counts for trend visualization.
SELECT
    period,
    count(*)::int           AS pattern_count,
    sum(call_count)::int    AS total_calls,
    sum(total_ms)::int      AS total_ms
FROM orm_stats
WHERE env_id = $1
  AND n1_detected = true
  AND period >= $2
GROUP BY period
ORDER BY period DESC
LIMIT $3;

-- name: ListN1RecordingPatterns :many
-- Extracts N+1 patterns from profiler recordings' n1_patterns JSONB.
SELECT
    pr.id,
    pr.name,
    pr.n1_patterns,
    pr.total_ms,
    pr.recorded_at
FROM profiler_recordings pr
WHERE pr.env_id = $1
  AND pr.n1_patterns IS NOT NULL
  AND pr.recorded_at >= $2
ORDER BY pr.recorded_at DESC
LIMIT $3;
