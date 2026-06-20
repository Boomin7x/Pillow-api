package kyc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/pkg/resilience"
)

func TestCaseReconciler_FlagsStuckCases(t *testing.T) {
	stuck := []domain.VerificationCase{
		{ID: "c1", UserID: "u1", Status: domain.StatusInReview, UpdatedAt: time.Now().Add(-48 * time.Hour)},
	}

	var auditEvents []*domain.AuditEvent
	repo := &mockRepo{
		listPendingCasesFn: nil,
	}
	repo.listCasesStuckFn = func(_ context.Context, _ time.Time, _ int) ([]domain.VerificationCase, error) {
		return stuck, nil
	}
	audit := &mockAudit{
		appendFn: func(_ context.Context, e *domain.AuditEvent) error {
			auditEvents = append(auditEvents, e)
			return nil
		},
	}
	notifier := &mockNotifier{}

	r := kyc.NewCaseReconciler(repo, audit, notifier, 24*time.Hour)
	r.RunOnce(context.Background())

	if len(auditEvents) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditEvents))
	}
	if auditEvents[0].EventType != "kyc_case_stuck_in_review" {
		t.Errorf("event type = %q, want kyc_case_stuck_in_review", auditEvents[0].EventType)
	}
}

func TestCaseReconciler_EmptyResultIsNoOp(t *testing.T) {
	repo := &mockRepo{}
	repo.listCasesStuckFn = func(_ context.Context, _ time.Time, _ int) ([]domain.VerificationCase, error) {
		return nil, nil
	}
	audit := &mockAudit{}
	r := kyc.NewCaseReconciler(repo, audit, &mockNotifier{}, 24*time.Hour)
	r.RunOnce(context.Background())
}

func TestCaseReconciler_RepoErrorIsSwallowed(t *testing.T) {
	repo := &mockRepo{}
	repo.listCasesStuckFn = func(_ context.Context, _ time.Time, _ int) ([]domain.VerificationCase, error) {
		return nil, errors.New("db down")
	}
	r := kyc.NewCaseReconciler(repo, &mockAudit{}, &mockNotifier{}, 24*time.Hour)
	r.RunOnce(context.Background())
}

func TestSanctionsRescreener_ScreensProfiles(t *testing.T) {
	profiles := []domain.KYCProfile{
		{UserID: "u1", Tier: domain.TierVerified},
	}
	var screened []string
	repo := &mockRepo{}
	repo.listProfilesRescreenFn = func(_ context.Context, _ domain.Tier, _ int, _ string) ([]domain.KYCProfile, error) {
		if len(screened) > 0 {
			return nil, nil
		}
		return profiles, nil
	}
	sanctions := &mockSanctions{
		screenFn: func(_ context.Context, r domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
			screened = append(screened, r.UserID)
			return approvedResult(), nil
		},
	}
	audit := &mockAudit{}
	s := kyc.NewSanctionsRescreener(repo, audit, sanctions, &mockKYCService{})
	s.RunOnce(context.Background())

	if len(screened) != 1 || screened[0] != "u1" {
		t.Errorf("screened = %v, want [u1]", screened)
	}
}

func TestSanctionsRescreener_EnforcesOnHit(t *testing.T) {
	repo := &mockRepo{}
	repo.listProfilesRescreenFn = func(_ context.Context, _ domain.Tier, _ int, cursor string) ([]domain.KYCProfile, error) {
		if cursor != "" {
			return nil, nil
		}
		return []domain.KYCProfile{{UserID: "u1", Tier: domain.TierVerified}}, nil
	}
	var createdCases int
	repo.createCaseFn = func(_ context.Context, vc *domain.VerificationCase) error {
		createdCases++
		vc.ID = "sanctions-case-1"
		return nil
	}
	sanctions := &mockSanctions{
		screenFn: func(_ context.Context, _ domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
			return rejectedResult(), nil
		},
	}
	var applied []domain.VerdictResult
	svc := &mockKYCService{
		applyVerdictFn: func(_ context.Context, r domain.VerdictResult) error {
			applied = append(applied, r)
			return nil
		},
	}

	s := kyc.NewSanctionsRescreener(repo, &mockAudit{}, sanctions, svc)
	s.RunOnce(context.Background())

	if createdCases != 1 {
		t.Fatalf("expected 1 sanctions case opened, got %d", createdCases)
	}
	if len(applied) != 1 {
		t.Fatalf("expected 1 verdict applied, got %d", len(applied))
	}
	if applied[0].Verdict != domain.VerdictRejected {
		t.Errorf("verdict = %q, want rejected", applied[0].Verdict)
	}
	if applied[0].CaseID != "sanctions-case-1" {
		t.Errorf("case id = %q, want sanctions-case-1", applied[0].CaseID)
	}
}

