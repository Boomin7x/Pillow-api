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

- [ ] Add `RateLimiter` interface to `internal/domain/auth.go`:
  ```go
  type RateLimiter interface {
      Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
      Remaining(ctx context.Context, key string, limit int, window time.Duration) (int, error)
  }
  ```
- [ ] Implement `redisRateLimiter` in `internal/auth/repository.go` (or new `internal/infrastructure/redis/ratelimiter.go`) using ZADD + ZREMRANGEBYSCORE + ZCARD in a single Lua script (atomicity guarantee)
- [ ] Create `internal/middleware/ratelimit.go`:
  - `LimitByIP(store RateLimiter, limit int, window time.Duration) fiber.Handler`
  - `LimitByEmail(store RateLimiter, limit int, window time.Duration) fiber.Handler` (reads body, hashes email, resets body reader)
- [ ] Set `Retry-After` response header on 429 responses (seconds until window resets)
- [ ] Apply limiters in `internal/app/router.go` to each auth route
- [ ] Add rate limiter config to `internal/config/config.go`:
  ```go
  type RateLimitConfig struct {
      LoginIPLimit      int
      LoginEmailLimit   int
      RegisterIPLimit   int
      RefreshIPLimit    int
      OAuthIPLimit      int
  }
  ```
- [ ] Document new env vars in `.env.example`
- [ ] Update `docs/api/openapi.yaml` — add `429` responses with `Retry-After` header to login, register, refresh
- [ ] Write unit tests: `internal/middleware/ratelimit_test.go` — mock the `RateLimiter` interface, test allow/block/backoff paths

---

### 1.2 — Async Audit Logger

**Design reference:** AuthDesign.md §9

Currently `LogEvent` is synchronous — a slow Postgres write adds latency to the login hot path. Replace with a channel-based goroutine that batches inserts every 100ms.

The writer must flush its buffer on shutdown (graceful shutdown in Phase 4 depends on this).

- [ ] Create `internal/auth/auditlogger.go`:
  - `type auditLogger struct { ch chan *domain.AuthEvent; db *gorm.DB }`
  - `func NewAuditLogger(db *gorm.DB, bufSize int) *auditLogger`
  - `func (l *auditLogger) Log(event *domain.AuthEvent)` — non-blocking send on channel; drop + slog.Error if channel full
  - `func (l *auditLogger) Run(ctx context.Context)` — goroutine: select on 100ms ticker or batch size 50, INSERT all pending events in one query; exit cleanly when ctx is cancelled after flushing
- [ ] Change `domain.AuthRepository.LogEvent` signature to accept the logger as a dependency (or inject `*auditLogger` into `authRepository` alongside `*gorm.DB` and `*redis.Client`)
- [ ] Start `auditLogger.Run(ctx)` in `internal/app/app.go`; pass the cancel func to `App.Shutdown()`
- [ ] Write unit tests: verify that `Log()` is non-blocking under channel pressure and that `Run()` flushes all pending events before returning

---

### 1.3 — Real Readiness Probe

**Design reference:** AuthDesign.md §10.3

The current `/health/ready` always returns 200. It must actually verify connectivity.

- [ ] Inject `*gorm.DB` and `*redis.Client` into the readiness handler in `internal/app/router.go`
- [ ] Ping Postgres via `sqlDB.PingContext(ctx)` and Redis via `rdb.Ping(ctx)` with a 2-second timeout
- [ ] Return 503 with `AppError{Code: CodeInternal}` if either fails
- [ ] Update `docs/api/openapi.yaml` — add `503` response to `/health/ready`

---

## Phase 2 — Token Infrastructure

> **Why second:** The JWKS endpoint is a hard prerequisite for any downstream microservice that will verify tokens independently. Graceful shutdown is required before the service can be promoted to production.

---

### 2.1 — JWKS Endpoint

**Design reference:** AuthDesign.md §10.2

Expose `GET /.well-known/jwks.json` returning the RSA public key in JWK format. Multiple `kid`s must be supported to enable zero-downtime key rotation.

The response is cacheable — add `Cache-Control: public, max-age=300` so downstream services auto-refresh every 5 minutes without hammering the endpoint.

- [ ] Add `PublicKeySet() ([]JWK, error)` to `domain.TokenIssuer` interface
- [ ] Implement in `internal/infrastructure/tokenutil/jwt.go`:
  - Encode `rsa.PublicKey` as JWK (`kty`, `use`, `alg`, `kid`, `n`, `e`)
  - Return as `[]JWK` — slice supports serving multiple keys during rotation window
- [ ] Create `GET /.well-known/jwks.json` route in `internal/app/router.go`:
  ```go
  f.Get("/.well-known/jwks.json", func(c *fiber.Ctx) error {
      c.Set("Cache-Control", "public, max-age=300")
      keys, err := deps.issuer.PublicKeySet()
      ...
      return c.JSON(fiber.Map{"keys": keys})
  })
  ```
