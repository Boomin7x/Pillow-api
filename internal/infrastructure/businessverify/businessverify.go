package businessverify

import (
	"context"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/providerhttp"
	"github.com/kodiahbertrand/pillow/pkg/resilience"
)

type businessVerifier struct {
	exec *providerhttp.Executor
}

func NewVerifier(cfg config.ExternalProviderConfig) *businessVerifier {
	client := resilience.NewClient(resilience.Config{
		Timeout:    cfg.Timeout,
		MaxRetries: cfg.MaxRetries,
	})
	return &businessVerifier{exec: providerhttp.NewExecutor(client, cfg.BaseURL, cfg.APIKey)}
}

type businessRequest struct {
	UserID             string `json:"user_id"`
	BusinessName       string `json:"business_name"`
	RegistrationNumber string `json:"registration_number"`
}

func (v *businessVerifier) VerifyBusiness(ctx context.Context, request domain.BusinessVerificationRequest) (*domain.ProviderCheckResult, error) {
	return v.exec.Post(ctx, "/v1/business/verify", businessRequest{
		UserID:             request.UserID,
		BusinessName:       request.BusinessName,
		RegistrationNumber: request.RegistrationNumber,
	})
}