func TestSanctionsRescreener_HitAlreadyEnforcedIsSkipped(t *testing.T) {
	repo := &mockRepo{}
	repo.listProfilesRescreenFn = func(_ context.Context, _ domain.Tier, _ int, cursor string) ([]domain.KYCProfile, error) {
		if cursor != "" {
			return nil, nil
		}
		return []domain.KYCProfile{{UserID: "u1", Tier: domain.TierVerified}}, nil
	}
	repo.findCheckByProviderEventIDFn = func(_ context.Context, _ string) (*domain.Check, error) {
		return &domain.Check{ID: "existing"}, nil
	}
	repo.createCaseFn = func(_ context.Context, _ *domain.VerificationCase) error {
		t.Fatal("must not open a case for an already-enforced sanctions hit")
		return nil
	}
	sanctions := &mockSanctions{
		screenFn: func(_ context.Context, _ domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
			return rejectedResult(), nil
		},
	}
	s := kyc.NewSanctionsRescreener(repo, &mockAudit{}, sanctions, &mockKYCService{})
	s.RunOnce(context.Background())
}

func TestSanctionsRescreener_CircuitOpenIsSwallowed(t *testing.T) {
	repo := &mockRepo{}
	repo.listProfilesRescreenFn = func(_ context.Context, _ domain.Tier, _ int, _ string) ([]domain.KYCProfile, error) {
		return []domain.KYCProfile{{UserID: "u1"}}, nil
	}
	sanctions := &mockSanctions{
		screenFn: func(_ context.Context, _ domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
			return nil, resilience.ErrCircuitOpen
		},
	}
	s := kyc.NewSanctionsRescreener(repo, &mockAudit{}, sanctions, &mockKYCService{})
	s.RunOnce(context.Background())
}

func TestSanctionsRescreener_HitWithoutEventIDIsSkipped(t *testing.T) {
	repo := &mockRepo{}
	repo.listProfilesRescreenFn = func(_ context.Context, _ domain.Tier, _ int, cursor string) ([]domain.KYCProfile, error) {
		if cursor != "" {
			return nil, nil
		}
		return []domain.KYCProfile{{UserID: "u1", Tier: domain.TierVerified}}, nil
	}
	sanctions := &mockSanctions{
		screenFn: func(_ context.Context, _ domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
			return &domain.ProviderCheckResult{Verdict: domain.VerdictRejected, ProviderEventID: ""}, nil
		},
	}
	s := kyc.NewSanctionsRescreener(repo, &mockAudit{}, sanctions, &mockKYCService{})
	s.RunOnce(context.Background())
}

func TestLicenseExpiryChecker_FlagsExpiringLicenses(t *testing.T) {
	expiry := time.Now().Add(48 * time.Hour)
	repo := &mockRepo{}
	repo.listChecksExpiringFn = func(_ context.Context, _ time.Time, _ int) ([]domain.Check, error) {
		return []domain.Check{{ID: "chk-1", CaseID: "case-1"}}, nil
	}
	repo.findCaseByIDFn = func(_ context.Context, _ string) (*domain.VerificationCase, error) {
		return &domain.VerificationCase{ID: "case-1", UserID: "u1"}, nil
	}
	var auditEvents []*domain.AuditEvent
	audit := &mockAudit{
		appendFn: func(_ context.Context, e *domain.AuditEvent) error {
			auditEvents = append(auditEvents, e)
			return nil
		},
	}
	l := kyc.NewLicenseExpiryChecker(repo, audit, &mockNotifier{}, 7*24*time.Hour)
	l.RunOnce(context.Background())

	if len(auditEvents) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditEvents))
	}
	if auditEvents[0].EventType != "kyc_license_expiring" {
		t.Errorf("event type = %q, want kyc_license_expiring", auditEvents[0].EventType)
	}
	_ = expiry
}

