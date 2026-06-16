package auth_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/auth"
	"github.com/kodiahbertrand/pillow/internal/domain"
)

type mockAuthService struct {
	registerFn       func(ctx context.Context, input domain.RegisterInput) (*domain.AuthResult, error)
	loginFn          func(ctx context.Context, input domain.LoginInput) (*domain.AuthResult, error)
	refreshFn        func(ctx context.Context, input domain.RefreshInput) (*domain.AuthResult, error)
	logoutFn         func(ctx context.Context, input domain.LogoutInput) error
	logoutAllFn      func(ctx context.Context, input domain.LogoutAllInput) error
	changePasswordFn func(ctx context.Context, input domain.ChangePasswordInput) error
	oauthLoginFn     func(ctx context.Context, input domain.OAuthLoginInput) (*domain.AuthResult, error)
}

func (m *mockAuthService) Register(ctx context.Context, input domain.RegisterInput) (*domain.AuthResult, error) {
	return m.registerFn(ctx, input)
}
func (m *mockAuthService) Login(ctx context.Context, input domain.LoginInput) (*domain.AuthResult, error) {
	return m.loginFn(ctx, input)
}
func (m *mockAuthService) Refresh(ctx context.Context, input domain.RefreshInput) (*domain.AuthResult, error) {
	return m.refreshFn(ctx, input)
}
func (m *mockAuthService) Logout(ctx context.Context, input domain.LogoutInput) error {
	if m.logoutFn != nil {
		return m.logoutFn(ctx, input)
	}
	return nil
}
func (m *mockAuthService) LogoutAll(ctx context.Context, input domain.LogoutAllInput) error {
	if m.logoutAllFn != nil {
		return m.logoutAllFn(ctx, input)
	}
	return nil
}
func (m *mockAuthService) ChangePassword(ctx context.Context, input domain.ChangePasswordInput) error {
	if m.changePasswordFn != nil {
		return m.changePasswordFn(ctx, input)
	}
	return nil
}
func (m *mockAuthService) OAuthLogin(ctx context.Context, input domain.OAuthLoginInput) (*domain.AuthResult, error) {
	return m.oauthLoginFn(ctx, input)
}

func newHandlerApp(svc domain.AuthService) *fiber.App {
	pkce, provider := noopOAuth()
	h := auth.NewHandler(svc, okIssuer(), pkce, provider)

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

	app.Post("/auth/register", h.Register)
	app.Post("/auth/login", h.Login)
	app.Post("/auth/refresh", h.Refresh)
	app.Post("/auth/logout", h.Logout)
	app.Post("/auth/logout-all", h.LogoutAll)
	app.Post("/auth/password", h.ChangePassword)
	app.Get("/auth/google", h.OAuthInitiate)
	app.Get("/auth/google/callback", h.OAuthCallback)

	withClaims := func(c *fiber.Ctx) error {
		c.Locals("claims", &domain.Claims{UserID: "u1", TokenID: "jti-1"})
		return c.Next()
	}
	app.Post("/auth/authed-logout", withClaims, h.Logout)
	app.Post("/auth/authed-logout-all", withClaims, h.LogoutAll)
	app.Post("/auth/authed-password", withClaims, h.ChangePassword)

	return app
}

