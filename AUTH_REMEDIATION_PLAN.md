# Auth Remediation Plan — Pillow Platform

> **Companion to:** `AUTH_AUDIT.md` (every fault `F1`–`F11`, `R1` is defined there).
> **Goal:** turn auth from "bare email+password" into a **server-authoritative identity → role → tier chain** that the KYC feature set can trust, and clean up the security/correctness debt — without regressing existing behaviour.

## How to work this plan (read once)
- Every `- [ ]` is **one small commit**. Do them **in order within a phase** — each leaves the build green.
- After **every** sub-task run: `go build ./... && go vet ./...`, the relevant `go test`, and (before pushing) `golangci-lint run ./...`.
- **"Verify:"** on each line tells you exactly how to know it's done. Don't tick a box until its Verify passes.
- A sub-task that changes a function signature or a constructor lists **"update call sites"** as its own step — never leave the tree half-wired.
- No comments in Go files (project rule) — names carry meaning; record decisions in this doc or the commit message.

## Guiding principles
1. **Identity is server-authoritative.** Roles and verification state are set by governed flows and proven by the server, never accepted from a request body (the rule already applied to `UserID`).
2. **One role type, one source.** `domain.Role` is the only role representation; it flows user → JWT claims → KYC profile → gate. No more `[]string{"user"}`.
3. **Each phase leaves the build green and ships value.** No big-bang.
4. **Backward compatibility.** Existing tokens (`roles: ["user"]`) and existing profiles must keep working through the transition (default/grandfather, don't break live sessions).

## Sequencing rationale
- **Phase A first** — independent, low-risk fixes (no schema), each a quick win; clears the deck and de-risks.
- **Phase B (roles)** — the central authorization fault `F1`; needs a migration; everything role-aware depends on it.
- **Phase C (identity verification)** — `F2`; needs a migration + an OTP/email provider seam; makes T1 real.
- **Phase D (sync + rotation)** — connect the chain end-to-end, mount the gate as consuming domains land, finish key rotation.

---

## Phase A — Correctness & hardening quick wins (no schema)
*Independent of each other; can ship in any order. Target: 1–2 days.*

> **✅ COMPLETE & senior-reviewed (2026-06-20).** A1–A8 all done; `golangci-lint run ./...` → **0 issues**, full unit suite green. Review notes: A2 also tightened the fail-open logic; A3's submitted test was a network-coupled false-green (fixed with an `idTokenVerifier` seam + deterministic offline tests); A5/A6/A7 + the A6/A7 tests were completed during review (`iss`/`aud` enforced, `kid` is now a stable public-key fingerprint).

### A1 — Fix logout cookie clearing (`F5`)
**Why:** the refresh cookie is set at `Path=/auth/refresh`, but `c.ClearCookie` targets `/`, so it never actually clears in the browser.
- [x] **A1.1** In `internal/auth/handler.go`, add a `clearRefreshCookie(c *fiber.Ctx)` helper next to `setRefreshCookie` that sets the cookie with `Name: refreshCookieName, Path: "/auth/refresh", Value: "", Expires: time.Now().Add(-time.Hour), MaxAge: -1, HTTPOnly: true, Secure: true, SameSite: "Strict"`. **Verify:** `go build ./...`.
- [x] **A1.2** In `Logout`, replace `c.ClearCookie(refreshCookieName)` with `h.clearRefreshCookie(c)`. **Verify:** `go build`.
- [x] **A1.3** In `LogoutAll`, same replacement. **Verify:** `go build`.
- [x] **A1.4** In `handler_test.go`, add a test that the logout response's `Set-Cookie` for `refresh_token` has `Path=/auth/refresh` and a past expiry / `Max-Age=0`. **Verify:** `go test ./internal/auth/...`.

### A2 — Make the blocklist failure mode observable (`F3`)
**Why:** if Redis is down, the revocation check is silently skipped and a revoked token is accepted. Keep fail-open (availability) but make the degradation visible.
- [x] **A2.1** In `internal/middleware/auth.go`, add `log/slog` and, in the `IsTokenBlocklisted` error branch, `slog.Warn("auth: blocklist check degraded, failing open", "error", err, "jti", claims.TokenID)` before falling through. **Verify:** `go build`.
- [x] **A2.2** In `middleware/auth_test.go`, add a case where the blocklist mock returns an error → assert the request still reaches the handler (proceeds). **Verify:** `go test ./internal/middleware/...`.
- [x] **A2.3** Record the decision (fail-open is intentional given the 15-min access TTL) in this doc / the commit message. **Verify:** decision written down.

### A3 — Verify Google ID-token signature (`F4`)
**Why:** the id_token signature is never checked; `iss`/`aud`/`sub` are read from an unverified payload.
- [x] **A3.1** `go get google.golang.org/api/idtoken` and `go mod tidy`. **Verify:** `go build ./...`.
- [x] **A3.2** In `internal/infrastructure/oauth/google.go` `ExchangeAndVerify`, after getting `rawIDToken`, call `payload, err := idtoken.Validate(ctx, rawIDToken, g.clientID)` and map `payload.Subject` → `ProviderID` and `payload.Claims["email"]` → `Email`. **Verify:** `go build`.
- [x] **A3.3** Delete the now-unused `parseGoogleIDToken`, `googleIDTokenClaims`, `extractAudience`. **Verify:** `golangci-lint run ./...` (no `unused`).
- [x] **A3.4** Test the bad-signature rejection. (Note: `idtoken.Validate` fetches Google's certs over the network — for a deterministic unit test use `idtoken.NewValidator(ctx, idtoken.WithHTTPClient(fake))` with a fake key source, or cover this path in an integration test and keep the service-level OAuth tests on the mocked `domain.OAuthProvider`.) **Verify:** `go test ./internal/infrastructure/oauth/...`.

### A4 — Log the refresh-cache populate failure (`F7`)
- [x] **A4.1** In `internal/auth/service.go` `issueTokenPairInFamily`, replace `_ = err` after `CacheRefreshToken(...)` with `slog.Warn("auth: cache refresh token failed", "error", err)` (add `log/slog` import). **Verify:** `go build`; `go test ./internal/auth/...`.

### A5 — Clear error for password change on OAuth-only accounts (`F8`)
- [x] **A5.1** In `ChangePassword` (`service.go`), when `FindCredentialByUserID` returns an `apperrors.NotFound`, return `apperrors.BadRequest("this account has no password set")` instead of the wrapped error. **Verify:** `go build`.
- [x] **A5.2** Add a service test: a user with no credential → `ChangePassword` returns `BadRequest` (not a 500). **Verify:** `go test ./internal/auth/...`.

### A6 — Add `iss`/`aud` to access tokens (`F9`)
- [x] **A6.1** Add `Issuer string` and `Audience string` to `config.JWTConfig`; load from `JWT_ISSUER` / `JWT_AUDIENCE` in `config.Load()` (dev defaults e.g. `pillow`); add both to `.env.example` with comments. **Verify:** `go build`.
- [x] **A6.2** Store `issuer`/`audience` on `jwtIssuer` (set in `NewJWTIssuer` from cfg); in `IssueAccessToken` add `"iss"` and `"aud"` to the claims. **Verify:** `go build`.
- [x] **A6.3** In `ValidateAccessToken`, pass `jwt.WithIssuer(j.issuer)` and `jwt.WithAudience(j.audience)` to `jwt.Parse`. **Verify:** `go build`.
- [x] **A6.4** Confirm `app.go`/`tokenutil.NewJWTIssuer` receive the new config (already passes `cfg.JWT`). **Verify:** `go build ./...`.
- [x] **A6.5** Test: a token minted with a different audience is rejected; same-audience round-trip passes. **Verify:** `go test ./internal/infrastructure/tokenutil/...`.

### A7 — Fix the malformed `kid` (`F6`, part 1)
- [x] **A7.1** In `tokenutil.NewJWTIssuer`, set `keyID` to a stable fingerprint of the public key: `fmt.Sprintf("pillow-%x", sha256.Sum256(pubDER)[:8])` (use the DER bytes you already parsed). Remove the `time.Now().Format("2006-q1")` expression. **Verify:** `go build`.
- [x] **A7.2** Confirm the JWT header `kid` equals the JWKS `kid` and validation still passes; update any test asserting the old `kid`. **Verify:** `go test ./internal/infrastructure/tokenutil/...`.

### A8 — Pass through OAuth display name (`F10`)
- [x] **A8.1** Add `Name string` to `domain.OAuthIdentityClaims`. **Verify:** `go build`.
- [x] **A8.2** In `google.go`, read the `name` claim from the validated payload and set `Name`. **Verify:** `go build`.
- [x] **A8.3** In `service.go` `OAuthLogin` new-user branch, set `DisplayName` to `input.Name` (fallback to `input.Email` when empty); add `Name` to `OAuthLoginInput` + pass it from the handler. **Verify:** `go build`.
- [x] **A8.4** Test: OAuth new-user with a `name` gets a non-email display name; without one falls back to email. **Verify:** `go test ./internal/auth/...`.

**Phase A done when:** `golangci-lint` clean; logout cookie cleared; OAuth signature verified; blocklist degradation logged; `iss`/`aud` enforced; all auth tests green.

---

## Phase B — Roles as identity authority (`F1`)
*The central fix. Needs a migration. Everything role-aware depends on it. Do B1→B5 in order.*

### B1 — Role on the user (schema + domain + repo)
- [ ] **B1.1** Add migration `migrations/000013_add_user_roles.up.sql` / `.down.sql`: create `user_roles (id uuid pk, user_id uuid not null references users(id) on delete cascade, role text not null, created_at timestamptz default now(), unique(user_id, role))` + index on `user_id`. `.down` drops the table. **Verify:** `make migrate-up` then `make migrate-down` round-trip (the 8.20 rollback test covers it).
- [ ] **B1.2** Add `UserRoleModel` to `internal/infrastructure/postgres/models.go` (gorm tags only here) with `TableName()` + `ToDomain`/`...From` mappers. **Verify:** `go build`.
- [ ] **B1.3** Add `Roles []Role` to `domain.User` (`Role` already exists in `domain/kyc.go`, same package). **Verify:** `go build`.
- [ ] **B1.4** Add to the `domain.AuthRepository` interface: `AssignRole(ctx, userID string, role Role) error`, `RemoveRole(ctx, userID string, role Role) error`, `ListRoles(ctx, userID string) ([]Role, error)`. **Verify:** `go build` (interface only).
- [ ] **B1.5** Implement those three on `authRepository` (`internal/auth/repository.go`): `AssignRole` is an idempotent upsert (`ON CONFLICT (user_id, role) DO NOTHING`); translate errors via `apperrors`. **Verify:** `go build`.
- [ ] **B1.6** Extend `test/integration/auth_*_test.go` (real Postgres): assign two roles → `ListRoles` returns both; assigning the same role twice is a no-op. **Verify:** `go test -tags=integration ./test/integration/... -run Role`.

### B2 — Roles in the JWT, sourced from the user
**Decision (least churn, backward-compatible):** keep `Claims.Roles []string` at the JWT edge (JWT claims are strings), but **populate it from the user's `[]domain.Role`** and provide typed helpers. Legacy tokens keep working.
- [ ] **B2.1** Add helpers in `domain/auth.go`: `func (c Claims) HasRole(r Role) bool` and a package helper `RoleStrings(roles []Role) []string` / `RolesFromStrings([]string) []Role` (unknown strings dropped, never error). **Verify:** `go build` + a small unit test.
- [ ] **B2.2** In `service.go` `issueTokenPairInFamily`, load the user's roles (`repo.ListRoles(ctx, user.ID)` or use `user.Roles` if already populated) and set `Claims.Roles = RoleStrings(roles)`. If the user has no roles, default to `[]string{string(RoleBuyer)}` (baseline). Remove the hardcoded `[]string{"user"}`. **Verify:** `go build`.
- [ ] **B2.3** Ensure every issue path has the user's roles available: in `Login`/`Refresh`/`OAuthLogin`, load roles when the user is fetched (or have `issueTokenPairInFamily` always `ListRoles`). **Update all call sites.** **Verify:** `go build` + `go test ./internal/auth/...`.
- [ ] **B2.4** Backward-compat test: a token whose `roles` is `["user"]` (legacy) still authenticates through `RequireAuth` and `RolesFromStrings` drops the unknown `"user"` without error. **Verify:** `go test ./internal/infrastructure/tokenutil/... ./internal/middleware/...`.

### B3 — Stop accepting *identity* role from KYC request bodies
**Why (read before coding):** there are two kinds of "role" in a request: the user's **identity role** (who they are — must come from claims, never the body) and a **target credential** a verification will *earn* (e.g. `SubmitLicense` says "I'm applying for an agent license"). The target credential is NOT an identity claim — the user doesn't have that role yet; they get it on approval (B5). So: drop the identity role from bodies; keep (and server-validate) the target-credential field on license/KYB.
- [ ] **B3.1** Remove `Role` from `StartVerificationRequest` in `internal/kyc/dto.go` (document/liveness/sanctions/payout_aml don't need a caller-supplied role). **Verify:** `go build` shows the handler break (fixed next).
- [ ] **B3.2** In the `StartVerification` handler, derive the role from `middleware.Claims(c)` (baseline role) and pass it into `domain.StartVerificationInput`. **Verify:** `go build`; handler test updated.
- [ ] **B3.3** Keep `SubmitLicenseRequest.Role` (agent|lender) but treat it as the **requested credential**, not identity — the service already validates it's `agent`/`lender`. Add a short doc note in `AUTH_AUDIT.md`/PR that this field is the target credential, granted on approval. **Verify:** existing license tests still pass.
- [ ] **B3.4** In `kyc.service.findOrCreateProfile`, source the profile role from the claims-derived baseline (B3.2), and **do not let a later request silently overwrite** an established profile role — if they differ, keep the existing (or return `apperrors.Conflict`, per chosen policy). **Verify:** service test: a user cannot flip their profile role by asserting a different one.
- [ ] **B3.5** Update `docs/api/openapi.yaml` — remove `role` from the `StartVerification` request schema; keep it on license. **Verify:** `go test ./test/... -run TestKYCRoutesDocumented` (swagger-sync) green.

### B4 — `RequireRole` middleware (defense-in-depth)
- [ ] **B4.1** Add `func RequireRole(roles ...domain.Role) fiber.Handler` to `internal/middleware/` (reads `Claims(c)`; `401` if no claims; `403` if none of the user's roles match). **Verify:** `go build`.
- [ ] **B4.2** Table test in `middleware/`: a matching role passes; a non-matching role gets `403`; missing claims get `401`. **Verify:** `go test ./internal/middleware/...`.
- [ ] **B4.3** (Composition onto real endpoints is deferred to **D2** — there are no role-exclusive endpoints yet.) **Verify:** noted.

### B5 — Role-grant governance
**Cross-domain note:** `internal/kyc` must NOT import `internal/auth`. Granting a user role from a KYC verdict goes through a **domain interface** wired in `app.go`.
- [ ] **B5.1** Write the role matrix in `AUTH_AUDIT.md` (or a new `docs/roles.md`): baseline roles chosen at registration (`buyer`/`renter`/`seller`/`landlord`/`service_pro`); elevated roles **earned** (`agent`/`lender` via license, `builder` via KYB). **Verify:** doc exists.
- [ ] **B5.2** Registration: add an optional validated `role` to `RegisterRequest`/`RegisterInput` (baseline roles only; reject `agent`/`lender`/`builder`); on register call `repo.AssignRole`. Default to `buyer` when absent. **Verify:** register test assigns the baseline role; an attempt to self-register as `agent` is rejected.
- [ ] **B5.3** Add `domain.RoleGranter interface { GrantRole(ctx, userID string, role Role) error }`; implement it with the auth repo's `AssignRole`; inject it into `kycService` (new constructor param, nil-safe). **Update call sites** (`buildKYC`, test helpers). **Verify:** `go build` + `go test ./internal/kyc/...`.
- [ ] **B5.4** In `kyc.service.advanceAfterVerification` (or `ApplyVerdict`'s terminal-verified path), when the check is `license` grant `agent`/`lender` (from the requested credential) and when `kyb` grant `builder`, via `RoleGranter`. Log-and-continue on error (non-critical side effect). **Verify:** service test asserts `GrantRole` called with the right role on an approved license/KYB.
- [ ] **B5.5** Integration test: complete a license verification → the user's `ListRoles` includes `agent` → the next issued token carries `agent`. **Verify:** `go test -tags=integration ./test/integration/... -run RoleGrant`.

**Phase B done when:** roles originate from the user record, ride in the JWT, are enforced where exclusive, and are no longer accepted as identity from request bodies; `domain.Role` is the single representation end-to-end.

---

## Phase C — Real identity verification → T1 (`F2`)
*Makes the trust ladder's base tier real. Needs a migration + provider seams. Do C1→C4 in order.*

### C1 — User identity fields
- [ ] **C1.1** Migration `000014_add_user_identity.up.sql` / `.down.sql`: add `phone TEXT`, `email_verified_at TIMESTAMPTZ`, `phone_verified_at TIMESTAMPTZ` to `users`. `.down` drops the three columns. **Verify:** `make migrate-up`/`migrate-down` round-trip.
- [ ] **C1.2** Add `Phone string`, `EmailVerifiedAt *time.Time`, `PhoneVerifiedAt *time.Time` to `domain.User` and map them in `UserModel` `ToDomain`/`...From`. **Verify:** `go build`.
- [ ] **C1.3** Add repo methods `SetEmailVerified(ctx, userID string) error` and `SetPhoneVerified(ctx, userID, phone string) error` to `AuthRepository` + impl. **Verify:** `go build`.

### C2 — Email verification flow
- [ ] **C2.1** Add `domain.Mailer interface { Send(ctx, to, subject, body string) error }`; implement a dev `slog` adapter in `internal/infrastructure/mailer/` (mirror the KYC notifier). **Verify:** `go build`.
- [ ] **C2.2** Choose stateless tokens: an HMAC-signed, expiring token containing `userID` + `email` (no schema). Add a small signer in `tokenutil` (reuse the JWT key or a config secret). **Verify:** unit test: sign → parse round-trips; tampered/expired rejected.
- [ ] **C2.3** On `Register`, generate the verification token and `Mailer.Send` a verify link. **Verify:** register test asserts `Mailer.Send` called once.
- [ ] **C2.4** Add `POST /auth/verify-email` (handler + route + `AuthService.VerifyEmail(ctx, token)`); on valid token call `repo.SetEmailVerified`. Rate-limit the route. **Verify:** handler test → `204` on valid, `400` on bad/expired.
- [ ] **C2.5** Integration test: register → extract token (from the dev mailer) → verify → `email_verified_at` set; replay/expired rejected. **Verify:** `go test -tags=integration ./test/integration/... -run EmailVerify`.
- [ ] **C2.6** Document `POST /auth/verify-email` in `openapi.yaml`. **Verify:** swagger-sync green.

### C3 — Phone OTP flow
- [ ] **C3.1** Add `domain.OTPSender interface { Send(ctx, phone, code string) error }` + dev `slog` adapter in `internal/infrastructure/otp/`. **Verify:** `go build`.
- [ ] **C3.2** OTP storage in Redis (key `otp:<userID>`, the code + an attempt counter, short TTL) via the existing redis client / a small store. **Verify:** unit/integration: set → get → expire.
- [ ] **C3.3** `POST /auth/phone/start` (handler+route+`AuthService.StartPhoneVerification(ctx, userID, phone)`): generate a 6-digit code, store it, `OTPSender.Send`. Rate-limit. **Verify:** test → `202`, OTP sent once.
- [ ] **C3.4** `POST /auth/phone/verify` (`AuthService.VerifyPhone(ctx, userID, code)`): check code + attempt cap; on success `repo.SetPhoneVerified`. **Verify:** test → success sets timestamp; wrong/expired code rejected; attempt cap enforced.
- [ ] **C3.5** Both routes behind `RequireAuth` (the user is logged in, verifying their own phone). **Verify:** unauthenticated → `401`.
- [ ] **C3.6** Document both endpoints in `openapi.yaml`. **Verify:** swagger-sync green.

### C4 — Wire verification → T1
**Cross-domain note:** KYC owns the tier; auth signals "identity confirmed" through a domain interface wired in `app.go`.
- [ ] **C4.1** Add `domain.IdentityConfirmer interface { ConfirmIdentified(ctx, userID string) error }`; implement it in the KYC service as a method that advances the profile to `T1 IDENTIFIED` (idempotent; creates the profile if absent). **Verify:** `go build` + kyc service test.
- [ ] **C4.2** Inject `IdentityConfirmer` into the auth service; after **both** `email_verified_at` and `phone_verified_at` are set (end of `VerifyEmail` / `VerifyPhone`), call `ConfirmIdentified`. Log-and-continue on error. **Update call sites** (app wiring). **Verify:** `go build`.
- [ ] **C4.3** Integration test: register → verify email + phone → KYC profile reads `T1` → `EvaluateAccess` for a T1 action (e.g. `contact_agent`) is satisfied. **Verify:** `go test -tags=integration ./test/integration/... -run IdentityToTier`.

**Phase C done when:** T1 is established by proven email + phone, not assumed; the tier model's base is backed by real signals.

---

## Phase D — Sync, enforcement & rotation

### D1 — End-to-end identity → role → tier sync test
- [ ] **D1.1** Integration test that walks the full chain: register (baseline role) → verify email + phone (→T1) → submit license (→ `agent` role granted, B5) → new token carries `agent` → `RequireRole(agent)` **and** `RequireKYC(operate_as_agent)` both pass. **Verify:** `go test -tags=integration ./test/integration/... -run AuthKYCChain` green against real PG + Redis.

### D2 — Mount the gates as consuming domains land (`F11`)
- [ ] **D2.1** When the **offers** endpoints are built: mount `RequireKYC(evaluator, ActionMakeOffer)` on them. **Verify:** below-T2 user gets `403`.
- [ ] **D2.2** When **listings** are built: `RequireRole(seller, landlord, agent)` + `RequireKYC(ActionListProperty)`. **Verify:** wrong-role `403`, missing-ownership `403`.
- [ ] **D2.3** When **payouts** are built: `RequireKYC(ActionReceivePayout/ActionReceiveRent)`. **Verify:** missing payout/AML qualification `403`.
- [ ] **D2.4** Remove `F11` from the open-faults list once at least one consuming endpoint enforces the gate. **Verify:** audit updated.

### D3 — Key rotation (`F6`, part 2)
- [ ] **D3.1** Config: accept a **set** of signing keys (e.g. a keys directory or `JWT_PRIVATE_KEYS` list) + which `kid` is current. **Verify:** `go build`; `make run` still boots with the single existing key.
- [ ] **D3.2** `jwtIssuer` loads N keys (map `kid → key`), signs with the current key's `kid`. **Verify:** issued token's header `kid` = current.
- [ ] **D3.3** `ValidateAccessToken` selects the verifying key by the token's `kid` (reject unknown `kid`). **Verify:** token signed by key A validates while A is loaded.
- [ ] **D3.4** `PublicKeySet` (JWKS) publishes **all** active keys. **Verify:** JWKS lists every loaded `kid`.
- [ ] **D3.5** Rotation test: a token signed by the previous key still validates while both keys are published; drops to invalid once the old key is removed. **Verify:** `go test ./internal/infrastructure/tokenutil/...`.

**Phase D done when:** the identity→role→tier chain is integration-proven; the tier gate is enforced on real consuming endpoints; keys can rotate without invalidating live sessions.

---

## Definition of done (whole effort)
- No role is ever read as identity from a request body; `domain.Role` is the single role type, sourced from the user record and carried in the JWT.
- T1 reflects proven email + phone; tiers no longer assume unverified identity.
- `golangci-lint` clean; coverage gates hold; auth + role + tier integration tests green.
- All `F1`–`F11` resolved (or consciously deferred with a recorded decision); `R1` remains fixed.

## Risk register
- **B2/B3 are a contract change** (claims shape + KYC request bodies). Stage behind the backward-compat grandfathering in B2; coordinate any API consumers.
- **B5.3 / C4.2 cross-domain seams** (`RoleGranter`, `IdentityConfirmer`) must be domain interfaces wired in `app.go` — `internal/kyc` and `internal/auth` never import each other.
- **C2/C3 add external dependencies** (mailer, SMS). Keep them behind interfaces with `slog` dev adapters so `make run` still boots with no real provider (the Phase-3 optional-keys pattern).
- **Migrations 000013/000014** must stay append-only and reversible (the 8.20 rollback gate enforces this).
