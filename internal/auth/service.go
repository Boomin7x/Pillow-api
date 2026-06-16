package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost         = 12
	refreshTokenTTL    = 30 * 24 * time.Hour
	dummyHashForTiming = "$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewY5n5RK74cAzJKK"
)

type authService struct {
	repo          domain.AuthRepository
	issuer        domain.TokenIssuer
	auditLogger   domain.AuditLogger
	pkceStore     domain.PKCEStore
	oauthProvider domain.OAuthProvider
}

func NewService(repo domain.AuthRepository, issuer domain.TokenIssuer, auditLogger domain.AuditLogger) domain.AuthService {
	return &authService{repo: repo, issuer: issuer, auditLogger: auditLogger}
}

func NewServiceWithOAuth(repo domain.AuthRepository, issuer domain.TokenIssuer, auditLogger domain.AuditLogger, pkce domain.PKCEStore, provider domain.OAuthProvider) domain.AuthService {
	return &authService{repo: repo, issuer: issuer, auditLogger: auditLogger, pkceStore: pkce, oauthProvider: provider}
}

func (s *authService) Register(ctx context.Context, input domain.RegisterInput) (*domain.AuthResult, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("register: hash password: %w", err)
	}

	user := &domain.User{
		Email:       input.Email,
		DisplayName: input.DisplayName,
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	cred := &domain.Credential{
		UserID:       user.ID,
		PasswordHash: string(hash),
	}
	if err := s.repo.CreateCredential(ctx, cred); err != nil {
		return nil, fmt.Errorf("register: store credential: %w", err)
	}

	result, err := s.issueTokenPair(ctx, user, "", "")
	if err != nil {
		return nil, fmt.Errorf("register: issue tokens: %w", err)
	}

	s.auditLogger.Log(ctx, &domain.AuthEvent{
		EventType: "register",
		UserID:    user.ID,
	})

	return result, nil
}

func (s *authService) Login(ctx context.Context, input domain.LoginInput) (*domain.AuthResult, error) {
	user, err := s.repo.FindUserByEmail(ctx, input.Email)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHashForTiming), []byte(input.Password))
		return nil, apperrors.Unauthorized("invalid email or password")
	}

	cred, err := s.repo.FindCredentialByUserID(ctx, user.ID)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHashForTiming), []byte(input.Password))
		return nil, apperrors.Unauthorized("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(input.Password)); err != nil {
		s.auditLogger.Log(ctx, &domain.AuthEvent{
			EventType: "login_failed",
			UserID:    user.ID,
			IP:        input.IPAddress,
			UserAgent: input.UserAgent,
		})
		return nil, apperrors.Unauthorized("invalid email or password")
	}

	result, err := s.issueTokenPair(ctx, user, input.IPAddress, input.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("login: issue tokens: %w", err)
	}

	s.auditLogger.Log(ctx, &domain.AuthEvent{
		EventType: "login",
		UserID:    user.ID,
		IP:        input.IPAddress,
		UserAgent: input.UserAgent,
	})

	return result, nil
}

