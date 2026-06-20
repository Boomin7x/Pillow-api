ALTER TABLE kyc_profiles ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_kyc_profiles_deleted_at ON kyc_profiles(deleted_at) WHERE deleted_at IS NULL;
