package auth

import (
	"context"
	"log/slog"
	"time"

	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	"gorm.io/gorm"
)

type tokenCleanup struct {
	db *gorm.DB
}

func NewTokenCleanup(db *gorm.DB) *tokenCleanup {
	return &tokenCleanup{db: db}
}

func (c *tokenCleanup) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.deleteExpired(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (c *tokenCleanup) deleteExpired(ctx context.Context) {
	result := c.db.WithContext(ctx).
		Where(
			"expires_at < NOW() OR (revoked_at IS NOT NULL AND revoked_at < NOW() - INTERVAL '90 days')",
		).
		Delete(&pgmodels.RefreshTokenModel{})

	if result.Error != nil {
		slog.Error("token cleanup: delete failed", "error", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		slog.Info("token cleanup: deleted expired tokens", "count", result.RowsAffected)
	}
}
