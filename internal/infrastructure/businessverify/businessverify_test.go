package businessverify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/businessverify"
)

func TestVerifyBusiness_SendsRegistrationAndMapsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/business/verify" {
			t.Errorf("path: got %q, want /v1/business/verify", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["registration_number"] != "REG-7" {
			t.Errorf("registration_number: got %q, want REG-7", body["registration_number"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"event_id":"kyb-1","verdict":"review","risk_score":40,"raw":null}`))
	}))
	defer server.Close()

	verifier := businessverify.NewVerifier(config.ExternalProviderConfig{BaseURL: server.URL, APIKey: "k"})
	result, err := verifier.VerifyBusiness(context.Background(), domain.BusinessVerificationRequest{
		UserID:             "user-1",
		BusinessName:       "Acme Homes",
		RegistrationNumber: "REG-7",
	})
	if err != nil {
		t.Fatalf("got err %v, want nil", err)
	}
	if result.Verdict != domain.VerdictReview {
		t.Errorf("verdict: got %v, want review", result.Verdict)
	}
}
