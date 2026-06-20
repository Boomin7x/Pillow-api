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
	retentionTTL  time.Duration
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
		retentionTTL:  cfg.RetentionTTL,
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

	path, err := v.resolveForUser(upload.UserID, reference)
	if err != nil {
		return "", err
	}
	userDir := filepath.Dir(path)
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		return "", fmt.Errorf("storage: create user vault directory: %w", err)
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
	err = os.Remove(path)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("storage: purge document: %w", err)
	}

	entry, userID, found := v.findInUserDirs(string(reference))
	if !found {
		return nil
	}
	userPath := filepath.Join(v.basePath, userID, entry.Name())
	if removeErr := os.Remove(userPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return fmt.Errorf("storage: purge document in user dir: %w", removeErr)
	}
	return nil
}

func (v *documentVault) PurgeUser(ctx context.Context, userID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("storage: purge user cancelled: %w", err)
	}
	userDir := filepath.Join(v.basePath, userID)
	entries, err := os.ReadDir(userDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("storage: purge user: read dir: %w", err)
	}
	count := 0
	for _, entry := range entries {
		path := filepath.Join(userDir, entry.Name())
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return count, fmt.Errorf("storage: purge user: remove %s: %w", entry.Name(), removeErr)
		}
		count++
	}
	if removeErr := os.Remove(userDir); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return count, fmt.Errorf("storage: purge user: remove dir: %w", removeErr)
	}
	return count, nil
}

func (v *documentVault) PurgeExpired(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("storage: purge expired cancelled: %w", err)
	}
	userDirs, err := os.ReadDir(v.basePath)
	if err != nil {
		return 0, fmt.Errorf("storage: purge expired: read base: %w", err)
	}
	cutoff := time.Now().Add(-v.retentionTTL)
	count := 0
	for _, userDir := range userDirs {
		if !userDir.IsDir() {
			continue
		}
		userPath := filepath.Join(v.basePath, userDir.Name())
		entries, err := os.ReadDir(userPath)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				path := filepath.Join(userPath, entry.Name())
				if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
					return count, fmt.Errorf("storage: purge expired: remove %s: %w", entry.Name(), removeErr)
				}
				count++
			}
		}
	}
	return count, nil
}

func (v *documentVault) sign(ref, expires string) string {
	mac := hmac.New(sha256.New, v.signingSecret)
	mac.Write([]byte(ref + "|" + expires))
	return hex.EncodeToString(mac.Sum(nil))
}

func (v *documentVault) findInUserDirs(reference string) (os.DirEntry, string, bool) {
	userDirs, err := os.ReadDir(v.basePath)
	if err != nil {
		return nil, "", false
	}
	for _, userDir := range userDirs {
		if !userDir.IsDir() {
			continue
		}
		userPath := filepath.Join(v.basePath, userDir.Name())
		entries, err := os.ReadDir(userPath)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.Name() == reference {
				return entry, userDir.Name(), true
			}
		}
	}
	return nil, "", false
}

func (v *documentVault) resolve(reference string) (string, error) {
	if _, err := uuid.Parse(reference); err != nil {
		return "", fmt.Errorf("storage: invalid document reference: %w", err)
	}
	return filepath.Join(v.basePath, reference), nil
}

func (v *documentVault) resolveForUser(userID, reference string) (string, error) {
	if _, err := uuid.Parse(reference); err != nil {
		return "", fmt.Errorf("storage: invalid document reference: %w", err)
	}
	return filepath.Join(v.basePath, userID, reference), nil
}
