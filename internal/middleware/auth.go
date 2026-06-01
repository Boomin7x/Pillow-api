package middleware

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
)

// tokenValidator is satisfied by domain.TokenIssuer.
type tokenValidator interface {
	ValidateAccessToken(token string) (*domain.Claims, error)
}

// blocklist is satisfied by domain.AuthRepository.
type blocklist interface {
	IsTokenBlocklisted(ctx context.Context, jti string) (bool, error)
}

// RequireAuth returns a Fiber middleware that validates the Bearer token,
// checks the blocklist, and injects *domain.Claims into c.Locals("claims").
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

// Claims extracts the injected *domain.Claims from the context.
// Returns nil if the middleware was not applied or the token was invalid.
func Claims(c *fiber.Ctx) *domain.Claims {
	v, _ := c.Locals("claims").(*domain.Claims)
	return v
}
