-- Rollback: restore acl_rules and record_rules columns, drop version.

ALTER TABLE schema_snapshots
    ADD COLUMN acl_rules    JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN record_rules JSONB NOT NULL DEFAULT '{}';

ALTER TABLE schema_snapshots
    DROP COLUMN version;
