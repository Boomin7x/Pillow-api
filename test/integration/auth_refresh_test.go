//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/auth"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
)

func TestIntegration_RefreshRotationAndTheftDetection(t *testing.T) {
	ctx := context.Background()
	db, cleanupDB := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanupDB()
	rdb, cleanupRedis := testhelpers.NewRedisContainer(t, ctx)
	defer cleanupRedis()

	repo := auth.NewRepository(db, rdb)
	svc := auth.NewService(repo, newIssuer(t), noopAudit{})

	registered, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "carol@example.com",
		Password:    "supersecret123",
		DisplayName: "Carol",
	})
	if err != nil {
		t.Fatalf("seed register: %v", err)
	}
	originalToken := registered.RefreshToken

	rotated, err := svc.Refresh(ctx, domain.RefreshInput{RawToken: originalToken})
	if err != nil {
		t.Fatalf("first refresh should rotate: %v", err)
	}
	if rotated.RefreshToken == originalToken {
		t.Error("refresh must return a rotated token, not the original")
	}

	_, err = svc.Refresh(ctx, domain.RefreshInput{RawToken: originalToken})
	var appErr *apperrors.AppError
	if !asAppError(err, &appErr) || appErr.Code != apperrors.CodeUnauthorized {
		t.Fatalf("replaying a rotated token should be unauthorized (theft), got %v", err)
	}

	user, err := repo.FindUserByEmail(ctx, "carol@example.com")
	if err != nil {
		t.Fatalf("find user: %v", err)
	}

	_, err = svc.Refresh(ctx, domain.RefreshInput{RawToken: rotated.RefreshToken})
	if !asAppError(err, &appErr) || appErr.Code != apperrors.CodeUnauthorized {
		t.Errorf("after theft detection the whole family must be revoked, got %v for user %s", err, user.ID)
	}
}