func (s *authService) Refresh(ctx context.Context, input domain.RefreshInput) (*domain.AuthResult, error) {
	hash := hashToken(input.RawToken)

	meta, err := s.repo.GetCachedRefreshToken(ctx, hash)
	if err != nil {
		rt, dbErr := s.repo.FindRefreshTokenByHash(ctx, hash)
		if dbErr != nil {
			return nil, apperrors.Unauthorized("invalid or expired refresh token")
		}
		if rt.RevokedAt != nil {
			if revokeErr := s.repo.RevokeTokenFamily(ctx, rt.FamilyID); revokeErr != nil {
				return nil, fmt.Errorf("refresh: revoke family: %w", revokeErr)
			}
			s.auditLogger.Log(ctx, &domain.AuthEvent{
				EventType: "token_theft_detected",
				UserID:    rt.UserID,
				IP:        input.IPAddress,
				Metadata:  map[string]any{"family_id": rt.FamilyID},
			})
			return nil, apperrors.Unauthorized("session invalidated — please log in again")
		}
		meta = &domain.RefreshTokenMeta{
			UserID:            rt.UserID,
			FamilyID:          rt.FamilyID,
			DeviceFingerprint: rt.DeviceFingerprint,
		}
	}

	incomingFingerprint := deviceFingerprint(input.UserAgent, input.IPAddress)
	if meta.DeviceFingerprint != incomingFingerprint {
		s.auditLogger.Log(ctx, &domain.AuthEvent{
			EventType: "fingerprint_mismatch",
			UserID:    meta.UserID,
			IP:        input.IPAddress,
			UserAgent: input.UserAgent,
		})
	}

	user, err := s.repo.FindUserByID(ctx, meta.UserID)
	if err != nil {
		return nil, fmt.Errorf("refresh: load user: %w", err)
	}

	if err := s.repo.RevokeRefreshTokenByHash(ctx, hash); err != nil {
		return nil, fmt.Errorf("refresh: revoke old token: %w", err)
	}
	if err := s.repo.DeleteCachedRefreshToken(ctx, hash); err != nil {
		return nil, fmt.Errorf("refresh: delete old cache entry: %w", err)
	}

	result, err := s.issueTokenPairInFamily(ctx, user, meta.FamilyID, input.IPAddress, input.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("refresh: issue new tokens: %w", err)
	}

	s.auditLogger.Log(ctx, &domain.AuthEvent{
		EventType: "refresh",
		UserID:    user.ID,
		IP:        input.IPAddress,
		UserAgent: input.UserAgent,
	})

	return result, nil
}

func (s *authService) Logout(ctx context.Context, input domain.LogoutInput) error {
	if input.AccessTokenJTI != "" && input.AccessTokenTTL > 0 {
		if err := s.repo.BlocklistToken(ctx, input.AccessTokenJTI, input.AccessTokenTTL); err != nil {
			return fmt.Errorf("logout: blocklist access token: %w", err)
		}
	}

	if input.RefreshTokenRaw != "" {
		hash := hashToken(input.RefreshTokenRaw)
		rt, err := s.repo.FindRefreshTokenByHash(ctx, hash)
		if err == nil {
			_ = s.repo.RevokeRefreshToken(ctx, rt.ID)
			_ = s.repo.DeleteCachedRefreshToken(ctx, hash)
		}
	}

	s.auditLogger.Log(ctx, &domain.AuthEvent{
		EventType: "logout",
		UserID:    input.UserID,
	})

	return nil
}

func (s *authService) issueTokenPair(ctx context.Context, user *domain.User, ip, ua string) (*domain.AuthResult, error) {
	return s.issueTokenPairInFamily(ctx, user, uuid.New().String(), ip, ua)
}

func (s *authService) issueTokenPairInFamily(ctx context.Context, user *domain.User, familyID, ip, ua string) (*domain.AuthResult, error) {
	tokenID := uuid.New().String()

	accessToken, err := s.issuer.IssueAccessToken(domain.Claims{
		UserID:    user.ID,
		Email:     user.Email,
		Roles:     []string{"user"},
		TokenID:   tokenID,
		SessionID: familyID,
	})
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}

	rawRefresh, err := s.issuer.IssueRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("issue refresh token: %w", err)
	}

	hash := hashToken(rawRefresh)
	fingerprint := deviceFingerprint(ua, ip)
	now := time.Now()

	rt := &domain.RefreshToken{
		UserID:            user.ID,
		TokenHash:         hash,
		FamilyID:          familyID,
		DeviceFingerprint: fingerprint,
		ExpiresAt:         now.Add(refreshTokenTTL),
	}

	if err := s.repo.StoreRefreshToken(ctx, rt); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	meta := domain.RefreshTokenMeta{
		UserID:            user.ID,
		FamilyID:          familyID,
		DeviceFingerprint: fingerprint,
	}
	if err := s.repo.CacheRefreshToken(ctx, hash, meta, refreshTokenTTL); err != nil {
		_ = err
	}

	return &domain.AuthResult{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		User:         user,
	}, nil
}

