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

type AuthResult struct {
	AccessToken  string
	RefreshToken string
	User         *User
}

type AuthService interface {
	Register(ctx context.Context, input RegisterInput) (*AuthResult, error)
	Login(ctx context.Context, input LoginInput) (*AuthResult, error)
	Refresh(ctx context.Context, input RefreshInput) (*AuthResult, error)
	Logout(ctx context.Context, input LogoutInput) error
}

type AuthRepository interface {
	CreateUser(ctx context.Context, u *User) error
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	FindUserByID(ctx context.Context, id string) (*User, error)

	CreateCredential(ctx context.Context, c *Credential) error
	FindCredentialByUserID(ctx context.Context, userID string) (*Credential, error)

	StoreRefreshToken(ctx context.Context, rt *RefreshToken) error
	FindRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id string) error
	RevokeTokenFamily(ctx context.Context, familyID string) error
	RevokeAllUserRefreshTokens(ctx context.Context, userID string) error

	UpsertOAuthIdentity(ctx context.Context, identity *OAuthIdentity) error
	FindOAuthIdentity(ctx context.Context, provider, providerID string) (*OAuthIdentity, error)

	CacheRefreshToken(ctx context.Context, hash string, meta RefreshTokenMeta, ttl time.Duration) error
	GetCachedRefreshToken(ctx context.Context, hash string) (*RefreshTokenMeta, error)
	DeleteCachedRefreshToken(ctx context.Context, hash string) error

	BlocklistToken(ctx context.Context, jti string, ttl time.Duration) error
	IsTokenBlocklisted(ctx context.Context, jti string) (bool, error)

	LogEvent(ctx context.Context, event *AuthEvent)
}

type TokenIssuer interface {
	IssueAccessToken(claims Claims) (string, error)
	IssueRefreshToken() (string, error)
	ValidateAccessToken(token string) (*Claims, error)
}
