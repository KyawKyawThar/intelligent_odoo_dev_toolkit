-- Rollback Section 15: Triggers

DROP TRIGGER IF EXISTS trg_anon_profiles_updated_at ON anon_profiles;
DROP TRIGGER IF EXISTS trg_subscriptions_updated_at ON subscriptions;
DROP TRIGGER IF EXISTS trg_environments_updated_at ON environments;
DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
DROP TRIGGER IF EXISTS trg_tenants_updated_at ON tenants;
DROP FUNCTION IF EXISTS set_updated_at();