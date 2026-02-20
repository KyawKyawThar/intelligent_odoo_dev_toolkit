-- Rollback Section 13: Alerts + Notifications
-- alert_deliveries references both alerts and notification_channels

ALTER TABLE IF EXISTS alert_deliveries DROP CONSTRAINT IF EXISTS alert_deliveries_alert_id_fkey;
ALTER TABLE IF EXISTS alert_deliveries DROP CONSTRAINT IF EXISTS alert_deliveries_channel_id_fkey;

DROP TABLE IF EXISTS alert_deliveries;
DROP TABLE IF EXISTS notification_channels;
DROP TABLE IF EXISTS alerts;