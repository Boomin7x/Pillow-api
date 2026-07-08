# Auth Implementation Audit — Pillow Platform

> **Date:** 2026-06-20
> **Reviewer:** Senior engineering review
> **Scope:** `internal/auth/` (service, repository, handler, dto), `internal/domain/auth.go`, `internal/domain/user.go`, `internal/middleware/auth.go`, `internal/infrastructure/tokenutil/` (JWT), `internal/infrastructure/oauth/` (Google), and the auth wiring in `internal/app/`.
> **Lens:** correctness, security, and **whether auth reflects the roles the system actually has** (`buyer`, `renter`, `seller`, `landlord`, `agent`, `lender`, `builder`, `service_pro`) and the KYC trust-tier model (T0→T3).

This document records **every fault found**, each with a severity, the exact location, why it matters, and the senior-lead solution approach. A companion file — `AUTH_REMEDIATION_PLAN.md` — turns these into an ordered, executable plan.

---

## 0. Executive summary

The auth slice is **structurally clean** (ports-and-adapters, RSA-signed JWTs, refresh-token rotation with reuse detection, PKCE OAuth, bcrypt, rate limiting) and the mechanics mostly work. **But it is disconnected from the system's role and identity model**, and that disconnect is the root of the most serious findings:

1. **Roles are not an identity authority.** Every access token is minted with a hardcoded `Roles: ["user"]`. The real roles (`buyer`…`service_pro`) live only on the KYC profile and are **self-asserted in request bodies**, never bound to the authenticated user. There is no role-based authorization anywhere.
2. **Identity verification (T1) does not exist in auth.** `domain.User` has no email-verified or phone fields; registration trusts whatever email is posted. The KYC tier model's foundational tier — *T1 = email + phone verified* — is therefore unbacked.

Everything else is a mix of medium security hardening gaps and small correctness bugs. None are catastrophic in isolation, but together they mean **auth and the KYC feature set are out of sync**: KYC reasons about roles and tiers that auth neither establishes nor enforces.

### Severity tally
| Severity | Count | IDs |
|---|---|---|
| High | 2 | F1, F2 |
| Medium | 4 | F3, F4, F5, F6 |
| Low | 5 | F7, F8, F9, F10, F11 |
| Already fixed (this branch) | 1 | R1 |

---

## 1. The role model (the headline) — F1

### F1 — Roles are self-asserted, not an identity authority · **HIGH**
**Where:**
- `internal/auth/service.go:210` — `Roles: []string{"user"}` is hardcoded in `issueTokenPairInFamily` for **every** token (register, login, refresh, OAuth).
- `internal/domain/auth.go:49` — `Claims.Roles []string` (free-form strings).
- `internal/kyc/dto.go` — `StartVerificationRequest.Role`, `SubmitLicenseRequest.Role` accept the role **from the request body**.
- `internal/kyc/service.go` — `findOrCreateProfile(userID, role)` stamps the profile's role from that body on first call.

**What's wrong:**
The system has a typed role enum (`domain.Role`: `buyer`…`service_pro`) and stores a role on `KYCProfile` — but **nothing binds a user to a role**. The JWT (the only cryptographic identity authority) always says `["user"]`. KYC endpoints take the role from the caller's JSON. So:
- A `buyer` can call `POST /kyc/licenses` with `role: "agent"` and begin an agent (T3) verification. The service validates *internal consistency* (license requires `agent`/`lender`) but never checks the claim against an authoritative role the user actually holds — because none exists.
- The KYC profile's role is whatever the **first** verification request declared. It is mutable-by-assertion, not governed.
- There is **no RBAC**: no middleware or service path enforces "only a `lender` may do X." The plan's intent — *gate the action, not the role* — is sound, but it still assumes the acting role is **trustworthy**; here it is attacker-controlled.

**Why it matters for our roles:** `lender` (NMLS + EDD) and `builder` (KYB/UBO) are heavily regulated. If a user can self-declare these roles, the regulatory posture rests entirely on the downstream tier/qualification checks happening to be correct, with no defense-in-depth from identity. This is the single most important gap relative to "the roles we have."

