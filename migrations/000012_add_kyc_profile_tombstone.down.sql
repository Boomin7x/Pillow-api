DROP INDEX IF EXISTS idx_kyc_profiles_deleted_at;
ALTER TABLE kyc_profiles DROP COLUMN IF EXISTS deleted_at;
