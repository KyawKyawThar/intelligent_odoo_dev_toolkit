-- ════════════════════════════════════════════
--  Alerts
-- ════════════════════════════════════════════

-- name: CreateAlert :one
INSERT INTO alerts (
    env_id,
    type,
    severity,
    message,
    metadata
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetAlertByID :one
SELECT * FROM alerts
WHERE id = $1 AND env_id = $2
LIMIT 1;

-- name: GetAlert :one
SELECT * FROM alerts
WHERE id = $1
LIMIT 1;

-- name: ListAlerts :many
SELECT * FROM alerts a
WHERE a.env_id = $1
ORDER BY a.created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListUnacknowledgedAlerts :many
SELECT * FROM alerts a
WHERE a.env_id = $1 AND a.acknowledged = false
ORDER BY a.created_at DESC
LIMIT $2;

-- name: ListAlertsByType :many
SELECT * FROM alerts a
WHERE a.env_id = $1 AND a.type = $2
ORDER BY a.created_at DESC
LIMIT $3;

-- name: CountUnacknowledgedAlerts :one
SELECT count(*) FROM alerts
WHERE env_id = $1 AND acknowledged = false;

-- name: AcknowledgeAlert :one
UPDATE alerts
SET
    acknowledged = true,
    acknowledged_by = $2,
    acknowledged_at = now()
WHERE id = $1 AND env_id = $3
RETURNING *;

-- name: AcknowledgeAllAlerts :execresult
UPDATE alerts
SET
    acknowledged = true,
    acknowledged_by = $2,
    acknowledged_at = now()
WHERE env_id = $1 AND acknowledged = false;

-- name: HasRecentAlert :one
SELECT EXISTS (
    SELECT 1 FROM alerts
    WHERE env_id = $1
      AND type = $2
      AND metadata @> $3::jsonb
      AND created_at > now() - make_interval(mins => $4::int)
) AS exists;

-- name: DeleteOldAlerts :execresult
DELETE FROM alerts
WHERE env_id IN (
    SELECT id FROM environments WHERE tenant_id = $1
)
AND alerts.created_at < $2;

-- ════════════════════════════════════════════
--  Notification Channels
-- ════════════════════════════════════════════

-- name: CreateNotificationChannel :one
INSERT INTO notification_channels (
    tenant_id,
    name,
    type,
    config,
    is_active
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetNotificationChannel :one
SELECT * FROM notification_channels
WHERE id = $1 AND tenant_id = $2
LIMIT 1;

-- name: ListNotificationChannels :many
SELECT * FROM notification_channels nc
WHERE nc.tenant_id = $1
ORDER BY nc.created_at;

-- name: ListActiveNotificationChannels :many
SELECT * FROM notification_channels nc
WHERE nc.tenant_id = $1 AND nc.is_active = true
ORDER BY nc.created_at;

-- name: UpdateNotificationChannel :one
UPDATE notification_channels
SET
    name = $2,
    type = $3,
    config = $4,
    is_active = $5
WHERE id = $1 AND tenant_id = $6
RETURNING *;

-- name: DeleteNotificationChannel :exec
DELETE FROM notification_channels
WHERE id = $1 AND tenant_id = $2;

-- ════════════════════════════════════════════
--  Alert Deliveries (used by notification worker)
-- ════════════════════════════════════════════

-- name: CreateAlertDelivery :one
INSERT INTO alert_deliveries (
    alert_id,
    channel_id
) VALUES (
    $1, $2
) RETURNING *;


-- name: ListAlertDeliveries :many
SELECT * FROM alert_deliveries
WHERE alert_id = $1
ORDER BY alert_deliveries.created_at ASC;

-- name: ListPendingDeliveries :many
SELECT
    ad.id,
    ad.alert_id,
    ad.channel_id,
    ad.status,
    ad.attempt,
    ad.error,
    ad.sent_at,
    ad.created_at,
    nc.type AS channel_type,
    nc.config AS channel_config,
    a.type AS alert_type,
    a.severity AS alert_severity,
    a.message AS alert_message,
    a.metadata AS alert_metadata
FROM alert_deliveries ad
JOIN notification_channels nc ON ad.channel_id = nc.id
JOIN alerts a ON ad.alert_id = a.id
WHERE ad.status = 'pending'
ORDER BY ad.created_at ASC
LIMIT $1;

-- name: MarkDeliverySent :exec
UPDATE alert_deliveries
SET
    status = 'sent',
    sent_at = now()
WHERE id = $1;

-- name: MarkDeliveryFailed :exec
UPDATE alert_deliveries
SET
    status = 'failed',
    error = $2,
    attempt = attempt + 1
WHERE id = $1;

-- name: RetryFailedDeliveries :execresult
UPDATE alert_deliveries
SET status = 'pending'
WHERE status = 'failed'
  AND attempt < $1
  AND alert_deliveries.created_at > now() - interval '24 hours';