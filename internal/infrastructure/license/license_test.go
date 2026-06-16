package license_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/license"
)

func TestVerifyLicense_SendsLicenseAndMapsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/license/verify" {
			t.Errorf("path: got %q, want /v1/license/verify", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["license_number"] != "NMLS-42" {
			t.Errorf("license_number: got %q, want NMLS-42", body["license_number"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"event_id":"lic-1","verdict":"approved","risk_score":0,"raw":null}`))
	}))
	defer server.Close()

	verifier := license.NewVerifier(config.ExternalProviderConfig{BaseURL: server.URL, APIKey: "k"})
	result, err := verifier.VerifyLicense(context.Background(), domain.LicenseVerificationRequest{
		UserID:        "user-1",
		LicenseNumber: "NMLS-42",
		Jurisdiction:  "CA",
	})
	if err != nil {
		t.Fatalf("got err %v, want nil", err)
	}
	if result.ProviderEventID != "lic-1" {
		t.Errorf("event id: got %q, want lic-1", result.ProviderEventID)
	}
}
