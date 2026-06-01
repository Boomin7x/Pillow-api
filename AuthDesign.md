# AuthDesign.md — Pillow Authentication System

> **Project:** Pillow (Zillow-like real estate platform)
> **Stack:** Go · Fiber · GORM · PostgreSQL · Redis
> **Author:** System Design — Elite Architecture Review
> **Version:** 1.0.0

---

## Table of Contents

1. [Design Philosophy](#1-design-philosophy)
2. [System Overview](#2-system-overview)
3. [Architecture Layers](#3-architecture-layers)
   - 3.1 [API Gateway — Fiber Middleware Chain](#31-api-gateway--fiber-middleware-chain)
   - 3.2 [Auth Service](#32-auth-service)
   - 3.3 [Data Layer](#33-data-layer)
   - 3.4 [OAuth Broker](#34-oauth-broker)
4. [Token Strategy](#4-token-strategy)
   - 4.1 [Access Token (JWT · RS256)](#41-access-token-jwt--rs256)
   - 4.2 [Refresh Token (Opaque)](#42-refresh-token-opaque)
   - 4.3 [Token Lifecycle](#43-token-lifecycle)
   - 4.4 [RS256 vs HS256 — Decision Rationale](#44-rs256-vs-hs256--decision-rationale)
5. [Authentication Flows](#5-authentication-flows)
   - 5.1 [Email + Password Registration](#51-email--password-registration)
   - 5.2 [Email + Password Login](#52-email--password-login)
   - 5.3 [Google OAuth 2.0 (PKCE)](#53-google-oauth-20-pkce)
   - 5.4 [Token Refresh with Family Rotation](#54-token-refresh-with-family-rotation)
   - 5.5 [Logout and Revocation](#55-logout-and-revocation)
6. [Data Model](#6-data-model)
   - 6.1 [Table Definitions](#61-table-definitions)
   - 6.2 [Index Strategy](#62-index-strategy)
7. [Security Mechanisms](#7-security-mechanisms)
   - 7.1 [Password Hashing (bcrypt)](#71-password-hashing-bcrypt)
   - 7.2 [PKCE on OAuth](#72-pkce-on-oauth)
   - 7.3 [Device Fingerprinting](#73-device-fingerprinting)
   - 7.4 [Refresh Token Family Rotation](#74-refresh-token-family-rotation)
   - 7.5 [Token Blocklist](#75-token-blocklist)
   - 7.6 [Rate Limiting](#76-rate-limiting)
8. [Redis Usage Map](#8-redis-usage-map)
9. [Audit Logging](#9-audit-logging)
10. [Operational Standards](#10-operational-standards)
    - 10.1 [RS256 Key Rotation](#101-rs256-key-rotation)
    - 10.2 [JWKS Endpoint](#102-jwks-endpoint)
    - 10.3 [Health Checks](#103-health-checks)
    - 10.4 [Secrets Management](#104-secrets-management)
    - 10.5 [Structured Logging](#105-structured-logging)
    - 10.6 [Graceful Shutdown](#106-graceful-shutdown)
11. [Scalability Considerations](#11-scalability-considerations)
12. [Decision Log](#12-decision-log)

---

## 1. Design Philosophy

The Pillow auth system is built on three non-negotiable principles:

**Zero-trust at every boundary.** No service trusts any other by default. Every request carries a cryptographically signed token that is verified in-memory at the gateway — no database round-trip on the hot path. Internal services receive a validated claims object, never a raw token.

**Separation of concerns between identity and session.** The `users` table owns identity. The `credentials` table owns secrets. The `refresh_tokens` table owns session state. These are never conflated. An OAuth-only user has no row in `credentials`. A user with multiple devices has multiple rows in `refresh_tokens`. Each concern can evolve independently.

**Horizontal scalability from day one.** The Auth Service is stateless — all session state lives in Postgres and Redis. Any number of Auth Service instances can run behind a load balancer with no sticky sessions required. Redis handles fast-path lookups; Postgres is the system of record.

---

## 2. System Overview

```
┌─────────────────────────────────────────────────────────┐
│                        CLIENTS                          │
│          Web (React/Next.js)   ·   Mobile (iOS/Android) │
└────────────────────┬────────────────────────────────────┘
                     │ HTTPS
┌────────────────────▼────────────────────────────────────┐
│                    API GATEWAY  (Fiber)                  │
│  Rate Limiter → Token Extractor → Sig Verifier → Claims │
└────────────────────┬────────────────────────────────────┘
                     │ internal
┌────────────────────▼────────────────────────────────────┐
│                    AUTH SERVICE  (Go)                    │
│  Register · Login · Refresh · Revoke · OAuth Callback   │
│  Token Factory · Device Validator · Audit Logger        │
└──────────┬─────────────────────────┬────────────────────┘
           │                         │
┌──────────▼──────────┐   ┌──────────▼──────────────────┐
│   PostgreSQL (GORM) │   │          Redis               │
│  users              │   │  refresh token cache (30d)   │
│  credentials        │   │  token blocklist             │
│  refresh_tokens     │   │  rate limit counters         │
│  oauth_identities   │   │  login attempt counters      │
│  auth_events        │   └─────────────────────────────┘
└─────────────────────┘
           │
┌──────────▼──────────────────────────────────────────────┐
│                    OAUTH BROKER                          │
│   PKCE Init  →  Google Callback  →  Account Linker      │
│                          ↓                              │
│              issues Pillow token pair                   │
└─────────────────────────────────────────────────────────┘
```

---

## 3. Architecture Layers

### 3.1 API Gateway — Fiber Middleware Chain

Every inbound request passes through a strict, ordered middleware chain before reaching any handler. No handler ever touches raw token logic.

| Order | Middleware | Responsibility |
|---|---|---|
| 1 | **Rate Limiter** | Per-IP and per-account limits on sensitive endpoints |
| 2 | **Token Extractor** | Reads Bearer header or `httpOnly` cookie |
| 3 | **Sig Verifier** | Validates RS256 signature against JWKS public key (in-memory, no DB) |
| 4 | **Claims Injector** | Writes `user_id`, `roles`, `session_id` into `fiber.Ctx` |

**Why this order matters:** Rate limiting fires before any crypto work — a flood of invalid requests never reaches the verifier. The verifier fires before claims injection — a tampered token never pollutes the context.

### 3.2 Auth Service

The Auth Service is the sole owner of all identity and session logic. No other service in the Pillow platform issues or validates tokens.

| Handler | Description |
|---|---|
| `POST /auth/register` | Validates email uniqueness, hashes password (bcrypt cost 12), creates `users` + `credentials` rows, issues token pair |
| `POST /auth/login` | Fetches credential, calls `bcrypt.CompareHashAndPassword`, issues token pair on success |
| `POST /auth/refresh` | Validates opaque refresh token via Redis, checks device fingerprint, performs family rotation, issues new pair |
| `POST /auth/logout` | Adds access token to blocklist, deletes refresh token from Redis + Postgres |
| `GET /auth/google` | Initiates PKCE flow, redirects to Google consent screen |
| `GET /auth/google/callback` | Exchanges code, fetches userinfo, upserts `oauth_identities`, issues Pillow token pair |

Internal components within the Auth Service:

- **Token Factory** — generates RS256-signed JWTs (access) and cryptographically random opaque strings (refresh). Never called outside the Auth Service.
- **Device Validator** — hashes `User-Agent + IP` at token issuance. Validates the hash on every refresh call. A mismatch does not hard-block (mobile IPs change) but is logged and can trigger step-up auth.
- **Audit Logger** — writes to `auth_events` on every significant event. Non-blocking async write using Go channels; never in the hot path.

### 3.3 Data Layer

**PostgreSQL** is the system of record for all identity and session data. GORM is used for schema management and querying. All tables use UUID v7 primary keys — time-sortable, which keeps B-tree indexes cache-warm as new rows are appended.

**Redis** serves as the fast-path cache and ephemeral state store. It never holds the canonical truth — Postgres does. If Redis is wiped, sessions are invalidated gracefully (users re-authenticate); no data is permanently lost.

### 3.4 OAuth Broker

The broker handles the Google OAuth 2.0 Authorization Code flow with PKCE. On callback it immediately exchanges the authorization code for a Google `id_token`, verifies it, then discards it. Pillow never uses Google's token beyond this point — it issues its own token pair. This keeps the session model uniform: the rest of the platform never needs to know whether a user authenticated via email or Google.

Account linking strategy:

- If the Google email matches an existing Pillow account → link the OAuth identity, issue tokens for the existing account.
- If no match → create a new `users` row, create an `oauth_identities` row, issue tokens.
- If the email is already linked to a different Google account → return a 409 Conflict with a clear error message.

---

## 4. Token Strategy

### 4.1 Access Token (JWT · RS256)

```
Header:  { "alg": "RS256", "kid": "<key-id>", "typ": "JWT" }
Payload: {
  "sub":  "<user_uuid>",
  "email": "user@example.com",
  "roles": ["buyer"],
  "sid":   "<session_uuid>",
  "iat":   <unix>,
  "exp":   <unix + 900>   // 15 minutes
}
```

Verified in-memory at the gateway using the RS256 public key. No database lookup on the hot path. The `kid` field allows the gateway to select the correct key during key rotation windows.

### 4.2 Refresh Token (Opaque)

A 256-bit cryptographically random string (`crypto/rand`). Its SHA-256 hash is stored in Postgres (`refresh_tokens.token_hash`) and cached in Redis with a 30-day TTL. The raw token is only ever sent to the client via `httpOnly`, `Secure`, `SameSite=Strict` cookie. It is never stored in localStorage.

Additional metadata stored per refresh token:

| Field | Purpose |
|---|---|
| `family_id` | Groups all tokens issued from the same login session — enables family-wide revocation |
| `device_fingerprint` | SHA-256 of `User-Agent + IP` at issuance — validated on refresh |
| `expires_at` | Used for Postgres cleanup jobs |
| `revoked_at` | Set on logout or theft detection |

### 4.3 Token Lifecycle

```
Login / OAuth
    │
    ▼
Token Factory issues pair
    │
    ├──► Access JWT (15 min) ──► Client memory / Authorization header
    │
    └──► Refresh token (30d) ──► httpOnly cookie + Redis cache + Postgres
                │
                │ (access token expires after 15 min)
                ▼
         Client sends refresh token to POST /auth/refresh
                │
                ├── Redis lookup: token valid? device fingerprint match?
                │
                ├── YES → Family rotation: old token revoked, new pair issued
                │
                └── NO (old token replayed) → THEFT DETECTED
                         │
                         └── Entire token family blocklisted
                             All sessions for this family revoked
                             Audit event written
```

### 4.4 RS256 vs HS256 — Decision Rationale

| Criterion | RS256 (chosen) | HS256 |
|---|---|---|
| Key type | Asymmetric (private + public) | Symmetric (shared secret) |
| Who can verify | Any service with the public key | Only services that know the secret |
| Blast radius if downstream compromised | Zero — they only hold the public key | Full — shared secret is exposed |
| Key distribution | JWKS endpoint (`/.well-known/jwks.json`) | Out-of-band secret distribution |
| Key rotation | Smooth — serve multiple `kid`s simultaneously | Disruptive — secret must be updated everywhere atomically |
| Industry standard | Auth0, AWS Cognito, Google Identity | Internal monolith use cases |

**Decision:** RS256. As Pillow grows into a microservices architecture (listings service, search service, messaging service), each service verifies tokens independently using the public key. No shared secret means no shared blast radius.

---

## 5. Authentication Flows

### 5.1 Email + Password Registration

```
Client                Auth Service              Postgres
  │                        │                        │
  │── POST /auth/register ─►│                        │
  │   { email, password }   │                        │
  │                        │── SELECT users WHERE    │
  │                        │   email = ? ───────────►│
  │                        │◄── (empty) ─────────────│
  │                        │                        │
  │                        │  bcrypt.Hash(pw, 12)   │
  │                        │                        │
  │                        │── INSERT users ────────►│
  │                        │── INSERT credentials ──►│
  │                        │◄── OK ──────────────────│
  │                        │                        │
  │                        │  Token Factory          │
  │                        │  → JWT (15 min)         │
  │                        │  → refresh (30d)        │
  │                        │                        │
  │◄── 201 { access_token }│                        │
  │    Set-Cookie: refresh  │                        │
```

**Error cases handled:**
- Email already registered → `409 Conflict`
- Invalid email format → `422 Unprocessable Entity`
- Password below minimum strength → `422 Unprocessable Entity`

### 5.2 Email + Password Login

```
Client                Auth Service         Redis        Postgres
  │                        │                 │               │
  │── POST /auth/login ────►│                 │               │
  │   { email, password }   │                 │               │
  │                        │── SELECT credentials WHERE ─────►│
  │                        │   email = ?                      │
  │                        │◄── { password_hash } ───────────│
  │                        │                 │               │
  │                        │  bcrypt.Compare(pw, hash)       │
  │                        │                 │               │
  │                        │── SET refresh_token (TTL 30d) ─►│
  │                        │── INSERT refresh_tokens ────────►│
  │                        │── INSERT auth_events ───────────►│
  │                        │                 │               │
  │◄── 200 { access_token }│                 │               │
  │    Set-Cookie: refresh  │                 │               │
```

**Timing attack mitigation:** `bcrypt.CompareHashAndPassword` is called even when no user is found (against a dummy hash), ensuring constant-time response regardless of whether the email exists.

### 5.3 Google OAuth 2.0 (PKCE)

```
Client           Auth Service          Google          Postgres
  │                   │                   │                │
  │── GET /auth/google►│                  │                │
  │                   │  generate         │                │
  │                   │  code_verifier    │                │
  │                   │  code_challenge   │                │
  │◄── 302 redirect ──│                  │                │
  │    to Google       │                  │                │
  │                   │                  │                │
  │── Google consent ─────────────────────►               │
  │◄── redirect with code ────────────────│               │
  │                   │                  │                │
  │── GET /auth/google/callback ──────────►               │
  │   ?code=...        │                  │                │
  │                   │── POST /token ───►│                │
  │                   │   + code_verifier │                │
  │                   │◄── id_token ──────│                │
  │                   │                  │                │
  │                   │  verify id_token  │                │
  │                   │  extract email,   │                │
  │                   │  provider_id      │                │
  │                   │                  │                │
  │                   │── UPSERT oauth_identities ────────►│
  │                   │── UPSERT users ────────────────────►│
  │                   │── INSERT auth_events ──────────────►│
  │                   │                  │                │
  │◄── 200 { access_token }              │                │
  │    Set-Cookie: refresh               │                │
```

### 5.4 Token Refresh with Family Rotation

```
Client           Auth Service        Redis          Postgres
  │                   │                │                │
  │── POST /auth/refresh              │                │
  │   Cookie: refresh  │               │                │
  │                   │── GET token_hash ─────────────►│
  │                   │◄── { family_id, fingerprint } ─│
  │                   │                │                │
  │                   │  validate device fingerprint    │
  │                   │                │                │
  │                   │── DEL old token ──────────────►│
  │                   │── SET new token (TTL 30d) ─────►│
  │                   │── UPDATE refresh_tokens ────────►│
  │                   │   (revoked_at = old, new row)   │
  │                   │                │                │
  │◄── 200 { new access_token }        │                │
  │    Set-Cookie: new refresh         │                │
```

**Theft detection:** If an already-rotated (old) refresh token is submitted, the family is entirely revoked in Redis and Postgres. The legitimate user's next request will fail, prompting re-authentication, and the attacker's stolen token becomes immediately invalid.

### 5.5 Logout and Revocation

```
Client           Auth Service        Redis          Postgres
  │                   │                │                │
  │── POST /auth/logout               │                │
  │   Authorization: Bearer <jwt>      │                │
  │   Cookie: refresh  │               │                │
  │                   │── SET blocklist:<jti>          │
  │                   │   TTL = remaining JWT lifetime ►│
  │                   │── DEL refresh_token ───────────►│
  │                   │── UPDATE refresh_tokens.revoked_at ►│
  │                   │── INSERT auth_events ───────────►│
  │                   │                │                │
  │◄── 204 No Content  │               │                │
```

Logout-from-all-devices: deletes all `refresh_tokens` rows for the user in Postgres and flushes all matching Redis keys. Each active JWT is individually blocklisted.

---

## 6. Data Model

### 6.1 Table Definitions

```sql
-- Canonical identity record
CREATE TABLE users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(), -- UUID v7 in app layer
    email        TEXT NOT NULL UNIQUE,
    display_name TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Email/password credentials (absent for OAuth-only users)
CREATE TABLE credentials (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL,  -- bcrypt cost 12
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id)
);

-- OAuth provider links (one row per user per provider)
CREATE TABLE oauth_identities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,  -- 'google', 'apple', etc.
    provider_id     TEXT NOT NULL,  -- provider's user ID
    access_token    TEXT,           -- encrypted at rest (AES-256-GCM)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_id)
);

-- Refresh token metadata (raw token stored only in Redis)
CREATE TABLE refresh_tokens (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash          TEXT NOT NULL UNIQUE,  -- SHA-256 of opaque token
    family_id           UUID NOT NULL,
    device_fingerprint  TEXT NOT NULL,         -- SHA-256(user_agent + ip)
    expires_at          TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Append-only security audit trail
CREATE TABLE auth_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type  TEXT NOT NULL,  -- 'login', 'logout', 'login_failed', 'refresh', 'token_theft', 'provider_link'
    user_id     UUID REFERENCES users(id),
    ip          INET,
    user_agent  TEXT,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (created_at);  -- monthly partitions at scale
```

### 6.2 Index Strategy

```sql
-- users
CREATE UNIQUE INDEX idx_users_email ON users(email);

-- oauth_identities
CREATE UNIQUE INDEX idx_oauth_provider ON oauth_identities(provider, provider_id);
CREATE INDEX idx_oauth_user_id ON oauth_identities(user_id);

-- refresh_tokens
CREATE UNIQUE INDEX idx_refresh_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_user_id ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_family_id ON refresh_tokens(family_id);
CREATE INDEX idx_refresh_expires_at ON refresh_tokens(expires_at)
    WHERE revoked_at IS NULL;  -- partial index for cleanup jobs

-- auth_events
CREATE INDEX idx_auth_events_user_id ON auth_events(user_id, created_at DESC);
CREATE INDEX idx_auth_events_type ON auth_events(event_type, created_at DESC);
```

**Justification:** The partial index on `refresh_tokens.expires_at` is a standard big-tech pattern — the cleanup job only needs to scan non-revoked, expired rows. The composite index on `auth_events(user_id, created_at DESC)` supports the security dashboard query ("show me all events for this user, newest first") without a full table scan.

---

## 7. Security Mechanisms

### 7.1 Password Hashing (bcrypt)

- **Algorithm:** bcrypt via `golang.org/x/crypto/bcrypt`
- **Cost factor:** 12 (~250ms on modern hardware — expensive enough to defeat GPU brute-force, cheap enough not to bottleneck a login endpoint under normal load)
- **Never stored:** raw password is zeroed from memory after hashing
- **Never logged:** password fields are excluded from all structured log output at the model level

Argon2id is the modern recommended alternative. It is considered for a future migration path but bcrypt at cost 12 is well-understood, battle-tested, and Go's implementation is production-grade.

### 7.2 PKCE on OAuth

PKCE (Proof Key for Code Exchange, RFC 7636) is mandatory on the Google OAuth flow.

1. Auth Service generates a cryptographically random `code_verifier` (43–128 chars, base64url)
2. `code_challenge = BASE64URL(SHA256(code_verifier))` is sent to Google
3. On callback, Google verifies the `code_verifier` matches the challenge before issuing tokens
4. An intercepted authorization code is useless without the `code_verifier`, which never leaves the Auth Service

This is a requirement for any public OAuth client and is enforced by default in modern Google OAuth flows.

### 7.3 Device Fingerprinting

At refresh token issuance, the Auth Service computes:

```
device_fingerprint = SHA256(User-Agent + ":" + IP)
```

This hash is stored on the `refresh_tokens` row and in Redis alongside the token. On every `/auth/refresh` call, the fingerprint is recomputed and compared.

A mismatch is **not a hard block** — mobile clients change IPs frequently (cell → WiFi transitions). It is:
- Logged as an `auth_events` entry with `event_type = 'fingerprint_mismatch'`
- Optionally used to trigger step-up authentication (email confirmation) for high-value actions

A hard block fires only when combined with the family rotation theft signal.

### 7.4 Refresh Token Family Rotation

Every refresh token belongs to a `family_id` — a UUID assigned at initial login and shared across all tokens issued from that session's refresh chain.

On `/auth/refresh`:
1. Lookup the submitted token hash in Redis
2. If found and not revoked → issue new token (new token hash, same `family_id`), mark old token `revoked_at = NOW()`
3. If **not found or already revoked** → token was already rotated, which means it is either expired or being replayed by a thief
   - Mark ALL tokens with this `family_id` as revoked in Postgres
   - Delete all matching Redis keys
   - Insert `auth_events` with `event_type = 'token_theft_detected'`
   - Return `401 Unauthorized` — the legitimate user must re-authenticate

This is the same mechanism used by Auth0 and described in IETF RFC 6749 security best practices.

### 7.5 Token Blocklist

Short-lived JWTs cannot be revoked by design — they are stateless. The blocklist bridges this gap.

On logout or revocation, the access token's `jti` (JWT ID claim) is written to Redis:

```
Key:   blocklist:<jti>
Value: 1
TTL:   remaining lifetime of the JWT (exp - now)
```

The gateway's Sig Verifier middleware checks this key after validating the signature. A hit → `401 Unauthorized`. After the JWT's natural expiry, the Redis key auto-deletes — no cleanup job needed.

**Memory footprint:** A typical 15-minute access token produces a Redis entry for at most 15 minutes. At 100,000 concurrent logged-out sessions, this is a negligible memory footprint.

### 7.6 Rate Limiting

Implemented at the gateway middleware layer using a sliding window counter in Redis.

| Endpoint | Limit | Window | Scope |
|---|---|---|---|
| `POST /auth/login` | 10 attempts | 1 minute | Per IP |
| `POST /auth/login` | 5 attempts | 15 minutes | Per email |
| `POST /auth/register` | 3 attempts | 1 minute | Per IP |
| `POST /auth/refresh` | 30 attempts | 1 minute | Per IP |
| `GET /auth/google` | 20 attempts | 1 minute | Per IP |

After a per-email lockout on `/auth/login`, a 429 response is returned with a `Retry-After` header. The lockout duration doubles on each subsequent violation (exponential backoff) — a pattern used by Google and GitHub to defeat distributed credential stuffing.

---

## 8. Redis Usage Map

| Key Pattern | Value | TTL | Purpose |
|---|---|---|---|
| `refresh:<token_hash>` | `{ user_id, family_id, fingerprint }` | 30 days | Fast refresh token lookup, avoids Postgres round-trip |
| `blocklist:<jti>` | `1` | Remaining JWT lifetime | Instant access token revocation |
| `ratelimit:ip:<ip>:<endpoint>` | Counter | Sliding window (60s) | Per-IP rate limiting |
| `ratelimit:email:<email>:login` | Counter | Sliding window (900s) | Per-account login throttle |
| `login_attempts:<email>` | Counter | 15 minutes | Login lockout tracking |

Redis is configured with `maxmemory-policy = volatile-lru` — under memory pressure, keys with TTLs are evicted before persistent data. This ensures blocklist entries (which are critical) survive longer than rate limit counters, since blocklist keys have shorter TTLs and are touched more recently.

---

## 9. Audit Logging

Every significant auth event is written to the `auth_events` table asynchronously via a Go channel. The writer goroutine batches inserts using a 100ms flush window to minimize write pressure on Postgres.

| `event_type` | Trigger |
|---|---|
| `register` | Successful registration |
| `login` | Successful email/password login |
| `login_failed` | Failed login attempt (wrong password or unknown email) |
| `oauth_login` | Successful Google OAuth login |
| `oauth_link` | Google account linked to existing Pillow account |
| `refresh` | Successful token refresh |
| `fingerprint_mismatch` | Device fingerprint changed on refresh |
| `logout` | Explicit logout |
| `logout_all` | Logout from all devices |
| `token_theft_detected` | Rotated token replayed — family revoked |
| `password_change` | Password updated |

The `metadata` JSONB column stores event-specific data (e.g., which `family_id` was revoked, how many sessions were terminated). This column is intentionally schemaless — it allows the audit trail to be extended without migrations.

`auth_events` is **append-only by policy**. No UPDATE or DELETE is ever issued against this table. At scale, it is partitioned monthly and older partitions are archived to cold storage (S3 + Parquet) for compliance and forensics.

---

## 10. Operational Standards

### 10.1 RS256 Key Rotation

Private keys are rotated every 90 days. The rotation procedure:

1. Generate new RSA-4096 key pair
2. Add the new public key to the JWKS endpoint with a new `kid`
3. Begin signing new JWTs with the new private key
4. Keep the old public key in JWKS for the duration of the maximum access token lifetime (15 minutes)
5. Remove the old public key from JWKS after the window
6. Revoke the old private key in the secrets manager

This zero-downtime rotation means no in-flight tokens are invalidated during the rotation window.

### 10.2 JWKS Endpoint

```
GET /.well-known/jwks.json
```

Returns the set of public keys currently valid for token verification. The gateway caches this response for 5 minutes with background refresh. All downstream microservices that need to verify Pillow tokens point to this endpoint — they never receive private key material.

```json
{
  "keys": [
    {
      "kty": "RSA",
      "use": "sig",
      "alg": "RS256",
      "kid": "pillow-2025-q1",
      "n": "...",
      "e": "AQAB"
    }
  ]
}
```

### 10.3 Health Checks

The Auth Service exposes two endpoints for Kubernetes (or any orchestrator):

| Endpoint | Type | Checks |
|---|---|---|
| `GET /health/live` | Liveness | Process is running and not deadlocked |
| `GET /health/ready` | Readiness | Postgres connection pool healthy + Redis ping successful |

A pod is removed from the load balancer rotation only when `/health/ready` fails — not `/health/live`. This prevents traffic from reaching an instance whose dependencies are degraded.

### 10.4 Secrets Management

All secrets are injected via environment variables at runtime from a secrets manager (HashiCorp Vault or AWS Secrets Manager). Never committed to version control or baked into Docker images.

| Secret | Storage |
|---|---|
| RS256 private key (PEM) | Vault KV, rotated every 90 days |
| Postgres DSN | Vault KV |
| Redis password | Vault KV |
| Google OAuth client secret | Vault KV |
| JWT signing key passphrase | Vault KV |

### 10.5 Structured Logging

All log output is JSON, using Go's `slog` package (standard library, Go 1.21+) or `uber/zap` for higher throughput needs.

Every log entry carries:

```json
{
  "time": "2025-01-15T10:30:00Z",
  "level": "INFO",
  "trace_id": "abc123",
  "user_id": "uuid-...",
  "endpoint": "/auth/refresh",
  "latency_ms": 12,
  "ip": "redacted",
  "msg": "token refreshed"
}
```

**Never logged:** passwords, raw tokens, full JWTs, email addresses in production (hashed instead), PII beyond what is required for forensics.

### 10.6 Graceful Shutdown

Fiber's `ShutdownWithTimeout(10 * time.Second)` is called on `SIGTERM`. This allows:
- In-flight login and refresh requests to complete
- The audit logger goroutine to flush its buffer
- Active database transactions to commit or roll back cleanly

A deploy that kills processes mid-login results in users seeing errors. Graceful shutdown eliminates this class of incident.

---

## 11. Scalability Considerations

| Concern | Approach |
|---|---|
| Auth Service horizontal scaling | Stateless — scale to N instances behind a load balancer. All state in Postgres + Redis. |
| Postgres write pressure | `auth_events` writes are batched (100ms window). `refresh_tokens` inserts are low-frequency. Read-heavy queries use indexes. |
| Redis availability | Redis Sentinel or Redis Cluster for HA. If Redis is unavailable, the Auth Service falls back to Postgres for refresh token lookup (slower, but correct). |
| Token verification at scale | RS256 verification is in-memory at the gateway — O(1) CPU, zero network I/O. Each downstream microservice verifies independently. |
| `refresh_tokens` table growth | A background job runs nightly: `DELETE FROM refresh_tokens WHERE expires_at < NOW() OR revoked_at < NOW() - INTERVAL '90 days'`. The partial index on `expires_at WHERE revoked_at IS NULL` keeps this efficient. |
| `auth_events` table growth | Monthly range partitioning. Older partitions detached and archived to cold storage. The active partition is always small. |

---

## 12. Decision Log

| Decision | Rationale | Alternatives Considered |
|---|---|---|
| RS256 over HS256 | Asymmetric — downstream services verify without holding a shared secret. Zero blast radius per service. | HS256 — rejected due to shared secret distribution risk in multi-service architecture |
| Opaque refresh tokens over refresh JWTs | Opaque tokens can be revoked instantly via Redis delete. A refresh JWT cannot be revoked without a blocklist, negating its statelessness advantage. | Refresh JWT — rejected due to inability to revoke without a blocklist (equivalent complexity, more attack surface) |
| Separate `credentials` table | OAuth-only users have no password. Separation keeps the schema honest and prevents null fields on `users`. | `password_hash` column on `users` — rejected due to nullable columns for a security-critical field |
| bcrypt cost 12 | ~250ms login time is acceptable UX. Below cost 12, GPU brute-force becomes feasible within hours on a leaked hash file. | Cost 10 (default, too fast), Argon2id (preferred long-term, deferred pending Go ecosystem maturity) |
| `family_id` on refresh tokens | Enables detecting token theft by replay detection of rotated tokens. Industry-standard approach (IETF, Auth0). | Per-token revocation only — rejected because it allows a stolen token to be used once before detection |
| UUID v7 primary keys | Time-sortable — sequential inserts stay in the same B-tree page, reducing index fragmentation. Globally unique without coordination. | UUID v4 — rejected due to random distribution causing index fragmentation at scale. Auto-increment int — rejected due to exposing record counts |
| Async audit logger | Auth event writes must not add latency to the login hot path. A 100ms batched channel write is invisible to the user. | Synchronous insert — rejected because a slow Postgres write would directly delay the login response |

---

*End of AuthDesign.md — Pillow Authentication System*
