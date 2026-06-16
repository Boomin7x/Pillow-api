package ownership

import (
	"context"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/providerhttp"
	"github.com/kodiahbertrand/pillow/pkg/resilience"
)

type ownershipVerifier struct {
	exec *providerhttp.Executor
}

func NewVerifier(cfg config.ExternalProviderConfig) *ownershipVerifier {
	client := resilience.NewClient(resilience.Config{
		Timeout:    cfg.Timeout,
		MaxRetries: cfg.MaxRetries,
	})
	return &ownershipVerifier{exec: providerhttp.NewExecutor(client, cfg.BaseURL, cfg.APIKey)}
}

type ownershipRequest struct {
	UserID          string `json:"user_id"`
	PropertyAddress string `json:"property_address"`
	ClaimantName    string `json:"claimant_name"`
}

func (v *ownershipVerifier) VerifyOwnership(ctx context.Context, request domain.OwnershipVerificationRequest) (*domain.ProviderCheckResult, error) {
	return v.exec.Post(ctx, "/v1/ownership/verify", ownershipRequest{
		UserID:          request.UserID,
		PropertyAddress: request.PropertyAddress,
		ClaimantName:    request.ClaimantName,
	})
}
