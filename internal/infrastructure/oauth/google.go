package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	goauth2 "golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type googleProvider struct {
	cfg      *goauth2.Config
	clientID string
}

func NewGoogleProvider(cfg config.OAuthConfig) domain.OAuthProvider {
	return &googleProvider{
		cfg: &goauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.GoogleRedirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
		clientID: cfg.GoogleClientID,
	}
}

func (g *googleProvider) BuildAuthURL(state, codeChallenge string) string {
	return g.cfg.AuthCodeURL(
		state,
		goauth2.SetAuthURLParam("code_challenge", codeChallenge),
		goauth2.SetAuthURLParam("code_challenge_method", "S256"),
		goauth2.SetAuthURLParam("access_type", "offline"),
		goauth2.SetAuthURLParam("prompt", "consent"),
	)
}

func (g *googleProvider) ExchangeAndVerify(ctx context.Context, code, codeVerifier string) (*domain.OAuthIdentityClaims, error) {
	token, err := g.cfg.Exchange(ctx, code,
		goauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("oauth: exchange code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, fmt.Errorf("oauth: id_token missing from token response")
	}

	claims, err := parseGoogleIDToken(rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oauth: parse id_token: %w", err)
	}

	if claims.iss != "accounts.google.com" && claims.iss != "https://accounts.google.com" {
		return nil, fmt.Errorf("oauth: invalid issuer: %q", claims.iss)
	}
	if claims.aud != g.clientID {
		return nil, fmt.Errorf("oauth: audience mismatch")
	}
	if claims.sub == "" {
		return nil, fmt.Errorf("oauth: missing sub claim")
	}

	return &domain.OAuthIdentityClaims{
		ProviderID: claims.sub,
		Email:      claims.email,
	}, nil
}

type googleIDTokenClaims struct {
	sub   string
	email string
	iss   string
	aud   string
}

func parseGoogleIDToken(rawToken string) (*googleIDTokenClaims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}

	var raw struct {
		Sub   string          `json:"sub"`
		Email string          `json:"email"`
		Iss   string          `json:"iss"`
		Aud   json.RawMessage `json:"aud"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal JWT claims: %w", err)
	}

	aud := extractAudience(raw.Aud)

	return &googleIDTokenClaims{
		sub:   raw.Sub,
		email: raw.Email,
		iss:   raw.Iss,
		aud:   aud,
	}, nil
}

func extractAudience(raw json.RawMessage) string {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single
	}
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err == nil && len(multiple) > 0 {
		return multiple[0]
	}
	return ""
}
