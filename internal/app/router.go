package app

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	infraredis "github.com/kodiahbertrand/pillow/internal/infrastructure/redis"
	"github.com/kodiahbertrand/pillow/internal/middleware"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type authRoutes struct {
	register       fiber.Handler
	login          fiber.Handler
	refresh        fiber.Handler
	logout         fiber.Handler
	logoutAll      fiber.Handler
	changePassword fiber.Handler
	oauthInitiate  fiber.Handler
	oauthCallback  fiber.Handler
}

type kycRoutes struct {
	getProfile            fiber.Handler
	startVerification     fiber.Handler
	uploadDocument        fiber.Handler
	getCase               fiber.Handler
	startOwnershipClaim   fiber.Handler
	submitLicense         fiber.Handler
	startBusinessVerif    fiber.Handler
	handleProviderWebhook fiber.Handler
	deleteProfile         fiber.Handler
}

type routeDeps struct {
	auth        authRoutes
	kyc         kycRoutes
	issuer      domain.TokenIssuer
	authRepo    domain.AuthRepository
	rateLimiter *infraredis.RateLimiter
	db          *gorm.DB
	redis       *goredis.Client
	rl          config.RateLimitConfig
}

func registerRoutes(f *fiber.App, deps routeDeps) {
	f.Get("/health/live", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	f.Get("/health/ready", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()

		sqlDB, err := deps.db.DB()
		if err != nil {
			return apperrors.Internal("database unavailable")
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return apperrors.Internal("database unavailable")
		}
		if err := deps.redis.Ping(ctx).Err(); err != nil {
			return apperrors.Internal("cache unavailable")
		}
		return c.SendStatus(fiber.StatusOK)
	})

	authGroup := f.Group("/auth")

	authGroup.Post("/register",
		middleware.LimitByIP(deps.rateLimiter, "register", deps.rl.RegisterIPLimit, deps.rl.RegisterIPWindow),
		deps.auth.register,
	)

	authGroup.Post("/login",
		middleware.LimitByIP(deps.rateLimiter, "login", deps.rl.LoginIPLimit, deps.rl.LoginIPWindow),
		middleware.LimitByEmail(deps.rateLimiter, deps.rl.LoginEmailLimit, deps.rl.LoginEmailBaseWindow),
		deps.auth.login,
	)

	authGroup.Post("/refresh",
		middleware.LimitByIP(deps.rateLimiter, "refresh", deps.rl.RefreshIPLimit, deps.rl.RefreshIPWindow),
		deps.auth.refresh,
	)

	protected := middleware.RequireAuth(deps.issuer, deps.authRepo)
	authGroup.Post("/logout", protected, deps.auth.logout)
	authGroup.Post("/logout-all", protected, deps.auth.logoutAll)
	authGroup.Post("/password", protected, deps.auth.changePassword)

	authGroup.Get("/google",
		middleware.LimitByIP(deps.rateLimiter, "oauth", deps.rl.OAuthIPLimit, deps.rl.OAuthIPWindow),
		deps.auth.oauthInitiate,
	)
	authGroup.Get("/google/callback", deps.auth.oauthCallback)

	f.Get("/.well-known/jwks.json", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "public, max-age=300")
		return c.JSON(fiber.Map{"keys": deps.issuer.PublicKeySet()})
	})

	f.Post("/kyc/webhooks/provider",
		middleware.LimitByIP(deps.rateLimiter, "kyc_webhook", deps.rl.KYCWebhookIPLimit, deps.rl.KYCWebhookIPWindow),
		deps.kyc.handleProviderWebhook,
	)

	kycGroup := f.Group("/kyc")
	kycGroup.Use(middleware.RequireAuth(deps.issuer, deps.authRepo))

	kycGroup.Get("/profile", deps.kyc.getProfile)
	kycGroup.Delete("/profile", deps.kyc.deleteProfile)
	kycGroup.Post("/verifications",
		middleware.LimitByIP(deps.rateLimiter, "kyc_start", deps.rl.KYCStartIPLimit, deps.rl.KYCStartIPWindow),
		deps.kyc.startVerification,
	)
	kycGroup.Post("/verifications/:id/documents",
		middleware.LimitByIP(deps.rateLimiter, "kyc_upload", deps.rl.KYCUploadIPLimit, deps.rl.KYCUploadIPWindow),
		deps.kyc.uploadDocument,
	)
	kycGroup.Get("/verifications/:id", deps.kyc.getCase)
	kycGroup.Post("/ownership-claims",
		middleware.LimitByIP(deps.rateLimiter, "kyc_ownership", deps.rl.KYCOwnershipIPLimit, deps.rl.KYCOwnershipIPWindow),
		deps.kyc.startOwnershipClaim,
	)
	kycGroup.Post("/licenses",
		middleware.LimitByIP(deps.rateLimiter, "kyc_license", deps.rl.KYCLicenseIPLimit, deps.rl.KYCLicenseIPWindow),
		deps.kyc.submitLicense,
	)
	kycGroup.Post("/business",
		middleware.LimitByIP(deps.rateLimiter, "kyc_business", deps.rl.KYCBusinessIPLimit, deps.rl.KYCBusinessIPWindow),
		deps.kyc.startBusinessVerif,
	)
}
