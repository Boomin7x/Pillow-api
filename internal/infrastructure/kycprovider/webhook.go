package kycprovider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/kodiahbertrand/pillow/internal/domain"
)

type webhookVerifier struct {
	secret []byte
}

func NewWebhookVerifier(secret string) *webhookVerifier {
	return &webhookVerifier{secret: []byte(secret)}
}

func (v *webhookVerifier) Verify(payload []byte, signature string) error {
	if len(v.secret) == 0 {
		return domain.ErrInvalidWebhookSignature
	}
	sig, err := hex.DecodeString(signature)
	if err != nil {
		return domain.ErrInvalidWebhookSignature
	}
	mac := hmac.New(sha256.New, v.secret)
	mac.Write(payload)
	expected := mac.Sum(nil)
	if !hmac.Equal(sig, expected) {
		return domain.ErrInvalidWebhookSignature
	}
	return nil
}
