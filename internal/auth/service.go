package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost          = 12
	refreshTokenTTL     = 30 * 24 * time.Hour
	dummyHashForTiming  = "$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewY5n5RK74cAzJKK"
)

type authService struct {
	repo   domain.AuthRepository
	issuer domain.TokenIssuer
}

func NewService(repo domain.AuthRepository, issuer domain.TokenIssuer) domain.AuthService {
	return &authService{repo: repo, issuer: issuer}
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

	s.repo.LogEvent(ctx, &domain.AuthEvent{
		EventType: "register",
		UserID:    user.ID,
	})

	return result, nil
}

func (s *authService) Login(ctx context.Context, input domain.LoginInput) (*domain.AuthResult, error) {
	user, err := s.repo.FindUserByEmail(ctx, input.Email)
	if err != nil {
		// timing attack mitigation: always run bcrypt even when no user found
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHashForTiming), []byte(input.Password))
		return nil, apperrors.Unauthorized("invalid email or password")
	}

	cred, err := s.repo.FindCredentialByUserID(ctx, user.ID)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHashForTiming), []byte(input.Password))
		return nil, apperrors.Unauthorized("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(input.Password)); err != nil {
		s.repo.LogEvent(ctx, &domain.AuthEvent{
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

	s.repo.LogEvent(ctx, &domain.AuthEvent{
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
		// Cache miss — check Postgres before treating as theft
		rt, dbErr := s.repo.FindRefreshTokenByHash(ctx, hash)
		if dbErr != nil {
			return nil, apperrors.Unauthorized("invalid or expired refresh token")
		}
		if rt.RevokedAt != nil {
			// Token already rotated — theft detected
			if revokeErr := s.repo.RevokeTokenFamily(ctx, rt.FamilyID); revokeErr != nil {
				return nil, fmt.Errorf("refresh: revoke family: %w", revokeErr)
			}
			s.repo.LogEvent(ctx, &domain.AuthEvent{
				EventType: "token_theft_detected",
				UserID:    rt.UserID,
				IP:        input.IPAddress,
				Metadata:  map[string]any{"family_id": rt.FamilyID},
			})
			return nil, apperrors.Unauthorized("session invalidated — please log in again")
		}
		// Rebuild meta from Postgres
		meta = &domain.RefreshTokenMeta{
			UserID:            rt.UserID,
			FamilyID:          rt.FamilyID,
			DeviceFingerprint: rt.DeviceFingerprint,
		}
	}

	incomingFingerprint := deviceFingerprint(input.UserAgent, input.IPAddress)
	if meta.DeviceFingerprint != incomingFingerprint {
		s.repo.LogEvent(ctx, &domain.AuthEvent{
			EventType: "fingerprint_mismatch",
			UserID:    meta.UserID,
			IP:        input.IPAddress,
			UserAgent: input.UserAgent,
		})
		// Soft signal — log but do not block (mobile IPs change)
	}

	user, err := s.repo.FindUserByID(ctx, meta.UserID)
	if err != nil {
		return nil, fmt.Errorf("refresh: load user: %w", err)
	}

	// Rotate: revoke old token, issue new pair within the same family
	if err := s.repo.RevokeRefreshToken(ctx, hash); err != nil {
		return nil, fmt.Errorf("refresh: revoke old token: %w", err)
	}
	if err := s.repo.DeleteCachedRefreshToken(ctx, hash); err != nil {
		return nil, fmt.Errorf("refresh: delete old cache entry: %w", err)
	}

	result, err := s.issueTokenPairInFamily(ctx, user, meta.FamilyID, input.IPAddress, input.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("refresh: issue new tokens: %w", err)
	}

	s.repo.LogEvent(ctx, &domain.AuthEvent{
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

	s.repo.LogEvent(ctx, &domain.AuthEvent{
		EventType: "logout",
		UserID:    input.UserID,
	})

	return nil
}

// --- internal helpers ---

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
		// Non-fatal: Postgres is authoritative; Redis is a cache.
		_ = err
	}

	return &domain.AuthResult{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		User:         user,
	}, nil
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h)
}

func deviceFingerprint(ua, ip string) string {
	h := sha256.Sum256([]byte(ua + ":" + ip))
	return fmt.Sprintf("%x", h)
}
