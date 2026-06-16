# authPlan.md — Pillow Authentication System

> **Living document.** Mark tasks `[x]` as they are completed. Every task maps directly to the design in `AuthDesign.md`. Tasks are ordered by dependency — complete each phase before starting the next.

---

## Status Legend

```
[ ] Not started
[~] In progress
[x] Complete
```

---

## Audit — What Is Already Shipped

| Component | Status | Location |
|---|---|---|
| Domain types & interfaces | [x] | `internal/domain/auth.go`, `user.go` |
| GORM models + mapping methods | [x] | `internal/infrastructure/postgres/models.go` |
| RS256 JWT issue + verify | [x] | `internal/infrastructure/tokenutil/jwt.go` |
| Postgres repository (all methods) | [x] | `internal/auth/repository.go` |
| Redis cache + blocklist | [x] | `internal/auth/repository.go` |
| Register flow | [x] | `internal/auth/service.go`, `handler.go` |
| Login with timing-attack mitigation | [x] | `internal/auth/service.go` |
| Refresh with family rotation + theft detection | [x] | `internal/auth/service.go` |
| Logout (single device) | [x] | `internal/auth/service.go`, `handler.go` |
| httpOnly refresh token cookie | [x] | `internal/auth/handler.go` |
| Device fingerprint stored on token | [x] | `internal/auth/service.go` |
| Auth middleware (JWT verify + blocklist) | [x] | `internal/middleware/auth.go` |
| CORS middleware | [x] | `internal/app/app.go` |
| Typed config + fail-fast startup | [x] | `internal/config/config.go` |
| AppError taxonomy + global error handler | [x] | `internal/apperrors/errors.go`, `internal/app/app.go` |
| Health endpoints (`/health/live`, `/health/ready`) | [x] | `internal/app/router.go` |
| All 5 DB migrations (up + down) | [x] | `migrations/` |
| OpenAPI 3.0 spec (auth endpoints) | [x] | `docs/api/openapi.yaml` |
| Swagger UI | [x] | `internal/app/docs.go` |
| Service-layer unit tests | [x] | `internal/auth/service_test.go` |

---

## Phase 1 — Security Hardening

> **Why first:** Rate limiting and the async audit logger are cross-cutting. They must be in place before OAuth or any external traffic is allowed. A login endpoint without rate limiting is a credential stuffing target on day one.

---

### 1.1 — Redis Sliding-Window Rate Limiter

**Design reference:** AuthDesign.md §7.6

Implement a reusable middleware in `internal/middleware/ratelimit.go` backed by a Redis sliding-window (ZADD + ZREMRANGEBYSCORE + ZCARD). The middleware is parameterised — each endpoint gets its own limit and window configuration.

**Rate limits to enforce:**

| Endpoint | Scope | Limit | Window |
|---|---|---|---|
| `POST /auth/login` | Per IP | 10 | 60 s |
| `POST /auth/login` | Per email (from body) | 5 | 900 s |
| `POST /auth/register` | Per IP | 3 | 60 s |
| `POST /auth/refresh` | Per IP | 30 | 60 s |
| `GET /auth/google` | Per IP | 20 | 60 s |

The per-email limiter must parse the request body to extract the email field. Use `c.Body()` + JSON decode; do not use `c.BodyParser` (it consumes the body and breaks downstream parsing — copy the body bytes back into the context).

Redis key format (matches `AuthDesign.md §8`):
```
ratelimit:ip:<ip>:<endpoint_slug>     → per-IP counters
ratelimit:email:<sha256(email)>:login → per-email counters (hash PII)
```

Exponential backoff: after the per-email limit is hit, double the lockout window on each subsequent violation. Store the backoff multiplier as a separate Redis key alongside the counter.

**Checklist:**

