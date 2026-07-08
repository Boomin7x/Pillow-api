//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	infrakyccache "github.com/kodiahbertrand/pillow/internal/infrastructure/kyccache"
	infranotify "github.com/kodiahbertrand/pillow/internal/infrastructure/notify"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	infraredis "github.com/kodiahbertrand/pillow/internal/infrastructure/redis"
	infrastorage "github.com/kodiahbertrand/pillow/internal/infrastructure/storage"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/pkg/metrics"
	"github.com/kodiahbertrand/pillow/pkg/resilience"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
)

var (
	_ domain.IdentityVerifier  = verdictProvider{}
	_ domain.SanctionsScreener = verdictProvider{}
	_ domain.OwnershipVerifier = verdictProvider{}
	_ domain.LicenseVerifier   = verdictProvider{}
	_ domain.BusinessVerifier  = verdictProvider{}
)

type verdictProvider struct {
	verdict domain.Verdict
	checkID string
	err     error
}

func (p verdictProvider) VerifyDocument(_ context.Context, _ domain.DocumentVerificationRequest) (*domain.ProviderCheckResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.result(), nil
}

func (p verdictProvider) VerifyLiveness(_ context.Context, _ domain.LivenessRequest) (*domain.ProviderCheckResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.result(), nil
}

func (p verdictProvider) Screen(_ context.Context, _ domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.result(), nil
}

func (p verdictProvider) VerifyOwnership(_ context.Context, _ domain.OwnershipVerificationRequest) (*domain.ProviderCheckResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.result(), nil
}

func (p verdictProvider) VerifyLicense(_ context.Context, _ domain.LicenseVerificationRequest) (*domain.ProviderCheckResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.result(), nil
}

func (p verdictProvider) VerifyBusiness(_ context.Context, _ domain.BusinessVerificationRequest) (*domain.ProviderCheckResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.result(), nil
}

func (p verdictProvider) result() *domain.ProviderCheckResult {
	return &domain.ProviderCheckResult{
		ProviderEventID: p.checkID,
		Verdict:         p.verdict,
		RiskScore:       5,
	}
}

type journeyEnv struct {
	svc      domain.KYCService
	repo     domain.KYCRepository
	audit    domain.AuditRepository
	consumer *kyc.QueueConsumer
	queue    domain.VerificationQueue
	userID   string
	cleanup  func()
}

func newJourneyEnv(t *testing.T, p verdictProvider) journeyEnv {
	t.Helper()
	ctx := context.Background()

	db, cleanupDB := testhelpers.NewPostgresContainer(t, ctx)
	rdb, cleanupRedis := testhelpers.NewRedisContainer(t, ctx)

	vault, err := infrastorage.NewDocumentVault(config.DocumentVaultConfig{
		BasePath:         t.TempDir(),
		EncryptionKeyHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		SigningSecret:    "test-journey-signing-secret",
	})
	if err != nil {
		t.Fatalf("journey: new vault: %v", err)
	}

	repo := kyc.NewRepository(db)
	audit := kyc.NewAuditRepository(db)
	queue := infraredis.NewVerificationQueue(rdb)
	cache := infrakyccache.NewTierCache(rdb, time.Minute)

	svc := kyc.NewService(repo, audit, p, p, p, p, p, vault, infranotify.New(), cache, queue, metrics.NewNoop())

	consumer := kyc.NewQueueConsumer(repo, audit, svc, p, p, p, p, p, infranotify.New(), queue, cache, metrics.NewNoop(), false)

	user := &pgmodels.UserModel{Email: "journey@pillow.test", DisplayName: "Journey User"}
	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		t.Fatalf("journey: seed user: %v", err)
	}

	cleanup := func() {
		cleanupRedis()
		cleanupDB()
	}

	return journeyEnv{
		svc: svc, repo: repo, audit: audit,
		consumer: consumer, queue: queue,
		userID: user.ID, cleanup: cleanup,
	}
}

func drainQueue(t *testing.T, env journeyEnv) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		l, err := env.queue.Len(ctx)
		if err != nil {
			t.Fatalf("drainQueue: queue len: %v", err)
		}
		if l == 0 {
			return
		}
		env.consumer.RunOnce(ctx)
	}
	l, err := env.queue.Len(ctx)
	if err != nil {
		t.Fatalf("drainQueue: final queue len: %v", err)
	}
	if l > 0 {
		t.Errorf("drainQueue: queue not empty after 20 iterations, remaining: %d", l)
	}
}

func assertCanPerform(t *testing.T, svc domain.KYCService, userID string, action domain.Action, want bool) {
	t.Helper()
	ctx := context.Background()
	d, err := svc.EvaluateAccess(ctx, userID, action)
	if err != nil {
		t.Fatalf("assertCanPerform: EvaluateAccess(%s): %v", action, err)
	}
	if d.Satisfied != want {
		t.Errorf("EvaluateAccess(%s).Satisfied = %t, want %t", action, d.Satisfied, want)
	}
}

func TestBuyerJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictApproved}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionMakeOffer, false)

	_, err := env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleBuyer,
		Type:   domain.CheckSanctions,
	})
	if err != nil {
		t.Fatalf("buyer: start verification: %v", err)
	}

	drainQueue(t, env)

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("buyer: get profile: %v", err)
	}
	if profile.Tier != domain.TierVerified {
		t.Errorf("buyer: profile tier = %d, want %d", profile.Tier, domain.TierVerified)
	}

	assertCanPerform(t, env.svc, env.userID, domain.ActionMakeOffer, true)
}

func TestRenterJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictApproved}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionSubmitApplication, false)

	_, err := env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleRenter,
		Type:   domain.CheckSanctions,
	})
	if err != nil {
		t.Fatalf("renter: start verification: %v", err)
	}

	drainQueue(t, env)

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("renter: get profile: %v", err)
	}
	if profile.Tier != domain.TierVerified {
		t.Errorf("renter: profile tier = %d, want %d", profile.Tier, domain.TierVerified)
	}

	assertCanPerform(t, env.svc, env.userID, domain.ActionSubmitApplication, true)
}

func TestSellerJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictApproved}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionListProperty, false)

	_, err := env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleSeller,
		Type:   domain.CheckSanctions,
	})
	if err != nil {
		t.Fatalf("seller: start verification: %v", err)
	}

	drainQueue(t, env)

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("seller: get profile: %v", err)
	}
	if profile.Tier != domain.TierVerified {
		t.Errorf("seller: tier = %d, want %d after sanctions", profile.Tier, domain.TierVerified)
	}

	_, err = env.svc.StartOwnershipClaim(context.Background(), domain.StartOwnershipClaimInput{
		UserID:          env.userID,
		PropertyAddress: "123 Main St",
		ClaimantName:    "Journey User",
	})
	if err != nil {
		t.Fatalf("seller: start ownership claim: %v", err)
	}

	drainQueue(t, env)

	profile, err = env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("seller: get profile after ownership: %v", err)
	}
	if !profile.HasQualification(domain.QualificationOwnership) {
		t.Errorf("seller: missing QualificationOwnership after claim")
	}

	assertCanPerform(t, env.svc, env.userID, domain.ActionListProperty, true)
}

func TestAgentJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictApproved}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionOperateAsAgent, false)

	_, err := env.svc.SubmitLicense(context.Background(), domain.SubmitLicenseInput{
		UserID:        env.userID,
		Role:          domain.RoleAgent,
		LicenseNumber: "LIC-123",
		Jurisdiction:  "CA",
	})
	if err != nil {
		t.Fatalf("agent: submit license: %v", err)
	}

	drainQueue(t, env)

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("agent: get profile: %v", err)
	}
	if profile.Tier != domain.TierRegulated {
		t.Errorf("agent: tier = %d, want %d", profile.Tier, domain.TierRegulated)
	}
	if !profile.HasQualification(domain.QualificationLicense) {
		t.Errorf("agent: missing QualificationLicense")
	}

	assertCanPerform(t, env.svc, env.userID, domain.ActionOperateAsAgent, true)
}

func TestLenderJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictApproved}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionOperateAsLender, false)

	_, err := env.svc.SubmitLicense(context.Background(), domain.SubmitLicenseInput{
		UserID:        env.userID,
		Role:          domain.RoleLender,
		LicenseNumber: "NMLS-456",
		Jurisdiction:  "NY",
	})
	if err != nil {
		t.Fatalf("lender: submit license: %v", err)
	}

	drainQueue(t, env)

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("lender: get profile: %v", err)
	}
	if profile.Tier != domain.TierRegulated {
		t.Errorf("lender: tier = %d, want %d", profile.Tier, domain.TierRegulated)
	}
	if !profile.HasQualification(domain.QualificationLicense) {
		t.Errorf("lender: missing QualificationLicense")
	}

	assertCanPerform(t, env.svc, env.userID, domain.ActionOperateAsLender, true)
}

func TestBuilderJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictApproved}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionOperateAsBuilder, false)

	_, err := env.svc.StartBusinessVerification(context.Background(), domain.StartBusinessVerificationInput{
		UserID:             env.userID,
		Role:               domain.RoleBuilder,
		BusinessName:       "BuildCo",
		RegistrationNumber: "BR-789",
	})
	if err != nil {
		t.Fatalf("builder: start business verification: %v", err)
	}

	drainQueue(t, env)

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("builder: get profile: %v", err)
	}
	if profile.Tier != domain.TierRegulated {
		t.Errorf("builder: tier = %d, want %d", profile.Tier, domain.TierRegulated)
	}
	if !profile.HasQualification(domain.QualificationKYB) {
		t.Errorf("builder: missing QualificationKYB")
	}

	assertCanPerform(t, env.svc, env.userID, domain.ActionOperateAsBuilder, true)
}

func TestLandlordJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictApproved}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionReceiveRent, false)

	_, err := env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleLandlord,
		Type:   domain.CheckSanctions,
	})
	if err != nil {
		t.Fatalf("landlord: start verification: %v", err)
	}

	drainQueue(t, env)

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("landlord: get profile: %v", err)
	}
	if profile.Tier != domain.TierVerified {
		t.Errorf("landlord: tier = %d, want %d", profile.Tier, domain.TierVerified)
	}

	_, err = env.svc.StartOwnershipClaim(context.Background(), domain.StartOwnershipClaimInput{
		UserID:          env.userID,
		PropertyAddress: "456 Oak Ave",
		ClaimantName:    "Journey User",
	})
	if err != nil {
		t.Fatalf("landlord: start ownership claim: %v", err)
	}

	drainQueue(t, env)

	_, err = env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleLandlord,
		Type:   domain.CheckPayoutAML,
	})
	if err != nil {
		t.Fatalf("landlord: start payout aml: %v", err)
	}

	drainQueue(t, env)

	profile, err = env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("landlord: get profile final: %v", err)
	}
	if !profile.HasQualification(domain.QualificationOwnership) {
		t.Errorf("landlord: missing QualificationOwnership")
	}
	if !profile.HasQualification(domain.QualificationPayoutAML) {
		t.Errorf("landlord: missing QualificationPayoutAML")
	}

	assertCanPerform(t, env.svc, env.userID, domain.ActionReceiveRent, true)
}

func TestServiceProJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictApproved}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionReceivePayout, false)

	_, err := env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleServicePro,
		Type:   domain.CheckSanctions,
	})
	if err != nil {
		t.Fatalf("service pro: start verification: %v", err)
	}

	drainQueue(t, env)

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("service pro: get profile: %v", err)
	}
	if profile.Tier != domain.TierVerified {
		t.Errorf("service pro: tier = %d, want %d", profile.Tier, domain.TierVerified)
	}

	_, err = env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleServicePro,
		Type:   domain.CheckPayoutAML,
	})
	if err != nil {
		t.Fatalf("service pro: start payout aml: %v", err)
	}

	drainQueue(t, env)

	profile, err = env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("service pro: get profile final: %v", err)
	}
	if !profile.HasQualification(domain.QualificationPayoutAML) {
		t.Errorf("service pro: missing QualificationPayoutAML")
	}

	assertCanPerform(t, env.svc, env.userID, domain.ActionReceivePayout, true)
}

func TestRejectedVerdictJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictRejected}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionMakeOffer, false)

	vc, err := env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleBuyer,
		Type:   domain.CheckSanctions,
	})
	if err != nil {
		t.Fatalf("rejected: start verification: %v", err)
	}

	drainQueue(t, env)

	got, err := env.repo.FindCaseByID(context.Background(), vc.ID)
	if err != nil {
		t.Fatalf("rejected: find case: %v", err)
	}
	if got.Status != domain.StatusRejected {
		t.Errorf("rejected: case status = %q, want %q", got.Status, domain.StatusRejected)
	}

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("rejected: get profile: %v", err)
	}
	if profile.Tier != domain.TierAnonymous {
		t.Errorf("rejected: profile tier = %d, want %d (not advanced)", profile.Tier, domain.TierAnonymous)
	}

	assertCanPerform(t, env.svc, env.userID, domain.ActionMakeOffer, false)
}

func TestSanctionsHitJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictRejected}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	vc, err := env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleBuyer,
		Type:   domain.CheckSanctions,
	})
	if err != nil {
		t.Fatalf("sanctions hit: start verification: %v", err)
	}

	drainQueue(t, env)

	got, err := env.repo.FindCaseByID(context.Background(), vc.ID)
	if err != nil {
		t.Fatalf("sanctions hit: find case: %v", err)
	}
	if got.Status != domain.StatusRejected {
		t.Errorf("sanctions hit: case status = %q, want %q", got.Status, domain.StatusRejected)
	}
	if got.Type != domain.CheckSanctions {
		t.Errorf("sanctions hit: case type = %q, want %q", got.Type, domain.CheckSanctions)
	}

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("sanctions hit: get profile: %v", err)
	}
	if profile.Tier == domain.TierVerified {
		t.Errorf("sanctions hit: profile tier = %d, should not be TierVerified", profile.Tier)
	}
}

