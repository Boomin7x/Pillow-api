package ownership_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/ownership"
)

func TestVerifyOwnership_SendsClaimAndMapsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ownership/verify" {
			t.Errorf("path: got %q, want /v1/ownership/verify", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["property_address"] != "123 Main St" {
			t.Errorf("property_address: got %q, want 123 Main St", body["property_address"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"event_id":"own-1","verdict":"approved","risk_score":5,"raw":null}`))
	}))
	defer server.Close()

	verifier := ownership.NewVerifier(config.ExternalProviderConfig{BaseURL: server.URL, APIKey: "k"})
	result, err := verifier.VerifyOwnership(context.Background(), domain.OwnershipVerificationRequest{
		UserID:          "user-1",
		PropertyAddress: "123 Main St",
		ClaimantName:    "Alice",
	})
	if err != nil {
		t.Fatalf("got err %v, want nil", err)
	}
	if result.Verdict != domain.VerdictApproved {
		t.Errorf("verdict: got %v, want approved", result.Verdict)
	}
}
