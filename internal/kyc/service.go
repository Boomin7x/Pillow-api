package kyc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/pkg/metrics"
)

var _ domain.KYCService = (*kycService)(nil)

type kycService struct {
	repo      domain.KYCRepository
	audit     domain.AuditRepository
	identity  domain.IdentityVerifier
	sanctions domain.SanctionsScreener
	ownership domain.OwnershipVerifier
	license   domain.LicenseVerifier
	business  domain.BusinessVerifier
	vault     domain.DocumentVault
	notifier  domain.Notifier
	cache     domain.TierCache
	queue     domain.VerificationQueue
	metrics   metrics.Metrics
}

func NewService(
	repo domain.KYCRepository,
	audit domain.AuditRepository,
	identity domain.IdentityVerifier,
	sanctions domain.SanctionsScreener,
	ownership domain.OwnershipVerifier,
	license domain.LicenseVerifier,
	business domain.BusinessVerifier,
	vault domain.DocumentVault,
	notifier domain.Notifier,
	cache domain.TierCache,
	queue domain.VerificationQueue,
	met metrics.Metrics,
) domain.KYCService {
	return &kycService{
		repo: repo, audit: audit,
		identity: identity, sanctions: sanctions,
		ownership: ownership, license: license,
		business: business, vault: vault,
		notifier: notifier, cache: cache,
		queue: queue, metrics: met,
	}
}

func (s *kycService) GetProfile(ctx context.Context, userID string) (*domain.KYCProfile, error) {
	if cached := s.cachedProfile(ctx, userID); cached != nil {
		return cached, nil
	}

	profile, err := s.repo.FindProfileByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("kyc: get profile: %w", err)
	}

	if s.cache != nil {
		_ = s.cache.Set(ctx, userID, profile, 0)
	}
	return profile, nil
}

func (s *kycService) EvaluateAccess(ctx context.Context, userID string, action domain.Action) (*domain.AccessDecision, error) {
	profile, err := s.loadProfileForAccess(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("kyc: evaluate access: %w", err)
	}

	requirement, known := domain.RequirementFor(action)
	if !known {
		return nil, apperrors.BadRequest("unknown action")
	}

	missing := profile.MissingQualificationsFor(action)

	return &domain.AccessDecision{
		Action:                 action,
		RequiredTier:           requirement.MinTier,
		RequiredQualifications: requirement.Qualifications,
		MissingQualifications:  missing,
		Satisfied:              profile.CanPerform(action),
	}, nil
}

func (s *kycService) StartVerification(ctx context.Context, input domain.StartVerificationInput) (*domain.VerificationCase, error) {
	if !input.Role.Valid() {
		return nil, apperrors.BadRequest("invalid role")
	}

	profile, err := s.findOrCreateProfile(ctx, input.UserID, input.Role)
	if err != nil {
		return nil, fmt.Errorf("kyc: start verification: %w", err)
	}

	riskScore := EvaluateRisk(input.Signals)

	verificationCase := &domain.VerificationCase{
		UserID:    input.UserID,
		Type:      input.Type,
		Status:    domain.StatusPending,
		RiskScore: riskScore,
	}

	if err := s.repo.CreateCase(ctx, verificationCase); err != nil {
		return nil, fmt.Errorf("kyc: start verification: create case: %w", err)
	}

	if input.Type == domain.CheckSanctions || input.Type == domain.CheckPayoutAML {
		if err := s.enqueueJob(ctx, domain.VerificationJob{
			CaseID: verificationCase.ID,
			UserID: input.UserID,
			Type:   input.Type,
		}); err != nil {
			return nil, fmt.Errorf("kyc: start verification: %w", err)
		}
	}

	s.writeAudit(ctx, input.UserID, "verification_started", profile.Tier, map[string]any{
		"case_id": verificationCase.ID,
		"type":    string(input.Type),
	})

	return verificationCase, nil
}

