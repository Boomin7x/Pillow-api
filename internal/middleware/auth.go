package middleware

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
)

type tokenValidator interface {
	ValidateAccessToken(token string) (*domain.Claims, error)
}

type blocklist interface {
	IsTokenBlocklisted(ctx context.Context, jti string) (bool, error)
}

func RequireAuth(verifier tokenValidator, bl blocklist) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get(fiber.HeaderAuthorization)
		if !strings.HasPrefix(header, "Bearer ") {
			return apperrors.Unauthorized("missing or malformed Authorization header")
		}

		raw := strings.TrimPrefix(header, "Bearer ")
		claims, err := verifier.ValidateAccessToken(raw)
		if err != nil {
			return apperrors.Unauthorized("invalid or expired access token")
		}

		if claims.TokenID != "" {
			blocked, err := bl.IsTokenBlocklisted(c.UserContext(), claims.TokenID)
			if err == nil && blocked {
				return apperrors.Unauthorized("token has been revoked")
			}
		}

		c.Locals("claims", claims)
		return c.Next()
	}
}

func Claims(c *fiber.Ctx) *domain.Claims {
	v, _ := c.Locals("claims").(*domain.Claims)
	return v
}
