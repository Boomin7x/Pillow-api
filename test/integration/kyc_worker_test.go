//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/domain"
	infranotify "github.com/kodiahbertrand/pillow/internal/infrastructure/notify"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
	"gorm.io/gorm"
)

func TestIntegration_CaseReconcilerFlagsStuckCases(t *testing.T) {
	ctx := context.Background()
	db, cleanup := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanup()

	repo := kyc.NewRepository(db)
	audit := kyc.NewAuditRepository(db)

	userID := seedUserWithProfile(t, ctx, db, repo, "worker-stuck@example.com")
	caseID := seedCase(t, ctx, repo, userID, domain.CheckDocument)

	vc, _ := repo.FindCaseByID(ctx, caseID)
	vc.Status = domain.StatusInReview
	if err := repo.UpdateCase(ctx, vc); err != nil {
		t.Fatalf("move case to in_review: %v", err)
	}
	ageCase(t, ctx, db, caseID, 48*time.Hour)

	reconciler := kyc.NewCaseReconciler(repo, audit, infranotify.New(), 24*time.Hour)
	reconciler.RunOnce(ctx)

	if !hasAuditEvent(t, ctx, audit, userID, "kyc_case_stuck_in_review") {
		t.Error("expected a kyc_case_stuck_in_review audit event for the stuck case")
	}
}

func TestIntegration_CaseReconcilerIgnoresFreshCases(t *testing.T) {
	ctx := context.Background()
	db, cleanup := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanup()

	repo := kyc.NewRepository(db)
	audit := kyc.NewAuditRepository(db)

	userID := seedUserWithProfile(t, ctx, db, repo, "worker-fresh@example.com")
	caseID := seedCase(t, ctx, repo, userID, domain.CheckDocument)
	vc, _ := repo.FindCaseByID(ctx, caseID)
	vc.Status = domain.StatusInReview
	if err := repo.UpdateCase(ctx, vc); err != nil {
		t.Fatalf("move case to in_review: %v", err)
	}

	reconciler := kyc.NewCaseReconciler(repo, audit, infranotify.New(), 24*time.Hour)
	reconciler.RunOnce(ctx)

	if hasAuditEvent(t, ctx, audit, userID, "kyc_case_stuck_in_review") {
		t.Error("a case updated just now must not be flagged as stuck")
	}
}

func TestIntegration_LicenseExpiryCheckerFlagsExpiringChecks(t *testing.T) {
	ctx := context.Background()
	db, cleanup := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanup()

	repo := kyc.NewRepository(db)
	audit := kyc.NewAuditRepository(db)

	userID := seedUserWithProfile(t, ctx, db, repo, "worker-license@example.com")
	caseID := seedCase(t, ctx, repo, userID, domain.CheckLicense)

	expiresAt := time.Now().Add(24 * time.Hour)
	check := &domain.Check{
		CaseID:    caseID,
		Type:      domain.CheckLicense,
		Status:    domain.StatusVerified,
		Verdict:   domain.VerdictApproved,
		ExpiresAt: &expiresAt,
	}
	if err := repo.CreateCheck(ctx, check); err != nil {
		t.Fatalf("seed check: %v", err)
	}

	checker := kyc.NewLicenseExpiryChecker(repo, audit, infranotify.New(), 7*24*time.Hour)
	checker.RunOnce(ctx)

	events, err := audit.ListByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	var flagged *domain.AuditEvent
	for i := range events {
		if events[i].EventType == "kyc_license_expiring" {
			flagged = &events[i]
		}
	}
	if flagged == nil {
		t.Fatal("expected a kyc_license_expiring audit event")
	}
	if flagged.UserID != userID {
		t.Errorf("audit user_id = %q, want %q (resolved from the owning case)", flagged.UserID, userID)
	}
}

func TestIntegration_SanctionsRescreenerEnforcesHit(t *testing.T) {
	ctx := context.Background()
	db, cleanup := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanup()

	rejected := &domain.ProviderCheckResult{ProviderEventID: "rescreen-evt-1", Verdict: domain.VerdictRejected, RiskScore: 90}
	svc, repo, audit := newKYCService(db, fixedSanctions{result: rejected})

	user := &pgmodels.UserModel{Email: "worker-rescreen@example.com", DisplayName: "Rescreen User"}
	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := repo.CreateProfile(ctx, &domain.KYCProfile{
		UserID: user.ID,
		Role:   domain.RoleSeller,
		Tier:   domain.TierVerified,
		Status: domain.StatusVerified,
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	rescreener := kyc.NewSanctionsRescreener(repo, audit, fixedSanctions{result: rejected}, svc)
	rescreener.RunOnce(ctx)

	var sanctionsCase pgmodels.VerificationCaseModel
	if err := db.WithContext(ctx).
		Where("user_id = ? AND type = ?", user.ID, string(domain.CheckSanctions)).
		First(&sanctionsCase).Error; err != nil {
		t.Fatalf("expected a sanctions case to be opened: %v", err)
	}
	if domain.VerificationStatus(sanctionsCase.Status) != domain.StatusRejected {
		t.Errorf("sanctions case status = %q, want rejected", sanctionsCase.Status)
	}
	if n := countChecks(ctx, db, "rescreen-evt-1"); n != 1 {
		t.Errorf("sanctions check rows = %d, want 1", n)
	}

	rescreener.RunOnce(ctx)
	if n := countChecks(ctx, db, "rescreen-evt-1"); n != 1 {
		t.Errorf("after second rescreen, check rows = %d, want exactly 1 (idempotent enforcement)", n)
	}
}

func ageCase(t *testing.T, ctx context.Context, db *gorm.DB, caseID string, age time.Duration) {
	t.Helper()
	if err := db.WithContext(ctx).
		Model(&pgmodels.VerificationCaseModel{}).
		Where("id = ?", caseID).
		UpdateColumn("updated_at", time.Now().Add(-age)).Error; err != nil {
		t.Fatalf("age case: %v", err)
	}
}

func hasAuditEvent(t *testing.T, ctx context.Context, audit domain.AuditRepository, userID, eventType string) bool {
	t.Helper()
	events, err := audit.ListByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	for _, e := range events {
		if e.EventType == eventType {
			return true
		}
	}
	return false
}
