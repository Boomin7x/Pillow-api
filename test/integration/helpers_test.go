//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/tokenutil"
)

func asAppError(err error, target **apperrors.AppError) bool {
	return errors.As(err, target)
}

func newIssuer(t *testing.T) domain.TokenIssuer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	dir := t.TempDir()
	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")

	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	if err := os.WriteFile(pubPath, pubPEM, 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	issuer, err := tokenutil.NewJWTIssuer(config.JWTConfig{
		PrivateKeyPath: privPath,
		PublicKeyPath:  pubPath,
		AccessTTL:      15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new jwt issuer: %v", err)
	}
	return issuer
}

type noopAudit struct{}

func (noopAudit) Log(_ context.Context, _ *domain.AuthEvent) {}
