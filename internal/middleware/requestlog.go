package middleware

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const TraceIDLocalsKey = "trace_id"

func RequestLogger(logger *slog.Logger, redactIP bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		traceID := uuid.NewString()
		c.Locals(TraceIDLocalsKey, traceID)

		start := time.Now()
		chainErr := c.Next()
		latency := time.Since(start)

		attrs := []slog.Attr{
			slog.String("trace_id", traceID),
			slog.String("method", c.Method()),
			slog.String("endpoint", c.Path()),
			slog.Int("status", c.Response().StatusCode()),
			slog.Int64("latency_ms", latency.Milliseconds()),
			slog.String("ip", clientIP(c.IP(), redactIP)),
		}

		if claims := Claims(c); claims != nil {
			if claims.UserID != "" {
				attrs = append(attrs, slog.String("user_id", claims.UserID))
			}
			if claims.Email != "" {
				attrs = append(attrs, slog.String("email_hash", hashEmail(claims.Email)))
			}
		}

		logger.LogAttrs(c.UserContext(), slog.LevelInfo, "request", attrs...)
		return chainErr
	}
}

func clientIP(ip string, redact bool) string {
	if !redact {
		return ip
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "unknown"
	}
	if v4 := parsed.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.0/24", v4[0], v4[1], v4[2])
	}
	masked := parsed.Mask(net.CIDRMask(48, 128))
	return masked.String() + "/48"
}

func hashEmail(email string) string {
	h := sha256.Sum256([]byte(email))
	return fmt.Sprintf("%x", h)
}
