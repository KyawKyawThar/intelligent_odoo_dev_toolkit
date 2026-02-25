-- name: CreateProfilerRecording :one
INSERT INTO profiler_recordings (
    env_id,
    triggered_by,
    name,
    endpoint,
    total_ms,
    sql_count,
    sql_ms,
    python_ms,
    waterfall,
    compute_chain,
    n1_patterns,
    raw_log_ref
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
) RETURNING *;

-- name: GetProfilerRecording :one
SELECT * FROM profiler_recordings
WHERE id = $1 AND env_id = $2
LIMIT 1;

-- name: ListProfilerRecordings :many
SELECT id, env_id, triggered_by, name, endpoint,
       total_ms, sql_count, sql_ms, python_ms,
       n1_patterns IS NOT NULL AS has_n1,
       raw_log_ref, recorded_at
FROM profiler_recordings
WHERE env_id = $1
ORDER BY recorded_at DESC
LIMIT $2 OFFSET $3;

-- name: ListSlowRecordings :many
SELECT * FROM profiler_recordings
WHERE env_id = $1 AND total_ms > $2
ORDER BY total_ms DESC
LIMIT $3;

-- name: CountRecordingsByEnv :one
SELECT count(*) FROM profiler_recordings
WHERE env_id = $1;

-- name: DeleteOldProfilerRecordings :execresult
DELETE FROM profiler_recordings
WHERE env_id IN (
    SELECT id FROM environments WHERE tenant_id = $1
)
AND recorded_at < $2;