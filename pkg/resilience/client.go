package resilience

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("resilience: circuit breaker open")

type Config struct {
	Timeout          time.Duration
	MaxRetries       int
	BaseBackoff      time.Duration
	BreakerThreshold int
	BreakerCooldown  time.Duration
}

type Client struct {
	httpClient  *http.Client
	maxRetries  int
	baseBackoff time.Duration
	breaker     *breaker
}

func NewClient(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 100 * time.Millisecond
	}
	if cfg.BreakerThreshold <= 0 {
		cfg.BreakerThreshold = 5
	}
	if cfg.BreakerCooldown <= 0 {
		cfg.BreakerCooldown = 30 * time.Second
	}
	return &Client{
		httpClient:  &http.Client{Timeout: cfg.Timeout},
		maxRetries:  cfg.MaxRetries,
		baseBackoff: cfg.BaseBackoff,
		breaker: &breaker{
			threshold: cfg.BreakerThreshold,
			cooldown:  cfg.BreakerCooldown,
		},
	}
}

func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if !c.breaker.allow() {
		return nil, ErrCircuitOpen
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoff(attempt)):
			}
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("resilience: rewind request body: %w", err)
				}
				req.Body = body
			}
		}

		resp, err := c.httpClient.Do(req.WithContext(ctx))
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= http.StatusInternalServerError {
			lastErr = fmt.Errorf("resilience: upstream returned status %d", resp.StatusCode)
			_ = resp.Body.Close()
			continue
		}

		c.breaker.recordSuccess()
		return resp, nil
	}

	c.breaker.recordFailure()
	return nil, lastErr
}

func (c *Client) backoff(attempt int) time.Duration {
	exponent := time.Duration(1<<uint(attempt-1)) * c.baseBackoff
	jitter := time.Duration(rand.Int63n(int64(c.baseBackoff) + 1))
	return exponent + jitter
}

type breaker struct {
	mu        sync.Mutex
	failures  int
	threshold int
	cooldown  time.Duration
	openedAt  time.Time
	isOpen    bool
}

func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.isOpen {
		if time.Since(b.openedAt) >= b.cooldown {
			b.isOpen = false
			b.failures = 0
			return true
		}
		return false
	}
	return true
}

func (b *breaker) recordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.isOpen = false
}

func (b *breaker) recordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= b.threshold {
		b.isOpen = true
		b.openedAt = time.Now()
	}
}
