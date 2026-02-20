-- Rollback Section 2: Multi-Tenant Core
-- Drop inbound FKs from later migrations that reference tenants/users

-- From 000002_auth
ALTER TABLE IF EXISTS sessions DROP CONSTRAINT IF EXISTS sessions_user_id_fkey;
ALTER TABLE IF EXISTS sessions DROP CONSTRAINT IF EXISTS sessions_tenant_id_fkey;
ALTER TABLE IF EXISTS api_keys DROP CONSTRAINT IF EXISTS api_keys_tenant_id_fkey;
ALTER TABLE IF EXISTS api_keys DROP CONSTRAINT IF EXISTS api_keys_created_by_fkey;

-- From 000003_billing
ALTER TABLE IF EXISTS subscriptions DROP CONSTRAINT IF EXISTS subscriptions_tenant_id_fkey;

-- From 000004_environments
ALTER TABLE IF EXISTS environments DROP CONSTRAINT IF EXISTS environments_tenant_id_fkey;

-- From 000006_error_tracking
ALTER TABLE IF EXISTS error_groups DROP CONSTRAINT IF EXISTS error_groups_resolved_by_fkey;

-- From 000007_profiler
ALTER TABLE IF EXISTS profiler_recordings DROP CONSTRAINT IF EXISTS profiler_recordings_triggered_by_fkey;

-- From 000010_migration_scans
ALTER TABLE IF EXISTS migration_scans DROP CONSTRAINT IF EXISTS migration_scans_triggered_by_fkey;

-- From 000011_anonymization
ALTER TABLE IF EXISTS anon_profiles DROP CONSTRAINT IF EXISTS anon_profiles_tenant_id_fkey;
ALTER TABLE IF EXISTS anon_profiles DROP CONSTRAINT IF EXISTS anon_profiles_created_by_fkey;
ALTER TABLE IF EXISTS anon_profiles DROP CONSTRAINT IF EXISTS anon_profiles_last_run_by_fkey;

-- From 000012_alerts
ALTER TABLE IF EXISTS alerts DROP CONSTRAINT IF EXISTS alerts_acknowledged_by_fkey;
ALTER TABLE IF EXISTS notification_channels DROP CONSTRAINT IF EXISTS notification_channels_tenant_id_fkey;

-- From 000013_audit_log
ALTER TABLE IF EXISTS audit_logs DROP CONSTRAINT IF EXISTS audit_logs_tenant_id_fkey;
ALTER TABLE IF EXISTS audit_logs DROP CONSTRAINT IF EXISTS audit_logs_user_id_fkey;

DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;