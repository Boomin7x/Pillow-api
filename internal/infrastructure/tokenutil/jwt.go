package tokenutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
)

type jwtIssuer struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	accessTTL  time.Duration
	keyID      string
}

func NewJWTIssuer(cfg config.JWTConfig) (domain.TokenIssuer, error) {
	privBytes, err := os.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("tokenutil: read private key: %w", err)
	}

	pubBytes, err := os.ReadFile(cfg.PublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("tokenutil: read public key: %w", err)
	}

	privBlock, _ := pem.Decode(privBytes)
	if privBlock == nil {
		return nil, fmt.Errorf("tokenutil: decode private key PEM")
	}

	privKey, err := x509.ParsePKCS8PrivateKey(privBlock.Bytes)
	if err != nil {
		privKey, err = x509.ParsePKCS1PrivateKey(privBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("tokenutil: parse private key: %w", err)
		}
	}

	rsaPriv, ok := privKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("tokenutil: private key is not RSA")
	}

	pubBlock, _ := pem.Decode(pubBytes)
	if pubBlock == nil {
		return nil, fmt.Errorf("tokenutil: decode public key PEM")
	}

	pubKeyAny, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("tokenutil: parse public key: %w", err)
	}

	rsaPub, ok := pubKeyAny.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("tokenutil: public key is not RSA")
	}

	return &jwtIssuer{
		privateKey: rsaPriv,
		publicKey:  rsaPub,
		accessTTL:  cfg.AccessTTL,
		keyID:      "pillow-" + time.Now().Format("2006-q1"),
	}, nil
}

func (j *jwtIssuer) IssueAccessToken(claims domain.Claims) (string, error) {
	now := time.Now()

	jwtClaims := jwt.MapClaims{
		"sub":   claims.UserID,
		"email": claims.Email,
		"roles": claims.Roles,
		"sid":   claims.SessionID,
		"jti":   claims.TokenID,
		"iat":   now.Unix(),
		"exp":   now.Add(j.accessTTL).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtClaims)
	token.Header["kid"] = j.keyID

	signed, err := token.SignedString(j.privateKey)
	if err != nil {
		return "", fmt.Errorf("tokenutil: sign access token: %w", err)
	}

	return signed, nil
}

func (j *jwtIssuer) IssueRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("tokenutil: generate refresh token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (j *jwtIssuer) ValidateAccessToken(raw string) (*domain.Claims, error) {
	token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return j.publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("tokenutil: validate token: %w", err)
	}

	mc, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("tokenutil: invalid token claims")
	}

	roles, _ := mc["roles"].([]any)
	roleStrs := make([]string, 0, len(roles))
	for _, r := range roles {
		if s, ok := r.(string); ok {
			roleStrs = append(roleStrs, s)
		}
	}

	return &domain.Claims{
		UserID:    stringClaim(mc, "sub"),
		Email:     stringClaim(mc, "email"),
		Roles:     roleStrs,
		TokenID:   stringClaim(mc, "jti"),
		SessionID: stringClaim(mc, "sid"),
	}, nil
}

func stringClaim(mc jwt.MapClaims, key string) string {
	v, _ := mc[key].(string)
	return v
}

func NewTokenID() string { return uuid.New().String() }
