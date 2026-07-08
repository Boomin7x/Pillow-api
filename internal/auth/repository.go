package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type authRepository struct {
	db    *gorm.DB
	redis *redis.Client
}

func NewRepository(db *gorm.DB, rdb *redis.Client) *authRepository {
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

func (r *authRepository) AssignRole(ctx context.Context, userID, role string) error {
	model := &pgmodels.UserRoleModel{UserID: userID, Role: role}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "role"}},
			DoNothing: true,
		}).
		Create(model).Error; err != nil {
		return fmt.Errorf("auth: assign role: %w", err)
	}
	return nil
}

func (r *authRepository) RemoveRole(ctx context.Context, userID, role string) error {
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND role = ?", userID, role).
		Delete(&pgmodels.UserRoleModel{}).Error; err != nil {
		return fmt.Errorf("auth: remove role: %w", err)
	}
	return nil
}

func (r *authRepository) ListRoles(ctx context.Context, userID string) ([]string, error) {
	var roles []string
	if err := r.db.WithContext(ctx).
		Model(&pgmodels.UserRoleModel{}).
		Where("user_id = ?", userID).
		Order("created_at ASC").
		Pluck("role", &roles).Error; err != nil {
		return nil, fmt.Errorf("auth: list roles: %w", err)
	}
	return roles, nil
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
		return fmt.Errorf("auth: revoke refresh token by id: %w", err)
	}
	return nil
}

func (r *authRepository) RevokeRefreshTokenByHash(ctx context.Context, hash string) error {
	now := time.Now()
	if err := r.db.WithContext(ctx).Model(&pgmodels.RefreshTokenModel{}).
		Where("token_hash = ?", hash).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("auth: revoke refresh token by hash: %w", err)
	}
	return nil
}

func (r *authRepository) RevokeTokenFamily(ctx context.Context, familyID string) error {
	var hashes []string
	if err := r.db.WithContext(ctx).Model(&pgmodels.RefreshTokenModel{}).
		Where("family_id = ?", familyID).
		Pluck("token_hash", &hashes).Error; err != nil {
		return fmt.Errorf("auth: load token family hashes: %w", err)
	}

	now := time.Now()
	if err := r.db.WithContext(ctx).Model(&pgmodels.RefreshTokenModel{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("auth: revoke token family: %w", err)
	}

	if err := r.evictCachedRefreshTokens(ctx, hashes); err != nil {
		return err
	}
	return nil
}

func (r *authRepository) RevokeAllUserRefreshTokens(ctx context.Context, userID string) (int64, error) {
	var hashes []string
	if err := r.db.WithContext(ctx).Model(&pgmodels.RefreshTokenModel{}).
		Where("user_id = ?", userID).
		Pluck("token_hash", &hashes).Error; err != nil {
		return 0, fmt.Errorf("auth: load user token hashes: %w", err)
	}

	now := time.Now()
	result := r.db.WithContext(ctx).Model(&pgmodels.RefreshTokenModel{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now)
	if result.Error != nil {
		return 0, fmt.Errorf("auth: revoke all user refresh tokens: %w", result.Error)
	}

	if err := r.evictCachedRefreshTokens(ctx, hashes); err != nil {
		return 0, err
	}
	return result.RowsAffected, nil
}

func (r *authRepository) evictCachedRefreshTokens(ctx context.Context, hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	keys := make([]string, len(hashes))
	for i, h := range hashes {
		keys[i] = refreshKey(h)
	}
	if err := r.redis.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("auth: evict cached refresh tokens: %w", err)
	}
	return nil
}

func (r *authRepository) UpdateCredentialPassword(ctx context.Context, userID, passwordHash string) error {
	if err := r.db.WithContext(ctx).Model(&pgmodels.CredentialModel{}).
		Where("user_id = ?", userID).
		Update("password_hash", passwordHash).Error; err != nil {
		return fmt.Errorf("auth: update credential password: %w", err)
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

func (r *authRepository) FindOAuthIdentityByUserAndProvider(ctx context.Context, userID, provider string) (*domain.OAuthIdentity, error) {
	var model pgmodels.OAuthIdentityModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND provider = ?", userID, provider).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("oauth identity not found")
		}
		return nil, fmt.Errorf("auth: find oauth identity by user and provider: %w", err)
	}
	return model.ToDomain(), nil
}

func (r *authRepository) SaveVerifier(ctx context.Context, state, verifier string, ttl time.Duration) error {
	if err := r.redis.Set(ctx, pkceKey(state), verifier, ttl).Err(); err != nil {
		return fmt.Errorf("auth: save pkce verifier: %w", err)
	}
	return nil
}

func (r *authRepository) GetVerifier(ctx context.Context, state string) (string, error) {
	verifier, err := r.redis.Get(ctx, pkceKey(state)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", apperrors.NotFound("oauth state not found or expired")
		}
		return "", fmt.Errorf("auth: get pkce verifier: %w", err)
	}
	return verifier, nil
}

func (r *authRepository) DeleteVerifier(ctx context.Context, state string) error {
	if err := r.redis.Del(ctx, pkceKey(state)).Err(); err != nil {
		return fmt.Errorf("auth: delete pkce verifier: %w", err)
	}
	return nil
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

func refreshKey(hash string) string  { return "refresh:" + hash }
func blocklistKey(jti string) string { return "blocklist:" + jti }
func pkceKey(state string) string    { return "pkce:" + state }

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
