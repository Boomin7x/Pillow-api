package postgres

import (
	"time"

	"github.com/kodiahbertrand/pillow/internal/domain"
)

type UserModel struct {
	ID          string    `gorm:"primarykey;type:uuid;default:gen_random_uuid()"`
	Email       string    `gorm:"uniqueIndex;not null"`
	DisplayName string    `gorm:"not null;default:''"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"not null;autoUpdateTime"`
}

func (m *UserModel) TableName() string { return "users" }

func (m *UserModel) ToDomain() *domain.User {
	return &domain.User{
		ID:          m.ID,
		Email:       m.Email,
		DisplayName: m.DisplayName,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func UserModelFrom(u *domain.User) *UserModel {
	return &UserModel{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}

type CredentialModel struct {
	ID           string    `gorm:"primarykey;type:uuid;default:gen_random_uuid()"`
	UserID       string    `gorm:"type:uuid;not null;uniqueIndex"`
	PasswordHash string    `gorm:"not null"`
	CreatedAt    time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"not null;autoUpdateTime"`
}

func (m *CredentialModel) TableName() string { return "credentials" }

func (m *CredentialModel) ToDomain() *domain.Credential {
	return &domain.Credential{
		ID:           m.ID,
		UserID:       m.UserID,
		PasswordHash: m.PasswordHash,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func CredentialModelFrom(c *domain.Credential) *CredentialModel {
	return &CredentialModel{
		ID:           c.ID,
		UserID:       c.UserID,
		PasswordHash: c.PasswordHash,
	}
}

type OAuthIdentityModel struct {
	ID          string    `gorm:"primarykey;type:uuid;default:gen_random_uuid()"`
	UserID      string    `gorm:"type:uuid;not null;index"`
	Provider    string    `gorm:"not null"`
	ProviderID  string    `gorm:"not null"`
	AccessToken string    `gorm:""`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime"`
}

func (m *OAuthIdentityModel) TableName() string { return "oauth_identities" }

func (m *OAuthIdentityModel) ToDomain() *domain.OAuthIdentity {
	return &domain.OAuthIdentity{
		ID:          m.ID,
		UserID:      m.UserID,
		Provider:    m.Provider,
		ProviderID:  m.ProviderID,
		AccessToken: m.AccessToken,
		CreatedAt:   m.CreatedAt,
	}
}

func OAuthIdentityModelFrom(o *domain.OAuthIdentity) *OAuthIdentityModel {
	return &OAuthIdentityModel{
		ID:          o.ID,
		UserID:      o.UserID,
		Provider:    o.Provider,
		ProviderID:  o.ProviderID,
		AccessToken: o.AccessToken,
	}
}

type RefreshTokenModel struct {
	ID                string     `gorm:"primarykey;type:uuid;default:gen_random_uuid()"`
	UserID            string     `gorm:"type:uuid;not null;index"`
	TokenHash         string     `gorm:"uniqueIndex;not null"`
	FamilyID          string     `gorm:"type:uuid;not null;index"`
	DeviceFingerprint string     `gorm:"not null"`
	ExpiresAt         time.Time  `gorm:"not null"`
	RevokedAt         *time.Time `gorm:""`
	CreatedAt         time.Time  `gorm:"not null;autoCreateTime"`
}

func (m *RefreshTokenModel) TableName() string { return "refresh_tokens" }

func (m *RefreshTokenModel) ToDomain() *domain.RefreshToken {
	return &domain.RefreshToken{
		ID:                m.ID,
		UserID:            m.UserID,
		TokenHash:         m.TokenHash,
		FamilyID:          m.FamilyID,
		DeviceFingerprint: m.DeviceFingerprint,
		ExpiresAt:         m.ExpiresAt,
		RevokedAt:         m.RevokedAt,
		CreatedAt:         m.CreatedAt,
	}
}

func RefreshTokenModelFrom(rt *domain.RefreshToken) *RefreshTokenModel {
	return &RefreshTokenModel{
		ID:                rt.ID,
		UserID:            rt.UserID,
		TokenHash:         rt.TokenHash,
		FamilyID:          rt.FamilyID,
		DeviceFingerprint: rt.DeviceFingerprint,
		ExpiresAt:         rt.ExpiresAt,
		RevokedAt:         rt.RevokedAt,
	}
}

type AuthEventModel struct {
	ID        string         `gorm:"primarykey;type:uuid;default:gen_random_uuid()"`
	EventType string         `gorm:"not null;index"`
	UserID    string         `gorm:"type:uuid;index"`
	IP        string         `gorm:""`
	UserAgent string         `gorm:""`
	Metadata  map[string]any `gorm:"type:jsonb;serializer:json"`
	CreatedAt time.Time      `gorm:"not null;autoCreateTime;index"`
}

func (m *AuthEventModel) TableName() string { return "auth_events" }

func AuthEventModelFrom(e *domain.AuthEvent) *AuthEventModel {
	return &AuthEventModel{
		EventType: e.EventType,
		UserID:    e.UserID,
		IP:        e.IP,
		UserAgent: e.UserAgent,
		Metadata:  e.Metadata,
	}
}
