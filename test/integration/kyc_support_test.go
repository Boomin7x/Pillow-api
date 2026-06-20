//go:build integration

package integration

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	infranotify "github.com/kodiahbertrand/pillow/internal/infrastructure/notify"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/pkg/metrics"
	"gorm.io/gorm"
)

func kycErrorHandler(c *fiber.Ctx, err error) error {
	var appErr *apperrors.AppError
	if e, ok := err.(*apperrors.AppError); ok {
		appErr = e
	} else {
		appErr = apperrors.Internal("an unexpected error occurred")
	}
	return c.Status(apperrors.HTTPStatus(appErr.Code)).JSON(appErr)
}

func newKYCService(db *gorm.DB, sanctions domain.SanctionsScreener) (domain.KYCService, domain.KYCRepository, domain.AuditRepository) {
	repo := kyc.NewRepository(db)
	audit := kyc.NewAuditRepository(db)
	identity := stubProvider{}
	svc := kyc.NewService(repo, audit, identity, sanctions, identity, identity, identity, stubVault{}, infranotify.New(), nil, nil, metrics.NewNoop())
	return svc, repo, audit
}

type stubProvider struct{}

func (stubProvider) VerifyDocument(_ context.Context, _ domain.DocumentVerificationRequest) (*domain.ProviderCheckResult, error) {
	return approvedProviderResult(), nil
}

func (stubProvider) VerifyLiveness(_ context.Context, _ domain.LivenessRequest) (*domain.ProviderCheckResult, error) {
	return approvedProviderResult(), nil
}

func (stubProvider) VerifyOwnership(_ context.Context, _ domain.OwnershipVerificationRequest) (*domain.ProviderCheckResult, error) {
	return approvedProviderResult(), nil
}

func (stubProvider) VerifyLicense(_ context.Context, _ domain.LicenseVerificationRequest) (*domain.ProviderCheckResult, error) {
	return approvedProviderResult(), nil
}

func (stubProvider) VerifyBusiness(_ context.Context, _ domain.BusinessVerificationRequest) (*domain.ProviderCheckResult, error) {
	return approvedProviderResult(), nil
}

func approvedProviderResult() *domain.ProviderCheckResult {
	return &domain.ProviderCheckResult{Verdict: domain.VerdictApproved, RiskScore: 5}
}

type fixedSanctions struct {
	result *domain.ProviderCheckResult
}

func (s fixedSanctions) Screen(_ context.Context, _ domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
	return s.result, nil
}

type stubVault struct{}

func (stubVault) Store(_ context.Context, _ domain.DocumentUpload) (domain.DocumentReference, error) {
	return "stub-ref", nil
}

func (stubVault) SignedURL(_ context.Context, _ domain.DocumentReference, _ time.Duration) (string, error) {
	return "https://example.com/stub", nil
}

func (stubVault) Purge(_ context.Context, _ domain.DocumentReference) error { return nil }

func (stubVault) PurgeExpired(_ context.Context) (int, error) { return 0, nil }

func (stubVault) PurgeUser(_ context.Context, _ string) (int, error) { return 0, nil }
