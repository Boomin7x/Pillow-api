package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
)

type ipLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

type emailLimiter interface {
	AllowEmail(ctx context.Context, emailHash string, limit int, baseWindow time.Duration) (allowed bool, retryAfter time.Duration, err error)
}

func LimitByIP(store ipLimiter, endpointSlug string, limit int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := fmt.Sprintf("ratelimit:ip:%s:%s", c.IP(), endpointSlug)

		allowed, err := store.Allow(c.UserContext(), key, limit, window)
		if err != nil {
			return c.Next()
		}

		if !allowed {
			c.Set("Retry-After", fmt.Sprintf("%.0f", window.Seconds()))
			return apperrors.RateLimit("too many requests — try again later")
		}

		return c.Next()
	}
}

func LimitByEmail(store emailLimiter, limit int, baseWindow time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		email := extractEmail(c.Body())
		if email == "" {
			return c.Next()
		}

		h := sha256.Sum256([]byte(strings.ToLower(email)))
		emailHash := fmt.Sprintf("%x", h)

		allowed, retryAfter, err := store.AllowEmail(c.UserContext(), emailHash, limit, baseWindow)
		if err != nil {
			return c.Next()
		}

		if !allowed {
			c.Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			return apperrors.RateLimit("too many login attempts — try again later")
		}

		return c.Next()
	}
}

func extractEmail(body []byte) string {
	var req struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(body, &req)
	return req.Email
}
