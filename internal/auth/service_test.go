package auth_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/auth"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

func deviceFingerprintForTest(ua, ip string) string {
	h := sha256.Sum256([]byte(ua + ":" + ip))
	return fmt.Sprintf("%x", h)
}

type mockRepo struct {
	createUserFn                         func(ctx context.Context, u *domain.User) error
	findUserByEmailFn                    func(ctx context.Context, email string) (*domain.User, error)
	findUserByIDFn                       func(ctx context.Context, id string) (*domain.User, error)
	createCredentialFn                   func(ctx context.Context, c *domain.Credential) error
	findCredentialByUserIDFn             func(ctx context.Context, userID string) (*domain.Credential, error)
	storeRefreshTokenFn                  func(ctx context.Context, rt *domain.RefreshToken) error
	findRefreshTokenByHashFn             func(ctx context.Context, hash string) (*domain.RefreshToken, error)
	revokeRefreshTokenFn                 func(ctx context.Context, id string) error
	revokeRefreshTokenByHashFn           func(ctx context.Context, hash string) error
	revokeTokenFamilyFn                  func(ctx context.Context, familyID string) error
	revokeAllUserTokensFn                func(ctx context.Context, userID string) (int64, error)
	updateCredentialPasswordFn           func(ctx context.Context, userID, hash string) error
	upsertOAuthIdentityFn                func(ctx context.Context, identity *domain.OAuthIdentity) error
	findOAuthIdentityFn                  func(ctx context.Context, provider, providerID string) (*domain.OAuthIdentity, error)
	findOAuthIdentityByUserAndProviderFn func(ctx context.Context, userID, provider string) (*domain.OAuthIdentity, error)
	cacheRefreshTokenFn                  func(ctx context.Context, hash string, meta domain.RefreshTokenMeta, ttl time.Duration) error
	getCachedRefreshTokenFn              func(ctx context.Context, hash string) (*domain.RefreshTokenMeta, error)
	deleteCachedTokenFn                  func(ctx context.Context, hash string) error
	blocklistTokenFn                     func(ctx context.Context, jti string, ttl time.Duration) error
	isTokenBlocklistedFn                 func(ctx context.Context, jti string) (bool, error)
	assignRoleFn                         func(ctx context.Context, userID, role string) error
	removeRoleFn                         func(ctx context.Context, userID, role string) error
	listRolesFn                          func(ctx context.Context, userID string) ([]string, error)
}

func (m *mockRepo) AssignRole(ctx context.Context, userID, role string) error {
	if m.assignRoleFn != nil {
		return m.assignRoleFn(ctx, userID, role)
	}
	return nil
}

func (m *mockRepo) RemoveRole(ctx context.Context, userID, role string) error {
	if m.removeRoleFn != nil {
		return m.removeRoleFn(ctx, userID, role)
	}
	return nil
}

func (m *mockRepo) ListRoles(ctx context.Context, userID string) ([]string, error) {
	if m.listRolesFn != nil {
		return m.listRolesFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockRepo) CreateUser(ctx context.Context, u *domain.User) error {
	return m.createUserFn(ctx, u)
}
func (m *mockRepo) FindUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return m.findUserByEmailFn(ctx, email)
}
func (m *mockRepo) FindUserByID(ctx context.Context, id string) (*domain.User, error) {
	return m.findUserByIDFn(ctx, id)
}
func (m *mockRepo) CreateCredential(ctx context.Context, c *domain.Credential) error {
	return m.createCredentialFn(ctx, c)
}
func (m *mockRepo) FindCredentialByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	return m.findCredentialByUserIDFn(ctx, userID)
}
func (m *mockRepo) StoreRefreshToken(ctx context.Context, rt *domain.RefreshToken) error {
	return m.storeRefreshTokenFn(ctx, rt)
}
func (m *mockRepo) FindRefreshTokenByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	return m.findRefreshTokenByHashFn(ctx, hash)
}
func (m *mockRepo) RevokeRefreshToken(ctx context.Context, id string) error {
	if m.revokeRefreshTokenFn != nil {
		return m.revokeRefreshTokenFn(ctx, id)
	}
	return nil
}
func (m *mockRepo) RevokeRefreshTokenByHash(ctx context.Context, hash string) error {
	if m.revokeRefreshTokenByHashFn != nil {
		return m.revokeRefreshTokenByHashFn(ctx, hash)
	}
	return nil
}
func (m *mockRepo) RevokeTokenFamily(ctx context.Context, familyID string) error {
	if m.revokeTokenFamilyFn != nil {
		return m.revokeTokenFamilyFn(ctx, familyID)
	}
	return nil
}
func (m *mockRepo) RevokeAllUserRefreshTokens(ctx context.Context, userID string) (int64, error) {
	if m.revokeAllUserTokensFn != nil {
		return m.revokeAllUserTokensFn(ctx, userID)
	}
	return 0, nil
}
func (m *mockRepo) UpdateCredentialPassword(ctx context.Context, userID, hash string) error {
	if m.updateCredentialPasswordFn != nil {
		return m.updateCredentialPasswordFn(ctx, userID, hash)
	}
	return nil
}
func (m *mockRepo) UpsertOAuthIdentity(ctx context.Context, identity *domain.OAuthIdentity) error {
	if m.upsertOAuthIdentityFn != nil {
		return m.upsertOAuthIdentityFn(ctx, identity)
	}
	return nil
}
func (m *mockRepo) FindOAuthIdentity(ctx context.Context, provider, providerID string) (*domain.OAuthIdentity, error) {
	if m.findOAuthIdentityFn != nil {
		return m.findOAuthIdentityFn(ctx, provider, providerID)
	}
	return nil, apperrors.NotFound("not found")
}
func (m *mockRepo) FindOAuthIdentityByUserAndProvider(ctx context.Context, userID, provider string) (*domain.OAuthIdentity, error) {
	if m.findOAuthIdentityByUserAndProviderFn != nil {
		return m.findOAuthIdentityByUserAndProviderFn(ctx, userID, provider)
	}
	return nil, apperrors.NotFound("not found")
}
func (m *mockRepo) CacheRefreshToken(ctx context.Context, hash string, meta domain.RefreshTokenMeta, ttl time.Duration) error {
	if m.cacheRefreshTokenFn != nil {
		return m.cacheRefreshTokenFn(ctx, hash, meta, ttl)
	}
	return nil
}
func (m *mockRepo) GetCachedRefreshToken(ctx context.Context, hash string) (*domain.RefreshTokenMeta, error) {
	return m.getCachedRefreshTokenFn(ctx, hash)
}
func (m *mockRepo) DeleteCachedRefreshToken(ctx context.Context, hash string) error {
	if m.deleteCachedTokenFn != nil {
		return m.deleteCachedTokenFn(ctx, hash)
	}
	return nil
}
func (m *mockRepo) BlocklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	if m.blocklistTokenFn != nil {
		return m.blocklistTokenFn(ctx, jti, ttl)
	}
	return nil
}
func (m *mockRepo) IsTokenBlocklisted(ctx context.Context, jti string) (bool, error) {
	if m.isTokenBlocklistedFn != nil {
		return m.isTokenBlocklistedFn(ctx, jti)
	}
	return false, nil
}

