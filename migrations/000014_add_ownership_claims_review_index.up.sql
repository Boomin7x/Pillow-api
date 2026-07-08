CREATE INDEX IF NOT EXISTS idx_ownership_claims_review ON ownership_claims (status, created_at)
    WHERE status IN ('pending', 'in_review');
