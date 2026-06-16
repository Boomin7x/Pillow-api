package kycprovider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/kycprovider"
)

type identityProvider interface {
	VerifyDocument(ctx context.Context, request domain.DocumentVerificationRequest) (*domain.ProviderCheckResult, error)
	VerifyLiveness(ctx context.Context, request domain.LivenessRequest) (*domain.ProviderCheckResult, error)
	Screen(ctx context.Context, request domain.SanctionsRequest) (*domain.ProviderCheckResult, error)
}

func newProvider(t *testing.T, handler http.HandlerFunc) (identityProvider, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	p := kycprovider.NewProvider(config.ExternalProviderConfig{
		BaseURL:    server.URL,
		APIKey:     "test-key",
		MaxRetries: 0,
	})
	return p, server.Close
}

func jsonHandler(t *testing.T, wantPath, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("path: got %q, want %q", r.URL.Path, wantPath)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth header: got %q, want Bearer test-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func TestVerifyDocument_MapsApprovedResult(t *testing.T) {
	p, closeFn := newProvider(t, jsonHandler(t, "/v1/identity/document",
		`{"event_id":"evt-1","verdict":"approved","risk_score":12,"raw":{"score":0.1}}`))
	defer closeFn()

	result, err := p.VerifyDocument(context.Background(), domain.DocumentVerificationRequest{
		UserID:            "user-1",
		DocumentReference: "doc-ref",
	})
	if err != nil {
		t.Fatalf("got err %v, want nil", err)
	}
	if result.ProviderEventID != "evt-1" {
		t.Errorf("event id: got %q, want evt-1", result.ProviderEventID)
	}
	if result.Verdict != domain.VerdictApproved {
		t.Errorf("verdict: got %v, want approved", result.Verdict)
	}
	if result.RiskScore != domain.RiskScore(12) {
		t.Errorf("risk score: got %v, want 12", result.RiskScore)
	}
}

func TestVerifyLiveness_MapsRejectedResult(t *testing.T) {
	p, closeFn := newProvider(t, jsonHandler(t, "/v1/identity/liveness",
		`{"event_id":"evt-2","verdict":"rejected","risk_score":90,"raw":null}`))
	defer closeFn()

	result, err := p.VerifyLiveness(context.Background(), domain.LivenessRequest{UserID: "user-1"})
	if err != nil {
		t.Fatalf("got err %v, want nil", err)
	}
	if result.Verdict != domain.VerdictRejected {
		t.Errorf("verdict: got %v, want rejected", result.Verdict)
	}
}

func TestScreen_UnknownVerdictDefaultsToReview(t *testing.T) {
	p, closeFn := newProvider(t, jsonHandler(t, "/v1/sanctions/screen",
		`{"event_id":"evt-3","verdict":"something-weird","risk_score":50,"raw":null}`))
	defer closeFn()

	result, err := p.Screen(context.Background(), domain.SanctionsRequest{UserID: "user-1", FullName: "Bob"})
	if err != nil {
		t.Fatalf("got err %v, want nil", err)
	}
	if result.Verdict != domain.VerdictReview {
		t.Errorf("verdict: got %v, want review (safe default)", result.Verdict)
	}
}

func TestVerifyDocument_NonOKStatusIsError(t *testing.T) {
	p, closeFn := newProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	defer closeFn()

	if _, err := p.VerifyDocument(context.Background(), domain.DocumentVerificationRequest{UserID: "user-1"}); err == nil {
		t.Fatal("got nil err, want error on non-200 status")
	}
}
