package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
)

const refreshCookieName = "refresh_token"

const pkceTTL = 10 * time.Minute

const accessTokenBlocklistTTL = 15 * time.Minute

type authHandler struct {
	service       domain.AuthService
	issuer        domain.TokenIssuer
	pkceStore     domain.PKCEStore
	oauthProvider domain.OAuthProvider
	validator     *validator.Validate
}

func NewHandler(service domain.AuthService, issuer domain.TokenIssuer, pkce domain.PKCEStore, provider domain.OAuthProvider) *authHandler {
	return &authHandler{
		service:       service,
		issuer:        issuer,
		pkceStore:     pkce,
		oauthProvider: provider,
		validator:     validator.New(),
	}
}

func (h *authHandler) Register(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return apperrors.BadRequest("invalid request body")
	}
	if err := h.validator.Struct(req); err != nil {
		return apperrors.Validation(err.Error())
	}

	result, err := h.service.Register(c.UserContext(), domain.RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		return err
	}

	h.setRefreshCookie(c, result.RefreshToken)
	return c.Status(fiber.StatusCreated).JSON(authResponseFromDomain(result))
}

func (h *authHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return apperrors.BadRequest("invalid request body")
	}
	if err := h.validator.Struct(req); err != nil {
		return apperrors.Validation(err.Error())
	}

	result, err := h.service.Login(c.UserContext(), domain.LoginInput{
		Email:     req.Email,
		Password:  req.Password,
		IPAddress: c.IP(),
		UserAgent: c.Get(fiber.HeaderUserAgent),
	})
	if err != nil {
		return err
	}

	h.setRefreshCookie(c, result.RefreshToken)
	return c.Status(fiber.StatusOK).JSON(authResponseFromDomain(result))
}

func (h *authHandler) Refresh(c *fiber.Ctx) error {
	rawToken := c.Cookies(refreshCookieName)
	if rawToken == "" {
		return apperrors.Unauthorized("refresh token missing")
	}

	result, err := h.service.Refresh(c.UserContext(), domain.RefreshInput{
		RawToken:  rawToken,
		IPAddress: c.IP(),
		UserAgent: c.Get(fiber.HeaderUserAgent),
	})
	if err != nil {
		return err
	}

	h.setRefreshCookie(c, result.RefreshToken)
	return c.Status(fiber.StatusOK).JSON(authResponseFromDomain(result))
}

func (h *authHandler) Logout(c *fiber.Ctx) error {
	claims, ok := c.Locals("claims").(*domain.Claims)
	if !ok || claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}

	rawToken := c.Cookies(refreshCookieName)

	if err := h.service.Logout(c.UserContext(), domain.LogoutInput{
		AccessTokenJTI:  claims.TokenID,
		AccessTokenTTL:  accessTokenBlocklistTTL,
		RefreshTokenRaw: rawToken,
		UserID:          claims.UserID,
	}); err != nil {
		return err
	}

	h.clearRefreshCookie(c)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *authHandler) LogoutAll(c *fiber.Ctx) error {
	claims, ok := c.Locals("claims").(*domain.Claims)
	if !ok || claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}

	if err := h.service.LogoutAll(c.UserContext(), domain.LogoutAllInput{
		UserID:         claims.UserID,
		ActiveTokenJTI: claims.TokenID,
		ActiveTokenTTL: accessTokenBlocklistTTL,
	}); err != nil {
		return err
	}

	h.clearRefreshCookie(c)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *authHandler) ChangePassword(c *fiber.Ctx) error {
	claims, ok := c.Locals("claims").(*domain.Claims)
	if !ok || claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}

	var req ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return apperrors.BadRequest("invalid request body")
	}
	if err := h.validator.Struct(req); err != nil {
		return apperrors.Validation(err.Error())
	}

	if err := h.service.ChangePassword(c.UserContext(), domain.ChangePasswordInput{
		UserID:      claims.UserID,
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
	}); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *authHandler) OAuthInitiate(c *fiber.Ctx) error {
	state, err := randomBase64URL(32)
	if err != nil {
		return fmt.Errorf("oauth initiate: generate state: %w", err)
	}
	verifier, err := randomBase64URL(64)
	if err != nil {
		return fmt.Errorf("oauth initiate: generate verifier: %w", err)
	}

	challenge := pkceChallenge(verifier)

	if err := h.pkceStore.SaveVerifier(c.UserContext(), state, verifier, pkceTTL); err != nil {
		return fmt.Errorf("oauth initiate: save verifier: %w", err)
	}

	return c.Redirect(h.oauthProvider.BuildAuthURL(state, challenge), fiber.StatusFound)
}

func (h *authHandler) OAuthCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	if code == "" || state == "" {
		return apperrors.BadRequest("missing code or state parameter")
	}

	verifier, err := h.pkceStore.GetVerifier(c.UserContext(), state)
	if err != nil {
		return apperrors.BadRequest("invalid or expired oauth state")
	}
	if err := h.pkceStore.DeleteVerifier(c.UserContext(), state); err != nil {
		return fmt.Errorf("oauth callback: delete verifier: %w", err)
	}

	claims, err := h.oauthProvider.ExchangeAndVerify(c.UserContext(), code, verifier)
	if err != nil {
		return apperrors.Unauthorized("oauth token exchange failed")
	}

	result, err := h.service.OAuthLogin(c.UserContext(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: claims.ProviderID,
		Email:      claims.Email,
		Name:       claims.Name,
		IPAddress:  c.IP(),
		UserAgent:  c.Get(fiber.HeaderUserAgent),
	})
	if err != nil {
		return err
	}

	h.setRefreshCookie(c, result.RefreshToken)
	return c.Status(fiber.StatusOK).JSON(authResponseFromDomain(result))
}

func (h *authHandler) setRefreshCookie(c *fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Strict",
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		Path:     "/auth/refresh",
	})
}

func (h *authHandler) clearRefreshCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/auth/refresh",
		Expires:  time.Now().Add(-time.Hour),
		MaxAge:   -1,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Strict",
	})
}

func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
