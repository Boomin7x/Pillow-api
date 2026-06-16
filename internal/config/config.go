package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	App       AppConfig
	Database  DatabaseConfig
	Redis     RedisConfig
	JWT       JWTConfig
	OAuth     OAuthConfig
	RateLimit RateLimitConfig
	KYC       KYCConfig
}

type AppConfig struct {
	Env  string
	Port string
	Name string
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

type JWTConfig struct {
	PrivateKeyPath string
	PublicKeyPath  string
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
}

type OAuthConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
}

type KYCConfig struct {
	IdentityProvider ExternalProviderConfig
	Ownership        ExternalProviderConfig
	License          ExternalProviderConfig
	Business         ExternalProviderConfig
	Vault            DocumentVaultConfig
}

type ExternalProviderConfig struct {
	BaseURL    string
	APIKey     string
	Timeout    time.Duration
	MaxRetries int
}

type DocumentVaultConfig struct {
	BasePath         string
	EncryptionKeyHex string
	SigningSecret    string
	PublicBaseURL    string
	RetentionTTL     time.Duration
}

type RateLimitConfig struct {
	LoginIPLimit         int
	LoginIPWindow        time.Duration
	LoginEmailLimit      int
	LoginEmailBaseWindow time.Duration
	RegisterIPLimit      int
	RegisterIPWindow     time.Duration
	RefreshIPLimit       int
	RefreshIPWindow      time.Duration
	OAuthIPLimit         int
	OAuthIPWindow        time.Duration
}

func Load() (*Config, error) {
	var missing []string

	require := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	cfg := &Config{
		App: AppConfig{
			Env:  getEnv("APP_ENV", "development"),
			Port: getEnv("APP_PORT", "8080"),
			Name: getEnv("APP_NAME", "pillow"),
		},
		Database: DatabaseConfig{
			URL:             require("DATABASE_URL"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: time.Duration(getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 5)) * time.Minute,
		},
		Redis: RedisConfig{
			URL:      require("REDIS_URL"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			PrivateKeyPath: require("JWT_PRIVATE_KEY_PATH"),
			PublicKeyPath:  require("JWT_PUBLIC_KEY_PATH"),
			AccessTTL:      time.Duration(getEnvInt("JWT_ACCESS_TTL_MINUTES", 15)) * time.Minute,
			RefreshTTL:     time.Duration(getEnvInt("JWT_REFRESH_TTL_DAYS", 30)) * 24 * time.Hour,
		},
		OAuth: OAuthConfig{
			GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
			GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
			GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", ""),
		},
		KYC: KYCConfig{
			IdentityProvider: ExternalProviderConfig{
				BaseURL:    getEnv("KYC_IDENTITY_PROVIDER_URL", ""),
				APIKey:     getEnv("KYC_IDENTITY_PROVIDER_API_KEY", ""),
				Timeout:    time.Duration(getEnvInt("KYC_IDENTITY_PROVIDER_TIMEOUT_SECONDS", 10)) * time.Second,
				MaxRetries: getEnvInt("KYC_IDENTITY_PROVIDER_MAX_RETRIES", 2),
			},
			Ownership: ExternalProviderConfig{
				BaseURL:    getEnv("KYC_OWNERSHIP_PROVIDER_URL", ""),
				APIKey:     getEnv("KYC_OWNERSHIP_PROVIDER_API_KEY", ""),
				Timeout:    time.Duration(getEnvInt("KYC_OWNERSHIP_PROVIDER_TIMEOUT_SECONDS", 10)) * time.Second,
				MaxRetries: getEnvInt("KYC_OWNERSHIP_PROVIDER_MAX_RETRIES", 2),
			},
			License: ExternalProviderConfig{
				BaseURL:    getEnv("KYC_LICENSE_PROVIDER_URL", ""),
				APIKey:     getEnv("KYC_LICENSE_PROVIDER_API_KEY", ""),
				Timeout:    time.Duration(getEnvInt("KYC_LICENSE_PROVIDER_TIMEOUT_SECONDS", 10)) * time.Second,
				MaxRetries: getEnvInt("KYC_LICENSE_PROVIDER_MAX_RETRIES", 2),
			},
			Business: ExternalProviderConfig{
				BaseURL:    getEnv("KYC_BUSINESS_PROVIDER_URL", ""),
				APIKey:     getEnv("KYC_BUSINESS_PROVIDER_API_KEY", ""),
				Timeout:    time.Duration(getEnvInt("KYC_BUSINESS_PROVIDER_TIMEOUT_SECONDS", 10)) * time.Second,
				MaxRetries: getEnvInt("KYC_BUSINESS_PROVIDER_MAX_RETRIES", 2),
			},
			Vault: DocumentVaultConfig{
				BasePath:         getEnv("KYC_VAULT_BASE_PATH", "./kyc-documents"),
				EncryptionKeyHex: getEnv("KYC_VAULT_ENCRYPTION_KEY_HEX", ""),
				SigningSecret:    getEnv("KYC_VAULT_SIGNING_SECRET", ""),
				PublicBaseURL:    getEnv("KYC_VAULT_PUBLIC_BASE_URL", "http://localhost:8080/kyc/documents"),
				RetentionTTL:     time.Duration(getEnvInt("KYC_VAULT_RETENTION_DAYS", 90)) * 24 * time.Hour,
			},
		},
		RateLimit: RateLimitConfig{
			LoginIPLimit:         getEnvInt("RATELIMIT_LOGIN_IP_LIMIT", 10),
			LoginIPWindow:        time.Duration(getEnvInt("RATELIMIT_LOGIN_IP_WINDOW_SECONDS", 60)) * time.Second,
			LoginEmailLimit:      getEnvInt("RATELIMIT_LOGIN_EMAIL_LIMIT", 5),
			LoginEmailBaseWindow: time.Duration(getEnvInt("RATELIMIT_LOGIN_EMAIL_WINDOW_SECONDS", 900)) * time.Second,
			RegisterIPLimit:      getEnvInt("RATELIMIT_REGISTER_IP_LIMIT", 3),
			RegisterIPWindow:     time.Duration(getEnvInt("RATELIMIT_REGISTER_IP_WINDOW_SECONDS", 60)) * time.Second,
			RefreshIPLimit:       getEnvInt("RATELIMIT_REFRESH_IP_LIMIT", 30),
			RefreshIPWindow:      time.Duration(getEnvInt("RATELIMIT_REFRESH_IP_WINDOW_SECONDS", 60)) * time.Second,
			OAuthIPLimit:         getEnvInt("RATELIMIT_OAUTH_IP_LIMIT", 20),
			OAuthIPWindow:        time.Duration(getEnvInt("RATELIMIT_OAUTH_IP_WINDOW_SECONDS", 60)) * time.Second,
		},
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