- [x] Add `RateLimiter`, `EmailRateLimiter`, `AuditLogger` interfaces to `internal/domain/auth.go`
- [x] Implement `redisRateLimiter` in `internal/infrastructure/redis/ratelimiter.go` using ZADD + ZREMRANGEBYSCORE + ZCARD Lua script (atomic sliding window)
- [x] Create `internal/middleware/ratelimit.go` — `LimitByIP` and `LimitByEmail` (SHA-256 hashes email, exponential backoff on repeated violations)
- [x] Set `Retry-After` response header on 429 responses
- [x] Apply limiters in `internal/app/router.go` to register, login, refresh routes
- [x] Add `RateLimitConfig` to `internal/config/config.go` with all window/limit fields
- [x] Document all rate limit env vars in `.env.example`
- [x] Update `docs/api/openapi.yaml` — added `429` to register; login and refresh already had it
- [x] Write unit tests: `internal/middleware/ratelimit_test.go` — allow, block, fail-open, email hashing, PII protection

---

### 1.2 — Async Audit Logger

**Design reference:** AuthDesign.md §9

Currently `LogEvent` is synchronous — a slow Postgres write adds latency to the login hot path. Replace with a channel-based goroutine that batches inserts every 100ms.

The writer must flush its buffer on shutdown (graceful shutdown in Phase 4 depends on this).

- [x] Created `internal/auth/auditlogger.go` — channel-buffered (256), 100ms ticker, batch of 50, flushes on ctx cancel
- [x] Removed `LogEvent` from `domain.AuthRepository`; `AuditLogger` interface injected into `authService` as a separate dependency
- [x] `auditLogger.Run(ctx)` started as goroutine in `internal/app/app.go`; `App.Shutdown()` calls `cancel()` and blocks on `loggerDone` channel until flush completes
- [x] `authService.NewService` updated to accept `domain.AuditLogger` as third parameter; all service tests updated with `mockAuditLogger`

---

### 1.3 — Real Readiness Probe

**Design reference:** AuthDesign.md §10.3

The current `/health/ready` always returns 200. It must actually verify connectivity.

- [x] `*gorm.DB` and `*goredis.Client` injected into `routeDeps` in `internal/app/router.go`
- [x] `/health/ready` pings both with a 2-second `context.WithTimeout`; returns 503 if either fails
- [x] `docs/api/openapi.yaml` already had `503` on `/health/ready` — confirmed present

---

## Phase 2 — Token Infrastructure

> **Why second:** The JWKS endpoint is a hard prerequisite for any downstream microservice that will verify tokens independently. Graceful shutdown is required before the service can be promoted to production.

---

### 2.1 — JWKS Endpoint

**Design reference:** AuthDesign.md §10.2

Expose `GET /.well-known/jwks.json` returning the RSA public key in JWK format. Multiple `kid`s must be supported to enable zero-downtime key rotation.

The response is cacheable — add `Cache-Control: public, max-age=300` so downstream services auto-refresh every 5 minutes without hammering the endpoint.

- [x] Added `JWK` struct and `PublicKeySet() []JWK` to `domain.TokenIssuer` interface
- [x] Implemented in `internal/infrastructure/tokenutil/jwt.go` — encodes RSA public key as JWK with `kty`, `use`, `alg`, `kid`, `n`, `e` fields (base64url-encoded modulus and exponent)
- [x] Created `GET /.well-known/jwks.json` route in `internal/app/router.go` with `Cache-Control: public, max-age=300`
- [x] Updated `docs/api/openapi.yaml` — added `/.well-known/jwks.json` endpoint under discovery tag, added `JWK` and `JWKSet` schemas
- [x] Wrote unit tests in `internal/infrastructure/tokenutil/jwt_test.go` — verified JWK `n` and `e` values decode back to original public key; verified round-trip correctness

---

### 2.2 — Graceful Shutdown

**Design reference:** AuthDesign.md §10.6

- [x] `app.Shutdown()` calls `fiberApp.ShutdownWithTimeout(10 * time.Second)` (Phase 1)
- [x] `context.CancelFunc` added to `App` struct to signal goroutines (Phase 1)
- [x] `auditLogger.Done()` returns `<-chan struct{}` for shutdown coordination (Phase 1)
- [x] Audit logger drains remaining events on context cancel (Phase 1)

