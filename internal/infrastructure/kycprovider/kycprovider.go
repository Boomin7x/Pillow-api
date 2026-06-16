package kycprovider

import (
	"context"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/providerhttp"
	"github.com/kodiahbertrand/pillow/pkg/resilience"
)

type kycProvider struct {
	exec *providerhttp.Executor
}

func NewProvider(cfg config.ExternalProviderConfig) *kycProvider {
	client := resilience.NewClient(resilience.Config{
		Timeout:    cfg.Timeout,
		MaxRetries: cfg.MaxRetries,
	})
	return &kycProvider{exec: providerhttp.NewExecutor(client, cfg.BaseURL, cfg.APIKey)}
}

type documentRequest struct {
	UserID            string `json:"user_id"`
	DocumentReference string `json:"document_reference"`
}

type livenessRequest struct {
	UserID            string `json:"user_id"`
	DocumentReference string `json:"document_reference"`
}

type sanctionsRequest struct {
	UserID   string `json:"user_id"`
	FullName string `json:"full_name"`
}

func (p *kycProvider) VerifyDocument(ctx context.Context, request domain.DocumentVerificationRequest) (*domain.ProviderCheckResult, error) {
	return p.exec.Post(ctx, "/v1/identity/document", documentRequest{
		UserID:            request.UserID,
		DocumentReference: string(request.DocumentReference),
	})
}

func (p *kycProvider) VerifyLiveness(ctx context.Context, request domain.LivenessRequest) (*domain.ProviderCheckResult, error) {
	return p.exec.Post(ctx, "/v1/identity/liveness", livenessRequest{
		UserID:            request.UserID,
		DocumentReference: string(request.DocumentReference),
	})
}

func (p *kycProvider) Screen(ctx context.Context, request domain.SanctionsRequest) (*domain.ProviderCheckResult, error) {
	return p.exec.Post(ctx, "/v1/sanctions/screen", sanctionsRequest{
		UserID:   request.UserID,
		FullName: request.FullName,
	})
}
