package kyc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/kyc"
)

type mockRepo struct {
	createProfileFn              func(ctx context.Context, p *domain.KYCProfile) error
	findProfileByUserIDFn        func(ctx context.Context, userID string) (*domain.KYCProfile, error)
	updateProfileFn              func(ctx context.Context, p *domain.KYCProfile) error
	tombstoneProfileFn           func(ctx context.Context, userID string) error
	createCaseFn                 func(ctx context.Context, vc *domain.VerificationCase) error
	findCaseByIDFn               func(ctx context.Context, id string) (*domain.VerificationCase, error)
	updateCaseFn                 func(ctx context.Context, vc *domain.VerificationCase) error
	listPendingCasesFn           func(ctx context.Context, limit int) ([]domain.VerificationCase, error)
	listCasesStuckFn             func(ctx context.Context, olderThan time.Time, limit int) ([]domain.VerificationCase, error)
	createCheckFn                func(ctx context.Context, c *domain.Check) error
	findCheckByProviderEventIDFn func(ctx context.Context, pid string) (*domain.Check, error)
	updateCheckFn                func(ctx context.Context, c *domain.Check) error
	listChecksExpiringFn         func(ctx context.Context, t time.Time, limit int) ([]domain.Check, error)
	createOwnershipClaimFn       func(ctx context.Context, oc *domain.OwnershipClaim) error
	findOwnershipClaimByIDFn     func(ctx context.Context, id string) (*domain.OwnershipClaim, error)
	updateOwnershipClaimFn       func(ctx context.Context, oc *domain.OwnershipClaim) error
	listProfilesRescreenFn       func(ctx context.Context, minTier domain.Tier, limit int, cursor string) ([]domain.KYCProfile, error)
}

func (m *mockRepo) CreateProfile(ctx context.Context, p *domain.KYCProfile) error {
	if m.createProfileFn != nil {
		return m.createProfileFn(ctx, p)
	}
	return nil
}

func (m *mockRepo) FindProfileByUserID(ctx context.Context, userID string) (*domain.KYCProfile, error) {
	if m.findProfileByUserIDFn != nil {
		return m.findProfileByUserIDFn(ctx, userID)
	}
	return nil, apperrors.NotFound("kyc profile not found")
}

func (m *mockRepo) UpdateProfile(ctx context.Context, p *domain.KYCProfile) error {
	if m.updateProfileFn != nil {
		return m.updateProfileFn(ctx, p)
	}
	return nil
}

func (m *mockRepo) CreateCase(ctx context.Context, vc *domain.VerificationCase) error {
	if m.createCaseFn != nil {
		return m.createCaseFn(ctx, vc)
	}
	vc.ID = "case-id"
	vc.CreatedAt = time.Now()
	vc.UpdatedAt = time.Now()
	return nil
}

func (m *mockRepo) FindCaseByID(ctx context.Context, id string) (*domain.VerificationCase, error) {
	if m.findCaseByIDFn != nil {
		return m.findCaseByIDFn(ctx, id)
	}
	return nil, apperrors.NotFound("verification case not found")
}

func (m *mockRepo) UpdateCase(ctx context.Context, vc *domain.VerificationCase) error {
	if m.updateCaseFn != nil {
		return m.updateCaseFn(ctx, vc)
	}
	return nil
}

