package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	goauth2 "golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

func newTestProvider(t *testing.T, verify idTokenVerifier) *googleProvider {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a","token_type":"Bearer","id_token":"dummy.id.token"}`))
	}))
	t.Cleanup(ts.Close)

	return &googleProvider{
		cfg: &goauth2.Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/callback",
			Endpoint:     goauth2.Endpoint{AuthURL: ts.URL, TokenURL: ts.URL},
		},
		clientID: "test-client-id",
		verify:   verify,
	}
}

func TestExchangeAndVerify_RejectsInvalidIDToken(t *testing.T) {
	g := newTestProvider(t, func(_ context.Context, _, _ string) (*idtoken.Payload, error) {
		return nil, errors.New("invalid signature")
	})

	_, err := g.ExchangeAndVerify(context.Background(), "code", "verifier")
	if err == nil {
		t.Fatal("expected an error when id_token verification fails, got nil")
	}
}

func TestExchangeAndVerify_MapsVerifiedClaims(t *testing.T) {
	g := newTestProvider(t, func(_ context.Context, _, aud string) (*idtoken.Payload, error) {
		if aud != "test-client-id" {
			t.Errorf("audience = %q, want test-client-id", aud)
		}
		return &idtoken.Payload{
			Subject: "google-sub-123",
			Claims:  map[string]any{"email": "user@example.com"},
		}, nil
	})

	claims, err := g.ExchangeAndVerify(context.Background(), "code", "verifier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.ProviderID != "google-sub-123" {
		t.Errorf("ProviderID = %q, want google-sub-123", claims.ProviderID)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("Email = %q, want user@example.com", claims.Email)
	}
}
