package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	"github.com/kodiahbertrand/pillow/internal/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	direction := "up"
	if len(os.Args) > 1 {
		direction = os.Args[1]
	}

	m, err := migrate.New("file://migrations", cfg.Database.URL)
	if err != nil {
		slog.Error("migrate init failed", "error", err)
		os.Exit(1)
	}
	defer m.Close()

	switch direction {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			slog.Error("migrate up failed", "error", err)
			os.Exit(1)
		}
		slog.Info("migrate up: done")
	case "down":
		if err := m.Steps(-1); err != nil {
			slog.Error("migrate down failed", "error", err)
			os.Exit(1)
		}
		slog.Info("migrate down: one step rolled back")
	default:
		slog.Error("unknown direction", "direction", direction)
		os.Exit(1)
	}
}