func (m *mockRepo) ListPendingCases(ctx context.Context, limit int) ([]domain.VerificationCase, error) {
	if m.listPendingCasesFn != nil {
		return m.listPendingCasesFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockRepo) ListCasesStuckInReview(ctx context.Context, olderThan time.Time, limit int) ([]domain.VerificationCase, error) {
	if m.listCasesStuckFn != nil {
		return m.listCasesStuckFn(ctx, olderThan, limit)
	}
	return nil, nil
}

func (m *mockRepo) ListChecksExpiringBefore(ctx context.Context, t time.Time, limit int) ([]domain.Check, error) {
	if m.listChecksExpiringFn != nil {
		return m.listChecksExpiringFn(ctx, t, limit)
	}
	return nil, nil
}

func (m *mockRepo) TombstoneProfile(ctx context.Context, userID string) error {
	if m.tombstoneProfileFn != nil {
		return m.tombstoneProfileFn(ctx, userID)
	}
	return nil
}

func (m *mockRepo) ListProfilesForRescreen(ctx context.Context, minTier domain.Tier, limit int, cursor string) ([]domain.KYCProfile, error) {
	if m.listProfilesRescreenFn != nil {
		return m.listProfilesRescreenFn(ctx, minTier, limit, cursor)
	}
	return nil, nil
}

func (m *mockRepo) CreateCheck(ctx context.Context, c *domain.Check) error {
	if m.createCheckFn != nil {
		return m.createCheckFn(ctx, c)
	}
	c.ID = "check-id"
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	return nil
}

func (m *mockRepo) FindCheckByProviderEventID(ctx context.Context, pid string) (*domain.Check, error) {
	if m.findCheckByProviderEventIDFn != nil {
		return m.findCheckByProviderEventIDFn(ctx, pid)
	}
	return nil, apperrors.NotFound("kyc check not found")
}

func (m *mockRepo) UpdateCheck(ctx context.Context, c *domain.Check) error {
	if m.updateCheckFn != nil {
		return m.updateCheckFn(ctx, c)
	}
	return nil
}

func (m *mockRepo) CreateOwnershipClaim(ctx context.Context, oc *domain.OwnershipClaim) error {
	if m.createOwnershipClaimFn != nil {
		return m.createOwnershipClaimFn(ctx, oc)
	}
	oc.ID = "claim-id"
	oc.CreatedAt = time.Now()
	oc.UpdatedAt = time.Now()
	return nil
}

func (m *mockRepo) FindOwnershipClaimByID(ctx context.Context, id string) (*domain.OwnershipClaim, error) {
	if m.findOwnershipClaimByIDFn != nil {
		return m.findOwnershipClaimByIDFn(ctx, id)
	}
	return nil, apperrors.NotFound("ownership claim not found")
}

func (m *mockRepo) UpdateOwnershipClaim(ctx context.Context, oc *domain.OwnershipClaim) error {
	if m.updateOwnershipClaimFn != nil {
		return m.updateOwnershipClaimFn(ctx, oc)
	}
	return nil
}

type mockAudit struct {
	appendFn     func(ctx context.Context, e *domain.AuditEvent) error
	listByUserFn func(ctx context.Context, userID string) ([]domain.AuditEvent, error)
}

func (m *mockAudit) Append(ctx context.Context, e *domain.AuditEvent) error {
	if m.appendFn != nil {
		return m.appendFn(ctx, e)
	}
	return nil
}

func (m *mockAudit) ListByUserID(ctx context.Context, userID string) ([]domain.AuditEvent, error) {
	if m.listByUserFn != nil {
		return m.listByUserFn(ctx, userID)
	}
	return nil, nil
}

type mockIdentity struct {
	verifyDocumentFn func(ctx context.Context, r domain.DocumentVerificationRequest) (*domain.ProviderCheckResult, error)
	verifyLivenessFn func(ctx context.Context, r domain.LivenessRequest) (*domain.ProviderCheckResult, error)
}

func (m *mockIdentity) VerifyDocument(ctx context.Context, r domain.DocumentVerificationRequest) (*domain.ProviderCheckResult, error) {
	if m.verifyDocumentFn != nil {
		return m.verifyDocumentFn(ctx, r)
	}
	return approvedResult(), nil
}

func (m *mockIdentity) VerifyLiveness(ctx context.Context, r domain.LivenessRequest) (*domain.ProviderCheckResult, error) {
	if m.verifyLivenessFn != nil {
		return m.verifyLivenessFn(ctx, r)
	}
	return approvedResult(), nil
}

type mockSanctions struct {
	screenFn func(ctx context.Context, r domain.SanctionsRequest) (*domain.ProviderCheckResult, error)
}

func (m *mockSanctions) Screen(ctx context.Context, r domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
	if m.screenFn != nil {
		return m.screenFn(ctx, r)
	}
	return approvedResult(), nil
}

type mockOwnership struct {
	verifyOwnershipFn func(ctx context.Context, r domain.OwnershipVerificationRequest) (*domain.ProviderCheckResult, error)
}

func (m *mockOwnership) VerifyOwnership(ctx context.Context, r domain.OwnershipVerificationRequest) (*domain.ProviderCheckResult, error) {
	if m.verifyOwnershipFn != nil {
		return m.verifyOwnershipFn(ctx, r)
	}
	return approvedResult(), nil
}

type mockLicense struct {
	verifyLicenseFn func(ctx context.Context, r domain.LicenseVerificationRequest) (*domain.ProviderCheckResult, error)
}

func (m *mockLicense) VerifyLicense(ctx context.Context, r domain.LicenseVerificationRequest) (*domain.ProviderCheckResult, error) {
	if m.verifyLicenseFn != nil {
		return m.verifyLicenseFn(ctx, r)
	}
	return approvedResult(), nil
}

type mockBusiness struct {
	verifyBusinessFn func(ctx context.Context, r domain.BusinessVerificationRequest) (*domain.ProviderCheckResult, error)
}

func (m *mockBusiness) VerifyBusiness(ctx context.Context, r domain.BusinessVerificationRequest) (*domain.ProviderCheckResult, error) {
	if m.verifyBusinessFn != nil {
		return m.verifyBusinessFn(ctx, r)
	}
	return approvedResult(), nil
}

type mockVault struct {
	storeFn        func(ctx context.Context, u domain.DocumentUpload) (domain.DocumentReference, error)
	signedURLFn    func(ctx context.Context, r domain.DocumentReference, ttl time.Duration) (string, error)
	purgeFn        func(ctx context.Context, r domain.DocumentReference) error
	purgeExpiredFn func(ctx context.Context) (int, error)
	purgeUserFn    func(ctx context.Context, userID string) (int, error)
}

func (m *mockVault) Store(ctx context.Context, u domain.DocumentUpload) (domain.DocumentReference, error) {
	if m.storeFn != nil {
		return m.storeFn(ctx, u)
	}
	return "doc-ref", nil
}

func (m *mockVault) SignedURL(ctx context.Context, r domain.DocumentReference, ttl time.Duration) (string, error) {
	if m.signedURLFn != nil {
		return m.signedURLFn(ctx, r, ttl)
	}
	return "https://example.com/doc?sig=abc", nil
}

func (m *mockVault) Purge(ctx context.Context, r domain.DocumentReference) error {
	if m.purgeFn != nil {
		return m.purgeFn(ctx, r)
	}
	return nil
}

func (m *mockVault) PurgeExpired(ctx context.Context) (int, error) {
	if m.purgeExpiredFn != nil {
		return m.purgeExpiredFn(ctx)
	}
	return 0, nil
}

func (m *mockVault) PurgeUser(ctx context.Context, userID string) (int, error) {
	if m.purgeUserFn != nil {
		return m.purgeUserFn(ctx, userID)
	}
	return 0, nil
}

type mockNotifier struct {
	notifyFn func(ctx context.Context, n domain.Notification) error
}

func (m *mockNotifier) Notify(ctx context.Context, n domain.Notification) error {
	if m.notifyFn != nil {
		return m.notifyFn(ctx, n)
	}
	return nil
}

type mockTierCache struct {
	getFn func(ctx context.Context, userID string) (*domain.KYCProfile, error)
	setFn func(ctx context.Context, userID string, profile *domain.KYCProfile, ttl time.Duration) error
	delFn func(ctx context.Context, userID string) error
}

func (m *mockTierCache) Get(ctx context.Context, userID string) (*domain.KYCProfile, error) {
	if m.getFn != nil {
		return m.getFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockTierCache) Set(ctx context.Context, userID string, profile *domain.KYCProfile, ttl time.Duration) error {
	if m.setFn != nil {
		return m.setFn(ctx, userID, profile, ttl)
	}
	return nil
}

func (m *mockTierCache) Del(ctx context.Context, userID string) error {
	if m.delFn != nil {
		return m.delFn(ctx, userID)
	}
	return nil
}

func newServiceWithCache(repo domain.KYCRepository, cache domain.TierCache) domain.KYCService {
	return kyc.NewService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{}, &mockNotifier{}, cache, nil, &mockMetrics{})
}

func approvedResult() *domain.ProviderCheckResult {
	return &domain.ProviderCheckResult{
		ProviderEventID: "evt-abc",
		Verdict:         domain.VerdictApproved,
		RiskScore:       10,
		RawPayload:      map[string]any{"source": "test"},
	}
}

func rejectedResult() *domain.ProviderCheckResult {
	return &domain.ProviderCheckResult{
		ProviderEventID: "evt-rej",
		Verdict:         domain.VerdictRejected,
		RiskScore:       80,
		RawPayload:      map[string]any{"reason": "mismatch"},
	}
}

func reviewResult() *domain.ProviderCheckResult {
	return &domain.ProviderCheckResult{
		ProviderEventID: "evt-review",
		Verdict:         domain.VerdictReview,
		RiskScore:       50,
		RawPayload:      map[string]any{"note": "manual check"},
	}
}

func defaultProfile(userID string) *domain.KYCProfile {
	return &domain.KYCProfile{
		UserID: userID,
		Role:   domain.RoleBuyer,
		Tier:   domain.TierAnonymous,
		Status: domain.StatusPending,
	}
}

func newService(
	repo domain.KYCRepository,
	audit domain.AuditRepository,
	identity domain.IdentityVerifier,
	sanctions domain.SanctionsScreener,
	ownership domain.OwnershipVerifier,
	license domain.LicenseVerifier,
	business domain.BusinessVerifier,
	vault domain.DocumentVault,
) domain.KYCService {
	return kyc.NewService(repo, audit, identity, sanctions, ownership, license, business, vault, &mockNotifier{}, nil, nil, &mockMetrics{})
}

func TestGetProfile(t *testing.T) {
	t.Run("existing profile", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified}, nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		p, err := svc.GetProfile(context.Background(), "u1")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.UserID != "u1" {
			t.Errorf("userID = %q, want %q", p.UserID, "u1")
		}
		if p.Tier != domain.TierVerified {
			t.Errorf("tier = %v, want %v", p.Tier, domain.TierVerified)
		}
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mockRepo{}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.GetProfile(context.Background(), "unknown")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestEvaluateAccess(t *testing.T) {
	ctx := context.Background()

	t.Run("satisfied when tier meets requirement", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified}, nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		dec, err := svc.EvaluateAccess(ctx, "u1", domain.ActionMakeOffer)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !dec.Satisfied {
			t.Error("expected satisfied")
		}
		if len(dec.MissingQualifications) != 0 {
			t.Errorf("missing qualifications = %v, want empty", dec.MissingQualifications)
		}
	})

	t.Run("not satisfied when tier too low", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return &domain.KYCProfile{UserID: userID, Tier: domain.TierIdentified}, nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		dec, err := svc.EvaluateAccess(ctx, "u1", domain.ActionMakeOffer)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dec.Satisfied {
			t.Error("expected not satisfied")
		}
		if dec.RequiredTier != domain.TierVerified {
			t.Errorf("required tier = %v, want %v", dec.RequiredTier, domain.TierVerified)
		}
	})

	t.Run("missing qualifications listed", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified}, nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		dec, err := svc.EvaluateAccess(ctx, "u1", domain.ActionReceiveRent)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dec.Satisfied {
			t.Error("expected not satisfied")
		}
		if len(dec.MissingQualifications) != 2 {
			t.Errorf("missing qualifications count = %d, want 2", len(dec.MissingQualifications))
		}
	})

	t.Run("in-memory anonymous profile when none exists", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
				return nil, apperrors.NotFound("kyc profile not found")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		dec, err := svc.EvaluateAccess(ctx, "new-user", domain.ActionBrowse)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !dec.Satisfied {
			t.Error("expected satisfied for anonymous browse")
		}
	})

	t.Run("unknown action returns error", func(t *testing.T) {
		repo := &mockRepo{}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.EvaluateAccess(ctx, "u1", "nonexistent_action")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeBadRequest {
			t.Errorf("expected CodeBadRequest, got %v", err)
		}
	})

	t.Run("repo error propagates", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
				return nil, errors.New("connection refused")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.EvaluateAccess(ctx, "u1", domain.ActionBrowse)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestStartVerification(t *testing.T) {
	ctx := context.Background()

	t.Run("creates pending case", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return defaultProfile(userID), nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		vc, err := svc.StartVerification(ctx, domain.StartVerificationInput{
			UserID: "u1",
			Role:   domain.RoleBuyer,
			Type:   domain.CheckDocument,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vc.Status != domain.StatusPending {
			t.Errorf("status = %v, want %v", vc.Status, domain.StatusPending)
		}
		if vc.Type != domain.CheckDocument {
			t.Errorf("type = %v, want %v", vc.Type, domain.CheckDocument)
		}
	})

	t.Run("sanctions case created as pending and enqueued", func(t *testing.T) {
		var caseCreated bool
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return defaultProfile(userID), nil
			},
			createCaseFn: func(_ context.Context, c *domain.VerificationCase) error {
				caseCreated = true
				c.ID = "case-1"
				return nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		vc, err := svc.StartVerification(ctx, domain.StartVerificationInput{
			UserID: "u1",
			Role:   domain.RoleBuyer,
			Type:   domain.CheckSanctions,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !caseCreated {
			t.Error("expected case to be created")
		}
		if vc.Status != domain.StatusPending {
			t.Errorf("status = %v, want %v", vc.Status, domain.StatusPending)
		}
	})

	t.Run("creates profile when none exists", func(t *testing.T) {
		var created bool
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
				return nil, apperrors.NotFound("not found")
			},
			createProfileFn: func(_ context.Context, p *domain.KYCProfile) error {
				created = true
				return nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		vc, err := svc.StartVerification(ctx, domain.StartVerificationInput{
			UserID: "new-u1",
			Role:   domain.RoleBuyer,
			Type:   domain.CheckDocument,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !created {
			t.Error("expected profile to be created")
		}
		if vc.UserID != "new-u1" {
			t.Errorf("userID = %q, want %q", vc.UserID, "new-u1")
		}
	})

	t.Run("risk step-up reflected in case risk score", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return defaultProfile(userID), nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		vc, err := svc.StartVerification(ctx, domain.StartVerificationInput{
			UserID: "u1",
			Role:   domain.RoleBuyer,
			Type:   domain.CheckDocument,
			Signals: domain.RiskSignals{
				DocumentTampered: true,
				Velocity:         10,
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vc.RiskScore < 50 {
			t.Errorf("risk score = %d, expected high for tampered document", vc.RiskScore)
		}
		if !vc.RiskScore.RequiresStepUp() {
			t.Error("expected step-up for high risk score")
		}
	})
}

func TestSubmitDocument(t *testing.T) {
	ctx := context.Background()

	t.Run("enqueues job and returns nil for owned case", func(t *testing.T) {
		var identityCalled bool
		repo := &mockRepo{
			findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
				return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
			},
		}
		identity := &mockIdentity{
			verifyDocumentFn: func(_ context.Context, _ domain.DocumentVerificationRequest) (*domain.ProviderCheckResult, error) {
				identityCalled = true
				return nil, errors.New("should not be called inline")
			},
		}
		svc := newService(repo, &mockAudit{}, identity, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.SubmitDocument(ctx, domain.SubmitDocumentInput{
			UserID:            "u1",
			CaseID:            "case-1",
			DocumentReference: "doc-ref-1",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if identityCalled {
			t.Error("identity should not be called inline; verification is async")
		}
	})

	t.Run("forbidden when user does not own case", func(t *testing.T) {
		repo := &mockRepo{
			findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
				return &domain.VerificationCase{ID: id, UserID: "other-user", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.SubmitDocument(ctx, domain.SubmitDocumentInput{
			UserID:            "u1",
			CaseID:            "case-1",
			DocumentReference: "doc-ref-1",
		})

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeForbidden {
			t.Errorf("expected CodeForbidden, got %v", err)
		}
	})
}

func TestUploadDocument(t *testing.T) {
	ctx := context.Background()

	ownedCase := func(_ context.Context, id string) (*domain.VerificationCase, error) {
		return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
	}

	t.Run("stores document and returns pending case", func(t *testing.T) {
		var stored bool
		var identityCalled bool
		repo := &mockRepo{
			findCaseByIDFn: ownedCase,
		}
		vault := &mockVault{
			storeFn: func(_ context.Context, _ domain.DocumentUpload) (domain.DocumentReference, error) {
				stored = true
				return "doc-ref-1", nil
			},
		}
		identity := &mockIdentity{
			verifyDocumentFn: func(_ context.Context, _ domain.DocumentVerificationRequest) (*domain.ProviderCheckResult, error) {
				identityCalled = true
				return nil, errors.New("should not be called inline")
			},
		}
		svc := newService(repo, &mockAudit{}, identity, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, vault)

		vc, err := svc.UploadDocument(ctx, domain.UploadDocumentInput{
			UserID: "u1", CaseID: "case-1", ContentType: "image/jpeg", Content: []byte("bytes"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !stored {
			t.Error("expected document to be stored in vault")
		}
		if identityCalled {
			t.Error("identity should not be called inline; verification is async")
		}
		if vc.Status != domain.StatusPending {
			t.Errorf("status = %v, want %v", vc.Status, domain.StatusPending)
		}
	})

	t.Run("case not found propagates", func(t *testing.T) {
		svc := newService(&mockRepo{}, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
		_, err := svc.UploadDocument(ctx, domain.UploadDocumentInput{UserID: "u1", CaseID: "nope", ContentType: "image/png", Content: []byte("x")})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("forbidden when caller does not own case", func(t *testing.T) {
		repo := &mockRepo{
			findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
				return &domain.VerificationCase{ID: id, UserID: "other", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
		_, err := svc.UploadDocument(ctx, domain.UploadDocumentInput{UserID: "u1", CaseID: "case-1", ContentType: "image/png", Content: []byte("x")})
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeForbidden {
			t.Errorf("expected CodeForbidden, got %v", err)
		}
	})

	t.Run("vault store error propagates", func(t *testing.T) {
		repo := &mockRepo{findCaseByIDFn: ownedCase}
		vault := &mockVault{
			storeFn: func(_ context.Context, _ domain.DocumentUpload) (domain.DocumentReference, error) {
				return "", errors.New("vault unavailable")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, vault)
		_, err := svc.UploadDocument(ctx, domain.UploadDocumentInput{UserID: "u1", CaseID: "case-1", ContentType: "image/png", Content: []byte("x")})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestGetCase(t *testing.T) {
	ctx := context.Background()

	t.Run("returns case when owned by user", func(t *testing.T) {
		repo := &mockRepo{
			findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
				return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		vc, err := svc.GetCase(ctx, "u1", "case-1")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vc.ID != "case-1" {
			t.Errorf("id = %q, want %q", vc.ID, "case-1")
		}
	})

	t.Run("forbidden when user does not own case", func(t *testing.T) {
		repo := &mockRepo{
			findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
				return &domain.VerificationCase{ID: id, UserID: "other-user", Type: domain.CheckDocument}, nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.GetCase(ctx, "u1", "case-1")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeForbidden {
			t.Errorf("expected CodeForbidden, got %v", err)
		}
	})

	t.Run("not found propagates", func(t *testing.T) {
		repo := &mockRepo{}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.GetCase(ctx, "u1", "nonexistent")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestApplyVerdict(t *testing.T) {
	ctx := context.Background()

	t.Run("processes approved verdict", func(t *testing.T) {
		var updated bool
		repo := &mockRepo{
			findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
				return nil, apperrors.NotFound("not found")
			},
			findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
				return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
			},
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return &domain.KYCProfile{UserID: userID, Tier: domain.TierAnonymous}, nil
			},
			updateCaseFn: func(_ context.Context, vc *domain.VerificationCase) error {
				updated = true
				if vc.Status != domain.StatusVerified {
					t.Errorf("case status = %v, want %v", vc.Status, domain.StatusVerified)
				}
				return nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: "evt-abc",
			CaseID:          "case-1",
			Type:            domain.CheckDocument,
			Verdict:         domain.VerdictApproved,
			RiskScore:       10,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Error("expected case update")
		}
	})

	t.Run("processes rejected verdict", func(t *testing.T) {
		var updated bool
		repo := &mockRepo{
			findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
				return nil, apperrors.NotFound("not found")
			},
			findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
				return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusInReview}, nil
			},
			updateCaseFn: func(_ context.Context, vc *domain.VerificationCase) error {
				updated = true
				return nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: "evt-rej",
			CaseID:          "case-1",
			Type:            domain.CheckDocument,
			Verdict:         domain.VerdictRejected,
			RiskScore:       80,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Error("expected case update")
		}
	})

	t.Run("idempotent on duplicate provider event", func(t *testing.T) {
		var checkCreated int
		repo := &mockRepo{
			findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
				return &domain.Check{
					ProviderEventID: "evt-abc",
					Status:          domain.StatusVerified,
				}, nil
			},
			createCheckFn: func(_ context.Context, _ *domain.Check) error {
				checkCreated++
				return nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: "evt-abc",
			CaseID:          "case-1",
			Type:            domain.CheckDocument,
			Verdict:         domain.VerdictApproved,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if checkCreated > 0 {
			t.Error("expected no check creation for duplicate")
		}
	})

	t.Run("idempotent no-op when a non-terminal check already exists for the event id", func(t *testing.T) {
		var checkCreated bool
		repo := &mockRepo{
			findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
				return &domain.Check{
					ProviderEventID: "evt-abc",
					Status:          domain.StatusInReview,
				}, nil
			},
			createCheckFn: func(_ context.Context, _ *domain.Check) error {
				checkCreated = true
				return nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: "evt-abc",
			CaseID:          "case-1",
			Type:            domain.CheckDocument,
			Verdict:         domain.VerdictApproved,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if checkCreated {
			t.Error("expected no check creation when a check already exists for the event id")
		}
	})
}

func TestStartOwnershipClaim(t *testing.T) {
	ctx := context.Background()

	t.Run("creates pending claim and enqueues verification", func(t *testing.T) {
		var ownershipCalled bool
		repo := &mockRepo{
			createOwnershipClaimFn: func(_ context.Context, oc *domain.OwnershipClaim) error {
				oc.ID = "claim-1"
				return nil
			},
		}
		ownership := &mockOwnership{
			verifyOwnershipFn: func(_ context.Context, _ domain.OwnershipVerificationRequest) (*domain.ProviderCheckResult, error) {
				ownershipCalled = true
				return nil, errors.New("should not be called inline")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, ownership, &mockLicense{}, &mockBusiness{}, &mockVault{})

		claim, err := svc.StartOwnershipClaim(ctx, domain.StartOwnershipClaimInput{
			UserID:          "u1",
			PropertyAddress: "123 Main St",
			ClaimantName:    "Alice",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ownershipCalled {
			t.Error("ownership verification should not be called inline")
		}
		if claim.Status != domain.StatusPending {
			t.Errorf("status = %v, want %v", claim.Status, domain.StatusPending)
		}
		if claim.Method != domain.OwnershipMethodPublicRecord {
			t.Errorf("method = %v, want %v", claim.Method, domain.OwnershipMethodPublicRecord)
		}
	})
}

func TestSubmitLicense(t *testing.T) {
	ctx := context.Background()

	t.Run("enqueues job and returns pending case", func(t *testing.T) {
		var caseCreated bool
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified, Role: domain.RoleAgent}, nil
			},
			createCaseFn: func(_ context.Context, c *domain.VerificationCase) error {
				caseCreated = true
				c.ID = "case-1"
				return nil
			},
			updateProfileFn: func(_ context.Context, _ *domain.KYCProfile) error {
				t.Error("unexpected profile update")
				return nil
			},
			createCheckFn: func(_ context.Context, _ *domain.Check) error {
				t.Error("unexpected create check call")
				return nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		vc, err := svc.SubmitLicense(ctx, domain.SubmitLicenseInput{
			UserID:        "u1",
			Role:          domain.RoleAgent,
			LicenseNumber: "LIC-12345",
			Jurisdiction:  "CA",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vc.Status != domain.StatusPending {
			t.Errorf("status = %v, want %v", vc.Status, domain.StatusPending)
		}
		if !caseCreated {
			t.Error("expected case to be created")
		}
	})

	t.Run("returns within latency budget even with slow provider", func(t *testing.T) {
		slowLicense := &mockLicense{
			verifyLicenseFn: func(_ context.Context, _ domain.LicenseVerificationRequest) (*domain.ProviderCheckResult, error) {
				time.Sleep(5 * time.Second)
				return approvedResult(), nil
			},
		}
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified, Role: domain.RoleAgent}, nil
			},
			createCaseFn: func(_ context.Context, c *domain.VerificationCase) error {
				c.ID = "case-1"
				return nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, slowLicense, &mockBusiness{}, &mockVault{})

		start := time.Now()
		_, err := svc.SubmitLicense(ctx, domain.SubmitLicenseInput{
			UserID:        "u1",
			Role:          domain.RoleAgent,
			LicenseNumber: "LIC-12345",
			Jurisdiction:  "CA",
		})
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if duration > 200*time.Millisecond {
			t.Errorf("SubmitLicense took %v, want < 200ms (provider sleeps 5s)", duration)
		}
	})
}

func TestStartBusinessVerification(t *testing.T) {
	ctx := context.Background()

	t.Run("creates pending case and enqueues job", func(t *testing.T) {
		var businessCalled bool
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified, Role: domain.RoleBuilder}, nil
			},
			createCaseFn: func(_ context.Context, c *domain.VerificationCase) error {
				c.ID = "case-1"
				return nil
			},
		}
		business := &mockBusiness{
			verifyBusinessFn: func(_ context.Context, _ domain.BusinessVerificationRequest) (*domain.ProviderCheckResult, error) {
				businessCalled = true
				return nil, errors.New("should not be called inline")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, business, &mockVault{})

		vc, err := svc.StartBusinessVerification(ctx, domain.StartBusinessVerificationInput{
			UserID:             "u1",
			Role:               domain.RoleBuilder,
			BusinessName:       "Acme Corp",
			RegistrationNumber: "REG-123",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if businessCalled {
			t.Error("business verification should not be called inline")
		}
		if vc.Status != domain.StatusPending {
			t.Errorf("status = %v, want %v", vc.Status, domain.StatusPending)
		}
	})

	t.Run("creates profile when none exists", func(t *testing.T) {
		var created bool
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
				return nil, apperrors.NotFound("not found")
			},
			createProfileFn: func(_ context.Context, p *domain.KYCProfile) error {
				created = true
				return nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		vc, err := svc.StartBusinessVerification(ctx, domain.StartBusinessVerificationInput{
			UserID:             "new-u1",
			Role:               domain.RoleBuilder,
			BusinessName:       "Acme Corp",
			RegistrationNumber: "REG-123",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !created {
			t.Error("expected profile to be created")
		}
		if vc.UserID != "new-u1" {
			t.Errorf("userID = %q, want %q", vc.UserID, "new-u1")
		}
	})
}

func TestRoleValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService(&mockRepo{}, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

	assertBadRequest := func(t *testing.T, err error) {
		t.Helper()
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeBadRequest {
			t.Errorf("expected CodeBadRequest, got %v", err)
		}
	}

	t.Run("start verification rejects invalid role", func(t *testing.T) {
		_, err := svc.StartVerification(ctx, domain.StartVerificationInput{UserID: "u1", Role: "wizard", Type: domain.CheckDocument})
		assertBadRequest(t, err)
	})

	t.Run("submit license rejects non-agent non-lender role", func(t *testing.T) {
		_, err := svc.SubmitLicense(ctx, domain.SubmitLicenseInput{UserID: "u1", Role: domain.RoleBuyer, LicenseNumber: "LIC", Jurisdiction: "CA"})
		assertBadRequest(t, err)
	})

	t.Run("submit license accepts lender role", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified, Role: domain.RoleLender}, nil
			},
		}
		lenderSvc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
		_, err := lenderSvc.SubmitLicense(ctx, domain.SubmitLicenseInput{UserID: "u1", Role: domain.RoleLender, LicenseNumber: "NMLS-1", Jurisdiction: "CA"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("business verification rejects non-builder role", func(t *testing.T) {
		_, err := svc.StartBusinessVerification(ctx, domain.StartBusinessVerificationInput{UserID: "u1", Role: domain.RoleAgent, BusinessName: "Acme", RegistrationNumber: "REG"})
		assertBadRequest(t, err)
	})
}

func TestEvaluateRisk(t *testing.T) {
	t.Run("low risk with no signals", func(t *testing.T) {
		signals := domain.RiskSignals{}
		risk := kyc.EvaluateRisk(signals)
		if risk.Band() != domain.RiskBandLow {
			t.Errorf("band = %v, want low", risk.Band())
		}
	})

	t.Run("medium risk with tampered document", func(t *testing.T) {
		signals := domain.RiskSignals{DocumentTampered: true}
		risk := kyc.EvaluateRisk(signals)
		if risk.Band() != domain.RiskBandMedium {
			t.Errorf("band = %v, want medium (score=%d)", risk.Band(), risk)
		}
		if !risk.RequiresStepUp() {
			t.Error("expected step-up")
		}
	})

	t.Run("high risk with tampered document and high velocity", func(t *testing.T) {
		signals := domain.RiskSignals{DocumentTampered: true, Velocity: 6}
		risk := kyc.EvaluateRisk(signals)
		if risk.Band() != domain.RiskBandHigh {
			t.Errorf("band = %v, want high (score=%d)", risk.Band(), risk)
		}
		if !risk.RequiresManualReview() {
			t.Error("expected manual review")
		}
	})

	t.Run("low risk with low velocity", func(t *testing.T) {
		signals := domain.RiskSignals{Velocity: 1}
		risk := kyc.EvaluateRisk(signals)
		if risk.Band() != domain.RiskBandLow {
			t.Errorf("band = %v, want low", risk.Band())
		}
	})

	t.Run("risk score cumulative when multiple signals present", func(t *testing.T) {
		signals := domain.RiskSignals{
			DocumentTampered:  true,
			Velocity:          3,
			DeviceFingerprint: "",
		}
		risk := kyc.EvaluateRisk(signals)
		if risk < 60 {
			t.Errorf("risk score = %d, expected >= 60 for combined signals", risk)
		}
	})
}

func TestStartVerification_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("repo error creating profile", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
				return nil, apperrors.NotFound("not found")
			},
			createProfileFn: func(_ context.Context, _ *domain.KYCProfile) error {
				return errors.New("db error")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.StartVerification(ctx, domain.StartVerificationInput{
			UserID: "u1",
			Role:   domain.RoleBuyer,
			Type:   domain.CheckDocument,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("repo error creating case", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return defaultProfile(userID), nil
			},
			createCaseFn: func(_ context.Context, _ *domain.VerificationCase) error {
				return errors.New("db error")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.StartVerification(ctx, domain.StartVerificationInput{
			UserID: "u1",
			Role:   domain.RoleBuyer,
			Type:   domain.CheckDocument,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestSubmitDocument_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("case not found", func(t *testing.T) {
		repo := &mockRepo{}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.SubmitDocument(ctx, domain.SubmitDocumentInput{UserID: "u1", CaseID: "unknown", DocumentReference: "ref"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestApplyVerdict_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("check lookup returns non-NotFound error", func(t *testing.T) {
		repo := &mockRepo{
			findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
				return nil, errors.New("connection refused")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: "evt-1",
			CaseID:          "case-1",
			Type:            domain.CheckDocument,
			Verdict:         domain.VerdictApproved,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("case not found", func(t *testing.T) {
		repo := &mockRepo{
			findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
				return nil, apperrors.NotFound("not found")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: "evt-1",
			CaseID:          "unknown",
			Type:            domain.CheckDocument,
			Verdict:         domain.VerdictApproved,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("approved with license type advances tier and grants qualification", func(t *testing.T) {
		var profileUpdated bool
		repo := &mockRepo{
			findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
				return nil, apperrors.NotFound("not found")
			},
			findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
				return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckLicense, Status: domain.StatusPending}, nil
			},
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified}, nil
			},
			updateProfileFn: func(_ context.Context, p *domain.KYCProfile) error {
				profileUpdated = true
				if p.Tier != domain.TierRegulated {
					t.Errorf("tier = %v, want %v", p.Tier, domain.TierRegulated)
				}
				if !p.HasQualification(domain.QualificationLicense) {
					t.Error("expected license qualification")
				}
				return nil
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: "evt-lic",
			CaseID:          "case-1",
			Type:            domain.CheckLicense,
			Verdict:         domain.VerdictApproved,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !profileUpdated {
			t.Error("expected profile update")
		}
	})

	t.Run("advance after verification error propagates", func(t *testing.T) {
		repo := &mockRepo{
			findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
				return nil, apperrors.NotFound("not found")
			},
			findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
				return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
			},
			findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		err := svc.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: "evt-err",
			CaseID:          "case-1",
			Type:            domain.CheckDocument,
			Verdict:         domain.VerdictApproved,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestStartOwnershipClaim_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("repo error creating claim", func(t *testing.T) {
		repo := &mockRepo{
			createOwnershipClaimFn: func(_ context.Context, _ *domain.OwnershipClaim) error {
				return errors.New("db error")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.StartOwnershipClaim(ctx, domain.StartOwnershipClaimInput{UserID: "u1", PropertyAddress: "addr", ClaimantName: "name"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestSubmitLicense_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("profile not found and cannot be created", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.SubmitLicense(ctx, domain.SubmitLicenseInput{UserID: "u1", Role: domain.RoleAgent, LicenseNumber: "LIC", Jurisdiction: "CA"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("create case fails", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
				return defaultProfile(userID), nil
			},
			createCaseFn: func(_ context.Context, _ *domain.VerificationCase) error {
				return errors.New("db error")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.SubmitLicense(ctx, domain.SubmitLicenseInput{UserID: "u1", Role: domain.RoleAgent, LicenseNumber: "LIC", Jurisdiction: "CA"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestStartBusinessVerification_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("profile lookup non-NotFound error", func(t *testing.T) {
		repo := &mockRepo{
			findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

		_, err := svc.StartBusinessVerification(ctx, domain.StartBusinessVerificationInput{UserID: "u1", Role: domain.RoleBuilder, BusinessName: "Acme", RegistrationNumber: "REG"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestWriteAuditSwallowsError(t *testing.T) {
	audit := &mockAudit{
		appendFn: func(_ context.Context, _ *domain.AuditEvent) error {
			return errors.New("audit db error")
		},
	}
	repo := &mockRepo{
		findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
			return defaultProfile(userID), nil
		},
	}
	svc := newService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

	_, err := svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: "u1",
		Role:   domain.RoleBuyer,
		Type:   domain.CheckDocument,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTargetTierFor(t *testing.T) {
	t.Run("document from anonymous returns identified", func(t *testing.T) {
		got := kyc.TargetTierFor(domain.CheckDocument, domain.TierAnonymous)
		if got != domain.TierIdentified {
			t.Errorf("got %v, want %v", got, domain.TierIdentified)
		}
	})

	t.Run("document from identified stays identified", func(t *testing.T) {
		got := kyc.TargetTierFor(domain.CheckDocument, domain.TierIdentified)
		if got != domain.TierIdentified {
			t.Errorf("got %v, want %v", got, domain.TierIdentified)
		}
	})

	t.Run("liveness from anonymous returns verified", func(t *testing.T) {
		got := kyc.TargetTierFor(domain.CheckLiveness, domain.TierAnonymous)
		if got != domain.TierVerified {
			t.Errorf("got %v, want %v", got, domain.TierVerified)
		}
	})

	t.Run("liveness from verified stays verified", func(t *testing.T) {
		got := kyc.TargetTierFor(domain.CheckLiveness, domain.TierVerified)
		if got != domain.TierVerified {
			t.Errorf("got %v, want %v", got, domain.TierVerified)
		}
	})

	t.Run("sanctions from anonymous returns verified", func(t *testing.T) {
		got := kyc.TargetTierFor(domain.CheckSanctions, domain.TierAnonymous)
		if got != domain.TierVerified {
			t.Errorf("got %v, want %v", got, domain.TierVerified)
		}
	})

	t.Run("sanctions from verified stays verified", func(t *testing.T) {
		got := kyc.TargetTierFor(domain.CheckSanctions, domain.TierVerified)
		if got != domain.TierVerified {
			t.Errorf("got %v, want %v", got, domain.TierVerified)
		}
	})

	t.Run("license returns regulated regardless", func(t *testing.T) {
		got := kyc.TargetTierFor(domain.CheckLicense, domain.TierAnonymous)
		if got != domain.TierRegulated {
			t.Errorf("got %v, want %v", got, domain.TierRegulated)
		}
	})

	t.Run("kyb returns regulated regardless", func(t *testing.T) {
		got := kyc.TargetTierFor(domain.CheckKYB, domain.TierAnonymous)
		if got != domain.TierRegulated {
			t.Errorf("got %v, want %v", got, domain.TierRegulated)
		}
	})
}

func TestQualificationFor(t *testing.T) {
	t.Run("ownership returns ownership qualification", func(t *testing.T) {
		got := kyc.QualificationFor(domain.CheckOwnership)
		if got != domain.QualificationOwnership {
			t.Errorf("got %v, want %v", got, domain.QualificationOwnership)
		}
	})

	t.Run("license returns license qualification", func(t *testing.T) {
		got := kyc.QualificationFor(domain.CheckLicense)
		if got != domain.QualificationLicense {
			t.Errorf("got %v, want %v", got, domain.QualificationLicense)
		}
	})

	t.Run("kyb returns kyb qualification", func(t *testing.T) {
		got := kyc.QualificationFor(domain.CheckKYB)
		if got != domain.QualificationKYB {
			t.Errorf("got %v, want %v", got, domain.QualificationKYB)
		}
	})

	t.Run("payout aml returns payout qualification", func(t *testing.T) {
		got := kyc.QualificationFor(domain.CheckPayoutAML)
		if got != domain.QualificationPayoutAML {
			t.Errorf("got %v, want %v", got, domain.QualificationPayoutAML)
		}
	})

	t.Run("document returns empty", func(t *testing.T) {
		got := kyc.QualificationFor(domain.CheckDocument)
		if got != "" {
			t.Errorf("got %v, want empty", got)
		}
	})

	t.Run("liveness returns empty", func(t *testing.T) {
		got := kyc.QualificationFor(domain.CheckLiveness)
		if got != "" {
			t.Errorf("got %v, want empty", got)
		}
	})

	t.Run("sanctions returns empty", func(t *testing.T) {
		got := kyc.QualificationFor(domain.CheckSanctions)
		if got != "" {
			t.Errorf("got %v, want empty", got)
		}
	})
}

func TestEventForTerminalStatus(t *testing.T) {
	t.Run("verified returns approve", func(t *testing.T) {
		got := kyc.EventForTerminalStatus(domain.StatusVerified)
		if got != domain.EventApprove {
			t.Errorf("got %v, want %v", got, domain.EventApprove)
		}
	})

	t.Run("rejected returns reject", func(t *testing.T) {
		got := kyc.EventForTerminalStatus(domain.StatusRejected)
		if got != domain.EventReject {
			t.Errorf("got %v, want %v", got, domain.EventReject)
		}
	})

	t.Run("in review defaults to approve", func(t *testing.T) {
		got := kyc.EventForTerminalStatus(domain.StatusInReview)
		if got != domain.EventApprove {
			t.Errorf("got %v, want %v", got, domain.EventApprove)
		}
	})

	t.Run("pending defaults to approve", func(t *testing.T) {
		got := kyc.EventForTerminalStatus(domain.StatusPending)
		if got != domain.EventApprove {
			t.Errorf("got %v, want %v", got, domain.EventApprove)
		}
	})
}

func TestSubmitDocument_EnqueueDoesNotCreateCheckOrUpdateCase(t *testing.T) {
	ctx := context.Background()
	var checkAttempted bool
	var caseUpdated bool
	repo := &mockRepo{
		findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
			return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
		},
		createCheckFn: func(_ context.Context, _ *domain.Check) error {
			checkAttempted = true
			return nil
		},
		updateCaseFn: func(_ context.Context, _ *domain.VerificationCase) error {
			caseUpdated = true
			return nil
		},
	}
	svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

	err := svc.SubmitDocument(ctx, domain.SubmitDocumentInput{UserID: "u1", CaseID: "case-1", DocumentReference: "ref"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checkAttempted {
		t.Error("check creation should not happen inline; verification is async")
	}
	if caseUpdated {
		t.Error("case update should not happen inline; verification is async")
	}
}

func TestApplyVerdict_ErrorCreateCheck(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, apperrors.NotFound("not found")
		},
		findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
			return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
		},
		createCheckFn: func(_ context.Context, _ *domain.Check) error {
			return errors.New("db error")
		},
	}
	svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

	err := svc.ApplyVerdict(ctx, domain.VerdictResult{
		ProviderEventID: "evt-1",
		CaseID:          "case-1",
		Type:            domain.CheckDocument,
		Verdict:         domain.VerdictApproved,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestApplyVerdict_ErrorUpdateCase(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, apperrors.NotFound("not found")
		},
		findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
			return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
		},
		updateCaseFn: func(_ context.Context, _ *domain.VerificationCase) error {
			return errors.New("db error")
		},
	}
	svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

	err := svc.ApplyVerdict(ctx, domain.VerdictResult{
		ProviderEventID: "evt-1",
		CaseID:          "case-1",
		Type:            domain.CheckDocument,
		Verdict:         domain.VerdictApproved,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStartOwnershipClaim_EnqueueDoesNotUpdateClaim(t *testing.T) {
	ctx := context.Background()
	var claimUpdated bool
	repo := &mockRepo{
		createOwnershipClaimFn: func(_ context.Context, oc *domain.OwnershipClaim) error {
			oc.ID = "claim-1"
			return nil
		},
		updateOwnershipClaimFn: func(_ context.Context, _ *domain.OwnershipClaim) error {
			claimUpdated = true
			return nil
		},
	}
	svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

	_, err := svc.StartOwnershipClaim(ctx, domain.StartOwnershipClaimInput{UserID: "u1", PropertyAddress: "addr", ClaimantName: "name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claimUpdated {
		t.Error("claim update should not happen inline; verification is async")
	}
}

func TestStartBusinessVerification_ErrorCreateProfile(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{
		findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
			return nil, apperrors.NotFound("not found")
		},
		createProfileFn: func(_ context.Context, _ *domain.KYCProfile) error {
			return errors.New("db error")
		},
	}
	svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

	_, err := svc.StartBusinessVerification(ctx, domain.StartBusinessVerificationInput{UserID: "u1", Role: domain.RoleBuilder, BusinessName: "Acme", RegistrationNumber: "REG"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStartBusinessVerification_ErrorCreateCase(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{
		createCaseFn: func(_ context.Context, _ *domain.VerificationCase) error {
			return errors.New("db error")
		},
	}
	svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

	_, err := svc.StartBusinessVerification(ctx, domain.StartBusinessVerificationInput{UserID: "u1", Role: domain.RoleBuilder, BusinessName: "Acme", RegistrationNumber: "REG"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStartBusinessVerification_EnqueueDoesNotCallVerifyOrCreateCheck(t *testing.T) {
	ctx := context.Background()
	var businessCalled bool
	var checkCreated bool
	repo := &mockRepo{
		createCaseFn: func(_ context.Context, c *domain.VerificationCase) error {
			c.ID = "case-1"
			return nil
		},
		createCheckFn: func(_ context.Context, _ *domain.Check) error {
			checkCreated = true
			return nil
		},
	}
	business := &mockBusiness{
		verifyBusinessFn: func(_ context.Context, _ domain.BusinessVerificationRequest) (*domain.ProviderCheckResult, error) {
			businessCalled = true
			return nil, errors.New("provider error")
		},
	}
	svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, business, &mockVault{})

	_, err := svc.StartBusinessVerification(ctx, domain.StartBusinessVerificationInput{UserID: "u1", Role: domain.RoleBuilder, BusinessName: "Acme", RegistrationNumber: "REG"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if businessCalled {
		t.Error("business verification should not be called inline")
	}
	if checkCreated {
		t.Error("check creation should not happen inline")
	}
}

func TestAdvanceAfterVerification_ErrorUpdateProfile(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, apperrors.NotFound("not found")
		},
		findCaseByIDFn: func(_ context.Context, id string) (*domain.VerificationCase, error) {
			return &domain.VerificationCase{ID: id, UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
		},
		findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
			return &domain.KYCProfile{UserID: "u1", Tier: domain.TierAnonymous}, nil
		},
		updateProfileFn: func(_ context.Context, _ *domain.KYCProfile) error {
			return errors.New("db error")
		},
	}
	svc := newService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})

	err := svc.ApplyVerdict(ctx, domain.VerdictResult{
		ProviderEventID: "evt-1",
		CaseID:          "case-1",
		Type:            domain.CheckDocument,
		Verdict:         domain.VerdictApproved,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEvaluateRisk_IPWithoutGeo(t *testing.T) {
	signals := domain.RiskSignals{
		IPAddress:         "203.0.113.1",
		GeoCountry:        "",
		DeviceFingerprint: "fp-456",
	}
	score := kyc.EvaluateRisk(signals)
	if score < 10 {
		t.Errorf("expected score >= 10 for IP without geo, got %d", score)
	}
}

func TestNotifyOnTerminalVerdict(t *testing.T) {
	ctx := context.Background()

	var notified []domain.Notification
	notifier := &mockNotifier{
		notifyFn: func(_ context.Context, n domain.Notification) error {
			notified = append(notified, n)
			return nil
		},
	}

	repo := &mockRepo{
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, apperrors.NotFound("not found")
		},
		findCaseByIDFn: func(_ context.Context, _ string) (*domain.VerificationCase, error) {
			return &domain.VerificationCase{
				ID:     "case-1",
				UserID: "u1",
				Status: domain.StatusPending,
			}, nil
		},
		findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
			return &domain.KYCProfile{UserID: "u1", Tier: domain.TierAnonymous}, nil
		},
	}

	svc := kyc.NewService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{}, notifier, nil, nil, &mockMetrics{})

	err := svc.ApplyVerdict(ctx, domain.VerdictResult{
		ProviderEventID: "evt-notify-1",
		CaseID:          "case-1",
		Type:            domain.CheckDocument,
		Verdict:         domain.VerdictApproved,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notified) == 0 {
		t.Fatal("expected notify to be called on terminal verdict, got zero calls")
	}
	if notified[0].CaseID != "case-1" {
		t.Errorf("notification case_id = %q, want %q", notified[0].CaseID, "case-1")
	}
}

func TestNotifyErrorDoesNotFailOperation(t *testing.T) {
	ctx := context.Background()

	notifier := &mockNotifier{
		notifyFn: func(_ context.Context, _ domain.Notification) error {
			return errors.New("push service down")
		},
	}

	repo := &mockRepo{
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, apperrors.NotFound("not found")
		},
		findCaseByIDFn: func(_ context.Context, _ string) (*domain.VerificationCase, error) {
			return &domain.VerificationCase{
				ID:     "case-2",
				UserID: "u1",
				Status: domain.StatusPending,
			}, nil
		},
		findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
			return &domain.KYCProfile{UserID: "u1", Tier: domain.TierAnonymous}, nil
		},
	}

	svc := kyc.NewService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{}, notifier, nil, nil, &mockMetrics{})

	err := svc.ApplyVerdict(ctx, domain.VerdictResult{
		ProviderEventID: "evt-notify-2",
		CaseID:          "case-2",
		Type:            domain.CheckDocument,
		Verdict:         domain.VerdictApproved,
	})
	if err != nil {
		t.Fatalf("notify error must not propagate; got: %v", err)
	}
}

func TestGetProfile_CacheHitSkipsRepo(t *testing.T) {
	repoCalled := false
	repo := &mockRepo{
		findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
			repoCalled = true
			return nil, errors.New("repo must not be called on a cache hit")
		},
	}
	cache := &mockTierCache{
		getFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
			return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified}, nil
		},
	}
	svc := newServiceWithCache(repo, cache)

	profile, err := svc.GetProfile(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.Tier != domain.TierVerified {
		t.Errorf("tier = %v, want T2_VERIFIED", profile.Tier)
	}
	if repoCalled {
		t.Error("repo was called despite a cache hit")
	}
}

func TestGetProfile_CacheMissPopulates(t *testing.T) {
	repo := &mockRepo{
		findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
			return &domain.KYCProfile{UserID: userID, Tier: domain.TierIdentified}, nil
		},
	}
	var setUser string
	cache := &mockTierCache{
		getFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
			return nil, nil
		},
		setFn: func(_ context.Context, userID string, _ *domain.KYCProfile, _ time.Duration) error {
			setUser = userID
			return nil
		},
	}
	svc := newServiceWithCache(repo, cache)

	if _, err := svc.GetProfile(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if setUser != "u1" {
		t.Errorf("cache not populated on miss; setUser = %q", setUser)
	}
}

func TestGetProfile_CacheErrorFallsBackToRepo(t *testing.T) {
	repoCalled := false
	repo := &mockRepo{
		findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
			repoCalled = true
			return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified}, nil
		},
	}
	cache := &mockTierCache{
		getFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
			return nil, errors.New("redis down")
		},
	}
	svc := newServiceWithCache(repo, cache)

	profile, err := svc.GetProfile(context.Background(), "u1")
	if err != nil {
		t.Fatalf("cache error must not fail the request; got: %v", err)
	}
	if !repoCalled {
		t.Error("expected fallback to repo on cache error")
	}
	if profile.Tier != domain.TierVerified {
		t.Errorf("tier = %v, want T2_VERIFIED", profile.Tier)
	}
}

func TestEvaluateAccess_CacheHitSkipsRepo(t *testing.T) {
	repoCalled := false
	repo := &mockRepo{
		findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
			repoCalled = true
			return nil, errors.New("repo must not be called on a cache hit")
		},
	}
	cache := &mockTierCache{
		getFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
			return &domain.KYCProfile{UserID: userID, Tier: domain.TierVerified}, nil
		},
	}
	svc := newServiceWithCache(repo, cache)

	decision, err := svc.EvaluateAccess(context.Background(), "u1", domain.ActionMakeOffer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Satisfied {
		t.Error("verified user should satisfy make_offer")
	}
	if repoCalled {
		t.Error("repo was called despite a cache hit on the gating path")
	}
}

func TestInvalidateCache_ErrorIsSwallowed(t *testing.T) {
	repo := &mockRepo{
		findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
			return &domain.KYCProfile{UserID: userID, Role: domain.RoleAgent, Tier: domain.TierVerified, Status: domain.StatusVerified}, nil
		},
	}
	cache := &mockTierCache{
		delFn: func(_ context.Context, _ string) error {
			return errors.New("redis down")
		},
	}
	svc := newServiceWithCache(repo, cache)

	_, err := svc.SubmitLicense(context.Background(), domain.SubmitLicenseInput{
		UserID:        "u1",
		Role:          domain.RoleAgent,
		LicenseNumber: "LIC-1",
		Jurisdiction:  "CA",
	})
	if err != nil {
		t.Fatalf("a cache invalidation error must not fail the operation; got: %v", err)
	}
}

func TestTierChangeInvalidatesCache_OnSubmitLicense(t *testing.T) {
	repo := &mockRepo{
		findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
			return &domain.KYCProfile{
					UserID: userID, Role: domain.RoleAgent,
					Tier: domain.TierVerified, Status: domain.StatusVerified,
				},
				nil
		},
		createCaseFn: func(_ context.Context, c *domain.VerificationCase) error {
			c.ID = "case-1"
			return nil
		},
		updateProfileFn: func(_ context.Context, _ *domain.KYCProfile) error {
			return nil
		},
	}
	var invalidated string
	cache := &mockTierCache{
		delFn: func(_ context.Context, userID string) error {
			invalidated = userID
			return nil
		},
	}
	svc := newServiceWithCache(repo, cache)

	_, err := svc.SubmitLicense(context.Background(), domain.SubmitLicenseInput{
		UserID:        "u1",
		Role:          domain.RoleAgent,
		LicenseNumber: "LIC-1",
		Jurisdiction:  "CA",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if invalidated != "" {
		t.Errorf("cache invalidation should not happen on submit license (async); got invalidated=%q", invalidated)
	}
}

type mockQueue struct {
	enqueueFn func(ctx context.Context, job domain.VerificationJob) error
	dequeueFn func(ctx context.Context, timeout time.Duration) (*domain.VerificationJob, error)
	lenFn     func(ctx context.Context) (int, error)
}

func (m *mockQueue) Enqueue(ctx context.Context, job domain.VerificationJob) error {
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, job)
	}
	return nil
}

func (m *mockQueue) Dequeue(ctx context.Context, timeout time.Duration) (*domain.VerificationJob, error) {
	if m.dequeueFn != nil {
		return m.dequeueFn(ctx, timeout)
	}
	return nil, nil
}

func (m *mockQueue) Len(ctx context.Context) (int, error) {
	if m.lenFn != nil {
		return m.lenFn(ctx)
	}
	return 0, nil
}

type mockMetrics struct {
	recordVerdictFn       func(checkType string, verdict string)
	recordVendorLatencyFn func(provider string, d time.Duration)
	setQueueDepthFn       func(n int)
	incCircuitOpenFn      func(provider string)
}

func (m *mockMetrics) RecordVerdict(checkType string, verdict string) {
	if m.recordVerdictFn != nil {
		m.recordVerdictFn(checkType, verdict)
	}
}

func (m *mockMetrics) RecordVendorLatency(provider string, d time.Duration) {
	if m.recordVendorLatencyFn != nil {
		m.recordVendorLatencyFn(provider, d)
	}
}

func (m *mockMetrics) SetQueueDepth(n int) {
	if m.setQueueDepthFn != nil {
		m.setQueueDepthFn(n)
	}
}

func (m *mockMetrics) IncCircuitOpen(provider string) {
	if m.incCircuitOpenFn != nil {
		m.incCircuitOpenFn(provider)
	}
}

func newServiceWithQueue(repo domain.KYCRepository, queue domain.VerificationQueue) domain.KYCService {
	return kyc.NewService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{}, &mockNotifier{}, nil, queue, &mockMetrics{})
}

func TestSubmitLicense_EnqueuesJobWithProviderInputs(t *testing.T) {
	repo := &mockRepo{
		findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
			return &domain.KYCProfile{UserID: userID, Role: domain.RoleAgent, Tier: domain.TierVerified, Status: domain.StatusVerified}, nil
		},
		createCaseFn: func(_ context.Context, vc *domain.VerificationCase) error {
			vc.ID = "case-lic-1"
			return nil
		},
	}
	var got domain.VerificationJob
	queue := &mockQueue{
		enqueueFn: func(_ context.Context, job domain.VerificationJob) error {
			got = job
			return nil
		},
	}
	svc := newServiceWithQueue(repo, queue)

	if _, err := svc.SubmitLicense(context.Background(), domain.SubmitLicenseInput{
		UserID:        "u1",
		Role:          domain.RoleAgent,
		LicenseNumber: "LIC-9",
		Jurisdiction:  "CA",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Type != domain.CheckLicense {
		t.Errorf("job type = %q, want license", got.Type)
	}
	if got.CaseID != "case-lic-1" {
		t.Errorf("job case id = %q, want case-lic-1", got.CaseID)
	}
	if got.LicenseNumber != "LIC-9" || got.Jurisdiction != "CA" {
		t.Errorf("job provider inputs = %+v, want LIC-9/CA", got)
	}
}

func TestAuditEventWrittenForEveryDecisionPath(t *testing.T) {
	ctx := context.Background()

	type step struct {
		name string
		run  func(t *testing.T, audit *mockAudit)
	}

	ownedCase := &domain.VerificationCase{ID: "case-1", UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}
	pendingOwnership := &domain.OwnershipClaim{ID: "claim-1", UserID: "u1", Status: domain.StatusPending, Method: domain.OwnershipMethodPublicRecord}
	agentProfile := &domain.KYCProfile{UserID: "u1", Role: domain.RoleAgent, Tier: domain.TierVerified, Status: domain.StatusVerified}
	builderProfile := &domain.KYCProfile{UserID: "u1", Role: domain.RoleBuilder, Tier: domain.TierVerified, Status: domain.StatusVerified}
	buyerProfile := defaultProfile("u1")

	steps := []step{
		{
			name: "StartVerification",
			run: func(t *testing.T, audit *mockAudit) {
				repo := &mockRepo{findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
					return buyerProfile, nil
				}}
				svc := newService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
				_, err := svc.StartVerification(ctx, domain.StartVerificationInput{UserID: "u1", Role: domain.RoleBuyer, Type: domain.CheckDocument})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "SubmitDocument",
			run: func(t *testing.T, audit *mockAudit) {
				repo := &mockRepo{findCaseByIDFn: func(_ context.Context, _ string) (*domain.VerificationCase, error) {
					return ownedCase, nil
				}}
				svc := newService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
				if err := svc.SubmitDocument(ctx, domain.SubmitDocumentInput{UserID: "u1", CaseID: "case-1", DocumentReference: "doc-ref"}); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "UploadDocument",
			run: func(t *testing.T, audit *mockAudit) {
				repo := &mockRepo{findCaseByIDFn: func(_ context.Context, _ string) (*domain.VerificationCase, error) {
					return ownedCase, nil
				}}
				vault := &mockVault{storeFn: func(_ context.Context, _ domain.DocumentUpload) (domain.DocumentReference, error) {
					return "doc-ref", nil
				}}
				svc := newService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, vault)
				_, err := svc.UploadDocument(ctx, domain.UploadDocumentInput{UserID: "u1", CaseID: "case-1", ContentType: "image/jpeg", Content: []byte("data")})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "ApplyVerdictApproved",
			run: func(t *testing.T, audit *mockAudit) {
				repo := &mockRepo{
					findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
						return nil, apperrors.NotFound("not found")
					},
					findCaseByIDFn: func(_ context.Context, _ string) (*domain.VerificationCase, error) {
						return &domain.VerificationCase{ID: "case-1", UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
					},
					findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
						return buyerProfile, nil
					},
				}
				svc := newService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
				if err := svc.ApplyVerdict(ctx, domain.VerdictResult{ProviderEventID: "evt-1", CaseID: "case-1", Type: domain.CheckDocument, Verdict: domain.VerdictApproved}); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "ApplyVerdictRejected",
			run: func(t *testing.T, audit *mockAudit) {
				repo := &mockRepo{
					findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
						return nil, apperrors.NotFound("not found")
					},
					findCaseByIDFn: func(_ context.Context, _ string) (*domain.VerificationCase, error) {
						return &domain.VerificationCase{ID: "case-2", UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusInReview}, nil
					},
				}
				svc := newService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
				if err := svc.ApplyVerdict(ctx, domain.VerdictResult{ProviderEventID: "evt-2", CaseID: "case-2", Type: domain.CheckDocument, Verdict: domain.VerdictRejected}); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "ApplyVerdictReview",
			run: func(t *testing.T, audit *mockAudit) {
				repo := &mockRepo{
					findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
						return nil, apperrors.NotFound("not found")
					},
					findCaseByIDFn: func(_ context.Context, _ string) (*domain.VerificationCase, error) {
						return &domain.VerificationCase{ID: "case-3", UserID: "u1", Type: domain.CheckDocument, Status: domain.StatusPending}, nil
					},
				}
				svc := newService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
				if err := svc.ApplyVerdict(ctx, domain.VerdictResult{ProviderEventID: "evt-3", CaseID: "case-3", Type: domain.CheckDocument, Verdict: domain.VerdictReview}); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "StartOwnershipClaim",
			run: func(t *testing.T, audit *mockAudit) {
				repo := &mockRepo{
					createOwnershipClaimFn: func(_ context.Context, oc *domain.OwnershipClaim) error {
						*oc = *pendingOwnership
						oc.ID = "claim-1"
						return nil
					},
				}
				svc := newService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
				_, err := svc.StartOwnershipClaim(ctx, domain.StartOwnershipClaimInput{UserID: "u1", PropertyAddress: "1 Main St", ClaimantName: "Alice"})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "SubmitLicense",
			run: func(t *testing.T, audit *mockAudit) {
				repo := &mockRepo{
					findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
						return agentProfile, nil
					},
				}
				svc := newService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
				_, err := svc.SubmitLicense(ctx, domain.SubmitLicenseInput{UserID: "u1", Role: domain.RoleAgent, LicenseNumber: "LIC-1", Jurisdiction: "CA"})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "StartBusinessVerification",
			run: func(t *testing.T, audit *mockAudit) {
				repo := &mockRepo{
					findProfileByUserIDFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
						return builderProfile, nil
					},
				}
				svc := newService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{})
				_, err := svc.StartBusinessVerification(ctx, domain.StartBusinessVerificationInput{UserID: "u1", Role: domain.RoleBuilder, BusinessName: "Acme", RegistrationNumber: "REG-1"})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
	}

	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			var count int
			audit := &mockAudit{
				appendFn: func(_ context.Context, _ *domain.AuditEvent) error {
					count++
					return nil
				},
			}
			s.run(t, audit)
			if count == 0 {
				t.Error("no audit event written — decision path is not audited")
			}
		})
	}
}

func TestDeleteProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("purges vault, tombstones profile, writes audit", func(t *testing.T) {
		var purgedUser string
		var tombstonedUser string
		var auditWritten bool
		repo := &mockRepo{
			tombstoneProfileFn: func(_ context.Context, userID string) error {
				tombstonedUser = userID
				return nil
			},
		}
		vault := &mockVault{
			purgeUserFn: func(_ context.Context, userID string) (int, error) {
				purgedUser = userID
				return 1, nil
			},
		}
		audit := &mockAudit{
			appendFn: func(_ context.Context, _ *domain.AuditEvent) error {
				auditWritten = true
				return nil
			},
		}
		svc := kyc.NewService(repo, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, vault, &mockNotifier{}, nil, nil, &mockMetrics{})

		err := svc.DeleteProfile(ctx, "u1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if purgedUser != "u1" {
			t.Errorf("purgedUser = %q, want %q", purgedUser, "u1")
		}
		if tombstonedUser != "u1" {
			t.Errorf("tombstonedUser = %q, want %q", tombstonedUser, "u1")
		}
		if !auditWritten {
			t.Error("expected audit event for profile_deleted")
		}
	})

	t.Run("purge error propagates", func(t *testing.T) {
		vault := &mockVault{
			purgeUserFn: func(_ context.Context, _ string) (int, error) {
				return 0, errors.New("vault down")
			},
		}
		svc := kyc.NewService(&mockRepo{}, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, vault, &mockNotifier{}, nil, nil, &mockMetrics{})
		err := svc.DeleteProfile(ctx, "u1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("tombstone error propagates", func(t *testing.T) {
		repo := &mockRepo{
			tombstoneProfileFn: func(_ context.Context, _ string) error {
				return errors.New("db down")
			},
		}
		svc := kyc.NewService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{}, &mockNotifier{}, nil, nil, &mockMetrics{})
		err := svc.DeleteProfile(ctx, "u1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("audit error is swallowed", func(t *testing.T) {
		audit := &mockAudit{
			appendFn: func(_ context.Context, _ *domain.AuditEvent) error {
				return errors.New("audit down")
			},
		}
		svc := kyc.NewService(&mockRepo{}, audit, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{}, &mockNotifier{}, nil, nil, &mockMetrics{})
		err := svc.DeleteProfile(ctx, "u1")
		if err != nil {
			t.Fatalf("audit error should be swallowed: %v", err)
		}
	})
}

func TestEnqueueFailurePropagates(t *testing.T) {
	repo := &mockRepo{
		findProfileByUserIDFn: func(_ context.Context, userID string) (*domain.KYCProfile, error) {
			return &domain.KYCProfile{UserID: userID, Role: domain.RoleAgent, Tier: domain.TierVerified, Status: domain.StatusVerified}, nil
		},
		createCaseFn: func(_ context.Context, vc *domain.VerificationCase) error {
			vc.ID = "case-1"
			return nil
		},
	}
	queue := &mockQueue{
		enqueueFn: func(_ context.Context, _ domain.VerificationJob) error {
			return errors.New("redis down")
		},
	}
	svc := newServiceWithQueue(repo, queue)

	_, err := svc.SubmitLicense(context.Background(), domain.SubmitLicenseInput{
		UserID:        "u1",
		Role:          domain.RoleAgent,
		LicenseNumber: "LIC-9",
		Jurisdiction:  "CA",
	})
	if err == nil {
		t.Fatal("enqueue failure must propagate (a 202 would be a false success — the job is lost)")
	}
}
