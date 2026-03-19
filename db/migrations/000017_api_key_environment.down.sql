DROP INDEX IF EXISTS idx_api_keys_environment;
ALTER TABLE api_keys DROP COLUMN IF EXISTS environment_id;