func TestLicenseExpiryChecker_EmptyResultIsNoOp(t *testing.T) {
	repo := &mockRepo{}
	repo.listChecksExpiringFn = func(_ context.Context, _ time.Time, _ int) ([]domain.Check, error) {
		return nil, nil
	}
	l := kyc.NewLicenseExpiryChecker(repo, &mockAudit{}, &mockNotifier{}, 7*24*time.Hour)
	l.RunOnce(context.Background())
}

func TestLicenseExpiryChecker_RepoErrorIsSwallowed(t *testing.T) {
	repo := &mockRepo{}
	repo.listChecksExpiringFn = func(_ context.Context, _ time.Time, _ int) ([]domain.Check, error) {
		return nil, errors.New("db down")
	}
	l := kyc.NewLicenseExpiryChecker(repo, &mockAudit{}, &mockNotifier{}, 7*24*time.Hour)
	l.RunOnce(context.Background())
}

func TestVaultPurger_PurgesExpired(t *testing.T) {
	vault := &mockVault{
		purgeExpiredFn: func(_ context.Context) (int, error) {
			return 5, nil
		},
	}
	v := kyc.NewVaultPurger(vault)
	v.RunOnce(context.Background())
}

type queueConsumerTest struct {
	repo      domain.KYCRepository
	audit     domain.AuditRepository
	service   domain.KYCService
	identity  domain.IdentityVerifier
	sanctions domain.SanctionsScreener
	ownership domain.OwnershipVerifier
	license   domain.LicenseVerifier
	business  domain.BusinessVerifier
	notifier  domain.Notifier
	queue     domain.VerificationQueue
	cache     domain.TierCache
}

func newQueueConsumer(repo domain.KYCRepository, svc domain.KYCService, queue domain.VerificationQueue, providers ...any) *kyc.QueueConsumer {
	c := &queueConsumerTest{
		repo:      repo,
		audit:     &mockAudit{},
		service:   svc,
		identity:  &mockIdentity{},
		sanctions: &mockSanctions{},
		ownership: &mockOwnership{},
		license:   &mockLicense{},
		business:  &mockBusiness{},
		notifier:  &mockNotifier{},
		queue:     queue,
		cache:     &mockTierCache{},
	}
	for _, p := range providers {
		switch v := p.(type) {
		case *mockIdentity:
			c.identity = v
		case *mockSanctions:
			c.sanctions = v
		case *mockOwnership:
			c.ownership = v
		case *mockLicense:
			c.license = v
		case *mockBusiness:
			c.business = v
		}
	}
	return kyc.NewQueueConsumer(c.repo, c.audit, c.service, c.identity, c.sanctions, c.ownership, c.license, c.business, c.notifier, c.queue, c.cache, &mockMetrics{})
}

type metricRecorder struct {
	verdicts  []string
	latencies []string
	circuits  []string
	depths    []int
}

func (m *metricRecorder) RecordVerdict(checkType, verdict string) {
	m.verdicts = append(m.verdicts, checkType+":"+verdict)
}

func (m *metricRecorder) RecordVendorLatency(provider string, _ time.Duration) {
	m.latencies = append(m.latencies, provider)
}

func (m *metricRecorder) SetQueueDepth(n int) {
	m.depths = append(m.depths, n)
}

func (m *metricRecorder) IncCircuitOpen(provider string) {
	m.circuits = append(m.circuits, provider)
}

func oneJobQueue(job domain.VerificationJob) *mockQueue {
	delivered := false
	return &mockQueue{
		enqueueFn: nil,
		dequeueFn: func(_ context.Context, _ time.Duration) (*domain.VerificationJob, error) {
			if delivered {
				return nil, nil
			}
			delivered = true
			return &job, nil
		},
	}
}

func TestQueueConsumer_ProcessesLicenseJob(t *testing.T) {
	repo := &mockRepo{
		createCaseFn: nil,
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, nil
		},
	}
	license := &mockLicense{
		verifyLicenseFn: func(_ context.Context, _ domain.LicenseVerificationRequest) (*domain.ProviderCheckResult, error) {
			return approvedResult(), nil
		},
	}
	q := oneJobQueue(domain.VerificationJob{CaseID: "c1", UserID: "u1", Type: domain.CheckLicense, LicenseNumber: "LIC-1", Jurisdiction: "CA"})
	c := newQueueConsumer(repo, &mockKYCService{}, q, license)
	c.RunOnce(context.Background())
}