type mockIssuer struct {
	issueAccessFn  func(claims domain.Claims) (string, error)
	issueRefreshFn func() (string, error)
	validateFn     func(token string) (*domain.Claims, error)
	publicKeySetFn func() []domain.JWK
}

func (m *mockIssuer) IssueAccessToken(claims domain.Claims) (string, error) {
	return m.issueAccessFn(claims)
}
func (m *mockIssuer) IssueRefreshToken() (string, error) { return m.issueRefreshFn() }
func (m *mockIssuer) ValidateAccessToken(token string) (*domain.Claims, error) {
	return m.validateFn(token)
}
func (m *mockIssuer) PublicKeySet() []domain.JWK {
	if m.publicKeySetFn != nil {
		return m.publicKeySetFn()
	}
	return []domain.JWK{}
}

type mockAuditLogger struct {
	logFn func(ctx context.Context, event *domain.AuthEvent)
}

func (m *mockAuditLogger) Log(ctx context.Context, event *domain.AuthEvent) {
	if m.logFn != nil {
		m.logFn(ctx, event)
	}
}

func okIssuer() *mockIssuer {
	return &mockIssuer{
		issueAccessFn:  func(_ domain.Claims) (string, error) { return "access.token.signed", nil },
		issueRefreshFn: func() (string, error) { return "raw-refresh-token", nil },
		validateFn:     func(_ string) (*domain.Claims, error) { return nil, nil },
	}
}

func noopLogger() *mockAuditLogger {
	return &mockAuditLogger{}
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name        string
		input       domain.RegisterInput
		repoSetup   func(*mockRepo)
		wantErrCode apperrors.Code
	}{
		{
			name: "success",
			input: domain.RegisterInput{
				Email:       "alice@example.com",
				Password:    "securepass",
				DisplayName: "Alice",
			},
			repoSetup: func(r *mockRepo) {
				r.createUserFn = func(_ context.Context, u *domain.User) error {
					u.ID = "user-uuid-1"
					return nil
				}
				r.createCredentialFn = func(_ context.Context, _ *domain.Credential) error { return nil }
				r.storeRefreshTokenFn = func(_ context.Context, _ *domain.RefreshToken) error { return nil }
			},
		},
		{
			name:  "duplicate email",
			input: domain.RegisterInput{Email: "existing@example.com", Password: "pass"},
			repoSetup: func(r *mockRepo) {
				r.createUserFn = func(_ context.Context, _ *domain.User) error {
					return apperrors.Conflict("email already registered")
				}
			},
			wantErrCode: apperrors.CodeConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			tt.repoSetup(repo)
			svc := auth.NewService(repo, okIssuer(), noopLogger())

			result, err := svc.Register(context.Background(), tt.input)

			if tt.wantErrCode != "" {
				var appErr *apperrors.AppError
				if !errors.As(err, &appErr) {
					t.Fatalf("expected AppError, got %T: %v", err, err)
				}
				if appErr.Code != tt.wantErrCode {
					t.Errorf("error code = %q, want %q", appErr.Code, tt.wantErrCode)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.AccessToken == "" {
				t.Error("expected non-empty access token")
			}
			if result.RefreshToken == "" {
				t.Error("expected non-empty refresh token")
			}
		})
	}
}

