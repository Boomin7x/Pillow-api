//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/kyccache"
	infranotify "github.com/kodiahbertrand/pillow/internal/infrastructure/notify"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	infraredis "github.com/kodiahbertrand/pillow/internal/infrastructure/redis"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/pkg/metrics"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
)

func TestIntegration_VerificationQueueEndToEnd(t *testing.T) {
	ctx := context.Background()
	db, cleanupDB := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanupDB()
	rdb, cleanupRedis := testhelpers.NewRedisContainer(t, ctx)
	defer cleanupRedis()

	svc, repo, audit := newKYCService(db, fixedSanctions{result: approvedProviderResult()})

	user := &pgmodels.UserModel{Email: "queue-license@example.com", DisplayName: "Queue User"}
	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := repo.CreateProfile(ctx, &domain.KYCProfile{
		UserID: user.ID,
		Role:   domain.RoleAgent,
		Tier:   domain.TierVerified,
		Status: domain.StatusVerified,
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	verificationCase := &domain.VerificationCase{UserID: user.ID, Type: domain.CheckLicense, Status: domain.StatusPending}
	if err := repo.CreateCase(ctx, verificationCase); err != nil {
		t.Fatalf("seed case: %v", err)
	}

	queue := infraredis.NewVerificationQueue(rdb)
	if err := queue.Enqueue(ctx, domain.VerificationJob{
		CaseID:        verificationCase.ID,
		UserID:        user.ID,
		Type:          domain.CheckLicense,
		LicenseNumber: "LIC-1",
		Jurisdiction:  "CA",
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	consumer := kyc.NewQueueConsumer(
		repo, audit, svc,
		stubProvider{}, fixedSanctions{result: approvedProviderResult()},
		stubProvider{}, stubProvider{}, stubProvider{},
		infranotify.New(), queue, kyccache.NewTierCache(rdb, time.Minute),
		metrics.NewNoop(),
	)

	consumer.RunOnce(ctx)

	got, err := repo.FindCaseByID(ctx, verificationCase.ID)
	if err != nil {
		t.Fatalf("find case: %v", err)
	}
	if got.Status != domain.StatusVerified {
		t.Errorf("case status = %q, want verified (worker should have processed the queued job)", got.Status)
	}

	var checks int64
	db.WithContext(ctx).Model(&pgmodels.KYCCheckModel{}).Where("case_id = ?", verificationCase.ID).Count(&checks)
	if checks != 1 {
		t.Errorf("checks for case = %d, want 1", checks)
	}
}