**Senior solution approach:**
1. Make **role a first-class, server-controlled attribute of identity**, not a request field. Introduce a typed `domain.Role` on the user (or a `user_roles` table for multi-role users — landlords are often also buyers), assigned/changed only through governed flows (registration role selection, an admin/role-grant endpoint, or earned via verification).
2. **Carry roles in the JWT** from that authority: replace `Roles: []string{"user"}` with the user's actual roles, and change `Claims.Roles` to `[]domain.Role` (or keep `[]string` at the transport edge but map to `domain.Role` immediately).
3. **Stop accepting role from KYC request bodies.** Derive it from `Claims` (the same identity rule already enforced for `UserID`). `StartBusinessVerification` already hardcodes `builder` server-side — apply that pattern everywhere.
4. Add a thin **`RequireRole(roles…)`** middleware for endpoints that are genuinely role-exclusive, composed *before* the action/tier gate. Keep action→tier gating as the primary control; role becomes defense-in-depth + correct profile provenance.
5. Reconcile the two representations (see F-sync below): one role type, sourced once, flowing user → claims → profile → gate.

---

## 2. Identity verification gap

### F2 — No email or phone verification; T1 is unbacked · **HIGH**
**Where:** `internal/domain/user.go:8-14` (no `EmailVerified`/`PhoneVerified`/`Phone` fields); `internal/auth/service.go:38-71` (`Register` creates the user and immediately issues tokens); no verification flow exists anywhere.

**What's wrong:** The KYC trust ladder is `T0 ANONYMOUS → T1 IDENTIFIED (email + phone) → T2 VERIFIED …`. Auth has **no email verification and no phone capture/verification at all**. Registration trusts the posted email and logs the user straight in. So:
- "T1 IDENTIFIED" cannot actually be established — there is no signal that the email or phone was proven.
- Account-takeover and fake-account economics are worse: anyone can register any email without owning it.

**Why it matters for our roles:** Every role's journey starts at T1. If T1 is fictional, the whole tier model rests on sand — a `seller` listing a property or a `service_pro` receiving payouts may never have proven even a contactable identity.

**Senior solution approach:**
1. Add `Phone string`, `EmailVerifiedAt *time.Time`, `PhoneVerifiedAt *time.Time` to `domain.User` + a migration.
2. Add an **email-verification flow** (signed, single-use, expiring token emailed out; a `POST /auth/verify-email` consumes it) and a **phone OTP flow** (SMS provider behind a `domain.OTPSender` interface, mirroring the KYC provider pattern).
3. Have the KYC service treat "email + phone verified" as the gate that advances a profile to `T1` (it currently jumps tiers off provider checks only). Wire the verification timestamps into `EvaluateRisk`/tier logic.
4. Until built, **stop claiming T1 semantics** — document that registered-but-unverified users are effectively T0.

---

## 3. Security hardening

### F3 — Blocklist (token-revocation) check fails open · **MEDIUM**
**Where:** `internal/middleware/auth.go:33-38`
```go
if claims.TokenID != "" {
    blocked, err := bl.IsTokenBlocklisted(c.UserContext(), claims.TokenID)
    if err == nil && blocked {        // err != nil → fall through → request PROCEEDS
        return apperrors.Unauthorized("token has been revoked")
    }
}
```
**What's wrong:** If Redis is unavailable, `err != nil`, the revocation check is silently skipped, and a **revoked / logged-out access token is accepted**. A security control fails *open*.

**Why it matters:** A user who hit "log out" (or "log out all" after a compromise) still has a working token for up to the access-token TTL (15 min) during any Redis blip. For regulated roles this widens the compromise window.

**Senior solution approach:** Make the failure mode a **conscious, logged decision**. Given short-lived access tokens, fail-open is defensible for availability — but it must (a) `slog.Warn` the degradation so it's observable, and (b) emit a metric/alert. If the threat model prefers safety over availability, fail **closed** (reject when the blocklist can't be consulted). Recommend: log + metric now; make the policy configurable.

### F4 — Google ID-token signature is not verified · **MEDIUM**
**Where:** `internal/infrastructure/oauth/google.go:85-114` (`parseGoogleIDToken` base64-decodes `parts[1]` and reads claims) and `:62-70` (iss/aud/sub checked against the **unverified** payload).

