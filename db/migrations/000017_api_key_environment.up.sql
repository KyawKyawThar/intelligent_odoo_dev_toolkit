ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS environment_id UUID REFERENCES environments(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_api_keys_environment ON api_keys(environment_id);
