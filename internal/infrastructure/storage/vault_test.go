package storage_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/storage"
)

const testKeyHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func newVault(t *testing.T) (*vaultUnderTest, string) {
	t.Helper()
	basePath := t.TempDir()
	vault, err := storage.NewDocumentVault(config.DocumentVaultConfig{
		BasePath:         basePath,
		EncryptionKeyHex: testKeyHex,
		SigningSecret:    "test-signing-secret",
		PublicBaseURL:    "https://vault.test/kyc/documents",
	})
	if err != nil {
		t.Fatalf("new vault: %v", err)
	}
	return &vaultUnderTest{vault}, basePath
}

type vaultUnderTest struct {
	v interface {
		Store(ctx context.Context, upload domain.DocumentUpload) (domain.DocumentReference, error)
		SignedURL(ctx context.Context, reference domain.DocumentReference, ttl time.Duration) (string, error)
		Purge(ctx context.Context, reference domain.DocumentReference) error
	}
}

func TestVault_StoreEncryptsAtRest(t *testing.T) {
	vault, basePath := newVault(t)
	plaintext := []byte("super secret passport scan")

	ref, err := vault.v.Store(context.Background(), domain.DocumentUpload{
		UserID:      "user-1",
		ContentType: "image/png",
		Content:     plaintext,
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	stored, err := os.ReadFile(filepath.Join(basePath, string(ref)))
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if bytes.Contains(stored, plaintext) {
		t.Error("stored bytes contain plaintext; document is not encrypted at rest")
	}
}

func TestVault_SignedURLContainsSignatureAndExpiry(t *testing.T) {
	vault, _ := newVault(t)
	ref, err := vault.v.Store(context.Background(), domain.DocumentUpload{Content: []byte("x")})
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	signed, err := vault.v.SignedURL(context.Background(), ref, time.Minute)
	if err != nil {
		t.Fatalf("signed url: %v", err)
	}
	for _, want := range []string{"ref=", "exp=", "sig="} {
		if !strings.Contains(signed, want) {
			t.Errorf("signed url %q missing %q", signed, want)
		}
	}
}

func TestVault_PurgeRemovesDocumentAndIsIdempotent(t *testing.T) {
	vault, basePath := newVault(t)
	ref, err := vault.v.Store(context.Background(), domain.DocumentUpload{Content: []byte("x")})
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	if err := vault.v.Purge(context.Background(), ref); err != nil {
		t.Fatalf("first purge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(basePath, string(ref))); !os.IsNotExist(err) {
		t.Errorf("document still present after purge: %v", err)
	}
	if err := vault.v.Purge(context.Background(), ref); err != nil {
		t.Errorf("second purge: got %v, want nil (idempotent)", err)
	}
}

func TestVault_PurgeRejectsInvalidReference(t *testing.T) {
	vault, _ := newVault(t)
	if err := vault.v.Purge(context.Background(), domain.DocumentReference("../etc/passwd")); err == nil {
		t.Error("got nil err, want rejection of non-uuid reference")
	}
}

func TestNewDocumentVault_RejectsShortKey(t *testing.T) {
	_, err := storage.NewDocumentVault(config.DocumentVaultConfig{
		BasePath:         t.TempDir(),
		EncryptionKeyHex: "00010203",
		SigningSecret:    "s",
	})
	if err == nil {
		t.Error("got nil err, want rejection of short encryption key")
	}
}

func TestNewDocumentVault_RequiresSigningSecret(t *testing.T) {
	_, err := storage.NewDocumentVault(config.DocumentVaultConfig{
		BasePath:         t.TempDir(),
		EncryptionKeyHex: testKeyHex,
		SigningSecret:    "",
	})
	if err == nil {
		t.Error("got nil err, want rejection of empty signing secret")
	}
}
