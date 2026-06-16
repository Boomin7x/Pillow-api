package middleware_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/middleware"
)

type mockValidator struct {
	validateFn func(token string) (*domain.Claims, error)
}

func (m *mockValidator) ValidateAccessToken(token string) (*domain.Claims, error) {
	return m.validateFn(token)
}

type mockBlocklist struct {
	isBlocklistedFn func(ctx context.Context, jti string) (bool, error)
}

func (m *mockBlocklist) IsTokenBlocklisted(ctx context.Context, jti string) (bool, error) {
	if m.isBlocklistedFn != nil {
		return m.isBlocklistedFn(ctx, jti)
	}
	return false, nil
}

func newProtectedApp(v *mockValidator, bl *mockBlocklist) *fiber.App {
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

	app.Get("/protected", middleware.RequireAuth(v, bl), func(c *fiber.Ctx) error {
		claims := middleware.Claims(c)
		if claims == nil {
			return apperrors.Internal("claims not injected")
		}
		return c.SendString(claims.UserID)
	})

	return app
}

func TestRequireAuth_ValidToken(t *testing.T) {
	v := &mockValidator{
		validateFn: func(_ string) (*domain.Claims, error) {
			return &domain.Claims{UserID: "u1", TokenID: "jti-1"}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer valid.token")
	resp, err := newProtectedApp(v, &mockBlocklist{}).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body := make([]byte, 2)
	_, _ = resp.Body.Read(body)
	if string(body) != "u1" {
		t.Errorf("expected handler to read injected claims (user id u1), got %q", string(body))
	}
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	v := &mockValidator{
		validateFn: func(_ string) (*domain.Claims, error) {
			t.Error("validator should not run when Authorization header is absent")
			return nil, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp, err := newProtectedApp(v, &mockBlocklist{}).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	v := &mockValidator{
		validateFn: func(_ string) (*domain.Claims, error) {
			return nil, fmt.Errorf("token is expired")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer expired.token")
	resp, err := newProtectedApp(v, &mockBlocklist{}).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRequireAuth_BlocklistedToken(t *testing.T) {
	v := &mockValidator{
		validateFn: func(_ string) (*domain.Claims, error) {
			return &domain.Claims{UserID: "u1", TokenID: "jti-revoked"}, nil
		},
	}
	bl := &mockBlocklist{
		isBlocklistedFn: func(_ context.Context, jti string) (bool, error) {
			return jti == "jti-revoked", nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer revoked.token")
	resp, err := newProtectedApp(v, bl).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}
