-- Rollback Section 2: Multi-Tenant Core

DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS tenants CASCADE;