func TestLogin(t *testing.T) {
	const validHash = "$2a$12$tMQFmjZPYRrJkbEAvIXmyeH7LMoIqKsQf7LZzs9Fga8M5qoGT3V6K"

	tests := []struct {
		name        string
		input       domain.LoginInput
		repoSetup   func(*mockRepo)
		wantErrCode apperrors.Code
	}{
		{
			name:  "unknown email returns unauthorized",
			input: domain.LoginInput{Email: "ghost@example.com", Password: "pass"},
			repoSetup: func(r *mockRepo) {
				r.findUserByEmailFn = func(_ context.Context, _ string) (*domain.User, error) {
					return nil, apperrors.NotFound("user not found")
				}
			},
			wantErrCode: apperrors.CodeUnauthorized,
		},
		{
			name:  "wrong password returns unauthorized",
			input: domain.LoginInput{Email: "alice@example.com", Password: "wrongpass"},
			repoSetup: func(r *mockRepo) {
				r.findUserByEmailFn = func(_ context.Context, _ string) (*domain.User, error) {
					return &domain.User{ID: "u1", Email: "alice@example.com"}, nil
				}
				r.findCredentialByUserIDFn = func(_ context.Context, _ string) (*domain.Credential, error) {
					return &domain.Credential{PasswordHash: validHash}, nil
				}
			},
			wantErrCode: apperrors.CodeUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			tt.repoSetup(repo)
			svc := auth.NewService(repo, okIssuer(), noopLogger())

			_, err := svc.Login(context.Background(), tt.input)

			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected AppError, got %T: %v", err, err)
			}
			if appErr.Code != tt.wantErrCode {
				t.Errorf("code = %q, want %q", appErr.Code, tt.wantErrCode)
			}
		})
	}
}

