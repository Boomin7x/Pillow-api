# kyc-implementation-plan.md — Pillow Platform

> **Status:** Proposed
> **Owner:** Platform / Identity
> **Last updated:** 2026-06-16
> **Scope:** A stable, optimised, fully tested KYC vertical slice (`internal/kyc/`) that delivers risk-tiered, progressive identity verification for every Pillow role.

This plan is written to be executed top-to-bottom. Every phase has an exit criterion. A phase is not "done" until its exit criterion is green in CI. No phase may start before the previous phase's exit criterion is met, **except** where a phase is explicitly marked parallelisable.

This document is subordinate to `_rules/Folder-structure-rules.md`, `_rules/comment-rule.md`, `_rules/swagger-rules.md`, and `_rules/devXperience.md`. Where this plan and a rule appear to conflict, the rule wins and this plan is wrong — fix the plan.

---

## 0. Non-Negotiable Constraints (apply to every phase)

These are pulled directly from `_rules/` and restated here so no step can skip them.

- [ ] **Vertical slice.** All KYC code lives in `internal/kyc/` (`handler.go`, `service.go`, `repository.go`, `dto.go`, `service_test.go`, `handler_test.go`). One business capability, one folder (Rule A1, A2).
- [ ] **No cross-domain imports.** `internal/kyc/` never imports `internal/auth/`, `internal/user/`, etc. It receives other domains' behaviour as interfaces defined in `internal/domain/` and wired in `internal/app/app.go` (Rule A3, I1).
- [ ] **Pure domain.** `internal/domain/kyc.go` imports the standard library only — no `gorm`, no `fiber`, no `uuid`, no `redis` (Rule C3).
- [ ] **Strict inward dependencies.** Transport → Service → Domain ← Repository ← Infrastructure. Enforced by `golangci-lint depguard` (Rule 3).
- [ ] **No comments.** Zero comments of any kind in any Go file. Names carry the meaning. Only toolchain directives (`//go:build integration`, `//go:generate`) are permitted (`comment-rule.md`).
- [ ] **DTOs are not domain types.** Request/response structs live in `dto.go` with `ToDomain()` / `FromDomain()` mappers. Never return `domain.*` from a handler (Rule H6).
- [ ] **GORM models are infrastructure.** KYC tables get model structs in `internal/infrastructure/postgres/models.go` with `ToDomain()` / `ModelFrom()` mappers (Rule D1).
- [ ] **Errors flow through `apperrors`.** Repositories translate `gorm.ErrRecordNotFound` → `apperrors.NotFound`. Services return `*apperrors.AppError`. Handlers return the error; middleware maps to HTTP status (Rule J1–J4).
- [ ] **Config is typed and documented.** Every new var added to `internal/config/config.go` AND `.env.example` with a comment and safe default. No `os.Getenv` outside `config/` (Rule G1–G3).
- [ ] **Swagger ships with the code.** Every endpoint documented in `docs/api/openapi.yaml` in the same PR — path, tags `kyc`, request/response `$ref` schemas, every error status, examples, `security` block (`swagger-rules.md`).
- [ ] **Coverage gates.** Domain 100%, Service ≥90%, Handler ≥70%, Repository ≥60% (integration-supplemented). CI fails below threshold (Rule F4).
- [ ] **Migrations are versioned, sequential, reversible.** Append-only from `000006`. Every `.up.sql` has a `.down.sql`. Never edit an applied migration (Rule E2–E5).
- [ ] **External providers are infrastructure.** Every third-party identity vendor lives under `internal/infrastructure/` behind an interface declared in `internal/domain/kyc.go` (Rule C5, D3).

---

## 1. The Roles & Tier Model This Module Implements

KYC is mapped to **action risk, not role**. Roles inherit a *default* tier requirement, but the gate is the action. Today roles are a free-form `[]string` on auth claims (`internal/domain/auth.go:49`); Phase 1 promotes them to a typed enum in the domain.

