package app

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/auth"
	"github.com/kodiahbertrand/pillow/internal/config"
	infrapostgres "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	infraredis "github.com/kodiahbertrand/pillow/internal/infrastructure/redis"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/tokenutil"
)

type App struct {
	fiberApp *fiber.App
	cfg      *config.Config
}

func New(cfg *config.Config) (*App, error) {
	// --- Infrastructure ---
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

	// --- Auth domain ---
	authRepo := auth.NewRepository(db, rdb)
	authSvc := auth.NewService(authRepo, issuer)
	authHandler := auth.NewHandler(authSvc, issuer)

	// --- Fiber ---
	f := fiber.New(fiber.Config{
		ErrorHandler: errorHandler,
		AppName:      cfg.App.Name,
	})

	f.Use(recover.New())
	f.Use(logger.New())
	f.Use(cors.New())

	registerDocsRoutes(f)

	registerRoutes(f, routeDeps{
		auth: authRoutes{
			register: authHandler.Register,
			login:    authHandler.Login,
			refresh:  authHandler.Refresh,
			logout:   authHandler.Logout,
		},
		issuer:   issuer,
		authRepo: authRepo,
	})

	return &App{fiberApp: f, cfg: cfg}, nil
}

func (a *App) Listen() error {
	return a.fiberApp.Listen(":" + a.cfg.App.Port)
}

func (a *App) Shutdown() error {
	return a.fiberApp.Shutdown()
}

// errorHandler maps *apperrors.AppError to HTTP responses.
// All other errors become 500.
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
