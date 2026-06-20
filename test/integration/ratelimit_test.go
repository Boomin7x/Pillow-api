//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	infraredis "github.com/kodiahbertrand/pillow/internal/infrastructure/redis"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
)

func TestIntegration_RateLimiter_SlidingWindow(t *testing.T) {
	ctx := context.Background()
	rdb, cleanup := testhelpers.NewRedisContainer(t, ctx)
	defer cleanup()

	limiter := infraredis.NewRateLimiter(rdb)

	t.Run("allows up to the limit then blocks", func(t *testing.T) {
		const limit = 3
		key := "ratelimit:ip:1.2.3.4:kyc_license"

		for i := 1; i <= limit; i++ {
			allowed, err := limiter.Allow(ctx, key, limit, time.Minute)
			if err != nil {
				t.Fatalf("request %d: %v", i, err)
			}
			if !allowed {
				t.Fatalf("request %d should be allowed (under limit of %d)", i, limit)
			}
		}

		allowed, err := limiter.Allow(ctx, key, limit, time.Minute)
		if err != nil {
			t.Fatalf("request %d: %v", limit+1, err)
		}
		if allowed {
			t.Errorf("request %d should be blocked (limit %d exceeded)", limit+1, limit)
		}
	})

	t.Run("separate keys have independent budgets", func(t *testing.T) {
		a, err := limiter.Allow(ctx, "ratelimit:ip:9.9.9.9:kyc_ownership", 1, time.Minute)
		if err != nil || !a {
			t.Fatalf("first request on a fresh key should be allowed; allowed=%v err=%v", a, err)
		}
		// A different endpoint slug for the same IP must not be affected.
		b, err := limiter.Allow(ctx, "ratelimit:ip:9.9.9.9:kyc_business", 1, time.Minute)
		if err != nil || !b {
			t.Fatalf("a different endpoint bucket must have its own budget; allowed=%v err=%v", b, err)
		}
	})

	t.Run("window expiry frees the budget", func(t *testing.T) {
		key := "ratelimit:ip:5.5.5.5:kyc_start"
		window := 1 * time.Second

		allowed, err := limiter.Allow(ctx, key, 1, window)
		if err != nil || !allowed {
			t.Fatalf("first request should be allowed; allowed=%v err=%v", allowed, err)
		}
		blocked, err := limiter.Allow(ctx, key, 1, window)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			t.Fatal("second request within the window should be blocked")
		}

		time.Sleep(window + 200*time.Millisecond)

		allowedAgain, err := limiter.Allow(ctx, key, 1, window)
		if err != nil {
			t.Fatal(err)
		}
		if !allowedAgain {
			t.Error("after the window elapses the budget should be free again")
		}
	})
}
