package oauth

import (
	"context"
	"fmt"

	"google.golang.org/api/idtoken"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	goauth2 "golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type idTokenVerifier func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error)

type googleProvider struct {
	cfg      *goauth2.Config
	clientID string
	verify   idTokenVerifier
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
		verify: func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
			return idtoken.Validate(ctx, idToken, audience)
		},
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

	payload, err := g.verify(ctx, rawIDToken, g.clientID)
	if err != nil {
		return nil, fmt.Errorf("oauth: validate id_token: %w", err)
	}

	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)

	return &domain.OAuthIdentityClaims{
		ProviderID: payload.Subject,
		Email:      email,
		Name:       name,
	}, nil
}
