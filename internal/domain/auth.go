package domain

import (
	"context"
	"time"
)

type Credential struct {
	ID           string
	UserID       string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type OAuthIdentity struct {
	ID          string
	UserID      string
	Provider    string
	ProviderID  string
	AccessToken string
	CreatedAt   time.Time
}

type RefreshToken struct {
	ID                string
	UserID            string
	TokenHash         string
	FamilyID          string
	DeviceFingerprint string
	ExpiresAt         time.Time
	RevokedAt         *time.Time
	CreatedAt         time.Time
}

type AuthEvent struct {
	ID        string
	EventType string
	UserID    string
	IP        string
	UserAgent string
	Metadata  map[string]any
	CreatedAt time.Time
}

type Claims struct {
	UserID    string
	Email     string
	Roles     []string
	TokenID   string
	SessionID string
}

type RefreshTokenMeta struct {
	UserID            string
	FamilyID          string
	DeviceFingerprint string
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

type LoginInput struct {
	Email     string
	Password  string
	IPAddress string
	UserAgent string
}

type RefreshInput struct {
	RawToken  string
	IPAddress string
	UserAgent string
}

type LogoutInput struct {
	AccessTokenJTI  string
	AccessTokenTTL  time.Duration
	RefreshTokenRaw string
	UserID          string
}

type ChangePasswordInput struct {
	UserID      string
	OldPassword string
	NewPassword string
}

type AuthResult struct {
	AccessToken  string
	RefreshToken string
	User         *User
}

type LogoutAllInput struct {
	UserID         string
	ActiveTokenJTI string
	ActiveTokenTTL time.Duration
}

type AuthService interface {
	Register(ctx context.Context, input RegisterInput) (*AuthResult, error)
	Login(ctx context.Context, input LoginInput) (*AuthResult, error)
	Refresh(ctx context.Context, input RefreshInput) (*AuthResult, error)
	Logout(ctx context.Context, input LogoutInput) error
	LogoutAll(ctx context.Context, input LogoutAllInput) error
	ChangePassword(ctx context.Context, input ChangePasswordInput) error
	OAuthLogin(ctx context.Context, input OAuthLoginInput) (*AuthResult, error)
}

type AuthRepository interface {
	CreateUser(ctx context.Context, u *User) error
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	FindUserByID(ctx context.Context, id string) (*User, error)

	CreateCredential(ctx context.Context, c *Credential) error
	FindCredentialByUserID(ctx context.Context, userID string) (*Credential, error)
	UpdateCredentialPassword(ctx context.Context, userID, passwordHash string) error

	StoreRefreshToken(ctx context.Context, rt *RefreshToken) error
	FindRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id string) error
	RevokeRefreshTokenByHash(ctx context.Context, hash string) error
	RevokeTokenFamily(ctx context.Context, familyID string) error
	RevokeAllUserRefreshTokens(ctx context.Context, userID string) (int64, error)

	UpsertOAuthIdentity(ctx context.Context, identity *OAuthIdentity) error
	FindOAuthIdentity(ctx context.Context, provider, providerID string) (*OAuthIdentity, error)
	FindOAuthIdentityByUserAndProvider(ctx context.Context, userID, provider string) (*OAuthIdentity, error)

	CacheRefreshToken(ctx context.Context, hash string, meta RefreshTokenMeta, ttl time.Duration) error
	GetCachedRefreshToken(ctx context.Context, hash string) (*RefreshTokenMeta, error)
	DeleteCachedRefreshToken(ctx context.Context, hash string) error

	BlocklistToken(ctx context.Context, jti string, ttl time.Duration) error
	IsTokenBlocklisted(ctx context.Context, jti string) (bool, error)
}

type PKCEStore interface {
	SaveVerifier(ctx context.Context, state string, verifier string, ttl time.Duration) error
	GetVerifier(ctx context.Context, state string) (string, error)
	DeleteVerifier(ctx context.Context, state string) error
}

type OAuthIdentityClaims struct {
	ProviderID string
	Email      string
}

type OAuthProvider interface {
	BuildAuthURL(state, codeChallenge string) string
	ExchangeAndVerify(ctx context.Context, code, codeVerifier string) (*OAuthIdentityClaims, error)
}

type OAuthLoginInput struct {
	Provider   string
	ProviderID string
	Email      string
	IPAddress  string
	UserAgent  string
}

type JWK struct {
	KeyType   string `json:"kty"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	N         string `json:"n"`
	E         string `json:"e"`
}

type TokenIssuer interface {
	IssueAccessToken(claims Claims) (string, error)
	IssueRefreshToken() (string, error)
	ValidateAccessToken(token string) (*Claims, error)
	PublicKeySet() []JWK
}

type AuditLogger interface {
	Log(ctx context.Context, event *AuthEvent)
}

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

type EmailRateLimiter interface {
	AllowEmail(ctx context.Context, emailHash string, limit int, baseWindow time.Duration) (allowed bool, retryAfter time.Duration, err error)
}
