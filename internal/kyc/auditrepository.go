package kyc

import (
	"context"
	"fmt"

	"github.com/kodiahbertrand/pillow/internal/domain"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	"gorm.io/gorm"
)

type kycAuditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) *kycAuditRepository {
	return &kycAuditRepository{db: db}
}

func (r *kycAuditRepository) Append(ctx context.Context, event *domain.AuditEvent) error {
	model := pgmodels.KYCAuditEventModelFrom(event)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("kyc: append audit event: %w", err)
	}
	event.ID = model.ID
	event.CreatedAt = model.CreatedAt
	return nil
}

func (r *kycAuditRepository) ListByUserID(ctx context.Context, userID string) ([]domain.AuditEvent, error) {
	var models []pgmodels.KYCAuditEventModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("kyc: list audit events by user id: %w", err)
	}
	events := make([]domain.AuditEvent, len(models))
	for i := range models {
		events[i] = *models[i].ToDomain()
	}
	return events, nil
}
