package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/middleware"
)

type authRoutes struct {
	register fiber.Handler
	login    fiber.Handler
	refresh  fiber.Handler
	logout   fiber.Handler
}

type routeDeps struct {
	auth     authRoutes
	issuer   domain.TokenIssuer
	authRepo domain.AuthRepository
}

func registerRoutes(f *fiber.App, deps routeDeps) {
	// Health
	f.Get("/health/live", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	f.Get("/health/ready", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	// Auth routes (public)
	authGroup := f.Group("/auth")
	authGroup.Post("/register", deps.auth.register)
	authGroup.Post("/login", deps.auth.login)
	authGroup.Post("/refresh", deps.auth.refresh)

	// Auth routes (protected)
	protected := middleware.RequireAuth(deps.issuer, deps.authRepo)
	authGroup.Post("/logout", protected, deps.auth.logout)
}