| Role | Function | Default tier ceiling | KYC-critical action |
|---|---|---|---|
| `buyer` | Browse, save, request tours, make offers | T2 | Making an offer |
| `renter` | Search, apply, pay deposits | T2 | Submitting an application |
| `seller` | Claim & list owned property (FSBO) | T2 + ownership proof | Listing a property |
| `landlord` | List rentals, screen tenants, collect rent | T2 + ownership + payout/AML | Receiving rent |
| `agent` | List/represent on behalf of clients | T3 (license) | Operating as an agent |
| `lender` | Mortgage products, pre-qualification | T3 (NMLS + EDD) | Onboarding |
| `builder` | List new construction (entity) | T3 (KYB/UBO) | Onboarding the business |
| `service_pro` | Paid services to other users | T2 + payout/AML | Receiving a payout |

**Trust tiers** (the state machine the service implements):

```
T0 ANONYMOUS  → T1 IDENTIFIED (email+phone) → T2 VERIFIED (gov ID + liveness + sanctions)
                                                  → T2+ ownership / payout-AML
                                                  → T3 REGULATED (license / NMLS / KYB)
```

---

## Phase 0 — Foundation, Decisions & Guardrails ✅ DONE (2026-06-16)
*Goal: remove every ambiguity before a line of Go is written. Parallelisable with nothing.*

- [x] Confirm the role enum set above with product; lock the canonical string values. → locked in `docs/kyc-architecture.md` §2.
- [x] Select the IDV vendor strategy (single vendor v1, pluggable from day one). The interface is vendor-agnostic regardless; the decision only affects which adapter is built first. → `docs/kyc-architecture.md` §3.
- [x] Decide data-residency & retention policy for identity documents (encryption at rest, TTL purge, tombstoning for GDPR/CCPA delete without breaking the audit trail). → `docs/kyc-architecture.md` §4.
- [x] Add `depguard` rules for the new domain to `.golangci.yml`: `internal/domain/kyc.go` stdlib-only; `internal/kyc/handler.go` and `service.go` forbidden from importing `gorm`/`redis`; `internal/kyc/` forbidden from importing other `internal/<domain>` packages. → `.golangci.yml` created (depguard + errcheck).
- [x] Reserve migration numbers `000006`–`0000NN` for this module to avoid sequence gaps (Rule E4). → `000006`–`000010` reserved in `docs/kyc-architecture.md` §5.
- [x] Write the threat model: listing fraud, application fraud, payout/AML, license fraud, document tampering. This drives the risk engine in Phase 4. → `docs/kyc-architecture.md` §6.
- [x] **Exit criterion:** decisions recorded in `docs/` (architecture note), `.golangci.yml` updated, CI still green. → `docs/kyc-architecture.md` written; `go build ./...`, `go vet`, `go test ./...` all green.

---

## Phase 1 — Domain Modelling (`internal/domain/kyc.go`) ✅ DONE (2026-06-16)
*Goal: define the language of KYC in pure Go. No infrastructure, no transport.*

- [x] Create `internal/domain/kyc.go` (stdlib-only — Rule C3). → imports `context`, `errors`, `time` only.
- [x] Define value objects / enums: `Tier` (T0–T3), `Role`, `VerificationStatus` (`pending`, `in_review`, `verified`, `rejected`, `expired`), `CheckType` (`document`, `liveness`, `sanctions`, `ownership`, `license`, `kyb`, `payout_aml`), `RiskScore`. → plus `Qualification`, `Verdict`, `RiskBand`, `Action`.
- [x] Define entities: `KYCProfile` (per user: current tier, role, status), `VerificationCase` (one verification attempt), `Check` (one provider check + verdict), `OwnershipClaim`, `AuditEvent`.
- [x] Define business-rule methods on entities: e.g. `KYCProfile.CanPerform(action Action) bool`, `KYCProfile.RequiredTierFor(action Action) Tier`, `Check.IsTerminal() bool`. These are the heart of the gate logic and must be 100% covered. → also `HasQualification`, `GrantQualification`, `MissingQualificationsFor`, `RequirementFor`, `Tier.MeetsOrExceeds`, risk-band helpers.
- [x] Define the **state machine** as pure domain logic: legal transitions between tiers/statuses, with an explicit `Transition(from, event) (to, error)` so it is unit-testable with zero infrastructure. → `NextStatus(current, event)` + `ErrIllegalTransition`.
- [x] Define repository interfaces: `KYCRepository` (persist profiles/cases/checks), `AuditRepository`.
- [x] Define the service interface: `KYCService` (every method takes `context.Context` first — Rule H4).
- [x] Define **infrastructure-facing interfaces** here (implemented later under `internal/infrastructure/`): `IdentityVerifier`, `SanctionsScreener`, `OwnershipVerifier`, `LicenseVerifier`, `BusinessVerifier`, `DocumentVault`. Named by what they do, no `I` prefix (Rule H3).
- [x] Define input/output structs for service methods (`StartVerificationInput`, `SubmitDocumentInput`, `VerdictResult`, …).
- [x] **Tests:** `internal/domain/kyc_test.go` — table-driven coverage of every state transition and every `CanPerform` permutation. **100% coverage required** (Rule F4, F5).
- [x] **Exit criterion:** `go test ./internal/domain/...` green at 100%; `depguard` confirms zero non-stdlib imports. → `coverage: 100.0% of statements`; domain imports verified stdlib-only.

