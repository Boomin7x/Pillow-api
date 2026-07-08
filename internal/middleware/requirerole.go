package middleware

import (
	"slices"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
)

func RequireRole(role string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims := Claims(c)
		if claims == nil {
			return apperrors.Unauthorized("not authenticated")
		}
		if !slices.Contains(claims.Roles, role) {
			return apperrors.Forbidden("insufficient role")
		}
		return c.Next()
	}
}
