package kycprovider_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/kycprovider"
)

func validSignature(t *testing.T, secret, payload string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookVerifier(t *testing.T) {
	secret := "test-secret-32-bytes-long-padded!"
	payload := []byte(`{"event_id":"evt-1","verdict":"approved"}`)

	tests := []struct {
		name      string
		secret    string
		payload   []byte
		signature string
		wantErr   error
	}{
		{
			name:      "valid signature passes",
			secret:    secret,
			payload:   payload,
			signature: validSignature(t, secret, string(payload)),
		},
		{
			name:      "tampered payload fails",
			secret:    secret,
			payload:   append(payload, 'X'),
			signature: validSignature(t, secret, string(payload)),
			wantErr:   domain.ErrInvalidWebhookSignature,
		},
		{
			name:      "wrong secret fails",
			secret:    "wrong-secret",
			payload:   payload,
			signature: validSignature(t, secret, string(payload)),
			wantErr:   domain.ErrInvalidWebhookSignature,
		},
		{
			name:      "empty secret fails",
			secret:    "",
			payload:   payload,
			signature: validSignature(t, secret, string(payload)),
			wantErr:   domain.ErrInvalidWebhookSignature,
		},
		{
			name:      "malformed hex signature fails",
			secret:    secret,
			payload:   payload,
			signature: "not-valid-hex!!",
			wantErr:   domain.ErrInvalidWebhookSignature,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := kycprovider.NewWebhookVerifier(tc.secret)
			err := v.Verify(tc.payload, tc.signature)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("got %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