**What's wrong:** The ID token's RSA signature is never validated against Google's JWKS. The `iss`/`aud`/`sub` checks run on an unauthenticated payload.

**Nuance (why MEDIUM, not CRITICAL):** the token is obtained server-to-server from Google's token endpoint via `oauth2.Exchange` over TLS (authenticated by the client secret + PKCE), so in the standard authorization-code flow it is authentic and Google explicitly permits skipping signature verification when the token is fetched directly over a TLS channel. The risk is **brittleness and spec-deviation**: the safety depends on an implicit property of the call path; if the flow changes (implicit/hybrid, a different provider, token passed through the client), it silently becomes an auth bypass.

**Senior solution approach:** Verify the signature with Google's public keys — simplest is `google.golang.org/api/idtoken.Validate(ctx, rawIDToken, clientID)`, which checks signature **and** `iss`/`aud`/`exp` in one call. Replace the hand-rolled `parseGoogleIDToken` with it. This removes the implicit-trust footgun and makes adding a second IdP safe.

### F5 — Logout does not clear the refresh cookie (path mismatch) · **MEDIUM**
**Where:** `internal/auth/handler.go:230` sets the cookie with `Path: "/auth/refresh"`; `:121` and `:139` call `c.ClearCookie(refreshCookieName)` which targets the default path `"/"`.

**What's wrong:** A cookie is only cleared when the clear targets the **same path** it was set on. The refresh cookie lives at `/auth/refresh`, so `ClearCookie` at `/` does not remove it — the cookie lingers in the browser after logout.

**Why it matters:** Server-side revocation still happens (the token is revoked in Postgres + evicted from Redis — see R1), so the lingering cookie is *inert*. But it is a real bug, confuses debugging, and leaves a stale credential artifact on the device; if any future code path trusts the cookie's presence before revalidating server-side, it becomes exploitable.

**Senior solution approach:** Clear with the matching path: `c.ClearCookie()` won't take a path, so set an expired cookie explicitly (`c.Cookie(&fiber.Cookie{Name: refreshCookieName, Path: "/auth/refresh", Expires: past, MaxAge: -1, HTTPOnly: true, Secure: true, SameSite: "Strict"})`). Add a test asserting the `Set-Cookie` on logout has `Path=/auth/refresh` and a past expiry.

### F6 — Key ID is malformed and there is no key rotation · **MEDIUM**
**Where:** `internal/infrastructure/tokenutil/jwt.go:75` — `keyID: "pillow-" + time.Now().Format("2006-q1")`.

**What's wrong:** `q1` is **not** a Go time layout token, so it is emitted literally — the `kid` is effectively `pillow-2026-q1` (year + literal "q1"), not a real quarter. More importantly, the issuer holds exactly one key, the JWKS publishes one key, and `ValidateAccessToken` ignores the token's `kid`. There is **no key-rotation story**: rotating the signing key invalidates every live token instantly and there's no overlap window.

**Why it matters:** Key rotation is a baseline operational/security requirement. As written, a compromised or expiring key cannot be rotated gracefully.

**Senior solution approach:** (1) Fix the `kid` to a real, stable identifier (e.g., a hash/fingerprint of the public key, or a config-set version). (2) Support **N keys**: load a set, sign with the "current" one, and have `ValidateAccessToken` select the verifying key by the token's `kid` (publish all active keys in the JWKS). This enables overlap-window rotation. Scope to a follow-up; not blocking.

---

## 4. Correctness / lower-severity

### F7 — Refresh-cache populate error swallowed without a log · **LOW**
**Where:** `internal/auth/service.go:244-246` — `if err := s.repo.CacheRefreshToken(...); err != nil { _ = err }`.
**Issue:** A failed cache populate is silently ignored. Functionally OK (refresh falls back to the DB), but it violates the project's "log-and-discard, never silently drop" standard and hides Redis degradation. **Fix:** `slog.Warn("auth: cache refresh token failed", "error", err)`.

### F8 — `ChangePassword` on an OAuth-only account returns an opaque error · **LOW**
**Where:** `internal/auth/service.go:276-284` — `FindCredentialByUserID` returns `NotFound` for users who signed up via Google (no password credential), surfaced as a wrapped error.
**Issue:** OAuth-only users have no password; calling change-password yields a confusing 404/500 rather than a clear "no password set for this account." **Fix:** detect the no-credential case and return `apperrors.BadRequest("account has no password; set one via …")`, or add a set-password flow.