func TestLogin_Success(t *testing.T) {
	passwordHash := mustHashPassword(t, "correcthorsebatterystaple")

	repo := &mockRepo{
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "u1", Email: "alice@example.com", DisplayName: "Alice"}, nil
		},
		findCredentialByUserIDFn: func(_ context.Context, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: "u1", PasswordHash: passwordHash}, nil
		},
		storeRefreshTokenFn: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	result, err := svc.Login(context.Background(), domain.LoginInput{
		Email:    "alice@example.com",
		Password: "correcthorsebatterystaple",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if result.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if result.User.ID != "u1" {
		t.Errorf("user ID = %q, want %q", result.User.ID, "u1")
	}
}

func TestRefresh_Success(t *testing.T) {
	const userAgent = "test-agent"
	const ip = "203.0.113.7"
	fingerprint := deviceFingerprintForTest(userAgent, ip)

	var storedNewToken bool
	repo := &mockRepo{
		getCachedRefreshTokenFn: func(_ context.Context, _ string) (*domain.RefreshTokenMeta, error) {
			return &domain.RefreshTokenMeta{
				UserID:            "u1",
				FamilyID:          "family-1",
				DeviceFingerprint: fingerprint,
			}, nil
		},
		findUserByIDFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "u1", Email: "alice@example.com"}, nil
		},
		storeRefreshTokenFn: func(_ context.Context, _ *domain.RefreshToken) error {
			storedNewToken = true
			return nil
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	result, err := svc.Refresh(context.Background(), domain.RefreshInput{
		RawToken:  "valid-refresh-token",
		IPAddress: ip,
		UserAgent: userAgent,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Error("expected a rotated token pair")
	}
	if !storedNewToken {
		t.Error("expected a new refresh token to be stored after rotation")
	}
}

func TestRefresh_CacheMiss_PostgresFallback(t *testing.T) {
	const userAgent = "test-agent"
	const ip = "203.0.113.7"
	fingerprint := deviceFingerprintForTest(userAgent, ip)

	var postgresQueried bool
	repo := &mockRepo{
		getCachedRefreshTokenFn: func(_ context.Context, _ string) (*domain.RefreshTokenMeta, error) {
			return nil, apperrors.NotFound("cache miss")
		},
		findRefreshTokenByHashFn: func(_ context.Context, _ string) (*domain.RefreshToken, error) {
			postgresQueried = true
			return &domain.RefreshToken{
				ID:                "rt-id",
				UserID:            "u1",
				FamilyID:          "family-1",
				DeviceFingerprint: fingerprint,
			}, nil
		},
		findUserByIDFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "u1", Email: "alice@example.com"}, nil
		},
		storeRefreshTokenFn: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	result, err := svc.Refresh(context.Background(), domain.RefreshInput{
		RawToken:  "valid-refresh-token",
		IPAddress: ip,
		UserAgent: userAgent,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !postgresQueried {
		t.Error("expected Postgres fallback when Redis cache misses")
	}
	if result.AccessToken == "" {
		t.Error("expected token pair on successful fallback")
	}
}

func TestRefresh_FingerprintMismatch(t *testing.T) {
	var loggedEventType string
	auditLogger := &mockAuditLogger{
		logFn: func(_ context.Context, e *domain.AuthEvent) {
			if e.EventType == "fingerprint_mismatch" {
				loggedEventType = e.EventType
			}
		},
	}

	repo := &mockRepo{
		getCachedRefreshTokenFn: func(_ context.Context, _ string) (*domain.RefreshTokenMeta, error) {
			return &domain.RefreshTokenMeta{
				UserID:            "u1",
				FamilyID:          "family-1",
				DeviceFingerprint: "fingerprint-from-a-different-device",
			}, nil
		},
		findUserByIDFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "u1", Email: "alice@example.com"}, nil
		},
		storeRefreshTokenFn: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}

	svc := auth.NewService(repo, okIssuer(), auditLogger)
	result, err := svc.Refresh(context.Background(), domain.RefreshInput{
		RawToken:  "valid-refresh-token",
		IPAddress: "203.0.113.7",
		UserAgent: "test-agent",
	})

	if err != nil {
		t.Fatalf("expected mismatch to be logged but not block refresh, got error: %v", err)
	}
	if result.AccessToken == "" {
		t.Error("fingerprint mismatch must not block the refresh — token pair still issued")
	}
	if loggedEventType != "fingerprint_mismatch" {
		t.Error("expected a fingerprint_mismatch audit event to be logged")
	}
}

func TestRefresh_TheftDetected(t *testing.T) {
	revokedAt := time.Now().Add(-time.Minute)
	familyRevoked := false

	repo := &mockRepo{
		getCachedRefreshTokenFn: func(_ context.Context, _ string) (*domain.RefreshTokenMeta, error) {
			return nil, apperrors.NotFound("cache miss")
		},
		findRefreshTokenByHashFn: func(_ context.Context, _ string) (*domain.RefreshToken, error) {
			return &domain.RefreshToken{
				ID:        "rt-id",
				UserID:    "u1",
				FamilyID:  "family-1",
				RevokedAt: &revokedAt,
			}, nil
		},
		revokeTokenFamilyFn: func(_ context.Context, _ string) error {
			familyRevoked = true
			return nil
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	_, err := svc.Refresh(context.Background(), domain.RefreshInput{RawToken: "stolen-token"})

	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != apperrors.CodeUnauthorized {
		t.Errorf("code = %q, want %q", appErr.Code, apperrors.CodeUnauthorized)
	}
	if !familyRevoked {
		t.Error("expected token family to be revoked on theft detection")
	}
}

func TestLogout_BlocklistsAccessToken(t *testing.T) {
	var blocklistedJTI string

	repo := &mockRepo{
		blocklistTokenFn: func(_ context.Context, jti string, _ time.Duration) error {
			blocklistedJTI = jti
			return nil
		},
		findRefreshTokenByHashFn: func(_ context.Context, _ string) (*domain.RefreshToken, error) {
			return nil, apperrors.NotFound("not found")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	err := svc.Logout(context.Background(), domain.LogoutInput{
		AccessTokenJTI: "jti-abc",
		AccessTokenTTL: 5 * time.Minute,
		UserID:         "u1",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocklistedJTI != "jti-abc" {
		t.Errorf("blocklistedJTI = %q, want %q", blocklistedJTI, "jti-abc")
	}
}

func failingIssuer() *mockIssuer {
	return &mockIssuer{
		issueAccessFn:  func(_ domain.Claims) (string, error) { return "", fmt.Errorf("signing key unavailable") },
		issueRefreshFn: func() (string, error) { return "raw-refresh-token", nil },
		validateFn:     func(_ string) (*domain.Claims, error) { return nil, nil },
	}
}

func TestRegister_IssuerError(t *testing.T) {
	repo := &mockRepo{
		createUserFn: func(_ context.Context, u *domain.User) error {
			u.ID = "u1"
			return nil
		},
		createCredentialFn: func(_ context.Context, _ *domain.Credential) error { return nil },
	}

	svc := auth.NewService(repo, failingIssuer(), noopLogger())
	_, err := svc.Register(context.Background(), domain.RegisterInput{Email: "alice@example.com", Password: "securepass"})
	if err == nil {
		t.Fatal("expected register to fail when the token issuer fails")
	}
}

func TestLogout_RevokesRefreshToken(t *testing.T) {
	var revokedID string
	var deletedCacheHash string

	repo := &mockRepo{
		findRefreshTokenByHashFn: func(_ context.Context, _ string) (*domain.RefreshToken, error) {
			return &domain.RefreshToken{ID: "rt-1", UserID: "u1"}, nil
		},
		revokeRefreshTokenFn: func(_ context.Context, id string) error {
			revokedID = id
			return nil
		},
		deleteCachedTokenFn: func(_ context.Context, hash string) error {
			deletedCacheHash = hash
			return nil
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	err := svc.Logout(context.Background(), domain.LogoutInput{
		AccessTokenJTI:  "jti-1",
		AccessTokenTTL:  5 * time.Minute,
		RefreshTokenRaw: "raw-refresh-token",
		UserID:          "u1",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revokedID != "rt-1" {
		t.Errorf("revoked refresh token id = %q, want %q", revokedID, "rt-1")
	}
	if deletedCacheHash == "" {
		t.Error("expected the cached refresh token entry to be deleted on logout")
	}
}

func TestOAuthLogin_CreateUserError(t *testing.T) {
	repo := &mockRepo{
		findOAuthIdentityFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return nil, apperrors.NotFound("not found")
		},
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, apperrors.NotFound("not found")
		},
		createUserFn: func(_ context.Context, _ *domain.User) error {
			return fmt.Errorf("postgres write failed")
		},
	}

	_, err := oauthSvc(repo).OAuthLogin(context.Background(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-1",
		Email:      "alice@example.com",
	})
	if err == nil {
		t.Fatal("expected oauth login to fail when user creation fails")
	}
}

func TestOAuthLogin_ReturningUserIssuerError(t *testing.T) {
	repo := &mockRepo{
		findOAuthIdentityFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return &domain.OAuthIdentity{UserID: "u1", Provider: "google", ProviderID: "g-sub-1"}, nil
		},
		findUserByIDFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "u1", Email: "alice@example.com"}, nil
		},
	}

	pkce, provider := noopOAuth()
	svc := auth.NewServiceWithOAuth(repo, failingIssuer(), noopLogger(), pkce, provider)
	_, err := svc.OAuthLogin(context.Background(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-1",
		Email:      "alice@example.com",
	})
	if err == nil {
		t.Fatal("expected oauth login to fail when token issuance fails for a returning user")
	}
}

func TestChangePassword_UpdateError(t *testing.T) {
	currentHash := mustHashPassword(t, "correcthorsebatterystaple")
	repo := &mockRepo{
		findCredentialByUserIDFn: func(_ context.Context, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: "u1", PasswordHash: currentHash}, nil
		},
		updateCredentialPasswordFn: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("postgres write failed")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	err := svc.ChangePassword(context.Background(), domain.ChangePasswordInput{
		UserID:      "u1",
		OldPassword: "correcthorsebatterystaple",
		NewPassword: "newpassword123",
	})
	if err == nil {
		t.Fatal("expected change password to fail when the update write fails")
	}
}

func TestChangePassword_NoCredentialReturnsBadRequest(t *testing.T) {
	repo := &mockRepo{
		findCredentialByUserIDFn: func(_ context.Context, _ string) (*domain.Credential, error) {
			return nil, apperrors.NotFound("credential not found")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	err := svc.ChangePassword(context.Background(), domain.ChangePasswordInput{
		UserID:      "oauth-only-user",
		OldPassword: "whatever",
		NewPassword: "newpassword123",
	})

	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeBadRequest {
		t.Fatalf("expected BadRequest for an account with no password, got %v", err)
	}
}

func TestIssuedTokenCarriesRolesFromRepository(t *testing.T) {
	cases := []struct {
		name      string
		listRoles func(ctx context.Context, userID string) ([]string, error)
		wantRoles []string
	}{
		{
			name: "roles loaded from the user record",
			listRoles: func(_ context.Context, _ string) ([]string, error) {
				return []string{"user", "admin"}, nil
			},
			wantRoles: []string{"user", "admin"},
		},
		{
			name: "no stored roles falls back to baseline",
			listRoles: func(_ context.Context, _ string) ([]string, error) {
				return nil, nil
			},
			wantRoles: []string{"user"},
		},
		{
			name: "list failure falls back to baseline, never elevates",
			listRoles: func(_ context.Context, _ string) ([]string, error) {
				return nil, fmt.Errorf("db down")
			},
			wantRoles: []string{"user"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured domain.Claims
			issuer := &mockIssuer{
				issueAccessFn: func(claims domain.Claims) (string, error) {
					captured = claims
					return "access-token", nil
				},
				issueRefreshFn: func() (string, error) { return "refresh-token", nil },
			}
			repo := &mockRepo{
				createUserFn:       func(_ context.Context, u *domain.User) error { u.ID = "u1"; return nil },
				createCredentialFn: func(_ context.Context, _ *domain.Credential) error { return nil },
				storeRefreshTokenFn: func(_ context.Context, _ *domain.RefreshToken) error {
					return nil
				},
				cacheRefreshTokenFn: func(_ context.Context, _ string, _ domain.RefreshTokenMeta, _ time.Duration) error {
					return nil
				},
				listRolesFn: tc.listRoles,
			}

			svc := auth.NewService(repo, issuer, noopLogger())
			if _, err := svc.Register(context.Background(), domain.RegisterInput{
				Email:       "roles@example.com",
				Password:    "supersecret123",
				DisplayName: "Roles",
			}); err != nil {
				t.Fatalf("register: %v", err)
			}

			if len(captured.Roles) != len(tc.wantRoles) {
				t.Fatalf("roles = %v, want %v", captured.Roles, tc.wantRoles)
			}
			for i, r := range tc.wantRoles {
				if captured.Roles[i] != r {
					t.Errorf("roles[%d] = %q, want %q", i, captured.Roles[i], r)
				}
			}
		})
	}
}

func TestRegister_StoreRefreshTokenError(t *testing.T) {
	repo := &mockRepo{
		createUserFn: func(_ context.Context, u *domain.User) error {
			u.ID = "u1"
			return nil
		},
		createCredentialFn: func(_ context.Context, _ *domain.Credential) error { return nil },
		storeRefreshTokenFn: func(_ context.Context, _ *domain.RefreshToken) error {
			return fmt.Errorf("postgres write failed")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	_, err := svc.Register(context.Background(), domain.RegisterInput{Email: "alice@example.com", Password: "securepass"})
	if err == nil {
		t.Fatal("expected register to fail when storing the refresh token fails")
	}
}

func TestLogoutAll_BlocklistError(t *testing.T) {
	repo := &mockRepo{
		revokeAllUserTokensFn: func(_ context.Context, _ string) (int64, error) { return 2, nil },
		blocklistTokenFn: func(_ context.Context, _ string, _ time.Duration) error {
			return fmt.Errorf("redis unavailable")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	err := svc.LogoutAll(context.Background(), domain.LogoutAllInput{
		UserID:         "u1",
		ActiveTokenJTI: "jti-1",
		ActiveTokenTTL: time.Minute,
	})
	if err == nil {
		t.Fatal("expected logout-all to fail when blocklisting the active token fails")
	}
}

func TestRefresh_FindUserError(t *testing.T) {
	const userAgent = "test-agent"
	const ip = "203.0.113.7"
	repo := &mockRepo{
		getCachedRefreshTokenFn: func(_ context.Context, _ string) (*domain.RefreshTokenMeta, error) {
			return &domain.RefreshTokenMeta{
				UserID:            "u1",
				FamilyID:          "family-1",
				DeviceFingerprint: deviceFingerprintForTest(userAgent, ip),
			}, nil
		},
		findUserByIDFn: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, fmt.Errorf("postgres unavailable")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	_, err := svc.Refresh(context.Background(), domain.RefreshInput{
		RawToken:  "valid-token",
		IPAddress: ip,
		UserAgent: userAgent,
	})
	if err == nil {
		t.Fatal("expected refresh to fail when the user lookup fails")
	}
}

func TestOAuthLogin_LinkIdentityError(t *testing.T) {
	repo := &mockRepo{
		findOAuthIdentityFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return nil, apperrors.NotFound("not found")
		},
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "u-existing", Email: "alice@example.com"}, nil
		},
		findOAuthIdentityByUserAndProviderFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return nil, apperrors.NotFound("no prior link")
		},
		upsertOAuthIdentityFn: func(_ context.Context, _ *domain.OAuthIdentity) error {
			return fmt.Errorf("postgres write failed")
		},
	}

	_, err := oauthSvc(repo).OAuthLogin(context.Background(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-new",
		Email:      "alice@example.com",
	})
	if err == nil {
		t.Fatal("expected oauth login to fail when linking the identity fails")
	}
}

func TestOAuthLogin_FindUserByEmailError(t *testing.T) {
	repo := &mockRepo{
		findOAuthIdentityFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return nil, apperrors.NotFound("not found")
		},
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, fmt.Errorf("postgres unavailable")
		},
	}

	_, err := oauthSvc(repo).OAuthLogin(context.Background(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-1",
		Email:      "alice@example.com",
	})
	if err == nil {
		t.Fatal("expected oauth login to fail on a non-notfound email lookup error")
	}
}

func TestLogin_AuditEventOnFailedLogin(t *testing.T) {
	const validHash = "$2a$12$tMQFmjZPYRrJkbEAvIXmyeH7LMoIqKsQf7LZzs9Fga8M5qoGT3V6K"
	var loggedEventType string

	auditLogger := &mockAuditLogger{
		logFn: func(_ context.Context, e *domain.AuthEvent) {
			loggedEventType = e.EventType
		},
	}

	repo := &mockRepo{
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "u1", Email: "alice@example.com"}, nil
		},
		findCredentialByUserIDFn: func(_ context.Context, _ string) (*domain.Credential, error) {
			return &domain.Credential{PasswordHash: validHash}, nil
		},
	}

	svc := auth.NewService(repo, okIssuer(), auditLogger)
	_, _ = svc.Login(context.Background(), domain.LoginInput{Email: "alice@example.com", Password: "wrong"})

	if loggedEventType != "login_failed" {
		t.Errorf("audit event = %q, want %q", loggedEventType, "login_failed")
	}
}

type mockPKCEStore struct {
	saveVerifierFn   func(ctx context.Context, state, verifier string, ttl time.Duration) error
	getVerifierFn    func(ctx context.Context, state string) (string, error)
	deleteVerifierFn func(ctx context.Context, state string) error
}

func (m *mockPKCEStore) SaveVerifier(ctx context.Context, state, verifier string, ttl time.Duration) error {
	if m.saveVerifierFn != nil {
		return m.saveVerifierFn(ctx, state, verifier, ttl)
	}
	return nil
}
func (m *mockPKCEStore) GetVerifier(ctx context.Context, state string) (string, error) {
	if m.getVerifierFn != nil {
		return m.getVerifierFn(ctx, state)
	}
	return "verifier", nil
}
func (m *mockPKCEStore) DeleteVerifier(ctx context.Context, state string) error {
	if m.deleteVerifierFn != nil {
		return m.deleteVerifierFn(ctx, state)
	}
	return nil
}

type mockOAuthProvider struct {
	buildAuthURLFn      func(state, codeChallenge string) string
	exchangeAndVerifyFn func(ctx context.Context, code, verifier string) (*domain.OAuthIdentityClaims, error)
}

func (m *mockOAuthProvider) BuildAuthURL(state, codeChallenge string) string {
	if m.buildAuthURLFn != nil {
		return m.buildAuthURLFn(state, codeChallenge)
	}
	return "https://accounts.google.com/o/oauth2/auth?state=" + state
}
func (m *mockOAuthProvider) ExchangeAndVerify(ctx context.Context, code, verifier string) (*domain.OAuthIdentityClaims, error) {
	if m.exchangeAndVerifyFn != nil {
		return m.exchangeAndVerifyFn(ctx, code, verifier)
	}
	return &domain.OAuthIdentityClaims{ProviderID: "g-sub-123", Email: "alice@example.com"}, nil
}

func noopOAuth() (*mockPKCEStore, *mockOAuthProvider) {
	return &mockPKCEStore{}, &mockOAuthProvider{}
}

func oauthSvc(repo *mockRepo) domain.AuthService {
	pkce, provider := noopOAuth()
	return auth.NewServiceWithOAuth(repo, okIssuer(), noopLogger(), pkce, provider)
}

func TestOAuthLogin_NewUser(t *testing.T) {
	var createdUser *domain.User
	var upsertedIdentity *domain.OAuthIdentity

	repo := &mockRepo{
		findOAuthIdentityFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return nil, apperrors.NotFound("not found")
		},
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, apperrors.NotFound("not found")
		},
		createUserFn: func(_ context.Context, u *domain.User) error {
			u.ID = "new-user-id"
			createdUser = u
			return nil
		},
		upsertOAuthIdentityFn: func(_ context.Context, identity *domain.OAuthIdentity) error {
			upsertedIdentity = identity
			return nil
		},
		storeRefreshTokenFn: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}

	result, err := oauthSvc(repo).OAuthLogin(context.Background(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-123",
		Email:      "alice@example.com",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken == "" {
		t.Error("expected access token")
	}
	if createdUser == nil {
		t.Error("expected CreateUser to be called")
	}
	if upsertedIdentity == nil || upsertedIdentity.ProviderID != "g-sub-123" {
		t.Error("expected UpsertOAuthIdentity to be called with correct provider_id")
	}
}

func TestOAuthLogin_NewUser_UsesNameAsDisplayName(t *testing.T) {
	var createdUser *domain.User

	repo := &mockRepo{
		findOAuthIdentityFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return nil, apperrors.NotFound("not found")
		},
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, apperrors.NotFound("not found")
		},
		createUserFn: func(_ context.Context, u *domain.User) error {
			u.ID = "new-user-id"
			createdUser = u
			return nil
		},
		upsertOAuthIdentityFn: func(_ context.Context, _ *domain.OAuthIdentity) error { return nil },
		storeRefreshTokenFn:   func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}

	_, err := oauthSvc(repo).OAuthLogin(context.Background(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-456",
		Email:      "bob@example.com",
		Name:       "Bob Smith",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createdUser == nil || createdUser.DisplayName != "Bob Smith" {
		t.Errorf("DisplayName = %q, want %q", createdUser.DisplayName, "Bob Smith")
	}
}

func TestOAuthLogin_NewUser_FallsBackToEmail(t *testing.T) {
	var createdUser *domain.User

	repo := &mockRepo{
		findOAuthIdentityFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return nil, apperrors.NotFound("not found")
		},
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, apperrors.NotFound("not found")
		},
		createUserFn: func(_ context.Context, u *domain.User) error {
			u.ID = "new-user-id"
			createdUser = u
			return nil
		},
		upsertOAuthIdentityFn: func(_ context.Context, _ *domain.OAuthIdentity) error { return nil },
		storeRefreshTokenFn:   func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}

	_, err := oauthSvc(repo).OAuthLogin(context.Background(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-789",
		Email:      "carol@example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createdUser == nil || createdUser.DisplayName != "carol@example.com" {
		t.Errorf("DisplayName = %q, want %q", createdUser.DisplayName, "carol@example.com")
	}
}

func TestOAuthLogin_ReturningUser(t *testing.T) {
	repo := &mockRepo{
		findOAuthIdentityFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return &domain.OAuthIdentity{ID: "oi-1", UserID: "existing-user-id", Provider: "google", ProviderID: "g-sub-123"}, nil
		},
		findUserByIDFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "existing-user-id", Email: "alice@example.com"}, nil
		},
		storeRefreshTokenFn: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}

	result, err := oauthSvc(repo).OAuthLogin(context.Background(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-123",
		Email:      "alice@example.com",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.User.ID != "existing-user-id" {
		t.Errorf("user ID = %q, want %q", result.User.ID, "existing-user-id")
	}
}

func TestOAuthLogin_ExistingEmailLinksIdentity(t *testing.T) {
	var upsertedIdentity *domain.OAuthIdentity

	repo := &mockRepo{
		findOAuthIdentityFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return nil, apperrors.NotFound("not found")
		},
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "u-existing", Email: "alice@example.com"}, nil
		},
		findOAuthIdentityByUserAndProviderFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return nil, apperrors.NotFound("no prior google link")
		},
		upsertOAuthIdentityFn: func(_ context.Context, identity *domain.OAuthIdentity) error {
			upsertedIdentity = identity
			return nil
		},
		storeRefreshTokenFn: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}

	result, err := oauthSvc(repo).OAuthLogin(context.Background(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-new",
		Email:      "alice@example.com",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.User.ID != "u-existing" {
		t.Errorf("user ID = %q, want %q", result.User.ID, "u-existing")
	}
	if upsertedIdentity == nil || upsertedIdentity.ProviderID != "g-sub-new" {
		t.Error("expected identity to be linked with new provider_id")
	}
}

func TestOAuthLogin_ConflictWhenEmailLinkedToDifferentAccount(t *testing.T) {
	repo := &mockRepo{
		findOAuthIdentityFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return nil, apperrors.NotFound("not found")
		},
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "u-existing", Email: "alice@example.com"}, nil
		},
		findOAuthIdentityByUserAndProviderFn: func(_ context.Context, _, _ string) (*domain.OAuthIdentity, error) {
			return &domain.OAuthIdentity{ProviderID: "g-sub-OTHER"}, nil
		},
		storeRefreshTokenFn: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}

	_, err := oauthSvc(repo).OAuthLogin(context.Background(), domain.OAuthLoginInput{
		Provider:   "google",
		ProviderID: "g-sub-DIFFERENT",
		Email:      "alice@example.com",
	})

	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != apperrors.CodeConflict {
		t.Errorf("code = %q, want %q", appErr.Code, apperrors.CodeConflict)
	}
}