func TestOwnershipMismatchJourney(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictReview}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionListProperty, false)

	claim, err := env.svc.StartOwnershipClaim(context.Background(), domain.StartOwnershipClaimInput{
		UserID:          env.userID,
		PropertyAddress: "789 Pine St",
		ClaimantName:    "Journey User",
	})
	if err != nil {
		t.Fatalf("ownership mismatch: start claim: %v", err)
	}

	drainQueue(t, env)

	got, err := env.repo.FindOwnershipClaimByID(context.Background(), claim.ID)
	if err != nil {
		t.Fatalf("ownership mismatch: find claim: %v", err)
	}
	if got.Status != domain.StatusInReview {
		t.Errorf("ownership mismatch: claim status = %q, want %q", got.Status, domain.StatusInReview)
	}
	if got.Method != domain.OwnershipMethodDocument {
		t.Errorf("ownership mismatch: claim method = %q, want %q", got.Method, domain.OwnershipMethodDocument)
	}

	assertCanPerform(t, env.svc, env.userID, domain.ActionListProperty, false)
}

func TestLicenseExpiredJourney(t *testing.T) {
	ctx := context.Background()
	p := verdictProvider{verdict: domain.VerdictApproved}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	profile := &domain.KYCProfile{UserID: env.userID, Role: domain.RoleAgent, Tier: domain.TierVerified, Status: domain.StatusVerified}
	if err := env.repo.CreateProfile(ctx, profile); err != nil {
		t.Fatalf("license expired: create profile: %v", err)
	}

	vc := &domain.VerificationCase{UserID: env.userID, Type: domain.CheckLicense, Status: domain.StatusVerified}
	if err := env.repo.CreateCase(ctx, vc); err != nil {
		t.Fatalf("license expired: create case: %v", err)
	}

	past := time.Now().Add(-1 * time.Hour)
	check := &domain.Check{
		CaseID:          vc.ID,
		Type:            domain.CheckLicense,
		Status:          domain.StatusVerified,
		Verdict:         domain.VerdictApproved,
		ProviderEventID: "exp-evt",
		RiskScore:       5,
		ExpiresAt:       &past,
	}
	if err := env.repo.CreateCheck(ctx, check); err != nil {
		t.Fatalf("license expired: create check: %v", err)
	}

	checker := kyc.NewLicenseExpiryChecker(env.repo, env.audit, infranotify.New(), 2*time.Hour)
	checker.RunOnce(ctx)

	events, err := env.audit.ListByUserID(ctx, env.userID)
	if err != nil {
		t.Fatalf("license expired: list audit: %v", err)
	}
	found := false
	for _, evt := range events {
		if evt.EventType == "kyc_license_expiring" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("license expired: no kyc_license_expiring audit event")
	}
}

func TestVendorOutageJourney(t *testing.T) {
	p := verdictProvider{err: resilience.ErrCircuitOpen}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	vc, err := env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleBuyer,
		Type:   domain.CheckSanctions,
	})
	if err != nil {
		t.Fatalf("vendor outage: start verification: %v", err)
	}

	got, err := env.repo.FindCaseByID(context.Background(), vc.ID)
	if err != nil {
		t.Fatalf("vendor outage: find case: %v", err)
	}
	if got.Status != domain.StatusPending {
		t.Errorf("vendor outage: case status = %q, want %q before drain", got.Status, domain.StatusPending)
	}

	env.consumer.RunOnce(context.Background())

	got, err = env.repo.FindCaseByID(context.Background(), vc.ID)
	if err != nil {
		t.Fatalf("vendor outage: find case after run: %v", err)
	}
	if got.Status != domain.StatusPending {
		t.Errorf("vendor outage: case status = %q, want %q (still pending after circuit open)", got.Status, domain.StatusPending)
	}
}

func TestJourneyEnv_Smoke(t *testing.T) {
	p := verdictProvider{verdict: domain.VerdictApproved}
	env := newJourneyEnv(t, p)
	defer env.cleanup()

	assertCanPerform(t, env.svc, env.userID, domain.ActionMakeOffer, false)

	vc, err := env.svc.StartVerification(context.Background(), domain.StartVerificationInput{
		UserID: env.userID,
		Role:   domain.RoleBuyer,
		Type:   domain.CheckSanctions,
	})
	if err != nil {
		t.Fatalf("smoke: start verification: %v", err)
	}

	drainQueue(t, env)

	got, err := env.repo.FindCaseByID(context.Background(), vc.ID)
	if err != nil {
		t.Fatalf("smoke: find case: %v", err)
	}
	if got.Status != domain.StatusVerified {
		t.Errorf("smoke: case status = %q, want %q", got.Status, domain.StatusVerified)
	}

	profile, err := env.svc.GetProfile(context.Background(), env.userID)
	if err != nil {
		t.Fatalf("smoke: get profile: %v", err)
	}
	if profile.Tier != domain.TierVerified {
		t.Errorf("smoke: profile tier = %d, want %d", profile.Tier, domain.TierVerified)
	}
}