### F9 — Access tokens carry no `iss`/`aud` · **LOW**
**Where:** `internal/infrastructure/tokenutil/jwt.go:82-90` (no `iss`/`aud`), `:111-142` (not validated).
**Issue:** Fine for a single self-contained service, but defense-in-depth and any future multi-service/audience scoping want `iss` + `aud`. **Fix:** add `iss` (config-driven) and `aud`, validate both in `ValidateAccessToken` via `jwt.WithIssuer`/`jwt.WithAudience`.

### F10 — OAuth users get `DisplayName = email` · **LOW**
**Where:** `internal/auth/service.go` `OAuthLogin` (new-user branch sets `DisplayName: input.Email`).
**Issue:** Cosmetic; the Google `name`/`given_name` claim is available and discarded. **Fix:** pass the name claim through `OAuthIdentityClaims` and use it.

### F11 — `RequireKYC` tier-gate middleware is built but mounted on no route · **LOW (sync)**
**Where:** `internal/middleware/requirekyc.go` exists and is tested; `internal/app/router.go` mounts only `RequireAuth` on the KYC group.
**Issue:** The action→tier gate is never actually enforced at any HTTP edge. This is *expected* today — the gated actions (make-offer, list-property, receive-payout) belong to consuming domains (offers, listings, payouts) that aren't built yet — but it means the gate is currently dead code and must be tracked so it isn't forgotten when those domains land. **Fix:** track explicitly; mount `RequireKYC(evaluator, action)` on each consuming endpoint as those domains are built.

---

## 5. Already fixed on this branch (for the record)

### R1 — Refresh-token reuse detection now evicts the cache · **RESOLVED**
A prior review found that `RevokeTokenFamily` / `RevokeAllUserRefreshTokens` revoked rows in Postgres but did **not** evict the Redis meta-cache, so a rotated token survived family revocation (the cache-hit path trusted stale data). Fixed in `internal/auth/repository.go` (both bulk-revocation methods now `Pluck` the family/user token hashes and `Del` their `refresh:<hash>` keys via `evictCachedRefreshTokens`). Proven by `TestIntegration_RefreshRotationAndTheftDetection`. Listed here only so it isn't re-reported.

---

## 6. What is correct (so the picture is balanced)

- **Architecture:** clean ports-and-adapters; `domain` interfaces, infra adapters, transport thin. Auth never imports KYC and vice-versa.
- **Passwords:** bcrypt cost 12; constant-time dummy-hash compare on unknown email to resist user enumeration (`service.go:76-83`).
- **Refresh tokens:** opaque random 256-bit, stored **hashed** (SHA-256), rotated on every use, with family-based reuse detection (and now correct cache eviction — R1).
- **JWT:** RS256 with signing-method pinning in `ValidateAccessToken` (rejects `alg` confusion), `exp` enforced by the v5 parser, JWKS endpoint published.
- **OAuth:** authorization-code **+ PKCE** (S256), state→verifier in Redis with TTL, `iss`/`aud`/`sub` checks present (just on an unverified payload — F4).
- **Cookies:** `HttpOnly`, `Secure`, `SameSite=Strict`, path-scoped to `/auth/refresh`.
- **Rate limiting:** IP + per-email (with backoff) on the sensitive auth endpoints.
- **Errors:** flow through `apperrors`; GORM errors translated, not leaked.

---

## 7. Cross-cutting theme — auth ⇄ KYC are out of sync

The faults above cluster into one story: **auth establishes a bare identity (an email + a password) and stops there, while KYC reasons about roles and trust tiers that auth never produces or enforces.** Concretely:

- Two role representations exist (`Claims.Roles []string` = always `["user"]`; `domain.Role` enum on the profile) and **never meet** (F1).
- The trust ladder's base tier (T1 = verified email + phone) has **no auth implementation** (F2).
- The gate that would connect them (`RequireKYC`) is **unmounted** (F11).

The remediation plan (`AUTH_REMEDIATION_PLAN.md`) sequences the fixes so that identity → role → tier becomes one coherent, server-authoritative chain.