---

## Phase 2 — Persistence Layer ✅ DONE (2026-06-16)
*Goal: durable, queryable KYC state. Depends on Phase 1.*

- [x] Write migrations (append-only from `000006`, each with `.down.sql` — Rule E2, E5):
  - `000006_create_kyc_profiles`
  - `000007_create_verification_cases`
  - `000008_create_kyc_checks`
  - `000009_create_ownership_claims`
  - `000010_create_kyc_audit_events`
- [x] Index for the hot paths: lookup by `user_id`, by `status` (partial index on `pending`/`in_review` for the worker), by `case_id`. Use Postgres-native features (partial indexes, JSONB for raw provider payloads) — never AutoMigrate in prod (Rule E1). → partial index `idx_verification_cases_pending`; partial unique index `idx_kyc_checks_provider_event_id` for webhook idempotency.
- [x] Add GORM models to `internal/infrastructure/postgres/models.go` with `gorm` tags **only here** (Rule D1). Each model gets `ToDomain()` and `ModelFrom()` mappers. → `KYCProfileModel`, `VerificationCaseModel`, `KYCCheckModel`, `OwnershipClaimModel`, `KYCAuditEventModel`.
- [x] Implement `internal/kyc/repository.go` (`kycRepository` struct) against the domain interfaces. → plus `internal/kyc/auditrepository.go` (`kycAuditRepository`).
  - [x] `context.Context` first arg, `db.WithContext(ctx)` on every query (Rule C4).
  - [x] Translate `gorm.ErrRecordNotFound` → `apperrors.NotFound`; never leak GORM errors (Rule J2).
  - [x] Wrap with `fmt.Errorf("kyc: <operation>: %w", err)` (devXperience error standard).
  - [x] No business logic — storage decisions only.
- [x] Store raw provider payloads as JSONB (audit + replay), PII-sensitive document references tokenised, never the raw document bytes (those go to the `DocumentVault` in Phase 3). → `raw_payload`/`metadata` JSONB; document bytes live only in the vault.
- [x] **Tests:** `test/integration/kyc_repository_test.go` with `//go:build integration`, real Postgres via `testhelpers` testcontainers — no SQLite (Rule F3). → 3 lifecycle tests (profile, case+checks, ownership+audit) covering conflict + NotFound paths.
- [x] **Exit criterion:** `make migrate-up` + `make migrate-down` round-trip cleanly; integration tests green; repository coverage ≥60%. → verified up→down(×5)→up against a real Postgres 16 container (KYC tables dropped/recreated, auth tables untouched); all 3 integration tests PASS.

---

## Phase 3 — Infrastructure Adapters (Provider Layer) ✅ DONE (2026-06-16)
*Goal: pluggable, fault-tolerant external verification. Parallelisable with Phase 2 (both depend only on Phase 1 interfaces).*

