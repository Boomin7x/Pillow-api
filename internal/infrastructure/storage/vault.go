package storage

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
)

type documentVault struct {
	basePath      string
	aead          cipher.AEAD
	signingSecret []byte
	publicBaseURL string
}

func NewDocumentVault(cfg config.DocumentVaultConfig) (*documentVault, error) {
	key, err := hex.DecodeString(cfg.EncryptionKeyHex)
	if err != nil {
		return nil, fmt.Errorf("storage: decode encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("storage: encryption key must be 32 bytes (64 hex chars), got %d", len(key))
	}
	if cfg.SigningSecret == "" {
		return nil, errors.New("storage: signing secret is required")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("storage: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("storage: new gcm: %w", err)
	}

	if err := os.MkdirAll(cfg.BasePath, 0o700); err != nil {
		return nil, fmt.Errorf("storage: create vault directory: %w", err)
	}

	return &documentVault{
		basePath:      cfg.BasePath,
		aead:          aead,
		signingSecret: []byte(cfg.SigningSecret),
		publicBaseURL: cfg.PublicBaseURL,
	}, nil
}

func (v *documentVault) Store(ctx context.Context, upload domain.DocumentUpload) (domain.DocumentReference, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("storage: store cancelled: %w", err)
	}

	reference := uuid.NewString()

	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("storage: generate nonce: %w", err)
	}
	sealed := v.aead.Seal(nonce, nonce, upload.Content, []byte(reference))

	path, err := v.resolve(reference)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, sealed, 0o600); err != nil {
		return "", fmt.Errorf("storage: write document: %w", err)
	}
	return domain.DocumentReference(reference), nil
}

func (v *documentVault) SignedURL(ctx context.Context, reference domain.DocumentReference, ttl time.Duration) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("storage: signed url cancelled: %w", err)
	}

	ref := string(reference)
	expires := strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)
	signature := v.sign(ref, expires)

	query := url.Values{}
	query.Set("ref", ref)
	query.Set("exp", expires)
	query.Set("sig", signature)
	return v.publicBaseURL + "?" + query.Encode(), nil
}

func (v *documentVault) Purge(ctx context.Context, reference domain.DocumentReference) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("storage: purge cancelled: %w", err)
	}

	path, err := v.resolve(string(reference))
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("storage: purge document: %w", err)
	}
	return nil
}

func (v *documentVault) sign(ref, expires string) string {
	mac := hmac.New(sha256.New, v.signingSecret)
	mac.Write([]byte(ref + "|" + expires))
	return hex.EncodeToString(mac.Sum(nil))
}

func (v *documentVault) resolve(reference string) (string, error) {
	if _, err := uuid.Parse(reference); err != nil {
		return "", fmt.Errorf("storage: invalid document reference: %w", err)
	}
	return filepath.Join(v.basePath, reference), nil
}
