CREATE TABLE IF NOT EXISTS refresh_tokens (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash         TEXT NOT NULL,
    family_id          UUID NOT NULL,
    device_fingerprint TEXT NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL,
    revoked_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (token_hash)
);

CREATE INDEX IF NOT EXISTS idx_refresh_user_id   ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_family_id  ON refresh_tokens (family_id);
CREATE INDEX IF NOT EXISTS idx_refresh_expires_at ON refresh_tokens (expires_at)
    WHERE revoked_at IS NULL;
