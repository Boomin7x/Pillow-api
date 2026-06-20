//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	infranotify "github.com/kodiahbertrand/pillow/internal/infrastructure/notify"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	infrastorage "github.com/kodiahbertrand/pillow/internal/infrastructure/storage"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/pkg/metrics"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
)

func TestIntegration_DeleteProfile_TombstonesPurgesButKeepsAudit(t *testing.T) {
	ctx := context.Background()
	db, cleanup := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanup()

	repo := kyc.NewRepository(db)
	audit := kyc.NewAuditRepository(db)

	basePath := t.TempDir()
	vault, err := infrastorage.NewDocumentVault(config.DocumentVaultConfig{
		BasePath:         basePath,
		EncryptionKeyHex: strings.Repeat("ab", 32),
		SigningSecret:    "test-signing-secret",
		PublicBaseURL:    "http://localhost/docs",
		RetentionTTL:     90 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("new vault: %v", err)
	}

	svc := kyc.NewService(repo, audit, stubProvider{}, fixedSanctions{result: approvedProviderResult()},
		stubProvider{}, stubProvider{}, stubProvider{}, vault, infranotify.New(), nil, nil, metrics.NewNoop())

	user := &pgmodels.UserModel{Email: "gdpr-delete@example.com", DisplayName: "Delete Me"}
	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := repo.CreateProfile(ctx, &domain.KYCProfile{
		UserID: user.ID,
		Role:   domain.RoleSeller,
		Tier:   domain.TierVerified,
		Status: domain.StatusVerified,
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	// A pre-existing audit row that MUST survive deletion (regulatory).
	if err := audit.Append(ctx, &domain.AuditEvent{UserID: user.ID, EventType: "verification_started"}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	// A stored document for the user — must be purged.
	if _, err := vault.Store(ctx, domain.DocumentUpload{UserID: user.ID, ContentType: "image/jpeg", Content: []byte("secret")}); err != nil {
		t.Fatalf("store document: %v", err)
	}
	userDir := filepath.Join(basePath, user.ID)
	if _, err := os.Stat(userDir); err != nil {
		t.Fatalf("expected user vault dir to exist before delete: %v", err)
	}

	if err := svc.DeleteProfile(ctx, user.ID); err != nil {
		t.Fatalf("delete profile: %v", err)
	}

	// 1. Profile reads as not-found (tombstoned, excluded from reads).
	if _, err := repo.FindProfileByUserID(ctx, user.ID); !isNotFound(err) {
		t.Errorf("profile after delete: got %v, want NotFound", err)
	}

	// 2. Documents purged from the vault.
	if _, err := os.Stat(userDir); !os.IsNotExist(err) {
		t.Errorf("expected user vault dir purged, stat err = %v", err)
	}

	// 3. The append-only audit trail survives — both the pre-existing row and the deletion event.
	events, err := audit.ListByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	var sawOriginal, sawDeletion bool
	for _, e := range events {
		switch e.EventType {
		case "verification_started":
			sawOriginal = true
		case "profile_deleted":
			sawDeletion = true
		}
	}
	if !sawOriginal {
		t.Error("pre-existing audit row was destroyed by delete — regulatory violation")
	}
	if !sawDeletion {
		t.Error("expected a profile_deleted audit event")
	}
}