---

### 2.3 — Token Cleanup Background Job

**Design reference:** AuthDesign.md §11

Expired and long-revoked refresh tokens accumulate. A nightly cleanup job keeps the table small and the partial index hot.

SQL:
```sql
DELETE FROM refresh_tokens
WHERE expires_at < NOW()
   OR (revoked_at IS NOT NULL AND revoked_at < NOW() - INTERVAL '90 days');
```

- [x] Created `internal/auth/cleanup.go` with `tokenCleanup` struct, `NewTokenCleanup()` constructor, `Run(ctx, interval)` ticker loop, and `deleteExpired()` method that logs rows deleted
- [x] Wired into `internal/app/app.go` — started as goroutine with 24-hour interval, respects shutdown context
- [ ] Consider a separate `cmd/worker/main.go` binary for cleanup if needed (currently co-located in API for simplicity)

---

## Phase 3 — Google OAuth 2.0 with PKCE

> **Why third:** OAuth depends on the rate limiter (Phase 1) being in place before the callback endpoint is reachable. It also requires the JWKS endpoint (Phase 2) to be operational since the OAuth login path issues the same Pillow token pair.

---

### 3.1 — OAuth Config + Dependencies

- [x] Added `golang.org/x/oauth2` and `golang.org/x/oauth2/google` to `go.mod`
- [x] `OAuthConfig` already has all fields (`GoogleClientID`, `GoogleClientSecret`, `GoogleRedirectURL`) — no schema change needed
- [x] Added `PKCEStore` interface to `internal/domain/auth.go` with `SaveVerifier`, `GetVerifier`, `DeleteVerifier`
- [x] Added `OAuthProvider` interface to `internal/domain/auth.go` with `BuildAuthURL` and `ExchangeAndVerify`
- [x] Implemented PKCE store methods on `authRepository` in `internal/auth/repository.go` (Redis key: `pkce:<state>`, TTL 10 minutes)
- [x] Created `internal/infrastructure/oauth/google.go` — wraps `golang.org/x/oauth2`, implements `domain.OAuthProvider`, decodes JWT claims without external JWKS fetch (iss + aud verification)

---

### 3.2 — OAuth Initiation Handler

**Endpoint:** `GET /auth/google`

- [x] Added `OAuthInitiate(c *fiber.Ctx) error` to `authHandler`
- [x] Generates 32-byte random `state` (base64url) and 64-byte random `code_verifier` (base64url, 86 chars — within RFC 7636 43–128 range)
- [x] Derives `code_challenge = BASE64URL(SHA-256(code_verifier))`
- [x] Persists `state → code_verifier` in Redis via `PKCEStore.SaveVerifier` (10-minute TTL)
- [x] Builds Google OAuth URL with `code_challenge_method=S256`, `access_type=offline`, `prompt=consent`
- [x] Returns `302 Redirect` to Google consent screen
- [x] Registered `GET /auth/google` in `internal/app/router.go` with IP rate limiter (20 req / 60 s)
- [x] Added `GET /auth/google` to `docs/api/openapi.yaml`

---

### 3.3 — OAuth Callback Handler

**Endpoint:** `GET /auth/google/callback`

- [x] Added `OAuthCallback(c *fiber.Ctx) error` to `authHandler`
- [x] Extracts `code` and `state` from query params; returns 400 if either is missing
- [x] Looks up `code_verifier` from Redis by `state`; returns 400 if not found or expired
- [x] Deletes `state` key immediately (one-time use guarantee)
- [x] Calls `oauthProvider.ExchangeAndVerify(ctx, code, verifier)` — exchanges code, parses id_token, verifies `iss` and `aud`
- [x] Google tokens discarded after claim extraction; Pillow issues its own RS256 pair
- [x] Account linking logic implemented in `authService.OAuthLogin`:
  - Existing (provider, provider_id) → returning user, issues token pair
  - Unknown provider_id, email exists, no prior Google link → links identity, issues token pair
  - Unknown provider_id, email exists, different Google sub already linked → `409 Conflict`
  - Unknown provider_id, new email → creates user + identity, issues token pair
