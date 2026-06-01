package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/kodiahbertrand/pillow/internal/app"
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

	a, err := app.New(cfg)
	if err != nil {
		slog.Error("app init failed", "error", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", cfg.App.Port)
		if err := a.Listen(); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down")
	if err := a.Shutdown(); err != nil {
		slog.Error("shutdown error", "error", err)
		os.Exit(1)
	}
}
