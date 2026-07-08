package oauth_test

import (
	"testing"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/oauth"
)

func TestNewGoogleProvider_ReturnsProvider(t *testing.T) {
	p := oauth.NewGoogleProvider(config.OAuthConfig{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-secret",
		GoogleRedirectURL:  "http://localhost/callback",
	})
	if p == nil {
		t.Fatal("NewGoogleProvider returned nil")
	}
}