- [x] `internal/infrastructure/kycprovider/` — implements `IdentityVerifier` (document + liveness) and `SanctionsScreener`. One `New*()` constructor, typed client, errors wrapped (Rule C5). → `kycProvider` with `VerifyDocument`/`VerifyLiveness`/`Screen`.
- [x] `internal/infrastructure/ownership/` — implements `OwnershipVerifier` (deed/title/assessor lookup by address + name). This is Pillow's anti-fraud crown jewel.
- [x] `internal/infrastructure/license/` — implements `LicenseVerifier` (agent/broker license + NMLS lookup, with expiry).
- [x] `internal/infrastructure/businessverify/` — implements `BusinessVerifier` (KYB: registration + UBO + entity sanctions) for `builder`.
- [x] `internal/infrastructure/storage/` — `DocumentVault`: AES-256-GCM encryption at rest, HMAC short-lived signed URLs, idempotent TTL purge, path-traversal-safe references. Documents never touch Postgres.
- [x] **Vendor abstraction:** each adapter behind its domain interface so the service is vendor-agnostic and vendors can be swapped/failed-over without touching business logic. → shared `internal/infrastructure/providerhttp.Executor` centralises POST + decode + verdict mapping (unknown verdict → `review`, the safe default).
- [x] **Resilience:** every outbound call gets context-deadline, timeout, bounded retry with jittered backoff, and a circuit breaker. → `pkg/resilience.Client` (domain-free); `ErrCircuitOpen` surfaced so Phase 4 can degrade to "queued for review".
- [x] Config for each adapter (base URL, API key, timeout, retries) added to `config.go` + `.env.example` with comments (Rule G2). → **Deviation:** keys are optional (not `require()`d) to preserve the `make run` onboarding contract (devXperience); production must set them. Documented in `.env.example`.
- [x] **Tests:** adapter tests use an HTTP test server / fake transport — no live vendor calls in CI. Contract tests assert request shape and verdict mapping. → resilience 84.2%, kycprovider 100%, storage 79.2%; ownership/license/business each contract-tested.
- [x] **Exit criterion:** each adapter unit-tested; `depguard` confirms infra never imports the transport layer (Rule C5). → all unit tests PASS; `.golangci.yml` `infrastructure-no-inward` rule added (golangci-lint runs in CI; not installed locally).

---

## Phase 4 — Service Layer (`internal/kyc/service.go`)
*Goal: the brain — risk-tiered, progressive verification orchestration. Depends on Phases 1–3.*

- [ ] Implement `kycService` against `domain.KYCService`. Constructor accepts **interfaces only**: `KYCRepository`, `AuditRepository`, `IdentityVerifier`, `SanctionsScreener`, `OwnershipVerifier`, `LicenseVerifier`, `BusinessVerifier`, `DocumentVault` (Rule I3). No `*fiber.Ctx`, no `gorm` import (Rule C2).
- [ ] Implement **action→tier gating**: `EvaluateAccess(ctx, userID, action)` returns required tier and whether the user already satisfies it. Pure decision logic delegated to domain methods from Phase 1.
- [ ] Implement **progressive escalation**: start a `VerificationCase` only when an action demands a tier the user lacks. Never over-collect.
- [ ] Implement the **state machine driver**: persist transitions, idempotent and resumable (a dropped connection never restarts the flow). Every transition writes an `AuditEvent`.
- [ ] Implement the **risk engine**: combine device/velocity/geo signals + document-tamper signals into a `RiskScore` that decides step-up (active liveness, manual review) vs. step-through. Pure, table-tested rules.
- [ ] Implement **async verdict handling**: `ApplyVerdict(ctx, verdict)` consumes provider results (from the webhook path in Phase 6), advances the case, recomputes tier. Idempotent on provider event ID.
- [ ] Implement **ownership flow** (T2+): identity match → public-record match → fallback ladder (document upload → postcard-to-property code → manual review).
- [ ] Implement **perpetual monitoring** hooks: schedule sanctions re-screen and license-expiry re-check (executed by worker in Phase 6).
- [ ] All side-effecting audit writes: failures logged + discarded only where non-critical, with an explicit note (Rule J3 exception). Primary verification operation never silently swallows an error.
- [ ] Keep functions ≤60 lines; extract orchestration steps into named private methods (devXperience "long functions").
- [ ] **Tests:** `internal/kyc/service_test.go`, external `kyc_test` package, mock every repository/provider interface with function-field mocks (Rule F2). Table-driven, sentence-named tests covering: each tier transition, ownership fallback ladder, risk step-up, idempotent verdict replay, vendor-failure → queued-for-review. **≥90% coverage** (Rule F4).
- [ ] **Exit criterion:** service tests green at ≥90%; no infrastructure spun up in unit tests.

