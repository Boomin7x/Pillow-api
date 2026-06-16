package middleware_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/middleware"
)

type mockIPLimiter struct {
	allowFn func(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

func (m *mockIPLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return m.allowFn(ctx, key, limit, window)
}

type mockEmailLimiter struct {
	allowEmailFn func(ctx context.Context, emailHash string, limit int, baseWindow time.Duration) (bool, time.Duration, error)
}

func (m *mockEmailLimiter) AllowEmail(ctx context.Context, emailHash string, limit int, baseWindow time.Duration) (bool, time.Duration, error) {
	return m.allowEmailFn(ctx, emailHash, limit, baseWindow)
}

func newTestApp(handler fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusTooManyRequests).SendString(err.Error())
		},
	})
	app.Get("/test", handler, func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	app.Post("/test", handler, func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func TestLimitByIP_AllowsUnderLimit(t *testing.T) {
	store := &mockIPLimiter{
		allowFn: func(_ context.Context, _ string, _ int, _ time.Duration) (bool, error) {
			return true, nil
		},
	}

	app := newTestApp(middleware.LimitByIP(store, "test", 10, time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestLimitByIP_BlocksAtLimit(t *testing.T) {
	store := &mockIPLimiter{
		allowFn: func(_ context.Context, _ string, _ int, _ time.Duration) (bool, error) {
			return false, nil
		},
	}

	app := newTestApp(middleware.LimitByIP(store, "test", 10, time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header to be set when blocked")
	}
}

func TestLimitByIP_PassesThroughOnStoreError(t *testing.T) {
	store := &mockIPLimiter{
		allowFn: func(_ context.Context, _ string, _ int, _ time.Duration) (bool, error) {
			return false, fmt.Errorf("redis connection lost")
		},
	}

	app := newTestApp(middleware.LimitByIP(store, "test", 10, time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("store error should fail open: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestLimitByEmail_AllowsWhenEmailAbsent(t *testing.T) {
	called := false
	store := &mockEmailLimiter{
		allowEmailFn: func(_ context.Context, _ string, _ int, _ time.Duration) (bool, time.Duration, error) {
			called = true
			return false, time.Minute, nil
		},
	}

	app := newTestApp(middleware.LimitByEmail(store, 5, 15*time.Minute))
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"foo":"bar"}`))
	req.Header.Set("Content-Type", "application/json")
	_, _ = app.Test(req)

	if called {
		t.Error("email limiter should not be called when request body has no email field")
	}
}

func TestLimitByEmail_BlocksOnEmailViolation(t *testing.T) {
	store := &mockEmailLimiter{
		allowEmailFn: func(_ context.Context, emailHash string, _ int, _ time.Duration) (bool, time.Duration, error) {
			if emailHash == "" {
				t.Error("emailHash must not be empty")
			}
			return false, 15 * time.Minute, nil
		},
	}

	app := newTestApp(middleware.LimitByEmail(store, 5, 15*time.Minute))
	body := strings.NewReader(`{"email":"alice@example.com","password":"pass"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header when email is blocked")
	}
}

func TestLimitByEmail_EmailIsHashed(t *testing.T) {
	var capturedHash string

	store := &mockEmailLimiter{
		allowEmailFn: func(_ context.Context, emailHash string, _ int, _ time.Duration) (bool, time.Duration, error) {
			capturedHash = emailHash
			return true, 0, nil
		},
	}

	app := newTestApp(middleware.LimitByEmail(store, 5, 15*time.Minute))
	body := strings.NewReader(`{"email":"alice@example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	_, _ = io.ReadAll(resp.Body)

	if strings.Contains(capturedHash, "alice") {
		t.Errorf("email PII leaked into hash key: %q", capturedHash)
	}
	if len(capturedHash) != 64 {
		t.Errorf("expected SHA-256 hex string (64 chars), got %d chars", len(capturedHash))
	}
}
