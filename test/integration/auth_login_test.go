//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/auth"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
)

func TestIntegration_LoginAndLogoutBlocklist(t *testing.T) {
	ctx := context.Background()
	db, cleanupDB := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanupDB()
	rdb, cleanupRedis := testhelpers.NewRedisContainer(t, ctx)
	defer cleanupRedis()

	repo := auth.NewRepository(db, rdb)
	svc := auth.NewService(repo, newIssuer(t), noopAudit{})

	if _, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "bob@example.com",
		Password:    "correcthorsebattery",
		DisplayName: "Bob",
	}); err != nil {
		t.Fatalf("seed register: %v", err)
	}

	result, err := svc.Login(ctx, domain.LoginInput{
		Email:    "bob@example.com",
		Password: "correcthorsebattery",
	})
	if err != nil {
		t.Fatalf("login with correct password: %v", err)
	}
	if result.AccessToken == "" {
		t.Fatal("expected access token on successful login")
	}

	_, err = svc.Login(ctx, domain.LoginInput{
		Email:    "bob@example.com",
		Password: "wrongpassword",
	})
	var appErr *apperrors.AppError
	if !asAppError(err, &appErr) || appErr.Code != apperrors.CodeUnauthorized {
		t.Errorf("wrong password: expected unauthorized, got %v", err)
	}

	const jti = "integration-jti-1"
	if err := svc.Logout(ctx, domain.LogoutInput{
		AccessTokenJTI: jti,
		AccessTokenTTL: 5 * time.Minute,
		UserID:         "bob",
	}); err != nil {
		t.Fatalf("logout: %v", err)
	}

	blocked, err := repo.IsTokenBlocklisted(ctx, jti)
	if err != nil {
		t.Fatalf("check blocklist: %v", err)
	}
	if !blocked {
		t.Error("expected access token jti to be blocklisted in real Redis after logout")
	}
}