func TestQueueConsumer_ProcessesPayoutAMLJob(t *testing.T) {
	var applied domain.VerdictResult
	svc := &mockKYCService{
		applyVerdictFn: func(_ context.Context, r domain.VerdictResult) error {
			applied = r
			return nil
		},
	}
	sanctions := &mockSanctions{
		screenFn: func(_ context.Context, _ domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
			return &domain.ProviderCheckResult{ProviderEventID: "evt-pay", Verdict: domain.VerdictApproved}, nil
		},
	}
	q := oneJobQueue(domain.VerificationJob{CaseID: "c-pay", UserID: "u1", Type: domain.CheckPayoutAML})
	c := newQueueConsumer(&mockRepo{}, svc, q, sanctions)

	c.RunOnce(context.Background())

	if applied.Type != domain.CheckPayoutAML || applied.CaseID != "c-pay" || applied.Verdict != domain.VerdictApproved {
		t.Errorf("ApplyVerdict got %+v, want c-pay/payout_aml/approved", applied)
	}
}

func TestQueueConsumer_ProcessesKYBJob(t *testing.T) {
	repo := &mockRepo{
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, nil
		},
	}
	business := &mockBusiness{
		verifyBusinessFn: func(_ context.Context, _ domain.BusinessVerificationRequest) (*domain.ProviderCheckResult, error) {
			return approvedResult(), nil
		},
	}
	q := oneJobQueue(domain.VerificationJob{CaseID: "c2", UserID: "u2", Type: domain.CheckKYB, BusinessName: "Acme", RegistrationNumber: "123"})
	c := newQueueConsumer(repo, &mockKYCService{}, q, business)
	c.RunOnce(context.Background())
}

func TestQueueConsumer_ProcessesOwnershipJob(t *testing.T) {
	var updated *domain.OwnershipClaim
	repo := &mockRepo{
		findOwnershipClaimByIDFn: func(_ context.Context, id string) (*domain.OwnershipClaim, error) {
			return &domain.OwnershipClaim{ID: id, UserID: "u1", Status: domain.StatusPending}, nil
		},
		updateOwnershipClaimFn: func(_ context.Context, oc *domain.OwnershipClaim) error {
			updated = oc
			return nil
		},
	}
	ownership := &mockOwnership{
		verifyOwnershipFn: func(_ context.Context, _ domain.OwnershipVerificationRequest) (*domain.ProviderCheckResult, error) {
			return &domain.ProviderCheckResult{Verdict: domain.VerdictApproved}, nil
		},
	}
	q := oneJobQueue(domain.VerificationJob{CaseID: "claim-1", UserID: "u1", Type: domain.CheckOwnership, PropertyAddress: "1 Main St", ClaimantName: "Alice"})
	c := newQueueConsumer(repo, &mockKYCService{}, q, ownership)

	c.RunOnce(context.Background())

	if updated == nil || updated.Status != domain.StatusVerified {
		t.Errorf("expected ownership claim updated to verified; got %+v", updated)
	}
}

func TestQueueConsumer_OwnershipCompletionIsAudited(t *testing.T) {
	var events []*domain.AuditEvent
	audit := &mockAudit{
		appendFn: func(_ context.Context, e *domain.AuditEvent) error {
			events = append(events, e)
			return nil
		},
	}
	repo := &mockRepo{
		findOwnershipClaimByIDFn: func(_ context.Context, id string) (*domain.OwnershipClaim, error) {
			return &domain.OwnershipClaim{ID: id, UserID: "u1", Status: domain.StatusPending}, nil
		},
	}
	ownership := &mockOwnership{
		verifyOwnershipFn: func(_ context.Context, _ domain.OwnershipVerificationRequest) (*domain.ProviderCheckResult, error) {
			return &domain.ProviderCheckResult{Verdict: domain.VerdictApproved}, nil
		},
	}
	q := oneJobQueue(domain.VerificationJob{CaseID: "claim-1", UserID: "u1", Type: domain.CheckOwnership, PropertyAddress: "1 Main St", ClaimantName: "Alice"})
	c := kyc.NewQueueConsumer(repo, audit, &mockKYCService{}, &mockIdentity{}, &mockSanctions{}, ownership, &mockLicense{}, &mockBusiness{}, &mockNotifier{}, q, &mockTierCache{}, &mockMetrics{})

	c.RunOnce(context.Background())

	found := false
	for _, e := range events {
		if e.EventType == "ownership_claim_completed" {
			found = true
		}
	}
	if !found {
		t.Error("ownership completion must write an audit event (regulatory) — the verdict + qualification grant cannot be silent")
	}
}