---

## Phase 5 — Transport Layer (`internal/kyc/handler.go`, `dto.go`)
*Goal: the HTTP boundary. Depends on Phase 4.*

- [ ] `internal/kyc/dto.go`: request DTOs with validator tags + response DTOs, each with `ToDomain()` / `FromDomain()`. DTOs distinct from domain types (Rule H6). Document fields masked appropriately.
- [ ] `internal/kyc/handler.go` (`kycHandler`, depends on `domain.KYCService` interface — Rule C1):
  - `POST /kyc/verifications` — start a verification case for the current user.
  - `POST /kyc/verifications/{id}/documents` — upload ID document (returns signed-URL flow / accepts reference).
  - `GET /kyc/verifications/{id}` — poll case status (async-friendly).
  - `GET /kyc/profile` — current tier + what each role-action requires.
  - `POST /kyc/ownership-claims` — start property ownership verification.
  - `POST /kyc/licenses` — submit agent/NMLS license for T3.
  - `POST /kyc/business` — start KYB for builders.
  - Handlers do **format validation only** (`validate.Struct`), call the service, map to DTO, return. No business `if`, no manual HTTP status codes, no route self-registration (Rule C1, J4).
- [ ] **Tier-gating middleware** in `internal/middleware/` (e.g. `requiretier.go`): a reusable Fiber middleware that, given a required tier, calls the KYC service via its interface and rejects with `403` if unmet. Cross-domain middleware lives in `internal/middleware/`, not in a domain folder.
- [ ] Register every route in `internal/app/router.go` only; apply `auth` + `ratelimit` + tier-gate middleware there (Rule A5, I1).
- [ ] Wire the full graph in `internal/app/app.go`: instantiate adapters → repository → service → handler, inject downward. The only place concrete KYC types are constructed (Rule I1, I4 — no `init()`).
- [ ] **Swagger (same PR):** add `kyc` tag, all paths, all request/response schemas under `components/schemas` via `$ref`, every error status (`400/401/403/404/409/422/429/500`), `security: [{ bearerAuth: [] }]`, and ≥1 request + ≥1 success example per endpoint (`swagger-rules.md`).
- [ ] **Tests:** `internal/kyc/handler_test.go` with a mock `domain.KYCService` — assert binding, validation, status mapping, DTO shape. **≥70% coverage** (Rule F4).
- [ ] **Exit criterion:** handler tests green ≥70%; openapi spec matches every route in `router.go`; CI swagger check passes.

---

## Phase 6 — Async, Workers & Webhooks
*Goal: decouple perceived latency from vendor latency; keep state eventually consistent. Depends on Phase 5.*

- [ ] Webhook ingestion endpoint(s) for provider verdicts (signature-verified, idempotent on event ID) → calls `kycService.ApplyVerdict`. Register in `router.go`, document in swagger, `security: []` with signature verification noted.
- [ ] Extend the worker entrypoint (`cmd/worker/main.go` per the folder map; create if absent) for background jobs:
  - Reconciliation: poll providers for cases stuck `in_review` past SLA.
  - Perpetual monitoring: scheduled sanctions re-screen + license-expiry re-check.
  - Document TTL purge from the vault.
- [ ] Push-notification hook so users are pinged when an async verdict resolves (submit-and-continue UX). Notification dispatch behind an interface, wired in `app.go`.
- [ ] Idempotency + at-least-once delivery handling everywhere a provider can call twice.
- [ ] **Tests:** integration tests for webhook idempotency and worker reconciliation against real Postgres/Redis via testcontainers.
- [ ] **Exit criterion:** duplicate webhook delivery produces one state change; stuck-case reconciliation verified in integration test.

