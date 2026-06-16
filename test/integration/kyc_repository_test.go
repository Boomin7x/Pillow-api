//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
)

func TestIntegration_KYCProfileLifecycle(t *testing.T) {
	ctx := context.Background()
	db, cleanup := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanup()

	user := &pgmodels.UserModel{Email: "kyc-profile@example.com", DisplayName: "KYC User"}
	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	repo := kyc.NewRepository(db)

	profile := &domain.KYCProfile{
		UserID: user.ID,
		Role:   domain.RoleSeller,
		Tier:   domain.TierIdentified,
		Status: domain.StatusPending,
	}
	if err := repo.CreateProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	if err := repo.CreateProfile(ctx, &domain.KYCProfile{UserID: user.ID, Role: domain.RoleSeller, Status: domain.StatusPending}); err == nil {
		t.Error("duplicate profile: got nil err, want conflict")
	}

	profile.Tier = domain.TierVerified
	profile.Status = domain.StatusVerified
	profile.GrantQualification(domain.QualificationOwnership)
	if err := repo.UpdateProfile(ctx, profile); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	loaded, err := repo.FindProfileByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("find profile: %v", err)
	}
	if loaded.Tier != domain.TierVerified {
		t.Errorf("tier: got %v, want T2_VERIFIED", loaded.Tier)
	}
	if !loaded.HasQualification(domain.QualificationOwnership) {
		t.Error("expected ownership qualification to persist")
	}

	if _, err := repo.FindProfileByUserID(ctx, "00000000-0000-0000-0000-000000000000"); !isNotFound(err) {
		t.Errorf("missing profile: got %v, want NotFound", err)
	}
}

func TestIntegration_VerificationCaseAndChecks(t *testing.T) {
	ctx := context.Background()
	db, cleanup := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanup()

	user := &pgmodels.UserModel{Email: "kyc-case@example.com", DisplayName: "Case User"}
	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	repo := kyc.NewRepository(db)

	verificationCase := &domain.VerificationCase{
		UserID:    user.ID,
		Type:      domain.CheckDocument,
		Status:    domain.StatusPending,
		RiskScore: domain.RiskScore(20),
	}
	if err := repo.CreateCase(ctx, verificationCase); err != nil {
		t.Fatalf("create case: %v", err)
	}
	if verificationCase.ID == "" {
		t.Fatal("expected generated case id")
	}

	verificationCase.Status = domain.StatusInReview
	if err := repo.UpdateCase(ctx, verificationCase); err != nil {
		t.Fatalf("update case: %v", err)
	}

	pending, err := repo.ListPendingCases(ctx, 10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending cases: got %d, want 1", len(pending))
	}

	check := &domain.Check{
		CaseID:          verificationCase.ID,
		Type:            domain.CheckDocument,
		Status:          domain.StatusInReview,
		ProviderEventID: "evt-123",
		RawPayload:      map[string]any{"score": 0.2},
	}
	if err := repo.CreateCheck(ctx, check); err != nil {
		t.Fatalf("create check: %v", err)
	}

	if err := repo.CreateCheck(ctx, &domain.Check{CaseID: verificationCase.ID, Type: domain.CheckDocument, Status: domain.StatusInReview, ProviderEventID: "evt-123"}); err == nil {
		t.Error("duplicate provider event id: got nil err, want conflict")
	}

	found, err := repo.FindCheckByProviderEventID(ctx, "evt-123")
	if err != nil {
		t.Fatalf("find check: %v", err)
	}
	if found.ID != check.ID {
		t.Errorf("check id: got %q, want %q", found.ID, check.ID)
	}

	check.Status = domain.StatusVerified
	check.Verdict = domain.VerdictApproved
	if err := repo.UpdateCheck(ctx, check); err != nil {
		t.Fatalf("update check: %v", err)
	}
}

func TestIntegration_OwnershipClaimAndAudit(t *testing.T) {
	ctx := context.Background()
	db, cleanup := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanup()

	user := &pgmodels.UserModel{Email: "kyc-owner@example.com", DisplayName: "Owner User"}
	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	repo := kyc.NewRepository(db)
	auditRepo := kyc.NewAuditRepository(db)

	claim := &domain.OwnershipClaim{
		UserID:          user.ID,
		PropertyAddress: "123 Main St",
		ClaimantName:    "Owner User",
		Status:          domain.StatusPending,
		Method:          domain.OwnershipMethodPublicRecord,
	}
	if err := repo.CreateOwnershipClaim(ctx, claim); err != nil {
		t.Fatalf("create claim: %v", err)
	}

	claim.Status = domain.StatusVerified
	if err := repo.UpdateOwnershipClaim(ctx, claim); err != nil {
		t.Fatalf("update claim: %v", err)
	}

	loaded, err := repo.FindOwnershipClaimByID(ctx, claim.ID)
	if err != nil {
		t.Fatalf("find claim: %v", err)
	}
	if loaded.Status != domain.StatusVerified {
		t.Errorf("claim status: got %v, want verified", loaded.Status)
	}

	event := &domain.AuditEvent{
		UserID:    user.ID,
		EventType: "tier_upgraded",
		Tier:      domain.TierVerified,
		Metadata:  map[string]any{"from": "T1", "to": "T2"},
	}
	if err := auditRepo.Append(ctx, event); err != nil {
		t.Fatalf("append audit: %v", err)
	}

	events, err := auditRepo.ListByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events: got %d, want 1", len(events))
	}
	if events[0].EventType != "tier_upgraded" {
		t.Errorf("event type: got %q, want tier_upgraded", events[0].EventType)
	}
}

func isNotFound(err error) bool {
	var appErr *apperrors.AppError
	return asAppError(err, &appErr) && appErr.Code == apperrors.CodeNotFound
}