- [x] Logs `oauth_login`, `oauth_link`, or `oauth_register` audit event accordingly
- [x] Sets `refresh_token` httpOnly cookie, returns `AuthResponse`
- [x] Registered `GET /auth/google/callback` in `internal/app/router.go`
- [x] Added both OAuth endpoints to `docs/api/openapi.yaml`

---

## Phase 4 — Extended Auth Endpoints

---

### 4.1 — Logout All Devices

**Endpoint:** `POST /auth/logout-all`

**Design reference:** AuthDesign.md §5.5

- [x] Added `LogoutAll(ctx context.Context, input domain.LogoutAllInput) error` to `domain.AuthService` (`LogoutAllInput` carries `UserID`, `ActiveTokenJTI`, `ActiveTokenTTL`)
- [x] Implemented in `internal/auth/service.go`:
  - Calls `repo.RevokeAllUserRefreshTokens(ctx, userID)` — sets `revoked_at` on all non-revoked rows in Postgres, returns affected count
  - Calls `repo.BlocklistToken(ctx, activeJTI, activeJTITTL)` — blocklists the caller's current access token
  - Logs `logout_all` audit event with `metadata: {sessions_revoked: N}`
- [x] Added `LogoutAll(c *fiber.Ctx) error` handler in `internal/auth/handler.go` (requires `RequireAuth` middleware, clears refresh cookie)
- [x] Registered `POST /auth/logout-all` in `internal/app/router.go` (protected)
- [x] Added endpoint to `docs/api/openapi.yaml`

---

### 4.2 — Password Change

**Endpoint:** `POST /auth/password`

**Design reference:** AuthDesign.md §9 (event type `password_change`)

- [x] Added `ChangePassword(ctx context.Context, input ChangePasswordInput) error` to `domain.AuthService`
  ```go
  type ChangePasswordInput struct {
      UserID      string
      OldPassword string
      NewPassword string
  }
  ```
- [x] Implemented in `internal/auth/service.go`:
  - Fetches credential by `userID`
  - `bcrypt.CompareHashAndPassword(oldPassword, hash)` — returns 401 if wrong
  - `bcrypt.GenerateFromPassword(newPassword, 12)` — hashes new password
  - Updates credential row via `repo.UpdateCredentialPassword`
  - Logs `password_change` audit event
  - **Does not revoke existing sessions** — password change is not logout; the user decides whether to call `logout-all`
- [x] Added `ChangePassword(c *fiber.Ctx) error` handler in `internal/auth/handler.go` (requires `RequireAuth`)
- [x] Added `ChangePasswordRequest` DTO to `internal/auth/dto.go`:
  ```go
  type ChangePasswordRequest struct {
      OldPassword string `json:"old_password" validate:"required"`
      NewPassword string `json:"new_password" validate:"required,min=8"`
  }
  ```
- [x] Migration: no schema change needed — the `credentials` table already has `updated_at`
- [x] Registered `POST /auth/password` in `internal/app/router.go` (protected)
- [x] Added endpoint to `docs/api/openapi.yaml`

---

## Phase 5 — Test Coverage

> **Why its own phase:** Every endpoint and service method must have tests before the system is production-eligible. The coverage targets are defined in `Folder-structure-rules.md §F4`: Domain 100%, Service 90%, Handler 70%.

---

### 5.1 — Complete Service Unit Tests

File: `internal/auth/service_test.go`

