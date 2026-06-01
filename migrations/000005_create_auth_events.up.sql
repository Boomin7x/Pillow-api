CREATE TABLE IF NOT EXISTS auth_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type TEXT NOT NULL,
    user_id    UUID REFERENCES users (id),
    ip         TEXT,
    user_agent TEXT,
    metadata   JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auth_events_user_id ON auth_events (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_auth_events_type    ON auth_events (event_type, created_at DESC);
