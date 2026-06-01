package auth

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
)

const refreshCookieName = "refresh_token"

type authHandler struct {
	service   domain.AuthService
	issuer    domain.TokenIssuer
	validator *validator.Validate
}

func NewHandler(service domain.AuthService, issuer domain.TokenIssuer) *authHandler {
	return &authHandler{
		service:   service,
		issuer:    issuer,
		validator: validator.New(),
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

	// Calculate remaining JWT lifetime for the blocklist TTL.
	// The access token's exp claim drives the TTL so the Redis entry auto-deletes.
	if err := h.service.Logout(c.UserContext(), domain.LogoutInput{
		AccessTokenJTI:  claims.TokenID,
		AccessTokenTTL:  15 * time.Minute, // matches JWT_ACCESS_TTL_MINUTES default
		RefreshTokenRaw: rawToken,
		UserID:          claims.UserID,
	}); err != nil {
		return err
	}

	c.ClearCookie(refreshCookieName)
	return c.SendStatus(fiber.StatusNoContent)
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