- [x] `TestRegister` — success, duplicate email
- [x] `TestLogin` — unknown email, wrong password
- [x] `TestLogout` — blocklists access token (`TestLogout_BlocklistsAccessToken`, `TestLogout_RevokesRefreshToken`)
- [x] `TestLogin_Success` — valid credentials return token pair
- [x] `TestRefresh_Success` — cache hit, rotates token pair
- [x] `TestRefresh_CacheMiss_PostgresFallback` — Redis miss, Postgres hit, succeeds
- [x] `TestRefresh_TheftDetected` — rotated token replayed → family revoked, 401 returned
- [x] `TestRefresh_FingerprintMismatch` — mismatched fingerprint → audit event logged, not blocked
- [x] `TestLogoutAll` — all tokens revoked, active JTI blocklisted (`TestLogoutAll_RevokesSessionsAndBlocklistsToken`)
- [x] `TestChangePassword_Success`
- [x] `TestChangePassword_WrongOldPassword`
- [x] `TestOAuthLogin_NewUser` — no existing account → creates user + identity
- [x] `TestOAuthLogin_ExistingUser_SameProvider` — existing identity → loads user, issues tokens (`TestOAuthLogin_ReturningUser`)
- [x] `TestOAuthLogin_ExistingUser_EmailMatch` — email match, new provider → links identity (`TestOAuthLogin_ExistingEmailLinksIdentity`)
- [x] `TestOAuthLogin_Conflict` — email linked to different provider_id → 409 (`TestOAuthLogin_ConflictWhenEmailLinkedToDifferentAccount`)
- [x] Service-layer statement coverage at 90.5% (≥ 90% target per `Folder-structure-rules.md §F4`); error-path tests added for store/issuer/blocklist failures

---

### 5.2 — Handler Unit Tests

File: `internal/auth/handler_test.go`

Use `net/http/httptest` + Fiber's `app.Test()` method. Inject a `mockAuthService`.

- [x] `TestRegisterHandler_Success` — 201, access token in body, Set-Cookie header present
- [x] `TestRegisterHandler_InvalidBody` — 400 on malformed JSON
- [x] `TestRegisterHandler_ValidationError` — 422 on missing required field
- [x] `TestRegisterHandler_ConflictError` — 409 propagated from service
- [x] `TestLoginHandler_Success` — 200, token pair returned
- [x] `TestLoginHandler_Unauthorized` — 401 propagated
- [x] `TestRefreshHandler_MissingCookie` — 401 when cookie absent
- [x] `TestRefreshHandler_Success` — 200, new cookie set
- [x] `TestLogoutHandler_MissingAuth` — 401 when no Bearer header
- [x] `TestLogoutHandler_Success` — 204, cookie cleared
- [x] Also covered: logout-all, password-change, and both OAuth handlers (Handler coverage 84.3%, ≥ 70% target)

---

### 5.3 — Middleware Unit Tests

File: `internal/middleware/auth_test.go`

- [x] `TestRequireAuth_ValidToken` — injects claims into context
- [x] `TestRequireAuth_MissingHeader` — returns 401
- [x] `TestRequireAuth_ExpiredToken` — returns 401
- [x] `TestRequireAuth_BlocklistedToken` — returns 401

File: `internal/middleware/ratelimit_test.go`

- [x] `TestLimitByIP_Allow` — under limit, passes through (`TestLimitByIP_AllowsUnderLimit`)
- [x] `TestLimitByIP_Block` — at limit, returns 429 with `Retry-After` (`TestLimitByIP_BlocksAtLimit`)
- [x] `TestLimitByEmail_EmailHashed` — verifies email is SHA-256 hashed in Redis key (PII protection) (`TestLimitByEmail_EmailIsHashed`)

---

### 5.4 — Integration Tests

Directory: `test/integration/`

Use `testcontainers-go` to spin up real Postgres and Redis.

> Integration tests are gated behind the `integration` build tag (`//go:build integration`) so the default `go test ./...` stays green without Docker. Run them with `go test -tags=integration ./test/integration/...` on a host with Docker.

