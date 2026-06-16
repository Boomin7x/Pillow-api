package tokenutil_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/infrastructure/tokenutil"
)

func writePEMFiles(t *testing.T, privKey *rsa.PrivateKey) (privPath, pubPath string) {
	t.Helper()

	dir := t.TempDir()

	privBytes := x509.MarshalPKCS1PrivateKey(privKey)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	privPath = filepath.Join(dir, "private.pem")
	if err := os.WriteFile(privPath, privPEM, 0600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	pubPath = filepath.Join(dir, "public.pem")
	if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	return privPath, pubPath
}

func TestPublicKeySet_RoundTrip(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	privPath, pubPath := writePEMFiles(t, privKey)

	issuer, err := tokenutil.NewJWTIssuer(config.JWTConfig{
		PrivateKeyPath: privPath,
		PublicKeyPath:  pubPath,
		AccessTTL:      15 * time.Minute,
		RefreshTTL:     30 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewJWTIssuer: %v", err)
	}

	keys := issuer.PublicKeySet()
	if len(keys) == 0 {
		t.Fatal("PublicKeySet returned empty slice")
	}

	jwk := keys[0]

	if jwk.KeyType != "RSA" {
		t.Errorf("kty = %q, want %q", jwk.KeyType, "RSA")
	}
	if jwk.Use != "sig" {
		t.Errorf("use = %q, want %q", jwk.Use, "sig")
	}
	if jwk.Algorithm != "RS256" {
		t.Errorf("alg = %q, want %q", jwk.Algorithm, "RS256")
	}
	if jwk.KeyID == "" {
		t.Error("kid must not be empty")
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		t.Fatalf("decode n: %v", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		t.Fatalf("decode e: %v", err)
	}

	recoveredN := new(big.Int).SetBytes(nBytes)
	recoveredE := int(new(big.Int).SetBytes(eBytes).Int64())

	if recoveredN.Cmp(privKey.PublicKey.N) != 0 {
		t.Error("decoded n does not match original public key modulus")
	}
	if recoveredE != privKey.PublicKey.E {
		t.Errorf("decoded e = %d, want %d", recoveredE, privKey.PublicKey.E)
	}
}

func TestPublicKeySet_HasOneKeyPerIssuer(t *testing.T) {
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	privPath, pubPath := writePEMFiles(t, privKey)

	issuer, _ := tokenutil.NewJWTIssuer(config.JWTConfig{
		PrivateKeyPath: privPath,
		PublicKeyPath:  pubPath,
		AccessTTL:      15 * time.Minute,
	})

	keys := issuer.PublicKeySet()
	if len(keys) != 1 {
		t.Errorf("len(keys) = %d, want 1", len(keys))
	}
}
