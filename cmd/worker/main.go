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

	w, err := app.NewWorker(cfg)
	if err != nil {
		slog.Error("worker init failed", "error", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("worker starting", "enabled", cfg.Worker.Enabled)
	w.Run()

	<-quit
	slog.Info("worker shutting down")
	w.Shutdown()
}
