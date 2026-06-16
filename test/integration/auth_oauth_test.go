//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/auth"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
	"golang.org/x/oauth2"
)

func TestIntegration_PKCEStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	rdb, cleanupRedis := testhelpers.NewRedisContainer(t, ctx)
	defer cleanupRedis()

	db, cleanupDB := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanupDB()

	repo := auth.NewRepository(db, rdb)

	const state = "state-abc"
	const verifier = "verifier-xyz"

	if err := repo.SaveVerifier(ctx, state, verifier, 10*time.Minute); err != nil {
		t.Fatalf("save verifier: %v", err)
	}

	got, err := repo.GetVerifier(ctx, state)
	if err != nil {
		t.Fatalf("get verifier: %v", err)
	}
	if got != verifier {
		t.Errorf("verifier = %q, want %q", got, verifier)
	}

	if err := repo.DeleteVerifier(ctx, state); err != nil {
		t.Fatalf("delete verifier: %v", err)
	}

	_, err = repo.GetVerifier(ctx, state)
	if err == nil {
		t.Error("expected verifier to be gone after delete (one-time use)")
	}
}

func TestIntegration_OAuthLoginAccountLinking(t *testing.T) {
	ctx := context.Background()
	db, cleanupDB := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanupDB()
	rdb, cleanupRedis := testhelpers.NewRedisContainer(t, ctx)
	defer cleanupRedis()

	repo := auth.NewRepository(db, rdb)
	svc := auth.NewService(repo, newIssuer(t), noopAudit{})

	first, err := svc.OAuthLogin(ctx, domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-1",
		Email:      "dave@example.com",
	})
	if err != nil {
		t.Fatalf("first oauth login (new user): %v", err)
	}

	returning, err := svc.OAuthLogin(ctx, domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-1",
		Email:      "dave@example.com",
	})
	if err != nil {
		t.Fatalf("returning oauth login: %v", err)
	}
	if returning.User.ID != first.User.ID {
		t.Errorf("returning user id = %q, want %q", returning.User.ID, first.User.ID)
	}

	_, err = svc.OAuthLogin(ctx, domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-DIFFERENT",
		Email:      "dave@example.com",
	})
	var appErr *apperrors.AppError
	if !asAppError(err, &appErr) || appErr.Code != apperrors.CodeConflict {
		t.Errorf("email linked to a different provider_id should conflict, got %v", err)
	}
}

func TestIntegration_GoogleTokenExchangePKCE(t *testing.T) {
	var receivedVerifier string
	idToken := fakeIDToken(t, "g-sub-1", "erin@example.com", "test-client-id")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		receivedVerifier = r.Form.Get("code_verifier")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "google-access",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idToken,
		})
	}))
	defer server.Close()

	cfg := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		Endpoint:     oauth2.Endpoint{TokenURL: server.URL},
	}

	token, err := cfg.Exchange(context.Background(), "auth-code",
		oauth2.SetAuthURLParam("code_verifier", "the-pkce-verifier"),
	)
	if err != nil {
		t.Fatalf("token exchange: %v", err)
	}

	if receivedVerifier != "the-pkce-verifier" {
		t.Errorf("token endpoint received code_verifier = %q, want %q", receivedVerifier, "the-pkce-verifier")
	}
	if token.Extra("id_token") != idToken {
		t.Error("expected id_token to be returned from the stubbed Google token endpoint")
	}
}

func fakeIDToken(t *testing.T, sub, email, aud string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{
		"sub":   sub,
		"email": email,
		"iss":   "https://accounts.google.com",
		"aud":   aud,
	})
	if err != nil {
		t.Fatalf("marshal id token claims: %v", err)
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return header + "." + body + "." + signature
}
