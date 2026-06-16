CREATE TABLE IF NOT EXISTS verification_cases (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type       TEXT NOT NULL,
    status     TEXT NOT NULL,
    risk_score INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_verification_cases_user_id ON verification_cases (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_verification_cases_pending ON verification_cases (status, created_at)
    WHERE status IN ('pending', 'in_review');
