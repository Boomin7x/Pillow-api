DROP INDEX IF EXISTS idx_kyc_checks_expiry;

ALTER TABLE kyc_checks
    DROP COLUMN IF EXISTS expires_at;
