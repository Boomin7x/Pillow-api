package kyc

import (
	"context"
	"errors"
	"fmt"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	"gorm.io/gorm"
)

type kycRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *kycRepository {
	return &kycRepository{db: db}
}

func (r *kycRepository) CreateProfile(ctx context.Context, profile *domain.KYCProfile) error {
	model := pgmodels.KYCProfileModelFrom(profile)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		if isDuplicateKey(err) {
			return apperrors.Conflict("kyc profile already exists")
		}
		return fmt.Errorf("kyc: create profile: %w", err)
	}
	profile.CreatedAt = model.CreatedAt
	profile.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *kycRepository) FindProfileByUserID(ctx context.Context, userID string) (*domain.KYCProfile, error) {
	var model pgmodels.KYCProfileModel
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("kyc profile not found")
		}
		return nil, fmt.Errorf("kyc: find profile by user id: %w", err)
	}
	return model.ToDomain(), nil
}

func (r *kycRepository) UpdateProfile(ctx context.Context, profile *domain.KYCProfile) error {
	var existing pgmodels.KYCProfileModel
	if err := r.db.WithContext(ctx).Where("user_id = ?", profile.UserID).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NotFound("kyc profile not found")
		}
		return fmt.Errorf("kyc: find profile for update: %w", err)
	}
	updated := pgmodels.KYCProfileModelFrom(profile)
	updated.ID = existing.ID
	updated.CreatedAt = existing.CreatedAt
	if err := r.db.WithContext(ctx).Save(updated).Error; err != nil {
		return fmt.Errorf("kyc: update profile: %w", err)
	}
	profile.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *kycRepository) CreateCase(ctx context.Context, verificationCase *domain.VerificationCase) error {
	model := pgmodels.VerificationCaseModelFrom(verificationCase)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("kyc: create verification case: %w", err)
	}
	verificationCase.ID = model.ID
	verificationCase.CreatedAt = model.CreatedAt
	verificationCase.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *kycRepository) FindCaseByID(ctx context.Context, id string) (*domain.VerificationCase, error) {
	var model pgmodels.VerificationCaseModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("verification case not found")
		}
		return nil, fmt.Errorf("kyc: find verification case by id: %w", err)
	}
	return model.ToDomain(), nil
}

func (r *kycRepository) UpdateCase(ctx context.Context, verificationCase *domain.VerificationCase) error {
	model := pgmodels.VerificationCaseModelFrom(verificationCase)
	result := r.db.WithContext(ctx).Save(model)
	if result.Error != nil {
		return fmt.Errorf("kyc: update verification case: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.NotFound("verification case not found")
	}
	verificationCase.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *kycRepository) ListPendingCases(ctx context.Context, limit int) ([]domain.VerificationCase, error) {
	var models []pgmodels.VerificationCaseModel
	if err := r.db.WithContext(ctx).
		Where("status IN ?", []string{string(domain.StatusPending), string(domain.StatusInReview)}).
		Order("created_at ASC").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("kyc: list pending cases: %w", err)
	}
	cases := make([]domain.VerificationCase, len(models))
	for i := range models {
		cases[i] = *models[i].ToDomain()
	}
	return cases, nil
}

func (r *kycRepository) CreateCheck(ctx context.Context, check *domain.Check) error {
	model := pgmodels.KYCCheckModelFrom(check)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		if isDuplicateKey(err) {
			return apperrors.Conflict("kyc check already recorded for provider event")
		}
		return fmt.Errorf("kyc: create check: %w", err)
	}
	check.ID = model.ID
	check.CreatedAt = model.CreatedAt
	check.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *kycRepository) FindCheckByProviderEventID(ctx context.Context, providerEventID string) (*domain.Check, error) {
	var model pgmodels.KYCCheckModel
	if err := r.db.WithContext(ctx).
		Where("provider_event_id = ? AND provider_event_id <> ''", providerEventID).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("kyc check not found")
		}
		return nil, fmt.Errorf("kyc: find check by provider event id: %w", err)
	}
	return model.ToDomain(), nil
}

func (r *kycRepository) UpdateCheck(ctx context.Context, check *domain.Check) error {
	model := pgmodels.KYCCheckModelFrom(check)
	result := r.db.WithContext(ctx).Save(model)
	if result.Error != nil {
		return fmt.Errorf("kyc: update check: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.NotFound("kyc check not found")
	}
	check.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *kycRepository) CreateOwnershipClaim(ctx context.Context, claim *domain.OwnershipClaim) error {
	model := pgmodels.OwnershipClaimModelFrom(claim)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("kyc: create ownership claim: %w", err)
	}
	claim.ID = model.ID
	claim.CreatedAt = model.CreatedAt
	claim.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *kycRepository) FindOwnershipClaimByID(ctx context.Context, id string) (*domain.OwnershipClaim, error) {
	var model pgmodels.OwnershipClaimModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("ownership claim not found")
		}
		return nil, fmt.Errorf("kyc: find ownership claim by id: %w", err)
	}
	return model.ToDomain(), nil
}

func (r *kycRepository) UpdateOwnershipClaim(ctx context.Context, claim *domain.OwnershipClaim) error {
	model := pgmodels.OwnershipClaimModelFrom(claim)
	result := r.db.WithContext(ctx).Save(model)
	if result.Error != nil {
		return fmt.Errorf("kyc: update ownership claim: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.NotFound("ownership claim not found")
	}
	claim.UpdatedAt = model.UpdatedAt
	return nil
}

func isDuplicateKey(err error) bool {
	return err != nil && (containsSubstring(err.Error(), "23505") || containsSubstring(err.Error(), "duplicate key"))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
