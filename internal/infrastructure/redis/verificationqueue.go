package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/redis/go-redis/v9"
)

const verificationQueueKey = "kyc:verification:queue"

type verificationQueue struct {
	client *redis.Client
}

func NewVerificationQueue(client *redis.Client) domain.VerificationQueue {
	return &verificationQueue{client: client}
}

func (q *verificationQueue) Enqueue(ctx context.Context, job domain.VerificationJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("verification queue: enqueue: marshal: %w", err)
	}
	if err := q.client.RPush(ctx, verificationQueueKey, data).Err(); err != nil {
		return fmt.Errorf("verification queue: enqueue: %w", err)
	}
	return nil
}

func (q *verificationQueue) Len(ctx context.Context) (int, error) {
	n, err := q.client.LLen(ctx, verificationQueueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("verification queue: len: %w", err)
	}
	return int(n), nil
}

func (q *verificationQueue) Dequeue(ctx context.Context, timeout time.Duration) (*domain.VerificationJob, error) {
	result, err := q.client.BLPop(ctx, timeout, verificationQueueKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("verification queue: dequeue: %w", err)
	}
	if len(result) < 2 {
		return nil, fmt.Errorf("verification queue: dequeue: unexpected result length %d", len(result))
	}
	var job domain.VerificationJob
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("verification queue: dequeue: unmarshal: %w", err)
	}
	return &job, nil
}
