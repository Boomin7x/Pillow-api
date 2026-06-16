package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/middleware"
)

func newLoggedApp(buf *bytes.Buffer, redactIP bool, inject *domain.Claims) *fiber.App {
	logger := slog.New(slog.NewJSONHandler(buf, nil))
	app := fiber.New()
	app.Use(middleware.RequestLogger(logger, redactIP))
	app.Get("/probe", func(c *fiber.Ctx) error {
		if inject != nil {
			c.Locals("claims", inject)
		}
		if middleware.Claims(c) == nil && inject == nil {
			return c.SendString(c.Locals(middleware.TraceIDLocalsKey).(string))
		}
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func TestRequestLogger_InjectsTraceIDAndLogsFields(t *testing.T) {
	var buf bytes.Buffer
	app := newLoggedApp(&buf, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, buf.String())
	}

	for _, field := range []string{"trace_id", "method", "endpoint", "status", "latency_ms", "ip"} {
		if _, ok := line[field]; !ok {
			t.Errorf("expected log field %q to be present", field)
		}
	}
	if line["trace_id"] == "" {
		t.Error("expected a non-empty trace_id")
	}
}

func TestRequestLogger_RedactsIPInProduction(t *testing.T) {
	var buf bytes.Buffer
	app := newLoggedApp(&buf, true, nil)

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if !strings.Contains(buf.String(), "/24") {
		t.Errorf("expected IP to be redacted to a /24 prefix, got: %s", buf.String())
	}
	if strings.Contains(buf.String(), "203.0.113.7") {
		t.Errorf("full client IP leaked into log line: %s", buf.String())
	}
}

func TestRequestLogger_HashesEmailNeverLogsRaw(t *testing.T) {
	var buf bytes.Buffer
	claims := &domain.Claims{UserID: "u1", Email: "alice@example.com"}
	app := newLoggedApp(&buf, true, claims)

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if strings.Contains(buf.String(), "alice@example.com") {
		t.Errorf("raw email leaked into log line: %s", buf.String())
	}

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("log line is not valid JSON: %v", err)
	}
	if line["user_id"] != "u1" {
		t.Errorf("user_id = %v, want u1", line["user_id"])
	}
	if h, ok := line["email_hash"].(string); !ok || len(h) != 64 {
		t.Errorf("expected a 64-char SHA-256 email_hash, got %v", line["email_hash"])
	}
}
