package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/middleware"
)

type mockAccessEvaluator struct {
	evaluateAccessFn func(ctx context.Context, userID string, action domain.Action) (*domain.AccessDecision, error)
}

func (m *mockAccessEvaluator) EvaluateAccess(ctx context.Context, userID string, action domain.Action) (*domain.AccessDecision, error) {
	return m.evaluateAccessFn(ctx, userID, action)
}

func newKYCGatedApp(evaluator *mockAccessEvaluator, injectClaims bool) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			var appErr *apperrors.AppError
			if e, ok := err.(*apperrors.AppError); ok {
				appErr = e
			} else {
				appErr = apperrors.Internal("unexpected")
			}
			return c.Status(apperrors.HTTPStatus(appErr.Code)).JSON(appErr)
		},
	})

	if injectClaims {
		app.Use(func(c *fiber.Ctx) error {
			c.Locals("claims", &domain.Claims{UserID: "u1"})
			return c.Next()
		})
	}

	app.Get("/gated",
		middleware.RequireKYC(evaluator, domain.ActionListProperty),
		func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
	)

	return app
}

func doGet(t *testing.T, app *fiber.App) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/gated", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp.StatusCode
}

func TestRequireKYC(t *testing.T) {
	tests := []struct {
		name         string
		injectClaims bool
		evaluateFn   func(ctx context.Context, userID string, action domain.Action) (*domain.AccessDecision, error)
		wantStatus   int
	}{
		{
			name:         "satisfied passes through",
			injectClaims: true,
			evaluateFn: func(_ context.Context, _ string, action domain.Action) (*domain.AccessDecision, error) {
				return &domain.AccessDecision{Action: action, Satisfied: true}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:         "not satisfied is forbidden",
			injectClaims: true,
			evaluateFn: func(_ context.Context, _ string, action domain.Action) (*domain.AccessDecision, error) {
				return &domain.AccessDecision{Action: action, Satisfied: false}, nil
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:         "missing claims is unauthorized",
			injectClaims: false,
			evaluateFn: func(_ context.Context, _ string, _ domain.Action) (*domain.AccessDecision, error) {
				t.Error("evaluator must not be called without claims")
				return nil, nil
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:         "evaluator error propagates",
			injectClaims: true,
			evaluateFn: func(_ context.Context, _ string, _ domain.Action) (*domain.AccessDecision, error) {
				return nil, apperrors.Internal("evaluator down")
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:         "non-apperror is mapped to 500",
			injectClaims: true,
			evaluateFn: func(_ context.Context, _ string, _ domain.Action) (*domain.AccessDecision, error) {
				return nil, errors.New("boom")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newKYCGatedApp(&mockAccessEvaluator{evaluateAccessFn: tt.evaluateFn}, tt.injectClaims)
			if got := doGet(t, app); got != tt.wantStatus {
				t.Errorf("status = %d, want %d", got, tt.wantStatus)
			}
		})
	}
}
