package middleware

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
)

type kycAccessEvaluator interface {
	EvaluateAccess(ctx context.Context, userID string, action domain.Action) (*domain.AccessDecision, error)
}

func RequireKYC(evaluator kycAccessEvaluator, action domain.Action) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims := Claims(c)
		if claims == nil {
			return apperrors.Unauthorized("not authenticated")
		}

		decision, err := evaluator.EvaluateAccess(c.UserContext(), claims.UserID, action)
		if err != nil {
			return err
		}

		if !decision.Satisfied {
			return apperrors.Forbidden("insufficient KYC tier or missing qualifications for this action")
		}

		return c.Next()
	}
}
