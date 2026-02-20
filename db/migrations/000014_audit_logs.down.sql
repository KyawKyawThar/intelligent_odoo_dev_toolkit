-- Rollback Section 14: Audit Log
-- No inbound FKs point to audit_logs

DROP TABLE IF EXISTS audit_logs;