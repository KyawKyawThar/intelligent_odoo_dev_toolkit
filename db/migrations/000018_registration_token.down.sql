DROP INDEX IF EXISTS idx_environments_registration_token;

ALTER TABLE environments
    DROP COLUMN IF EXISTS registration_token,
    DROP COLUMN IF EXISTS registration_token_expires_at;
