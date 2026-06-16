package license

import (
	"context"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/providerhttp"
	"github.com/kodiahbertrand/pillow/pkg/resilience"
)

type licenseVerifier struct {
	exec *providerhttp.Executor
}

func NewVerifier(cfg config.ExternalProviderConfig) *licenseVerifier {
	client := resilience.NewClient(resilience.Config{
		Timeout:    cfg.Timeout,
		MaxRetries: cfg.MaxRetries,
	})
	return &licenseVerifier{exec: providerhttp.NewExecutor(client, cfg.BaseURL, cfg.APIKey)}
}

type licenseRequest struct {
	UserID        string `json:"user_id"`
	LicenseNumber string `json:"license_number"`
	Jurisdiction  string `json:"jurisdiction"`
}

func (v *licenseVerifier) VerifyLicense(ctx context.Context, request domain.LicenseVerificationRequest) (*domain.ProviderCheckResult, error) {
	return v.exec.Post(ctx, "/v1/license/verify", licenseRequest{
		UserID:        request.UserID,
		LicenseNumber: request.LicenseNumber,
		Jurisdiction:  request.Jurisdiction,
	})
}
