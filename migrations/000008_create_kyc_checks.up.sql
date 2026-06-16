CREATE TABLE IF NOT EXISTS kyc_checks (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id           UUID NOT NULL REFERENCES verification_cases (id) ON DELETE CASCADE,
    type              TEXT NOT NULL,
    status            TEXT NOT NULL,
    verdict           TEXT NOT NULL DEFAULT '',
    provider_event_id TEXT NOT NULL DEFAULT '',
    risk_score        INTEGER NOT NULL DEFAULT 0,
    raw_payload       JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kyc_checks_case_id ON kyc_checks (case_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_kyc_checks_provider_event_id ON kyc_checks (provider_event_id)
    WHERE provider_event_id <> '';
