CREATE TABLE IF NOT EXISTS kyc_profiles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role           TEXT NOT NULL,
    tier           INTEGER NOT NULL DEFAULT 0,
    status         TEXT NOT NULL,
    qualifications JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id)
);

CREATE INDEX IF NOT EXISTS idx_kyc_profiles_tier ON kyc_profiles (tier);