func postJSON(t *testing.T, app *fiber.App, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func setCookiePresent(resp *http.Response, name string) bool {
	for _, ck := range resp.Cookies() {
		if ck.Name == name {
			return true
		}
	}
	return false
}

func TestRegisterHandler_Success(t *testing.T) {
	svc := &mockAuthService{
		registerFn: func(_ context.Context, _ domain.RegisterInput) (*domain.AuthResult, error) {
			return &domain.AuthResult{
				AccessToken:  "access.token",
				RefreshToken: "refresh-token",
				User:         &domain.User{ID: "u1", Email: "alice@example.com", DisplayName: "Alice"},
			}, nil
		},
	}

	resp := postJSON(t, newHandlerApp(svc), "/auth/register",
		`{"email":"alice@example.com","password":"securepass","display_name":"Alice"}`)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var body auth.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.AccessToken != "access.token" {
		t.Errorf("access token = %q, want %q", body.AccessToken, "access.token")
	}
	if !setCookiePresent(resp, "refresh_token") {
		t.Error("expected refresh_token Set-Cookie header")
	}
}

func TestRegisterHandler_InvalidBody(t *testing.T) {
	svc := &mockAuthService{
		registerFn: func(_ context.Context, _ domain.RegisterInput) (*domain.AuthResult, error) {
			t.Error("service should not be called on malformed body")
			return nil, nil
		},
	}

	resp := postJSON(t, newHandlerApp(svc), "/auth/register", `{not-json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestRegisterHandler_ValidationError(t *testing.T) {
	svc := &mockAuthService{
		registerFn: func(_ context.Context, _ domain.RegisterInput) (*domain.AuthResult, error) {
			t.Error("service should not be called when validation fails")
			return nil, nil
		},
	}

	resp := postJSON(t, newHandlerApp(svc), "/auth/register",
		`{"email":"not-an-email","password":"x"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
	}
}

func TestRegisterHandler_ConflictError(t *testing.T) {
	svc := &mockAuthService{
		registerFn: func(_ context.Context, _ domain.RegisterInput) (*domain.AuthResult, error) {
			return nil, apperrors.Conflict("email already registered")
		},
	}

	resp := postJSON(t, newHandlerApp(svc), "/auth/register",
		`{"email":"alice@example.com","password":"securepass","display_name":"Alice"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestLoginHandler_Success(t *testing.T) {
	svc := &mockAuthService{
		loginFn: func(_ context.Context, _ domain.LoginInput) (*domain.AuthResult, error) {
			return &domain.AuthResult{
				AccessToken:  "access.token",
				RefreshToken: "refresh-token",
				User:         &domain.User{ID: "u1", Email: "alice@example.com"},
			}, nil
		},
	}

	resp := postJSON(t, newHandlerApp(svc), "/auth/login",
		`{"email":"alice@example.com","password":"securepass"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body auth.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.AccessToken == "" {
		t.Error("expected access token in response body")
	}
	if !setCookiePresent(resp, "refresh_token") {
		t.Error("expected refresh_token Set-Cookie header")
	}
}

func TestLoginHandler_Unauthorized(t *testing.T) {
	svc := &mockAuthService{
		loginFn: func(_ context.Context, _ domain.LoginInput) (*domain.AuthResult, error) {
			return nil, apperrors.Unauthorized("invalid email or password")
		},
	}

	resp := postJSON(t, newHandlerApp(svc), "/auth/login",
		`{"email":"alice@example.com","password":"wrong"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRefreshHandler_MissingCookie(t *testing.T) {
	svc := &mockAuthService{
		refreshFn: func(_ context.Context, _ domain.RefreshInput) (*domain.AuthResult, error) {
			t.Error("service should not be called when refresh cookie is absent")
			return nil, nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	resp, err := newHandlerApp(svc).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRefreshHandler_Success(t *testing.T) {
	svc := &mockAuthService{
		refreshFn: func(_ context.Context, _ domain.RefreshInput) (*domain.AuthResult, error) {
			return &domain.AuthResult{
				AccessToken:  "new.access.token",
				RefreshToken: "rotated-refresh-token",
				User:         &domain.User{ID: "u1", Email: "alice@example.com"},
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "old-refresh-token"})
	resp, err := newHandlerApp(svc).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if !setCookiePresent(resp, "refresh_token") {
		t.Error("expected a rotated refresh_token Set-Cookie header")
	}
}

func TestLogoutHandler_MissingAuth(t *testing.T) {
	svc := &mockAuthService{
		logoutFn: func(_ context.Context, _ domain.LogoutInput) error {
			t.Error("service should not be called without authenticated claims")
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	resp, err := newHandlerApp(svc).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestLogoutHandler_Success(t *testing.T) {
	var loggedOut bool
	svc := &mockAuthService{
		logoutFn: func(_ context.Context, _ domain.LogoutInput) error {
			loggedOut = true
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/authed-logout", nil)
	resp, err := newHandlerApp(svc).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if !loggedOut {
		t.Error("expected service.Logout to be called")
	}

	clearedCookie := false
	for _, ck := range resp.Cookies() {
		if ck.Name != "refresh_token" {
			continue
		}
		if ck.Value == "" || ck.MaxAge < 0 || (!ck.Expires.IsZero() && ck.Expires.Before(time.Now())) {
			clearedCookie = true
		}
	}
	if !clearedCookie {
		t.Error("expected refresh_token cookie to be cleared on logout")
	}

	_, _ = io.ReadAll(resp.Body)
}

func TestLogoutAllHandler_MissingAuth(t *testing.T) {
	svc := &mockAuthService{
		logoutAllFn: func(_ context.Context, _ domain.LogoutAllInput) error {
			t.Error("service should not be called without authenticated claims")
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/logout-all", nil)
	resp, err := newHandlerApp(svc).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestLogoutAllHandler_Success(t *testing.T) {
	var calledWithUser string
	svc := &mockAuthService{
		logoutAllFn: func(_ context.Context, input domain.LogoutAllInput) error {
			calledWithUser = input.UserID
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/authed-logout-all", nil)
	resp, err := newHandlerApp(svc).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if calledWithUser != "u1" {
		t.Errorf("LogoutAll called with user %q, want %q", calledWithUser, "u1")
	}
}

func TestChangePasswordHandler_MissingAuth(t *testing.T) {
	svc := &mockAuthService{}
	resp := postJSON(t, newHandlerApp(svc), "/auth/password",
		`{"old_password":"old","new_password":"newpassword123"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestChangePasswordHandler_ValidationError(t *testing.T) {
	svc := &mockAuthService{
		changePasswordFn: func(_ context.Context, _ domain.ChangePasswordInput) error {
			t.Error("service should not be called when validation fails")
			return nil
		},
	}
	resp := postJSON(t, newHandlerApp(svc), "/auth/authed-password",
		`{"old_password":"old","new_password":"short"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
	}
}

func TestChangePasswordHandler_Success(t *testing.T) {
	var called bool
	svc := &mockAuthService{
		changePasswordFn: func(_ context.Context, input domain.ChangePasswordInput) error {
			called = true
			if input.UserID != "u1" {
				t.Errorf("user id = %q, want u1", input.UserID)
			}
			return nil
		},
	}
	resp := postJSON(t, newHandlerApp(svc), "/auth/authed-password",
		`{"old_password":"oldpassword","new_password":"newpassword123"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if !called {
		t.Error("expected ChangePassword to be called")
	}
}

func TestChangePasswordHandler_InvalidBody(t *testing.T) {
	svc := &mockAuthService{}
	resp := postJSON(t, newHandlerApp(svc), "/auth/authed-password", `{bad json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestOAuthInitiateHandler_Redirects(t *testing.T) {
	svc := &mockAuthService{}
	req := httptest.NewRequest(http.MethodGet, "/auth/google", nil)
	resp, err := newHandlerApp(svc).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if resp.Header.Get("Location") == "" {
		t.Error("expected a Location header redirecting to the consent screen")
	}
}

func TestOAuthCallbackHandler_MissingParams(t *testing.T) {
	svc := &mockAuthService{
		oauthLoginFn: func(_ context.Context, _ domain.OAuthLoginInput) (*domain.AuthResult, error) {
			t.Error("service should not be called when code/state are absent")
			return nil, nil
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback", nil)
	resp, err := newHandlerApp(svc).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestOAuthCallbackHandler_Success(t *testing.T) {
	svc := &mockAuthService{
		oauthLoginFn: func(_ context.Context, _ domain.OAuthLoginInput) (*domain.AuthResult, error) {
			return &domain.AuthResult{
				AccessToken:  "access.token",
				RefreshToken: "refresh-token",
				User:         &domain.User{ID: "u1", Email: "alice@example.com"},
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=auth-code&state=state-1", nil)
	resp, err := newHandlerApp(svc).Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if !setCookiePresent(resp, "refresh_token") {
		t.Error("expected refresh_token cookie to be set after oauth callback")
	}
}
