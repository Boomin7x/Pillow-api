//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/kodiahbertrand/pillow/internal/auth"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
	"golang.org/x/crypto/bcrypt"
)

func TestIntegration_Register(t *testing.T) {
	ctx := context.Background()
	db, cleanupDB := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanupDB()
	rdb, cleanupRedis := testhelpers.NewRedisContainer(t, ctx)
	defer cleanupRedis()

	repo := auth.NewRepository(db, rdb)
	svc := auth.NewService(repo, newIssuer(t), noopAudit{})

	result, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "alice@example.com",
		Password:    "securepass123",
		DisplayName: "Alice",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatal("expected a token pair from register")
	}

	user, err := repo.FindUserByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("user not persisted: %v", err)
	}
	if user.DisplayName != "Alice" {
		t.Errorf("display name = %q, want %q", user.DisplayName, "Alice")
	}

	cred, err := repo.FindCredentialByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("credential not persisted: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte("securepass123")); err != nil {
		t.Errorf("stored password hash does not verify: %v", err)
	}

	_, err = svc.Register(ctx, domain.RegisterInput{
		Email:       "alice@example.com",
		Password:    "anotherpass",
		DisplayName: "Alice Again",
	})
	if err == nil {
		t.Error("expected duplicate email registration to fail")
	}
}
