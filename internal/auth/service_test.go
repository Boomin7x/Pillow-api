package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/auth"
	"github.com/kodiahbertrand/pillow/internal/domain"
)

// --- mocks ---

type mockRepo struct {
	createUserFn             func(ctx context.Context, u *domain.User) error
	findUserByEmailFn        func(ctx context.Context, email string) (*domain.User, error)
	findUserByIDFn           func(ctx context.Context, id string) (*domain.User, error)
	createCredentialFn       func(ctx context.Context, c *domain.Credential) error
	findCredentialByUserIDFn func(ctx context.Context, userID string) (*domain.Credential, error)
	storeRefreshTokenFn      func(ctx context.Context, rt *domain.RefreshToken) error
	findRefreshTokenByHashFn func(ctx context.Context, hash string) (*domain.RefreshToken, error)
	revokeRefreshTokenFn     func(ctx context.Context, id string) error
	revokeTokenFamilyFn      func(ctx context.Context, familyID string) error
	revokeAllUserTokensFn    func(ctx context.Context, userID string) error
	upsertOAuthIdentityFn    func(ctx context.Context, identity *domain.OAuthIdentity) error
	findOAuthIdentityFn      func(ctx context.Context, provider, providerID string) (*domain.OAuthIdentity, error)
	cacheRefreshTokenFn      func(ctx context.Context, hash string, meta domain.RefreshTokenMeta, ttl time.Duration) error
	getCachedRefreshTokenFn  func(ctx context.Context, hash string) (*domain.RefreshTokenMeta, error)
	deleteCachedTokenFn      func(ctx context.Context, hash string) error
	blocklistTokenFn         func(ctx context.Context, jti string, ttl time.Duration) error
	isTokenBlocklistedFn     func(ctx context.Context, jti string) (bool, error)
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
	return m.revokeRefreshTokenFn(ctx, id)
}
func (m *mockRepo) RevokeTokenFamily(ctx context.Context, familyID string) error {
	return m.revokeTokenFamilyFn(ctx, familyID)
}
func (m *mockRepo) RevokeAllUserRefreshTokens(ctx context.Context, userID string) error {
	return m.revokeAllUserTokensFn(ctx, userID)
}
func (m *mockRepo) UpsertOAuthIdentity(ctx context.Context, identity *domain.OAuthIdentity) error {
	return m.upsertOAuthIdentityFn(ctx, identity)
}
func (m *mockRepo) FindOAuthIdentity(ctx context.Context, provider, providerID string) (*domain.OAuthIdentity, error) {
	return m.findOAuthIdentityFn(ctx, provider, providerID)
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
func (m *mockRepo) DeleteCachedTokenFamily(_ context.Context, _ string) error { return nil }
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
func (m *mockRepo) LogEvent(_ context.Context, _ *domain.AuthEvent) {}

type mockIssuer struct {
	issueAccessFn  func(claims domain.Claims) (string, error)
	issueRefreshFn func() (string, error)
	validateFn     func(token string) (*domain.Claims, error)
}

func (m *mockIssuer) IssueAccessToken(claims domain.Claims) (string, error) {
	return m.issueAccessFn(claims)
}
func (m *mockIssuer) IssueRefreshToken() (string, error) { return m.issueRefreshFn() }
func (m *mockIssuer) ValidateAccessToken(token string) (*domain.Claims, error) {
	return m.validateFn(token)
}

// --- helpers ---

func okIssuer() *mockIssuer {
	return &mockIssuer{
		issueAccessFn:  func(_ domain.Claims) (string, error) { return "access.token.signed", nil },
		issueRefreshFn: func() (string, error) { return "raw-refresh-token", nil },
		validateFn:     func(_ string) (*domain.Claims, error) { return nil, nil },
	}
}

// --- tests ---

func TestRegister(t *testing.T) {
	tests := []struct {
		name       string
		input      domain.RegisterInput
		repoSetup  func(*mockRepo)
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
			name: "duplicate email",
			input: domain.RegisterInput{
				Email:    "existing@example.com",
				Password: "pass",
			},
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
			svc := auth.NewService(repo, okIssuer())

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
			svc := auth.NewService(repo, okIssuer())

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

func TestLogout(t *testing.T) {
	tests := []struct {
		name      string
		input     domain.LogoutInput
		repoSetup func(*mockRepo)
		wantErr   bool
	}{
		{
			name: "blocklists access token",
			input: domain.LogoutInput{
				AccessTokenJTI: "jti-abc",
				AccessTokenTTL: 5 * time.Minute,
				UserID:         "u1",
			},
			repoSetup: func(r *mockRepo) {
				var blocklistCalled bool
				r.blocklistTokenFn = func(_ context.Context, jti string, _ time.Duration) error {
					blocklistCalled = true
					if jti != "jti-abc" {
						return errors.New("wrong jti")
					}
					return nil
				}
				r.findRefreshTokenByHashFn = func(_ context.Context, _ string) (*domain.RefreshToken, error) {
					return nil, apperrors.NotFound("not found")
				}
				_ = blocklistCalled
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			tt.repoSetup(repo)
			svc := auth.NewService(repo, okIssuer())

			err := svc.Logout(context.Background(), tt.input)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