func (s *kycService) SubmitDocument(ctx context.Context, input domain.SubmitDocumentInput) error {
	verificationCase, err := s.repo.FindCaseByID(ctx, input.CaseID)
	if err != nil {
		return fmt.Errorf("kyc: submit document: %w", err)
	}
	if verificationCase.UserID != input.UserID {
		return apperrors.Forbidden("user does not own this verification case")
	}

	if err := s.enqueueJob(ctx, domain.VerificationJob{
		CaseID:            verificationCase.ID,
		UserID:            input.UserID,
		Type:              domain.CheckDocument,
		DocumentReference: input.DocumentReference,
	}); err != nil {
		return fmt.Errorf("kyc: submit document: %w", err)
	}

	s.writeAudit(ctx, input.UserID, "document_queued", 0, map[string]any{
		"case_id": verificationCase.ID,
		"status":  "pending",
	})

	return nil
}

func (s *kycService) UploadDocument(ctx context.Context, input domain.UploadDocumentInput) (*domain.VerificationCase, error) {
	verificationCase, err := s.repo.FindCaseByID(ctx, input.CaseID)
	if err != nil {
		return nil, fmt.Errorf("kyc: upload document: %w", err)
	}
	if verificationCase.UserID != input.UserID {
		return nil, apperrors.Forbidden("user does not own this verification case")
	}

	ref, err := s.vault.Store(ctx, domain.DocumentUpload{
		UserID:      input.UserID,
		ContentType: input.ContentType,
		Content:     input.Content,
	})
	if err != nil {
		return nil, fmt.Errorf("kyc: upload document: store: %w", err)
	}

	if err := s.enqueueJob(ctx, domain.VerificationJob{
		CaseID:            verificationCase.ID,
		UserID:            input.UserID,
		Type:              domain.CheckDocument,
		DocumentReference: ref,
	}); err != nil {
		return nil, fmt.Errorf("kyc: upload document: %w", err)
	}

	s.writeAudit(ctx, input.UserID, "document_uploaded", 0, map[string]any{
		"case_id": verificationCase.ID,
		"status":  "pending",
	})

	return verificationCase, nil
}

func (s *kycService) GetCase(ctx context.Context, userID, caseID string) (*domain.VerificationCase, error) {
	verificationCase, err := s.repo.FindCaseByID(ctx, caseID)
	if err != nil {
		return nil, fmt.Errorf("kyc: get case: %w", err)
	}
	if verificationCase.UserID != userID {
		return nil, apperrors.Forbidden("user does not own this verification case")
	}
	return verificationCase, nil
}

