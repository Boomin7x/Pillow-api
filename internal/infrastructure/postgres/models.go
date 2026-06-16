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

type KYCProfileModel struct {
	ID             string    `gorm:"primarykey;type:uuid;default:gen_random_uuid()"`
	UserID         string    `gorm:"type:uuid;not null;uniqueIndex"`
	Role           string    `gorm:"not null"`
	Tier           int       `gorm:"not null;default:0"`
	Status         string    `gorm:"not null"`
	Qualifications []string  `gorm:"type:jsonb;serializer:json"`
	CreatedAt      time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"not null;autoUpdateTime"`
}

func (m *KYCProfileModel) TableName() string { return "kyc_profiles" }

func (m *KYCProfileModel) ToDomain() *domain.KYCProfile {
	return &domain.KYCProfile{
		UserID:         m.UserID,
		Role:           domain.Role(m.Role),
		Tier:           domain.Tier(m.Tier),
		Status:         domain.VerificationStatus(m.Status),
		Qualifications: qualificationsToDomain(m.Qualifications),
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func KYCProfileModelFrom(p *domain.KYCProfile) *KYCProfileModel {
	return &KYCProfileModel{
		UserID:         p.UserID,
		Role:           string(p.Role),
		Tier:           int(p.Tier),
		Status:         string(p.Status),
		Qualifications: qualificationsToStrings(p.Qualifications),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

type VerificationCaseModel struct {
	ID        string    `gorm:"primarykey;type:uuid;default:gen_random_uuid()"`
	UserID    string    `gorm:"type:uuid;not null;index"`
	Type      string    `gorm:"not null"`
	Status    string    `gorm:"not null"`
	RiskScore int       `gorm:"not null;default:0"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime"`
}

func (m *VerificationCaseModel) TableName() string { return "verification_cases" }

func (m *VerificationCaseModel) ToDomain() *domain.VerificationCase {
	return &domain.VerificationCase{
		ID:        m.ID,
		UserID:    m.UserID,
		Type:      domain.CheckType(m.Type),
		Status:    domain.VerificationStatus(m.Status),
		RiskScore: domain.RiskScore(m.RiskScore),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func VerificationCaseModelFrom(c *domain.VerificationCase) *VerificationCaseModel {
	return &VerificationCaseModel{
		ID:        c.ID,
		UserID:    c.UserID,
		Type:      string(c.Type),
		Status:    string(c.Status),
		RiskScore: int(c.RiskScore),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

type KYCCheckModel struct {
	ID              string         `gorm:"primarykey;type:uuid;default:gen_random_uuid()"`
	CaseID          string         `gorm:"type:uuid;not null;index"`
	Type            string         `gorm:"not null"`
	Status          string         `gorm:"not null"`
	Verdict         string         `gorm:"not null;default:''"`
	ProviderEventID string         `gorm:"not null;default:''"`
	RiskScore       int            `gorm:"not null;default:0"`
	RawPayload      map[string]any `gorm:"type:jsonb;serializer:json"`
	CreatedAt       time.Time      `gorm:"not null;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"not null;autoUpdateTime"`
}

func (m *KYCCheckModel) TableName() string { return "kyc_checks" }

func (m *KYCCheckModel) ToDomain() *domain.Check {
	return &domain.Check{
		ID:              m.ID,
		CaseID:          m.CaseID,
		Type:            domain.CheckType(m.Type),
		Status:          domain.VerificationStatus(m.Status),
		Verdict:         domain.Verdict(m.Verdict),
		ProviderEventID: m.ProviderEventID,
		RiskScore:       domain.RiskScore(m.RiskScore),
		RawPayload:      m.RawPayload,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

func KYCCheckModelFrom(c *domain.Check) *KYCCheckModel {
	return &KYCCheckModel{
		ID:              c.ID,
		CaseID:          c.CaseID,
		Type:            string(c.Type),
		Status:          string(c.Status),
		Verdict:         string(c.Verdict),
		ProviderEventID: c.ProviderEventID,
		RiskScore:       int(c.RiskScore),
		RawPayload:      c.RawPayload,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}

type OwnershipClaimModel struct {
	ID              string    `gorm:"primarykey;type:uuid;default:gen_random_uuid()"`
	UserID          string    `gorm:"type:uuid;not null;index"`
	PropertyAddress string    `gorm:"not null"`
	ClaimantName    string    `gorm:"not null"`
	Status          string    `gorm:"not null"`
	Method          string    `gorm:"not null;default:''"`
	CreatedAt       time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"not null;autoUpdateTime"`
}

func (m *OwnershipClaimModel) TableName() string { return "ownership_claims" }

func (m *OwnershipClaimModel) ToDomain() *domain.OwnershipClaim {
	return &domain.OwnershipClaim{
		ID:              m.ID,
		UserID:          m.UserID,
		PropertyAddress: m.PropertyAddress,
		ClaimantName:    m.ClaimantName,
		Status:          domain.VerificationStatus(m.Status),
		Method:          domain.OwnershipVerificationMethod(m.Method),
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

func OwnershipClaimModelFrom(c *domain.OwnershipClaim) *OwnershipClaimModel {
	return &OwnershipClaimModel{
		ID:              c.ID,
		UserID:          c.UserID,
		PropertyAddress: c.PropertyAddress,
		ClaimantName:    c.ClaimantName,
		Status:          string(c.Status),
		Method:          string(c.Method),
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}

type KYCAuditEventModel struct {
	ID        string         `gorm:"primarykey;type:uuid;default:gen_random_uuid()"`
	UserID    string         `gorm:"type:uuid;index"`
	EventType string         `gorm:"not null;index"`
	Tier      int            `gorm:"not null;default:0"`
	Metadata  map[string]any `gorm:"type:jsonb;serializer:json"`
	CreatedAt time.Time      `gorm:"not null;autoCreateTime;index"`
}

func (m *KYCAuditEventModel) TableName() string { return "kyc_audit_events" }

func (m *KYCAuditEventModel) ToDomain() *domain.AuditEvent {
	return &domain.AuditEvent{
		ID:        m.ID,
		UserID:    m.UserID,
		EventType: m.EventType,
		Tier:      domain.Tier(m.Tier),
		Metadata:  m.Metadata,
		CreatedAt: m.CreatedAt,
	}
}

func KYCAuditEventModelFrom(e *domain.AuditEvent) *KYCAuditEventModel {
	return &KYCAuditEventModel{
		UserID:    e.UserID,
		EventType: e.EventType,
		Tier:      int(e.Tier),
		Metadata:  e.Metadata,
	}
}

func qualificationsToStrings(qs []domain.Qualification) []string {
	out := make([]string, len(qs))
	for i, q := range qs {
		out[i] = string(q)
	}
	return out
}

func qualificationsToDomain(ss []string) []domain.Qualification {
	out := make([]domain.Qualification, len(ss))
	for i, s := range ss {
		out[i] = domain.Qualification(s)
	}
	return out
}
