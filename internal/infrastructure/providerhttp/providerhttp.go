package providerhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/pkg/resilience"
)

type Executor struct {
	client  *resilience.Client
	baseURL string
	apiKey  string
}

func NewExecutor(client *resilience.Client, baseURL, apiKey string) *Executor {
	return &Executor{client: client, baseURL: baseURL, apiKey: apiKey}
}

type checkResponse struct {
	EventID   string         `json:"event_id"`
	Verdict   string         `json:"verdict"`
	RiskScore int            `json:"risk_score"`
	Raw       map[string]any `json:"raw"`
}

func (e *Executor) Post(ctx context.Context, path string, body any) (*domain.ProviderCheckResult, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("providerhttp: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("providerhttp: build request: %w", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("providerhttp: %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("providerhttp: %s: unexpected status %d", path, resp.StatusCode)
	}

	var decoded checkResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("providerhttp: decode response: %w", err)
	}

	return &domain.ProviderCheckResult{
		ProviderEventID: decoded.EventID,
		Verdict:         mapVerdict(decoded.Verdict),
		RiskScore:       domain.RiskScore(decoded.RiskScore),
		RawPayload:      decoded.Raw,
	}, nil
}

func mapVerdict(raw string) domain.Verdict {
	switch domain.Verdict(raw) {
	case domain.VerdictApproved:
		return domain.VerdictApproved
	case domain.VerdictRejected:
		return domain.VerdictRejected
	default:
		return domain.VerdictReview
	}
}
