package resilience_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/pkg/resilience"
)

func newRequest(t *testing.T, ctx context.Context, url string) *http.Request {
	t.Helper()
	body := "ping"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(body)), nil
	}
	return req
}

func TestClientDo_SucceedsOnFirstAttempt(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := resilience.NewClient(resilience.Config{BaseBackoff: time.Millisecond})
	resp, err := client.Do(context.Background(), newRequest(t, context.Background(), server.URL))
	if err != nil {
		t.Fatalf("got err %v, want nil", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls: got %d, want 1", got)
	}
}

func TestClientDo_RetriesServerErrorsThenSucceeds(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "ping" {
			t.Errorf("retried body: got %q, want ping", string(body))
		}
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := resilience.NewClient(resilience.Config{MaxRetries: 3, BaseBackoff: time.Millisecond})
	resp, err := client.Do(context.Background(), newRequest(t, context.Background(), server.URL))
	if err != nil {
		t.Fatalf("got err %v, want nil", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls: got %d, want 3", got)
	}
}

func TestClientDo_OpensCircuitAfterThreshold(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := resilience.NewClient(resilience.Config{
		MaxRetries:       0,
		BaseBackoff:      time.Millisecond,
		BreakerThreshold: 1,
		BreakerCooldown:  time.Hour,
	})

	if _, err := client.Do(context.Background(), newRequest(t, context.Background(), server.URL)); err == nil {
		t.Fatal("first call: got nil err, want upstream failure")
	}

	_, err := client.Do(context.Background(), newRequest(t, context.Background(), server.URL))
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Errorf("second call: got %v, want ErrCircuitOpen", err)
	}
}
