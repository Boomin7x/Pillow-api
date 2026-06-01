package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type authRepository struct {
	db    *gorm.DB
	redis *redis.Client
}

func NewRepository(db *gorm.DB, rdb *redis.Client) domain.AuthRepository {
	return &authRepository{db: db, redis: rdb}
}

func (r *authRepository) CreateUser(ctx context.Context, u *domain.User) error {
	model := pgmodels.UserModelFrom(u)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		if isDuplicateKey(err) {
			return apperrors.Conflict("email already registered")
		}
		return fmt.Errorf("auth: create user: %w", err)
	}
	u.ID = model.ID
	u.CreatedAt = model.CreatedAt
	u.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *authRepository) FindUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var model pgmodels.UserModel
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("user not found")
		}
		return nil, fmt.Errorf("auth: find user by email: %w", err)
	}
	return model.ToDomain(), nil
}

func (r *authRepository) FindUserByID(ctx context.Context, id string) (*domain.User, error) {
	var model pgmodels.UserModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("user not found")
		}
		return nil, fmt.Errorf("auth: find user by id: %w", err)
	}
	return model.ToDomain(), nil
}

func (r *authRepository) CreateCredential(ctx context.Context, c *domain.Credential) error {
	model := pgmodels.CredentialModelFrom(c)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("auth: create credential: %w", err)
	}
	c.ID = model.ID
	return nil
}

func (r *authRepository) FindCredentialByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	var model pgmodels.CredentialModel
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("credential not found")
		}
		return nil, fmt.Errorf("auth: find credential: %w", err)
	}
	return model.ToDomain(), nil
}

func (r *authRepository) StoreRefreshToken(ctx context.Context, rt *domain.RefreshToken) error {
	model := pgmodels.RefreshTokenModelFrom(rt)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("auth: store refresh token: %w", err)
	}
	rt.ID = model.ID
	return nil
}

func (r *authRepository) FindRefreshTokenByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	var model pgmodels.RefreshTokenModel
	if err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("refresh token not found")
		}
		return nil, fmt.Errorf("auth: find refresh token: %w", err)
	}
	return model.ToDomain(), nil
}

func (r *authRepository) RevokeRefreshToken(ctx context.Context, id string) error {
	now := time.Now()
	if err := r.db.WithContext(ctx).Model(&pgmodels.RefreshTokenModel{}).
		Where("id = ?", id).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("auth: revoke refresh token: %w", err)
	}
	return nil
}

func (r *authRepository) RevokeTokenFamily(ctx context.Context, familyID string) error {
	now := time.Now()
	if err := r.db.WithContext(ctx).Model(&pgmodels.RefreshTokenModel{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("auth: revoke token family: %w", err)
	}
	return nil
}

func (r *authRepository) RevokeAllUserRefreshTokens(ctx context.Context, userID string) error {
	now := time.Now()
	if err := r.db.WithContext(ctx).Model(&pgmodels.RefreshTokenModel{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("auth: revoke all user refresh tokens: %w", err)
	}
	return nil
}

func (r *authRepository) UpsertOAuthIdentity(ctx context.Context, identity *domain.OAuthIdentity) error {
	model := pgmodels.OAuthIdentityModelFrom(identity)
	result := r.db.WithContext(ctx).
		Where("provider = ? AND provider_id = ?", identity.Provider, identity.ProviderID).
		Assign(pgmodels.OAuthIdentityModel{UserID: identity.UserID, AccessToken: identity.AccessToken}).
		FirstOrCreate(model)
	if result.Error != nil {
		return fmt.Errorf("auth: upsert oauth identity: %w", result.Error)
	}
	identity.ID = model.ID
	return nil
}

func (r *authRepository) FindOAuthIdentity(ctx context.Context, provider, providerID string) (*domain.OAuthIdentity, error) {
	var model pgmodels.OAuthIdentityModel
	if err := r.db.WithContext(ctx).
		Where("provider = ? AND provider_id = ?", provider, providerID).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("oauth identity not found")
		}
		return nil, fmt.Errorf("auth: find oauth identity: %w", err)
	}
	return model.ToDomain(), nil
}

func (r *authRepository) CacheRefreshToken(ctx context.Context, hash string, meta domain.RefreshTokenMeta, ttl time.Duration) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("auth: marshal refresh token meta: %w", err)
	}
	if err := r.redis.Set(ctx, refreshKey(hash), b, ttl).Err(); err != nil {
		return fmt.Errorf("auth: cache refresh token: %w", err)
	}
	return nil
}

func (r *authRepository) GetCachedRefreshToken(ctx context.Context, hash string) (*domain.RefreshTokenMeta, error) {
	b, err := r.redis.Get(ctx, refreshKey(hash)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, apperrors.NotFound("refresh token not found in cache")
		}
		return nil, fmt.Errorf("auth: get cached refresh token: %w", err)
	}
	var meta domain.RefreshTokenMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, fmt.Errorf("auth: unmarshal refresh token meta: %w", err)
	}
	return &meta, nil
}

func (r *authRepository) DeleteCachedRefreshToken(ctx context.Context, hash string) error {
	if err := r.redis.Del(ctx, refreshKey(hash)).Err(); err != nil {
		return fmt.Errorf("auth: delete cached refresh token: %w", err)
	}
	return nil
}

func (r *authRepository) DeleteCachedTokenFamily(_ context.Context, _ string) error {
	return nil
}

func (r *authRepository) BlocklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	if err := r.redis.Set(ctx, blocklistKey(jti), 1, ttl).Err(); err != nil {
		return fmt.Errorf("auth: blocklist token: %w", err)
	}
	return nil
}

func (r *authRepository) IsTokenBlocklisted(ctx context.Context, jti string) (bool, error) {
	exists, err := r.redis.Exists(ctx, blocklistKey(jti)).Result()
	if err != nil {
		return false, fmt.Errorf("auth: check blocklist: %w", err)
	}
	return exists > 0, nil
}

func (r *authRepository) LogEvent(ctx context.Context, event *domain.AuthEvent) {
	model := pgmodels.AuthEventModelFrom(event)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		slog.Error("auth: write audit event", "event_type", event.EventType, "error", err)
	}
}

func refreshKey(hash string) string  { return "refresh:" + hash }
func blocklistKey(jti string) string { return "blocklist:" + jti }

func isDuplicateKey(err error) bool {
	return err != nil && (containsStr(err.Error(), "23505") || containsStr(err.Error(), "duplicate key"))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

var _ = sha256.Sum256 // used indirectly via hashToken in service