func TestQueueConsumer_ProcessesDocumentJob(t *testing.T) {
	repo := &mockRepo{
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, nil
		},
	}
	identity := &mockIdentity{
		verifyDocumentFn: func(_ context.Context, _ domain.DocumentVerificationRequest) (*domain.ProviderCheckResult, error) {
			return approvedResult(), nil
		},
	}
	q := oneJobQueue(domain.VerificationJob{CaseID: "c3", UserID: "u3", Type: domain.CheckDocument, DocumentReference: "doc-1"})
	c := newQueueConsumer(repo, &mockKYCService{}, q, identity)
	c.RunOnce(context.Background())
}

func TestQueueConsumer_ProcessesSanctionsJob(t *testing.T) {
	repo := &mockRepo{
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, nil
		},
	}
	sanctions := &mockSanctions{
		screenFn: func(_ context.Context, _ domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
			return approvedResult(), nil
		},
	}
	q := oneJobQueue(domain.VerificationJob{CaseID: "c4", UserID: "u4", Type: domain.CheckSanctions})
	c := newQueueConsumer(repo, &mockKYCService{}, q, sanctions)
	c.RunOnce(context.Background())
}

func TestQueueConsumer_DequeueErrorIsSwallowed(t *testing.T) {
	q := &mockQueue{
		dequeueFn: func(_ context.Context, _ time.Duration) (*domain.VerificationJob, error) {
			return nil, errors.New("redis down")
		},
	}
	c := newQueueConsumer(&mockRepo{}, &mockKYCService{}, q)
	c.RunOnce(context.Background())
}

func TestMetrics_VerdictRecordedOnApplyVerdict(t *testing.T) {
	rec := &metricRecorder{}
	met := &metricsDelegate{rec: rec}
	repo := &mockRepo{
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, apperrors.NotFound("not found")
		},
		findCaseByIDFn: func(_ context.Context, _ string) (*domain.VerificationCase, error) {
			return &domain.VerificationCase{ID: "c1", UserID: "u1", Status: domain.StatusPending}, nil
		},
	}
	svc := kyc.NewService(repo, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{}, &mockNotifier{}, nil, nil, met)

	_ = svc.ApplyVerdict(context.Background(), domain.VerdictResult{
		ProviderEventID: "evt-1",
		CaseID:          "c1",
		Type:            domain.CheckDocument,
		Verdict:         domain.VerdictApproved,
	})

	if len(rec.verdicts) == 0 {
		t.Fatal("expected at least one verdict recorded")
	}
	if rec.verdicts[0] != "document:approved" {
		t.Errorf("verdict = %q, want document:approved", rec.verdicts[0])
	}
}

func TestMetrics_LatencyRecordedOnQueueConsumer(t *testing.T) {
	rec := &metricRecorder{}
	met := &metricsDelegate{rec: rec}
	repo := &mockRepo{}
	svc := createVerdictService(met)
	license := &mockLicense{
		verifyLicenseFn: func(_ context.Context, _ domain.LicenseVerificationRequest) (*domain.ProviderCheckResult, error) {
			return approvedResult(), nil
		},
	}
	q := oneJobQueue(domain.VerificationJob{CaseID: "c1", UserID: "u1", Type: domain.CheckLicense, LicenseNumber: "LIC-1", Jurisdiction: "CA"})
	c := kyc.NewQueueConsumer(repo, &mockAudit{}, svc, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, license, &mockBusiness{}, &mockNotifier{}, q, &mockTierCache{}, met)
	c.RunOnce(context.Background())

	if len(rec.latencies) == 0 {
		t.Fatal("expected at least one latency recorded")
	}
	if rec.latencies[0] != "license" {
		t.Errorf("provider = %q, want license", rec.latencies[0])
	}
}

func TestMetrics_CircuitOpenIncrementedOnSanctionsFailure(t *testing.T) {
	rec := &metricRecorder{}
	met := &metricsDelegate{rec: rec}
	repo := &mockRepo{}
	sanctions := &mockSanctions{
		screenFn: func(_ context.Context, _ domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
			return nil, resilience.ErrCircuitOpen
		},
	}
	q := oneJobQueue(domain.VerificationJob{CaseID: "c1", UserID: "u1", Type: domain.CheckSanctions})
	c := kyc.NewQueueConsumer(repo, &mockAudit{}, &mockKYCService{}, &mockIdentity{}, sanctions, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockNotifier{}, q, &mockTierCache{}, met)
	c.RunOnce(context.Background())

	if len(rec.circuits) != 1 {
		t.Fatalf("expected 1 circuit open increment, got %d", len(rec.circuits))
	}
	if rec.circuits[0] != "sanctions" {
		t.Errorf("provider = %q, want sanctions", rec.circuits[0])
	}
}

