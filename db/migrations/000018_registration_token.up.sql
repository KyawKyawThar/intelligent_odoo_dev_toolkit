ALTER TABLE environments
    ADD COLUMN registration_token TEXT,
    ADD COLUMN registration_token_expires_at TIMESTAMPTZ;

CREATE UNIQUE INDEX idx_environments_registration_token
    ON environments (registration_token)
    WHERE registration_token IS NOT NULL;
