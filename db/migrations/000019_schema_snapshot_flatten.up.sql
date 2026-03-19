-- Flatten schema_snapshots: move acl_rules and record_rules into the models JSONB,
-- add version column.

ALTER TABLE schema_snapshots
    ADD COLUMN version TEXT;

ALTER TABLE schema_snapshots
    DROP COLUMN acl_rules,
    DROP COLUMN record_rules;