func TestMetrics_EmptyQueueProducesNoMetrics(t *testing.T) {
	rec := &metricRecorder{}
	met := &metricsDelegate{rec: rec}
	q := &mockQueue{
		dequeueFn: func(_ context.Context, _ time.Duration) (*domain.VerificationJob, error) {
			return nil, nil
		},
	}
	c := kyc.NewQueueConsumer(&mockRepo{}, &mockAudit{}, &mockKYCService{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockNotifier{}, q, &mockTierCache{}, met)
	c.RunOnce(context.Background())

	if len(rec.latencies) != 0 {
		t.Fatalf("expected no latency recording on empty queue, got %d", len(rec.latencies))
	}
}

func TestMetrics_UnknownJobTypeDoesNotPanic(t *testing.T) {
	rec := &metricRecorder{}
	met := &metricsDelegate{rec: rec}
	q := oneJobQueue(domain.VerificationJob{CaseID: "c1", UserID: "u1", Type: "unknown"})
	c := kyc.NewQueueConsumer(&mockRepo{}, &mockAudit{}, &mockKYCService{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockNotifier{}, q, &mockTierCache{}, met)
	c.RunOnce(context.Background())
}

func TestMetrics_QueueDepthEmittedEachRun(t *testing.T) {
	rec := &metricRecorder{}
	met := &metricsDelegate{rec: rec}
	q := &mockQueue{
		lenFn: func(_ context.Context) (int, error) { return 7, nil },
	}
	c := kyc.NewQueueConsumer(&mockRepo{}, &mockAudit{}, &mockKYCService{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockNotifier{}, q, &mockTierCache{}, met)

	c.RunOnce(context.Background())

	if len(rec.depths) != 1 || rec.depths[0] != 7 {
		t.Errorf("expected queue depth 7 emitted once, got %v", rec.depths)
	}
}

func TestMetrics_CircuitOpenCountedForNonSanctionsProvider(t *testing.T) {
	rec := &metricRecorder{}
	met := &metricsDelegate{rec: rec}
	license := &mockLicense{
		verifyLicenseFn: func(_ context.Context, _ domain.LicenseVerificationRequest) (*domain.ProviderCheckResult, error) {
			return nil, resilience.ErrCircuitOpen
		},
	}
	q := oneJobQueue(domain.VerificationJob{CaseID: "c1", UserID: "u1", Type: domain.CheckLicense, LicenseNumber: "LIC-1"})
	c := kyc.NewQueueConsumer(&mockRepo{}, &mockAudit{}, &mockKYCService{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, license, &mockBusiness{}, &mockNotifier{}, q, &mockTierCache{}, met)

	c.RunOnce(context.Background())

	if len(rec.circuits) != 1 || rec.circuits[0] != "license" {
		t.Errorf("expected circuit-open counted for license provider, got %v", rec.circuits)
	}
}

type metricsDelegate struct {
	rec *metricRecorder
}

func (m *metricsDelegate) RecordVerdict(checkType, verdict string) {
	m.rec.RecordVerdict(checkType, verdict)
}

func (m *metricsDelegate) RecordVendorLatency(provider string, d time.Duration) {
	m.rec.RecordVendorLatency(provider, d)
}

func (m *metricsDelegate) SetQueueDepth(n int) {
	m.rec.SetQueueDepth(n)
}

func (m *metricsDelegate) IncCircuitOpen(provider string) {
	m.rec.IncCircuitOpen(provider)
}

func createVerdictService(met *metricsDelegate) domain.KYCService {
	return kyc.NewService(&mockRepo{
		findCheckByProviderEventIDFn: func(_ context.Context, _ string) (*domain.Check, error) {
			return nil, nil
		},
	}, &mockAudit{}, &mockIdentity{}, &mockSanctions{}, &mockOwnership{}, &mockLicense{}, &mockBusiness{}, &mockVault{}, &mockNotifier{}, nil, nil, met)
}