- [ ] Update `docs/api/openapi.yaml` — document the JWKS endpoint and JWK schema
- [ ] Write unit test: verify JWK `n` and `e` values decode back to the original public key modulus and exponent

---

### 2.2 — Graceful Shutdown

**Design reference:** AuthDesign.md §10.6

- [ ] Change `app.Shutdown()` to call `fiberApp.ShutdownWithTimeout(10 * time.Second)`
- [ ] Add a `context.CancelFunc` to `App` struct to signal the audit logger goroutine to flush and stop
- [ ] In `cmd/api/main.go`: on SIGTERM, call `a.Shutdown()` and `cancel()` in order; wait for the audit logger to confirm flush completion before `os.Exit`
- [ ] Add a `Done() <-chan struct{}` method to `auditLogger` so `main.go` can block until flush is complete

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

- [ ] Create `internal/auth/cleanup.go`:
  - `type tokenCleanup struct { db *gorm.DB }`
  - `func NewTokenCleanup(db *gorm.DB) *tokenCleanup`
  - `func (c *tokenCleanup) Run(ctx context.Context, interval time.Duration)` — ticker loop, runs SQL above, logs rows deleted and duration
- [ ] Wire into `internal/app/app.go` — start as a goroutine, respect shutdown context
- [ ] Consider a separate `cmd/worker/main.go` binary for this if the cleanup interval is daily (preferred for separation of concerns — the API binary doesn't need a scheduler)

---

## Phase 3 — Google OAuth 2.0 with PKCE

> **Why third:** OAuth depends on the rate limiter (Phase 1) being in place before the callback endpoint is reachable. It also requires the JWKS endpoint (Phase 2) to be operational since the OAuth login path issues the same Pillow token pair.

---

### 3.1 — OAuth Config + Dependencies

- [ ] Add `golang.org/x/oauth2` and `golang.org/x/oauth2/google` to `go.mod` (`go get golang.org/x/oauth2`)
- [ ] Extend `internal/config/config.go` — `OAuthConfig` already has the fields; verify `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URL` are required (currently optional — make them required or conditionally required)
- [ ] Add a `PKCEStore` interface to `internal/domain/auth.go` for storing the `code_verifier` between the initiation and callback:
  ```go
  type PKCEStore interface {
      SaveVerifier(ctx context.Context, state string, verifier string, ttl time.Duration) error
      GetVerifier(ctx context.Context, state string) (string, error)
      DeleteVerifier(ctx context.Context, state string) error
  }
  ```
- [ ] Implement `redisPKCEStore` in `internal/auth/repository.go` (Redis keys: `pkce:<state>`, TTL 10 minutes)

---

### 3.2 — OAuth Initiation Handler

**Endpoint:** `GET /auth/google`

- [ ] Add `OAuthInitiate(c *fiber.Ctx) error` to `authHandler`
- [ ] Generate a cryptographically random `state` (32 bytes, base64url) — CSRF protection
- [ ] Generate a cryptographically random `code_verifier` (43–128 chars, base64url, RFC 7636 §4.1)
- [ ] Derive `code_challenge = BASE64URL(SHA256(code_verifier))`
- [ ] Persist `state → code_verifier` in Redis via `PKCEStore.SaveVerifier` (TTL 10 minutes)
- [ ] Build Google OAuth URL with:
  - `response_type=code`
  - `code_challenge_method=S256`
  - `code_challenge=<derived>`
  - `state=<random>`
  - `access_type=offline`
  - `prompt=consent`
- [ ] Return `302 Redirect` to the Google consent screen
- [ ] Register `GET /auth/google` in `internal/app/router.go` with the IP rate limiter (Phase 1.1)
- [ ] Add `GET /auth/google` to `docs/api/openapi.yaml`

---

### 3.3 — OAuth Callback Handler

**Endpoint:** `GET /auth/google/callback`

- [ ] Add `OAuthCallback(c *fiber.Ctx) error` to `authHandler`
- [ ] Extract `code` and `state` query params; return 400 if either is missing
- [ ] Look up `code_verifier` from Redis by `state`; return 400 if not found or expired
- [ ] Delete the `state` key immediately (one-time use)
- [ ] Exchange `code` + `code_verifier` for Google tokens using `golang.org/x/oauth2`
- [ ] Verify the Google `id_token`:
  - Validate audience matches `GOOGLE_CLIENT_ID`
  - Validate `iss` is `accounts.google.com` or `https://accounts.google.com`
  - Extract `sub` (provider_id) and `email`
- [ ] Discard Google tokens — Pillow issues its own pair
- [ ] Account linking logic (all in `authService.OAuthLogin`):
  1. Look up `oauth_identities` by `(provider=google, provider_id=<sub>)`
  2. **Found:** load the linked user, issue Pillow token pair
  3. **Not found, email matches existing user:** link the identity, issue token pair
  4. **Not found, no email match:** create new `users` row, create `oauth_identities` row, issue token pair
  5. **Email linked to a different Google account:** return 409 Conflict
- [ ] Add `OAuthLogin(ctx, OAuthLoginInput) (*AuthResult, error)` to `domain.AuthService` and implement in `internal/auth/service.go`
- [ ] Log `oauth_login` or `oauth_link` audit event
- [ ] Set refresh cookie, return `AuthResponse`
- [ ] Register `GET /auth/google/callback` in `internal/app/router.go`
- [ ] Add both OAuth endpoints to `docs/api/openapi.yaml`

---

## Phase 4 — Extended Auth Endpoints

---

### 4.1 — Logout All Devices

**Endpoint:** `POST /auth/logout-all`

**Design reference:** AuthDesign.md §5.5

- [ ] Add `LogoutAll(ctx context.Context, userID string, activeJTI string, activeJTITTL time.Duration) error` to `domain.AuthService`
- [ ] Implement in `internal/auth/service.go`:
  - Call `repo.RevokeAllUserRefreshTokens(ctx, userID)` — sets `revoked_at` on all non-revoked rows in Postgres
  - Call `repo.BlocklistToken(ctx, activeJTI, activeJTITTL)` — blocklist the caller's current access token
  - Log `logout_all` audit event with `metadata: {sessions_revoked: N}`
- [ ] Add `LogoutAll(c *fiber.Ctx) error` handler in `internal/auth/handler.go` (requires `RequireAuth` middleware)
- [ ] Register `POST /auth/logout-all` in `internal/app/router.go` (protected)
- [ ] Add endpoint to `docs/api/openapi.yaml`

---

### 4.2 — Password Change

**Endpoint:** `POST /auth/password`

**Design reference:** AuthDesign.md §9 (event type `password_change`)

- [ ] Add `ChangePassword(ctx context.Context, input ChangePasswordInput) error` to `domain.AuthService`
  ```go
  type ChangePasswordInput struct {
      UserID      string
      OldPassword string
      NewPassword string
  }
  ```
- [ ] Implement in `internal/auth/service.go`:
  - Fetch credential by `userID`
  - `bcrypt.Compare(oldPassword, hash)` — return 401 if wrong
  - `bcrypt.Generate(newPassword, 12)` — hash new password
  - Update credential row
  - Log `password_change` audit event
  - **Do not revoke existing sessions** — password change is not logout; let the user decide if they want `logout-all`
- [ ] Add `ChangePassword(c *fiber.Ctx) error` handler in `internal/auth/handler.go` (requires `RequireAuth`)
- [ ] Add `ChangePasswordRequest` DTO to `internal/auth/dto.go`:
  ```go
  type ChangePasswordRequest struct {
      OldPassword string `json:"old_password" validate:"required"`
      NewPassword string `json:"new_password" validate:"required,min=8"`
  }
  ```
- [ ] Add migration: no schema change needed — the `credentials` table already has `updated_at`
- [ ] Register `POST /auth/password` in `internal/app/router.go` (protected)
- [ ] Add endpoint to `docs/api/openapi.yaml`

---

## Phase 5 — Test Coverage

> **Why its own phase:** Every endpoint and service method must have tests before the system is production-eligible. The coverage targets are defined in `Folder-structure-rules.md §F4`: Domain 100%, Service 90%, Handler 70%.

---

### 5.1 — Complete Service Unit Tests

File: `internal/auth/service_test.go`

- [x] `TestRegister` — success, duplicate email
- [x] `TestLogin` — unknown email, wrong password
- [x] `TestLogout` — blocklists access token
- [ ] `TestLogin_Success` — valid credentials return token pair
- [ ] `TestRefresh_Success` — cache hit, rotates token pair
- [ ] `TestRefresh_CacheMiss_PostgresFallback` — Redis miss, Postgres hit, succeeds
- [ ] `TestRefresh_TheftDetected` — rotated token replayed → family revoked, 401 returned
- [ ] `TestRefresh_FingerprintMismatch` — mismatched fingerprint → audit event logged, not blocked
- [ ] `TestLogoutAll` — all tokens revoked, active JTI blocklisted
- [ ] `TestChangePassword_Success`
- [ ] `TestChangePassword_WrongOldPassword`
- [ ] `TestOAuthLogin_NewUser` — no existing account → creates user + identity
- [ ] `TestOAuthLogin_ExistingUser_SameProvider` — existing identity → loads user, issues tokens
- [ ] `TestOAuthLogin_ExistingUser_EmailMatch` — email match, new provider → links identity
- [ ] `TestOAuthLogin_Conflict` — email linked to different provider_id → 409

---

### 5.2 — Handler Unit Tests

File: `internal/auth/handler_test.go`

Use `net/http/httptest` + Fiber's `app.Test()` method. Inject a `mockAuthService`.

- [ ] `TestRegisterHandler_Success` — 201, access token in body, Set-Cookie header present
- [ ] `TestRegisterHandler_InvalidBody` — 400 on malformed JSON
- [ ] `TestRegisterHandler_ValidationError` — 422 on missing required field
- [ ] `TestRegisterHandler_ConflictError` — 409 propagated from service
- [ ] `TestLoginHandler_Success` — 200, token pair returned
- [ ] `TestLoginHandler_Unauthorized` — 401 propagated
- [ ] `TestRefreshHandler_MissingCookie` — 401 when cookie absent
- [ ] `TestRefreshHandler_Success` — 200, new cookie set
- [ ] `TestLogoutHandler_MissingAuth` — 401 when no Bearer header
- [ ] `TestLogoutHandler_Success` — 204, cookie cleared

---

### 5.3 — Middleware Unit Tests

File: `internal/middleware/auth_test.go`

- [ ] `TestRequireAuth_ValidToken` — injects claims into context
- [ ] `TestRequireAuth_MissingHeader` — returns 401
- [ ] `TestRequireAuth_ExpiredToken` — returns 401
- [ ] `TestRequireAuth_BlocklistedToken` — returns 401

File: `internal/middleware/ratelimit_test.go`

- [ ] `TestLimitByIP_Allow` — under limit, passes through
- [ ] `TestLimitByIP_Block` — at limit, returns 429 with `Retry-After`
- [ ] `TestLimitByEmail_EmailHashed` — verifies email is SHA-256 hashed in Redis key (PII protection)

---

### 5.4 — Integration Tests

Directory: `test/integration/`

Use `testcontainers-go` to spin up real Postgres and Redis.

- [ ] Add `testcontainers-go` to `go.mod`
- [ ] Create `test/testhelpers/containers.go`:
  - `NewPostgresContainer(t, ctx) (*gorm.DB, func())` — starts container, runs migrations, returns db + cleanup func
  - `NewRedisContainer(t, ctx) (*redis.Client, func())` — starts container, returns client + cleanup func
- [ ] `test/integration/auth_register_test.go` — full round trip through real Postgres
- [ ] `test/integration/auth_login_test.go` — login with real bcrypt + real Redis blocklist
- [ ] `test/integration/auth_refresh_test.go` — rotation against real Postgres + Redis; theft detection path
- [ ] `test/integration/auth_oauth_test.go` — stub Google's token endpoint using `httptest.NewServer`; verify full PKCE flow

---

## Phase 6 — Observability

---

### 6.1 — Structured Request Logging

**Design reference:** AuthDesign.md §10.5

The current Fiber logger middleware logs basic request info. Add a custom logger that outputs the fields the design requires.

- [ ] Replace `logger.New()` with a custom Fiber middleware in `internal/middleware/requestlog.go` that emits `slog` JSON with: `trace_id` (UUID, generated per request), `user_id` (from claims if authenticated), `endpoint`, `method`, `status`, `latency_ms`, `ip` (redacted to /24 prefix in production)
- [ ] Inject `trace_id` into `fiber.Ctx` locals at the top of the middleware chain so downstream handlers can attach it to log lines
- [ ] Never log: raw tokens, passwords, full email addresses in production (log SHA-256 of email for correlation)

---

### 6.2 — OpenAPI Spec Completion

The spec in `docs/api/openapi.yaml` must be updated as each endpoint above ships.

- [ ] Add `POST /auth/logout-all` (Phase 4.1)
- [ ] Add `POST /auth/password` (Phase 4.2)
- [ ] Add `GET /auth/google` (Phase 3.2)
- [ ] Add `GET /auth/google/callback` (Phase 3.3)
- [ ] Add `GET /.well-known/jwks.json` with `JWKSet` schema (Phase 2.1)
- [ ] Add `429` responses with `Retry-After` header to all rate-limited endpoints (Phase 1.1)
- [ ] Add `503` to `/health/ready` (Phase 1.3)

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

- [ ] Rate limiting on all sensitive endpoints (Phase 1.1)
- [ ] Real readiness probe (Phase 1.3)
- [ ] JWKS endpoint (Phase 2.1)
- [ ] Graceful shutdown (Phase 2.2)
- [ ] Google OAuth PKCE (Phase 3 — if OAuth is in scope for launch)
- [ ] Logout all devices (Phase 4.1)
- [ ] Service test coverage ≥ 90% (Phase 5.1 + 5.2)
- [ ] At least one integration test per auth flow (Phase 5.4)
- [ ] No passwords, tokens, or raw emails in any log line (Phase 6.1)
