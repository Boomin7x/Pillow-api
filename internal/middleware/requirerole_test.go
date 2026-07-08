package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/middleware"
)

func newRoleGatedApp(roles []string, injectClaims bool) *fiber.App {
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
			c.Locals("claims", &domain.Claims{UserID: "u1", Roles: roles})
			return c.Next()
		})
	}

	app.Get("/admin/ping", middleware.RequireRole("admin"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func TestAdminGroupChain_RequireAuthThenRequireRole(t *testing.T) {
	tokenRoles := map[string][]string{
		"admin-token": {"user", "admin"},
		"user-token":  {"user"},
	}
	validator := &mockValidator{
		validateFn: func(token string) (*domain.Claims, error) {
			roles, ok := tokenRoles[token]
			if !ok {
				return nil, fiber.ErrUnauthorized
			}
			return &domain.Claims{UserID: "u1", TokenID: "jti-1", Roles: roles}, nil
		},
	}

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
	admin := app.Group("/admin",
		middleware.RequireAuth(validator, &mockBlocklist{}),
		middleware.RequireRole("admin"),
	)
	admin.Get("/ping", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{name: "admin token reaches ping", authHeader: "Bearer admin-token", wantStatus: http.StatusOK},
		{name: "normal user token gets 403", authHeader: "Bearer user-token", wantStatus: http.StatusForbidden},
		{name: "no token gets 401", authHeader: "", wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}

func TestRequireRole(t *testing.T) {
	cases := []struct {
		name         string
		roles        []string
		injectClaims bool
		wantStatus   int
	}{
		{name: "admin role passes", roles: []string{"user", "admin"}, injectClaims: true, wantStatus: http.StatusOK},
		{name: "baseline user role is forbidden", roles: []string{"user"}, injectClaims: true, wantStatus: http.StatusForbidden},
		{name: "empty roles are forbidden", roles: nil, injectClaims: true, wantStatus: http.StatusForbidden},
		{name: "missing claims are unauthorized", injectClaims: false, wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newRoleGatedApp(tc.roles, tc.injectClaims)
			req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}
