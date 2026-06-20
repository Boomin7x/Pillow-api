package app

import (
	"context"
	"fmt"
	"time"

	"log/slog"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/auth"
	"github.com/kodiahbertrand/pillow/internal/config"
	infraoauth "github.com/kodiahbertrand/pillow/internal/infrastructure/oauth"
	infrapostgres "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	infraredis "github.com/kodiahbertrand/pillow/internal/infrastructure/redis"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/tokenutil"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/internal/middleware"
)

type App struct {
	fiberApp   *fiber.App
	cfg        *config.Config
	cancel     context.CancelFunc
	loggerDone <-chan struct{}
}

func New(cfg *config.Config) (*App, error) {
	db, err := infrapostgres.NewDB(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("app: init postgres: %w", err)
	}

	rdb, err := infraredis.NewClient(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("app: init redis: %w", err)
	}

	issuer, err := tokenutil.NewJWTIssuer(cfg.JWT)
	if err != nil {
		return nil, fmt.Errorf("app: init token issuer: %w", err)
	}

	auditLog := auth.NewAuditLogger(db, 256)
	ctx, cancel := context.WithCancel(context.Background())
	go auditLog.Run(ctx)
	go auth.NewTokenCleanup(db).Run(ctx, 24*time.Hour)

	rateLimiter := infraredis.NewRateLimiter(rdb)

	oauthProvider := infraoauth.NewGoogleProvider(cfg.OAuth)

	authRepo := auth.NewRepository(db, rdb)
	authSvc := auth.NewServiceWithOAuth(authRepo, issuer, auditLog, authRepo, oauthProvider)
	authHandler := auth.NewHandler(authSvc, issuer, authRepo, oauthProvider)

	kycComponents, err := buildKYC(cfg, db, rdb)
	if err != nil {
		cancel()
		return nil, err
	}
	kycHandler := kyc.NewHandler(kycComponents.service, kycComponents.webhookVerifier)

	f := fiber.New(fiber.Config{
		ErrorHandler: errorHandler,
		AppName:      cfg.App.Name,
	})

	requestLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	isProduction := cfg.App.Env == "production"

	f.Use(recover.New())
	f.Use(middleware.RequestLogger(requestLogger, isProduction))
	f.Use(cors.New())

	registerDocsRoutes(f)

	registerRoutes(f, routeDeps{
		auth: authRoutes{
			register:       authHandler.Register,
			login:          authHandler.Login,
			refresh:        authHandler.Refresh,
			logout:         authHandler.Logout,
			logoutAll:      authHandler.LogoutAll,
			changePassword: authHandler.ChangePassword,
			oauthInitiate:  authHandler.OAuthInitiate,
			oauthCallback:  authHandler.OAuthCallback,
		},
		kyc: kycRoutes{
			getProfile:            kycHandler.GetProfile,
			startVerification:     kycHandler.StartVerification,
			uploadDocument:        kycHandler.UploadDocument,
			getCase:               kycHandler.GetCase,
			startOwnershipClaim:   kycHandler.StartOwnershipClaim,
			submitLicense:         kycHandler.SubmitLicense,
			startBusinessVerif:    kycHandler.StartBusinessVerification,
			handleProviderWebhook: kycHandler.HandleProviderWebhook,
			deleteProfile:         kycHandler.DeleteProfile,
		},
		issuer:      issuer,
		authRepo:    authRepo,
		rateLimiter: rateLimiter,
		db:          db,
		redis:       rdb,
		rl:          cfg.RateLimit,
	})

	return &App{
		fiberApp:   f,
		cfg:        cfg,
		cancel:     cancel,
		loggerDone: auditLog.Done(),
	}, nil
}

func (a *App) Listen() error {
	return a.fiberApp.Listen(":" + a.cfg.App.Port)
}

func (a *App) Shutdown() error {
	err := a.fiberApp.ShutdownWithTimeout(10 * time.Second)
	a.cancel()
	<-a.loggerDone
	return err
}

func errorHandler(c *fiber.Ctx, err error) error {
	var appErr *apperrors.AppError
	switch e := err.(type) {
	case *apperrors.AppError:
		appErr = e
	default:
		appErr = apperrors.Internal("an unexpected error occurred")
	}
	return c.Status(apperrors.HTTPStatus(appErr.Code)).JSON(appErr)
}