func TestLogoutAll_RevokesSessionsAndBlocklistsToken(t *testing.T) {
	var revokedUserID string
	var blocklistedJTI string

	repo := &mockRepo{
		revokeAllUserTokensFn: func(_ context.Context, userID string) (int64, error) {
			revokedUserID = userID
			return 3, nil
		},
		blocklistTokenFn: func(_ context.Context, jti string, _ time.Duration) error {
			blocklistedJTI = jti
			return nil
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	err := svc.LogoutAll(context.Background(), domain.LogoutAllInput{
		UserID:         "u1",
		ActiveTokenJTI: "jti-xyz",
		ActiveTokenTTL: 15 * time.Minute,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revokedUserID != "u1" {
		t.Errorf("revokedUserID = %q, want %q", revokedUserID, "u1")
	}
	if blocklistedJTI != "jti-xyz" {
		t.Errorf("blocklistedJTI = %q, want %q", blocklistedJTI, "jti-xyz")
	}
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt hash: %v", err)
	}
	return string(h)
}

func TestChangePassword_Success(t *testing.T) {
	currentHash := mustHashPassword(t, "correcthorsebatterystaple")
	var updatedUserID string

	repo := &mockRepo{
		findCredentialByUserIDFn: func(_ context.Context, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: "u1", PasswordHash: currentHash}, nil
		},
		updateCredentialPasswordFn: func(_ context.Context, userID, _ string) error {
			updatedUserID = userID
			return nil
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	err := svc.ChangePassword(context.Background(), domain.ChangePasswordInput{
		UserID:      "u1",
		OldPassword: "correcthorsebatterystaple",
		NewPassword: "newpassword123",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updatedUserID != "u1" {
		t.Errorf("updatedUserID = %q, want %q", updatedUserID, "u1")
	}
}

func TestRegister_StoreCredentialError(t *testing.T) {
	repo := &mockRepo{
		createUserFn: func(_ context.Context, u *domain.User) error {
			u.ID = "u1"
			return nil
		},
		createCredentialFn: func(_ context.Context, _ *domain.Credential) error {
			return fmt.Errorf("postgres write failed")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	_, err := svc.Register(context.Background(), domain.RegisterInput{
		Email:    "alice@example.com",
		Password: "securepass",
	})
	if err == nil {
		t.Fatal("expected register to fail when credential store fails")
	}
}

func TestLogin_StoredUserButMissingCredential(t *testing.T) {
	repo := &mockRepo{
		findUserByEmailFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: "u1", Email: "alice@example.com"}, nil
		},
		findCredentialByUserIDFn: func(_ context.Context, _ string) (*domain.Credential, error) {
			return nil, apperrors.NotFound("credential not found")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	_, err := svc.Login(context.Background(), domain.LoginInput{Email: "alice@example.com", Password: "pass"})

	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeUnauthorized {
		t.Errorf("expected unauthorized when credential missing, got %v", err)
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	repo := &mockRepo{
		getCachedRefreshTokenFn: func(_ context.Context, _ string) (*domain.RefreshTokenMeta, error) {
			return nil, apperrors.NotFound("cache miss")
		},
		findRefreshTokenByHashFn: func(_ context.Context, _ string) (*domain.RefreshToken, error) {
			return nil, apperrors.NotFound("not found")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	_, err := svc.Refresh(context.Background(), domain.RefreshInput{RawToken: "unknown-token"})

	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeUnauthorized {
		t.Errorf("expected unauthorized for an unknown refresh token, got %v", err)
	}
}

func TestLogoutAll_RevokeError(t *testing.T) {
	repo := &mockRepo{
		revokeAllUserTokensFn: func(_ context.Context, _ string) (int64, error) {
			return 0, fmt.Errorf("postgres unavailable")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	err := svc.LogoutAll(context.Background(), domain.LogoutAllInput{UserID: "u1"})
	if err == nil {
		t.Fatal("expected logout-all to fail when revoke fails")
	}
}

func TestChangePassword_CredentialNotFound(t *testing.T) {
	repo := &mockRepo{
		findCredentialByUserIDFn: func(_ context.Context, _ string) (*domain.Credential, error) {
			return nil, apperrors.NotFound("credential not found")
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	err := svc.ChangePassword(context.Background(), domain.ChangePasswordInput{
		UserID:      "u1",
		OldPassword: "old",
		NewPassword: "newpassword123",
	})
	if err == nil {
		t.Fatal("expected change password to fail when credential is not found")
	}
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	currentHash := mustHashPassword(t, "correcthorsebatterystaple")

	repo := &mockRepo{
		findCredentialByUserIDFn: func(_ context.Context, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: "u1", PasswordHash: currentHash}, nil
		},
	}

	svc := auth.NewService(repo, okIssuer(), noopLogger())
	err := svc.ChangePassword(context.Background(), domain.ChangePasswordInput{
		UserID:      "u1",
		OldPassword: "wrongpassword",
		NewPassword: "newpassword123",
	})

	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != apperrors.CodeUnauthorized {
		t.Errorf("code = %q, want %q", appErr.Code, apperrors.CodeUnauthorized)
	}
}
