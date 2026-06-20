ALTER TABLE kyc_checks
    ADD COLUMN expires_at TIMESTAMPTZ;

CREATE INDEX idx_kyc_checks_expiry ON kyc_checks (expires_at)
    WHERE expires_at IS NOT NULL;