func (s *kycService) ApplyVerdict(ctx context.Context, result domain.VerdictResult) error {
	_, err := s.repo.FindCheckByProviderEventID(ctx, result.ProviderEventID)
	if err == nil {
		return nil
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeNotFound {
		return fmt.Errorf("kyc: apply verdict: %w", err)
	}

	verificationCase, err := s.repo.FindCaseByID(ctx, result.CaseID)
	if err != nil {
		return fmt.Errorf("kyc: apply verdict: %w", err)
	}

	check := &domain.Check{
		CaseID:          result.CaseID,
		Type:            result.Type,
		Status:          domain.StatusInReview,
		Verdict:         result.Verdict,
		ProviderEventID: result.ProviderEventID,
		RiskScore:       result.RiskScore,
		RawPayload:      result.RawPayload,
	}

	switch result.Verdict {
	case domain.VerdictApproved:
		check.Status = domain.StatusVerified
	case domain.VerdictRejected:
		check.Status = domain.StatusRejected
	}

	if err := s.repo.CreateCheck(ctx, check); err != nil {
		return fmt.Errorf("kyc: apply verdict: create check: %w", err)
	}

	s.metrics.RecordVerdict(string(result.Type), string(result.Verdict))

	if check.Status.IsTerminal() {
		var transitionErr error
		verificationCase.Status, transitionErr = domain.NextStatus(verificationCase.Status, domain.EventSubmit)
		if transitionErr == nil {
			verificationCase.Status, _ = domain.NextStatus(verificationCase.Status, EventForTerminalStatus(check.Status))
		}
		if updateErr := s.repo.UpdateCase(ctx, verificationCase); updateErr != nil {
			return fmt.Errorf("kyc: apply verdict: update case: %w", updateErr)
		}
		if verificationCase.Status == domain.StatusVerified {
			if updateErr := s.advanceAfterVerification(ctx, verificationCase.UserID, result.Type); updateErr != nil {
				return updateErr
			}
		}
		s.notifyResult(ctx, verificationCase.UserID, result.CaseID, verificationCase.Status)
	}

	s.writeAudit(ctx, verificationCase.UserID, "verdict_applied", 0, map[string]any{
		"case_id":  result.CaseID,
		"verdict":  string(result.Verdict),
		"event_id": result.ProviderEventID,
	})

	return nil
}

func (s *kycService) StartOwnershipClaim(ctx context.Context, input domain.StartOwnershipClaimInput) (*domain.OwnershipClaim, error) {
	claim := &domain.OwnershipClaim{
		UserID:          input.UserID,
		PropertyAddress: input.PropertyAddress,
		ClaimantName:    input.ClaimantName,
		Status:          domain.StatusPending,
		Method:          domain.OwnershipMethodPublicRecord,
	}

	if err := s.repo.CreateOwnershipClaim(ctx, claim); err != nil {
		return nil, fmt.Errorf("kyc: start ownership claim: %w", err)
	}

	if err := s.enqueueJob(ctx, domain.VerificationJob{
		CaseID:          claim.ID,
		UserID:          input.UserID,
		Type:            domain.CheckOwnership,
		PropertyAddress: input.PropertyAddress,
		ClaimantName:    input.ClaimantName,
	}); err != nil {
		return nil, fmt.Errorf("kyc: start ownership claim: %w", err)
	}

	s.writeAudit(ctx, input.UserID, "ownership_claim_started", 0, map[string]any{
		"claim_id": claim.ID,
		"status":   "pending",
	})

	return claim, nil
}

func (s *kycService) SubmitLicense(ctx context.Context, input domain.SubmitLicenseInput) (*domain.VerificationCase, error) {
	if input.Role != domain.RoleAgent && input.Role != domain.RoleLender {
		return nil, apperrors.BadRequest("license verification requires agent or lender role")
	}

	profile, err := s.findOrCreateProfile(ctx, input.UserID, input.Role)
	if err != nil {
		return nil, fmt.Errorf("kyc: submit license: %w", err)
	}

	verificationCase := &domain.VerificationCase{
		UserID: input.UserID,
		Type:   domain.CheckLicense,
		Status: domain.StatusPending,
	}

	if err := s.repo.CreateCase(ctx, verificationCase); err != nil {
		return nil, fmt.Errorf("kyc: submit license: create case: %w", err)
	}

	if err := s.enqueueJob(ctx, domain.VerificationJob{
		CaseID:        verificationCase.ID,
		UserID:        input.UserID,
		Type:          domain.CheckLicense,
		LicenseNumber: input.LicenseNumber,
		Jurisdiction:  input.Jurisdiction,
	}); err != nil {
		return nil, fmt.Errorf("kyc: submit license: %w", err)
	}

	s.writeAudit(ctx, input.UserID, "license_submitted", profile.Tier, map[string]any{
		"case_id": verificationCase.ID,
		"status":  "pending",
	})

	return verificationCase, nil
}

func (s *kycService) StartBusinessVerification(ctx context.Context, input domain.StartBusinessVerificationInput) (*domain.VerificationCase, error) {
	if input.Role != domain.RoleBuilder {
		return nil, apperrors.BadRequest("business verification requires builder role")
	}

	profile, err := s.findOrCreateProfile(ctx, input.UserID, input.Role)
	if err != nil {
		return nil, fmt.Errorf("kyc: start business verification: %w", err)
	}

	verificationCase := &domain.VerificationCase{
		UserID: input.UserID,
		Type:   domain.CheckKYB,
		Status: domain.StatusPending,
	}

	if err := s.repo.CreateCase(ctx, verificationCase); err != nil {
		return nil, fmt.Errorf("kyc: start business verification: create case: %w", err)
	}

	if err := s.enqueueJob(ctx, domain.VerificationJob{
		CaseID:             verificationCase.ID,
		UserID:             input.UserID,
		Type:               domain.CheckKYB,
		BusinessName:       input.BusinessName,
		RegistrationNumber: input.RegistrationNumber,
	}); err != nil {
		return nil, fmt.Errorf("kyc: start business verification: %w", err)
	}

	s.writeAudit(ctx, input.UserID, "business_verification_started", profile.Tier, map[string]any{
		"case_id": verificationCase.ID,
		"status":  "pending",
	})

	return verificationCase, nil
}

func (s *kycService) DeleteProfile(ctx context.Context, userID string) error {
	if _, err := s.vault.PurgeUser(ctx, userID); err != nil {
		return fmt.Errorf("kyc: delete profile: purge documents: %w", err)
	}
	if err := s.repo.TombstoneProfile(ctx, userID); err != nil {
		return fmt.Errorf("kyc: delete profile: tombstone: %w", err)
	}
	s.invalidateCache(ctx, userID)
	s.writeAudit(ctx, userID, "profile_deleted", 0, nil)
	return nil
}

func (s *kycService) findOrCreateProfile(ctx context.Context, userID string, role domain.Role) (*domain.KYCProfile, error) {
	profile, err := s.repo.FindProfileByUserID(ctx, userID)
	if err == nil {
		return profile, nil
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeNotFound {
		return nil, err
	}
	profile = &domain.KYCProfile{
		UserID: userID,
		Role:   role,
		Tier:   domain.TierAnonymous,
		Status: domain.StatusPending,
	}
	if err := s.repo.CreateProfile(ctx, profile); err != nil {
		return nil, err
	}
	s.invalidateCache(ctx, userID)
	return profile, nil
}

func (s *kycService) notifyResult(ctx context.Context, userID, caseID string, status domain.VerificationStatus) {
	if err := s.notifier.Notify(ctx, domain.Notification{
		UserID:    userID,
		EventType: "kyc_case_status_changed",
		CaseID:    caseID,
		Status:    status,
	}); err != nil {
		slog.Error("kyc: notify failed", "error", err, "user_id", userID, "case_id", caseID)
	}
}

func (s *kycService) writeAudit(ctx context.Context, userID string, eventType string, tier domain.Tier, metadata map[string]any) {
	evt := &domain.AuditEvent{
		UserID:    userID,
		EventType: eventType,
		Tier:      tier,
		Metadata:  metadata,
	}
	if err := s.audit.Append(ctx, evt); err != nil {
		slog.Error("kyc: audit append failed", "error", err, "event_type", eventType, "user_id", userID)
	}
}

func (s *kycService) advanceAfterVerification(ctx context.Context, userID string, checkType domain.CheckType) error {
	profile, err := s.repo.FindProfileByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("kyc: advance after verification: %w", err)
	}
	profile.Tier = TargetTierFor(checkType, profile.Tier)
	if q := QualificationFor(checkType); q != "" {
		profile.GrantQualification(q)
	}
	if err := s.repo.UpdateProfile(ctx, profile); err != nil {
		return fmt.Errorf("kyc: advance after verification: update: %w", err)
	}
	s.invalidateCache(ctx, userID)
	s.writeAudit(ctx, userID, "tier_advanced", profile.Tier, map[string]any{
		"new_tier": profile.Tier.String(),
	})
	return nil
}

func (s *kycService) buildCheck(caseID string, checkType domain.CheckType, result *domain.ProviderCheckResult) *domain.Check {
	status := domain.StatusInReview
	switch result.Verdict {
	case domain.VerdictApproved:
		status = domain.StatusVerified
	case domain.VerdictRejected:
		status = domain.StatusRejected
	}
	return &domain.Check{
		CaseID:          caseID,
		Type:            checkType,
		Status:          status,
		Verdict:         result.Verdict,
		ProviderEventID: result.ProviderEventID,
		RiskScore:       result.RiskScore,
		RawPayload:      result.RawPayload,
	}
}

func (s *kycService) loadProfileForAccess(ctx context.Context, userID string) (*domain.KYCProfile, error) {
	if cached := s.cachedProfile(ctx, userID); cached != nil {
		return cached, nil
	}

	profile, err := s.repo.FindProfileByUserID(ctx, userID)
	if err != nil {
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeNotFound {
			return nil, err
		}
		return &domain.KYCProfile{UserID: userID, Tier: domain.TierAnonymous}, nil
	}

	if s.cache != nil {
		_ = s.cache.Set(ctx, userID, profile, 0)
	}
	return profile, nil
}

func (s *kycService) cachedProfile(ctx context.Context, userID string) *domain.KYCProfile {
	if s.cache == nil {
		return nil
	}
	cached, err := s.cache.Get(ctx, userID)
	if err != nil {
		slog.Warn("kyc: tier cache read failed, falling back to repo", "error", err, "user_id", userID)
		return nil
	}
	return cached
}

func (s *kycService) invalidateCache(ctx context.Context, userID string) {
	if s.cache != nil {
		if err := s.cache.Del(ctx, userID); err != nil {
			slog.Warn("kyc: tier cache invalidate failed", "error", err, "user_id", userID)
		}
	}
}

func (s *kycService) enqueueJob(ctx context.Context, job domain.VerificationJob) error {
	if s.queue == nil {
		return nil
	}
	if err := s.queue.Enqueue(ctx, job); err != nil {
		return fmt.Errorf("kyc: enqueue %s job for case %s: %w", job.Type, job.CaseID, err)
	}
	return nil
}

func EvaluateRisk(signals domain.RiskSignals) domain.RiskScore {
	var score int
	if signals.DocumentTampered {
		score += 50
	}
	if signals.Velocity > 5 {
		score += 20
	} else if signals.Velocity > 2 {
		score += 10
	}
	if signals.DeviceFingerprint == "" {
		score += 5
	}
	if signals.IPAddress != "" && signals.GeoCountry == "" {
		score += 10
	}
	return domain.RiskScore(score)
}

func TargetTierFor(checkType domain.CheckType, current domain.Tier) domain.Tier {
	switch checkType {
	case domain.CheckDocument:
		if current < domain.TierIdentified {
			return domain.TierIdentified
		}
	case domain.CheckLiveness:
		if current < domain.TierVerified {
			return domain.TierVerified
		}
	case domain.CheckSanctions:
		if current < domain.TierVerified {
			return domain.TierVerified
		}
	case domain.CheckLicense, domain.CheckKYB:
		return domain.TierRegulated
	}
	return current
}

func QualificationFor(checkType domain.CheckType) domain.Qualification {
	switch checkType {
	case domain.CheckOwnership:
		return domain.QualificationOwnership
	case domain.CheckLicense:
		return domain.QualificationLicense
	case domain.CheckKYB:
		return domain.QualificationKYB
	case domain.CheckPayoutAML:
		return domain.QualificationPayoutAML
	default:
		return ""
	}
}

func EventForTerminalStatus(status domain.VerificationStatus) domain.TransitionEvent {
	switch status {
	case domain.StatusVerified:
		return domain.EventApprove
	case domain.StatusRejected:
		return domain.EventReject
	default:
		return domain.EventApprove
	}
}