---

## Phase 7 — Performance, Observability & Security Hardening
*Goal: big-tech production readiness. Parallelisable with Phase 6 where possible.*

- [ ] **Caching:** cache verified tier per user (Redis), invalidated on tier change. A verified user never re-verifies for a second action. License checks cached until expiry.
- [ ] **Rate limiting:** apply existing `internal/infrastructure/redis/ratelimiter.go` to verification-start and document-upload endpoints (anti-abuse).
- [ ] **Latency budget:** synchronous path does only instant work (capture, validate, enqueue). Assert p99 of sync endpoints excludes vendor round-trips.
- [ ] **Observability:** structured `slog` logs with trace/request IDs (extend `internal/middleware/requestlog.go`), metrics for pass-rate, drop-off per step, vendor latency, queue depth; alerts on pass-rate drop and vendor circuit-open.
- [ ] **Audit completeness:** every decision is an append-only `AuditEvent` (regulatory requirement); verify no decision path skips the audit write.
- [ ] **Security:** documents encrypted at rest + tokenised; signed URLs short-lived; PII access scoped; GDPR/CCPA delete via tombstoning preserves the audit trail. Run `/security-review` on the branch.
- [ ] **Exit criterion:** dashboards live; load test of verification-start + webhook path meets latency budget; security review clean.

---

## Phase 8 — End-to-End Testing & CI Gates
*Goal: prove the whole flow per role. Depends on Phases 1–7.*

- [ ] Integration tests per role journey: buyer→offer (T2), renter→application (T2), seller→listing (T2+ownership), landlord→payout (AML), agent (T3 license), lender (NMLS), builder (KYB), service_pro (payout). Real Postgres/Redis + faked providers.
- [ ] Negative-path tests: rejected verdict, sanctions hit, ownership mismatch with fallback, expired license, vendor outage → queued.
- [ ] Verify all coverage gates in CI: domain 100%, service ≥90%, handler ≥70%, repository ≥60% (Rule F4).
- [ ] Verify `depguard`, `errcheck`, swagger-sync, migration-rollback, and migration-sequence CI checks all pass (Rule 17).
- [ ] **Exit criterion:** full CI suite green; every role journey passes end-to-end.

---

## Phase 9 — Documentation & Staged Rollout
*Goal: ship safely. Depends on Phase 8.*

- [ ] Architecture doc in `docs/` covering tier model, state machine diagram, provider adapters, and the threat model.
- [ ] Confirm `.env.example` documents every new var with comment + safe default; confirm the `make` onboarding flow (`make keys` → `.env` → `make docker-up` → `make migrate-up` → `make run`) still works unbroken (devXperience onboarding contract).
- [ ] Feature-flag the tier-gate so it can ramp per role (start with `agent`/`lender` where regulation is hardest, expand to `seller`/`landlord`, then `buyer`/`renter`).
- [ ] Runbook: vendor outage handling, manual-review queue ops, re-screen failures.
- [ ] **Exit criterion:** flag live, runbook published, first role enabled in staging.

---

## Why This Plan Produces the Best Result

- **Friction lands only where trust is required.** Action→tier gating (not blanket KYC) protects conversion — a Zestimate browser hits zero KYC; a lender hits the full gauntlet.
- **The domain is testable in isolation.** State machine + risk rules live in pure `internal/domain/`, hit 100% coverage, and never need a database to validate.
- **Vendors are swappable and failures are survivable.** Every provider sits behind a domain interface with timeouts, breakers, and a queue-for-review fallback — no vendor outage becomes a user-facing 500.
- **Perceived latency is decoupled from vendor latency.** Async webhooks + worker reconciliation + push notifications mean the user submits and keeps moving.
- **It solves the real-estate threat model**, not generic ID-scan KYC: ownership/listing fraud (deed match + postcard ladder), license fraud (NMLS/state lookup), and payout AML.
- **It is mechanical to build and review** because it replicates the `internal/auth/` reference slice exactly and obeys every rule in `_rules/` — so CI, not opinion, enforces quality.

---

*End of kyc-implementation-plan.md — Pillow Platform*