- [x] Added `testcontainers-go` (+ postgres/redis modules) to `go.mod` (pinned `moby/go-archive v0.1.0` to resolve a Docker archive build conflict)
- [x] Created `test/testhelpers/containers.go`:
  - `NewPostgresContainer(t, ctx) (*gorm.DB, func())` — starts container, runs the versioned SQL migrations via `golang-migrate`, returns db + cleanup func
  - `NewRedisContainer(t, ctx) (*redis.Client, func())` — starts container, returns client + cleanup func
- [x] `test/integration/auth_register_test.go` — full round trip through real Postgres (user + credential persisted, bcrypt verifies, duplicate rejected)
- [x] `test/integration/auth_login_test.go` — login with real bcrypt + real Redis blocklist (logout blocklists the jti)
- [x] `test/integration/auth_refresh_test.go` — rotation against real Postgres + Redis; theft detection revokes the family
- [x] `test/integration/auth_oauth_test.go` — PKCE store round-trip in real Redis, OAuthLogin account-linking in real Postgres, and a stubbed Google token endpoint (`httptest.NewServer`) verifying the PKCE `code_verifier` round-trips

---

## Phase 6 — Observability

---

### 6.1 — Structured Request Logging

**Design reference:** AuthDesign.md §10.5

The current Fiber logger middleware logs basic request info. Add a custom logger that outputs the fields the design requires.

- [x] Replaced `logger.New()` with `middleware.RequestLogger` in `internal/middleware/requestlog.go` that emits `slog` JSON with: `trace_id` (UUID, generated per request), `user_id` (from claims if authenticated), `endpoint`, `method`, `status`, `latency_ms`, `ip` (redacted to /24 prefix when `APP_ENV=production`); wired in `internal/app/app.go`
- [x] Injects `trace_id` into `fiber.Ctx` locals (`middleware.TraceIDLocalsKey`) at the top of the middleware chain
- [x] Never logs raw tokens, passwords, or full emails — emails are logged as SHA-256 (`email_hash`) for correlation; covered by `requestlog_test.go`

---

### 6.2 — OpenAPI Spec Completion

The spec in `docs/api/openapi.yaml` must be updated as each endpoint above ships.

- [x] Add `POST /auth/logout-all` (Phase 4.1)
- [x] Add `POST /auth/password` (Phase 4.2)
- [x] Add `GET /auth/google` (Phase 3.2)
- [x] Add `GET /auth/google/callback` (Phase 3.3)
- [x] Add `GET /.well-known/jwks.json` with `JWKSet` schema (Phase 2.1)
- [x] Add `429` responses with `Retry-After` header to all rate-limited endpoints (Phase 1.1) — register, login, refresh, google
- [x] Add `503` to `/health/ready` (Phase 1.3)

---

## Dependency Graph

```
Phase 1 (Rate Limiting + Async Audit + Real Readiness)
    │
    ▼
Phase 2 (JWKS + Graceful Shutdown + Token Cleanup)
    │
    ▼
Phase 3 (Google OAuth — requires rate limiter + JWKS + token cleanup)
    │
    ▼
Phase 4 (Logout-All + Password Change — standalone, but test coverage comes next)
    │
    ▼
Phase 5 (Test Coverage — integration tests require all endpoints to exist)
    │
    ▼
Phase 6 (Observability — last because it wraps everything)
```

---

## Non-Negotiable Before Production

The following must all be `[x]` before the auth system handles real user traffic:

- [x] Rate limiting on all sensitive endpoints (Phase 1.1)
- [x] Real readiness probe (Phase 1.3)
- [x] JWKS endpoint (Phase 2.1)
- [x] Graceful shutdown (Phase 2.2)
- [x] Google OAuth PKCE (Phase 3 — if OAuth is in scope for launch)
- [x] Logout all devices (Phase 4.1)
- [x] Service test coverage ≥ 90% (Phase 5.1 + 5.2) — service.go 90.5%, handler.go 84.3%, middleware 93.9%
- [x] At least one integration test per auth flow (Phase 5.4) — register, login, refresh, oauth (run with `-tags=integration`)
- [x] No passwords, tokens, or raw emails in any log line (Phase 6.1)
