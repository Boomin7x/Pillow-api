CREATE TABLE IF NOT EXISTS ownership_claims (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    property_address TEXT NOT NULL,
    claimant_name    TEXT NOT NULL,
    status           TEXT NOT NULL,
    method           TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ownership_claims_user_id ON ownership_claims (user_id, created_at DESC);
