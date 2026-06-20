package kyccache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/redis/go-redis/v9"
)

type TierCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewTierCache(client *redis.Client, ttl time.Duration) *TierCache {
	return &TierCache{client: client, ttl: ttl}
}

func (c *TierCache) key(userID string) string {
	return "kyc:tier:" + userID
}

func (c *TierCache) Get(ctx context.Context, userID string) (*domain.KYCProfile, error) {
	data, err := c.client.Get(ctx, c.key(userID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("kyccache: get: %w", err)
	}
	var profile domain.KYCProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("kyccache: get: unmarshal: %w", err)
	}
	return &profile, nil
}

func (c *TierCache) Set(ctx context.Context, userID string, profile *domain.KYCProfile, ttl time.Duration) error {
	data, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("kyccache: set: marshal: %w", err)
	}
	if ttl <= 0 {
		ttl = c.ttl
	}
	if err := c.client.Set(ctx, c.key(userID), data, ttl).Err(); err != nil {
		return fmt.Errorf("kyccache: set: %w", err)
	}
	return nil
}

func (c *TierCache) Del(ctx context.Context, userID string) error {
	if err := c.client.Del(ctx, c.key(userID)).Err(); err != nil {
		return fmt.Errorf("kyccache: del: %w", err)
	}
	return nil
}
