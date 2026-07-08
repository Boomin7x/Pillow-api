package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/kodiahbertrand/pillow/internal/auth"
	"github.com/kodiahbertrand/pillow/internal/config"
	infrapostgres "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if len(os.Args) != 3 || (os.Args[1] != "grant" && os.Args[1] != "revoke") {
		slog.Error("usage: adminrole grant|revoke <email>")
		os.Exit(2)
	}
	action, email := os.Args[1], os.Args[2]

	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	db, err := infrapostgres.NewDB(cfg.Database)
	if err != nil {
		slog.Error("postgres init failed", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	repo := auth.NewRepository(db, nil)

	user, err := repo.FindUserByEmail(ctx, email)
	if err != nil {
		slog.Error("find user failed", "error", err, "email", email)
		os.Exit(1)
	}

	if action == "grant" {
		err = repo.AssignRole(ctx, user.ID, "admin")
	} else {
		err = repo.RemoveRole(ctx, user.ID, "admin")
	}
	if err != nil {
		slog.Error("role update failed", "error", err, "action", action)
		os.Exit(1)
	}

	slog.Info("admin role updated", "action", action, "user_id", user.ID, "email", email)
}
