package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var slidingWindowScript = redis.NewScript(`
local key      = KEYS[1]
local now_ms   = tonumber(ARGV[1])
local win_ms   = tonumber(ARGV[2])
local limit    = tonumber(ARGV[3])
local member   = ARGV[4]
redis.call('ZREMRANGEBYSCORE', key, 0, now_ms - win_ms)
local count = redis.call('ZCARD', key)
if count < limit then
  redis.call('ZADD', key, now_ms, member)
  redis.call('PEXPIRE', key, win_ms)
  return 1
end
return 0
`)

type RateLimiter struct {
	client *redis.Client
}

func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

func (r *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	nowMS := time.Now().UnixMilli()
	winMS := window.Milliseconds()
	member := uniqueMember()

	result, err := slidingWindowScript.Run(ctx, r.client,
		[]string{key},
		nowMS, winMS, limit, member,
	).Int()
	if err != nil {
		return false, fmt.Errorf("ratelimiter: allow: %w", err)
	}
	return result == 1, nil
}

func (r *RateLimiter) AllowEmail(ctx context.Context, emailHash string, limit int, baseWindow time.Duration) (bool, time.Duration, error) {
	backoffKey := "ratelimit:backoff:email:" + emailHash
	counterKey := "ratelimit:email:" + emailHash + ":login"

	multiplier := r.backoffMultiplier(ctx, backoffKey)
	effectiveWindow := baseWindow * time.Duration(multiplier)

	allowed, err := r.Allow(ctx, counterKey, limit, effectiveWindow)
	if err != nil {
		return false, 0, err
	}

	if !allowed {
		next := min(multiplier*2, 64)
		nextWindow := baseWindow * time.Duration(next)
		_ = r.client.Set(ctx, backoffKey, next, nextWindow).Err()
		return false, effectiveWindow, nil
	}

	return true, 0, nil
}

func (r *RateLimiter) backoffMultiplier(ctx context.Context, key string) int {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return 1
	}
	m, err := strconv.Atoi(val)
	if err != nil || m < 1 {
		return 1
	}
	return m
}

func uniqueMember() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return strconv.FormatInt(time.Now().UnixNano(), 10) + hex.EncodeToString(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
