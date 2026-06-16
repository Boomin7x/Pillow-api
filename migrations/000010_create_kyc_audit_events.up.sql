CREATE TABLE IF NOT EXISTS kyc_audit_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID REFERENCES users (id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    tier       INTEGER NOT NULL DEFAULT 0,
    metadata   JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kyc_audit_events_user_id ON kyc_audit_events (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_kyc_audit_events_type    ON kyc_audit_events (event_type, created_at DESC);