func (s *authService) LogoutAll(ctx context.Context, input domain.LogoutAllInput) error {
	count, err := s.repo.RevokeAllUserRefreshTokens(ctx, input.UserID)
	if err != nil {
		return fmt.Errorf("logout all: revoke tokens: %w", err)
	}

	if input.ActiveTokenJTI != "" && input.ActiveTokenTTL > 0 {
		if err := s.repo.BlocklistToken(ctx, input.ActiveTokenJTI, input.ActiveTokenTTL); err != nil {
			return fmt.Errorf("logout all: blocklist access token: %w", err)
		}
	}

	s.auditLogger.Log(ctx, &domain.AuthEvent{
		EventType: "logout_all",
		UserID:    input.UserID,
		Metadata:  map[string]any{"sessions_revoked": count},
	})

	return nil
}

func (s *authService) ChangePassword(ctx context.Context, input domain.ChangePasswordInput) error {
	cred, err := s.repo.FindCredentialByUserID(ctx, input.UserID)
	if err != nil {
		return fmt.Errorf("change password: find credential: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(input.OldPassword)); err != nil {
		return apperrors.Unauthorized("current password is incorrect")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("change password: hash password: %w", err)
	}

	if err := s.repo.UpdateCredentialPassword(ctx, input.UserID, string(newHash)); err != nil {
		return fmt.Errorf("change password: update credential: %w", err)
	}

	s.auditLogger.Log(ctx, &domain.AuthEvent{
		EventType: "password_change",
		UserID:    input.UserID,
	})

	return nil
}

func (s *authService) OAuthLogin(ctx context.Context, input domain.OAuthLoginInput) (*domain.AuthResult, error) {
	identity, err := s.repo.FindOAuthIdentity(ctx, input.Provider, input.ProviderID)
	if err == nil {
		user, err := s.repo.FindUserByID(ctx, identity.UserID)
		if err != nil {
			return nil, fmt.Errorf("oauth login: load user: %w", err)
		}
		result, err := s.issueTokenPair(ctx, user, input.IPAddress, input.UserAgent)
		if err != nil {
			return nil, fmt.Errorf("oauth login: issue tokens: %w", err)
		}
		s.auditLogger.Log(ctx, &domain.AuthEvent{EventType: "oauth_login", UserID: user.ID, IP: input.IPAddress})
		return result, nil
	}

	existingUser, err := s.repo.FindUserByEmail(ctx, input.Email)
	if err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) && appErr.Code == apperrors.CodeNotFound {
			newUser := &domain.User{Email: input.Email, DisplayName: input.Email}
			if err := s.repo.CreateUser(ctx, newUser); err != nil {
				return nil, fmt.Errorf("oauth login: create user: %w", err)
			}
			if err := s.repo.UpsertOAuthIdentity(ctx, &domain.OAuthIdentity{
				UserID:     newUser.ID,
				Provider:   input.Provider,
				ProviderID: input.ProviderID,
			}); err != nil {
				return nil, fmt.Errorf("oauth login: create identity: %w", err)
			}
			result, err := s.issueTokenPair(ctx, newUser, input.IPAddress, input.UserAgent)
			if err != nil {
				return nil, fmt.Errorf("oauth login: issue tokens: %w", err)
			}
			s.auditLogger.Log(ctx, &domain.AuthEvent{EventType: "oauth_register", UserID: newUser.ID, IP: input.IPAddress})
			return result, nil
		}
		return nil, fmt.Errorf("oauth login: find user by email: %w", err)
	}

	existingIdentity, err := s.repo.FindOAuthIdentityByUserAndProvider(ctx, existingUser.ID, input.Provider)
	if err == nil && existingIdentity.ProviderID != input.ProviderID {
		return nil, apperrors.Conflict("email already linked to a different " + input.Provider + " account")
	}

	if err := s.repo.UpsertOAuthIdentity(ctx, &domain.OAuthIdentity{
		UserID:     existingUser.ID,
		Provider:   input.Provider,
		ProviderID: input.ProviderID,
	}); err != nil {
		return nil, fmt.Errorf("oauth login: link identity: %w", err)
	}

	result, err := s.issueTokenPair(ctx, existingUser, input.IPAddress, input.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("oauth login: issue tokens: %w", err)
	}
	s.auditLogger.Log(ctx, &domain.AuthEvent{EventType: "oauth_link", UserID: existingUser.ID, IP: input.IPAddress})
	return result, nil
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h)
}

func deviceFingerprint(ua, ip string) string {
	h := sha256.Sum256([]byte(ua + ":" + ip))
	return fmt.Sprintf("%x", h)
}
