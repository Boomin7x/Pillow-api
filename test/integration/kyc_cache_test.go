//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/kyccache"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
)

func TestIntegration_TierCache(t *testing.T) {
	ctx := context.Background()
	rdb, cleanup := testhelpers.NewRedisContainer(t, ctx)
	defer cleanup()

	cache := kyccache.NewTierCache(rdb, time.Minute)

	t.Run("miss returns nil without error", func(t *testing.T) {
		got, err := cache.Get(ctx, "missing-user")
		if err != nil {
			t.Fatalf("a cache miss must not be an error; got: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil profile on miss, got %+v", got)
		}
	})

	t.Run("set then get returns the profile", func(t *testing.T) {
		profile := &domain.KYCProfile{UserID: "u1", Role: domain.RoleSeller, Tier: domain.TierVerified, Status: domain.StatusVerified}
		if err := cache.Set(ctx, "u1", profile, time.Minute); err != nil {
			t.Fatalf("set: %v", err)
		}
		got, err := cache.Get(ctx, "u1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got == nil || got.Tier != domain.TierVerified || got.UserID != "u1" {
			t.Errorf("got %+v, want tier T2_VERIFIED for u1", got)
		}
	})

	t.Run("del invalidates", func(t *testing.T) {
		profile := &domain.KYCProfile{UserID: "u2", Tier: domain.TierRegulated}
		if err := cache.Set(ctx, "u2", profile, time.Minute); err != nil {
			t.Fatalf("set: %v", err)
		}
		if err := cache.Del(ctx, "u2"); err != nil {
			t.Fatalf("del: %v", err)
		}
		got, err := cache.Get(ctx, "u2")
		if err != nil {
			t.Fatalf("get after del: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil after del, got %+v", got)
		}
	})
}
