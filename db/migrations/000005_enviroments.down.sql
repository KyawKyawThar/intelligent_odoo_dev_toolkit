-- Rollback Section 5: Environments + Agent Heartbeats
-- Drop inbound FKs from later migrations that reference environments

-- From 000006_schema_snapshots
ALTER TABLE IF EXISTS schema_snapshots DROP CONSTRAINT IF EXISTS schema_snapshots_env_id_fkey;

-- From 000006_error_tracking
ALTER TABLE IF EXISTS error_groups DROP CONSTRAINT IF EXISTS error_groups_env_id_fkey;

-- From 000007_profiler
ALTER TABLE IF EXISTS profiler_recordings DROP CONSTRAINT IF EXISTS profiler_recordings_env_id_fkey;

-- From 000008_perf_budgets
ALTER TABLE IF EXISTS perf_budgets DROP CONSTRAINT IF EXISTS perf_budgets_env_id_fkey;

-- From 000009_orm_stats
ALTER TABLE IF EXISTS orm_stats DROP CONSTRAINT IF EXISTS orm_stats_env_id_fkey;

-- From 000010_migration_scans
ALTER TABLE IF EXISTS migration_scans DROP CONSTRAINT IF EXISTS migration_scans_env_id_fkey;

-- From 000011_anonymization
ALTER TABLE IF EXISTS anon_profiles DROP CONSTRAINT IF EXISTS anon_profiles_source_env_fkey;
ALTER TABLE IF EXISTS anon_profiles DROP CONSTRAINT IF EXISTS anon_profiles_target_env_fkey;

-- From 000012_alerts
ALTER TABLE IF EXISTS alerts DROP CONSTRAINT IF EXISTS alerts_env_id_fkey;

DROP TABLE IF EXISTS agent_heartbeats;
DROP TABLE IF EXISTS environments;