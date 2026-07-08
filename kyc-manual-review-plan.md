# KYC Manual Review Plan (Admin as the Verification Provider)

> Server-side plan. Written 2026-07-05. Companion to `kyc-implementation-plan.md` (this repo) and the frontend's `kyc-implementation-plan/` (pillow-FE). Status: **proposed**.

## Why

The KYC pipeline is built for external verification providers (`internal/infrastructure/kycprovider/`), but all four provider slots (`KYC_IDENTITY_PROVIDER_URL`, `KYC_OWNERSHIP_PROVIDER_URL`, `KYC_LICENSE_PROVIDER_URL`, `KYC_BUSINESS_PROVIDER_URL`) are empty because we cannot afford a paid vendor yet. Today a submitted document sits in limbo forever unless a verdict is hand-signed against the webhook (`scripts/kyc-simulate-verdict.mjs` on the FE side) — fine for development, unusable for real users.

**Decision: a human admin becomes the verification provider.** Documents are reviewed manually in an admin console; verdicts flow through the exact same `ApplyVerdict` path the webhook and worker already use. Nothing about tiers, qualifications, case states, or the frontend changes. When a paid provider becomes affordable, we set its URL and the manual path becomes the fallback/appeals channel — zero rework.

## Story

As a Pillow admin, I can open a review queue, see every verification waiting on a human (ID documents, selfies, ownership claims, licenses, business registrations), inspect the submitted evidence securely, and approve or reject with one action — so users get verified within hours by a person instead of never by a machine we can't pay for.

As a user, nothing changes: I submit, I see "in review", and my tier advances when a verdict lands — I never know or care whether a vendor or an admin decided.

## What exists that this plan builds on (all verified in-repo)

| Capability | Where | Reused how |
|---|---|---|
| Single verdict entry point, idempotent via `ProviderEventID` dedup | `internal/kyc/service.go` `ApplyVerdict` | Admin verdicts call it with `ProviderEventID: "admin:" + uuid` |
| JWT already carries `roles[]` claims (all users get `["user"]`) | `internal/auth/service.go:211`, `tokenutil/jwt.go:92,136` | Add an `admin` role value; no token format change |
| Auth middleware | `internal/middleware/auth.go` `RequireAuth` | New `RequireRole("admin")` composes after it |
| Encrypted document vault with signed, expiring URLs | `internal/infrastructure/storage/vault.go` `SignedURL(ref, ttl)` | Admin views documents via short-TTL signed URLs; raw files never leave the vault path |
| Job queue + worker per check type | `internal/kyc/worker.go` `processJob` | A `manual` provider mode routes jobs to the review queue instead of an HTTP vendor |
| Audit log | `internal/kyc/auditrepository.go`, `writeAudit` | Every admin action audited with the admin's user id |
| FE polling + status UX | pillow-FE `useVerificationCase`, verification center | Unchanged — verdicts surface exactly like provider verdicts |

## Architecture

```
user submits (unchanged)
  POST /kyc/verifications + document upload ──► vault.Store ──► enqueueJob
                                                                    │
                                              worker.processJob     ▼
                        provider mode: manual ──► mark check "awaiting_manual_review"
                                                  (case status: in_review — honest UX)
                                                                    │
admin console (new)                                                 ▼
  GET  /admin/kyc/queue?status=&type=          ◄── lists everything awaiting a human
  GET  /admin/kyc/cases/:id                    ◄── case + vault SignedURL (10 min TTL)
  POST /admin/kyc/cases/:id/verdict            ──► service.ApplyVerdict
       {verdict, risk_score?, note}                 ProviderEventID "admin:<uuid>"
  GET/POST /admin/kyc/ownership-claims[...]    ──► same pattern for claims
                                                                    │
                                                                    ▼
                                    case → verified/rejected, tier/qualification granted,
                                    audit written — FE polling picks it up (unchanged)
```

Key property: **the admin console is just another provider implementation.** The domain layer cannot tell a human from a vendor, which is exactly why swapping a paid vendor in later costs nothing.

## Phases

### Phase 0 — Roles & admin plumbing

- [ ] Add `RequireRole(role string)` middleware (reads `roles` from the verified JWT claims already attached by `RequireAuth`; 403 `FORBIDDEN` otherwise). Unit tests: user token → 403, admin token → pass, no token → 401.
- [ ] Admin provisioning is **DB-only, never API**: a migration adds nothing (roles already stored on the user); grant via a small CLI/Make target (`make grant-admin EMAIL=...`) that appends `admin` to the user's roles. Document that the first admin is created by whoever operates the DB.
- [ ] Mount an `/admin` route group: `RequireAuth` + `RequireRole("admin")` + the existing per-IP rate limiter.

**Done when:** an `admin`-role token reaches a stub `/admin/ping` and a normal user gets a 403, proven by handler tests.

### Phase 1 — Manual provider mode (stop the limbo)

- [ ] Config: `KYC_REVIEW_MODE=manual|provider` (default `manual` when all provider URLs are empty — make the implicit state explicit).
- [ ] In `manual` mode the worker does **not** call vendor HTTP. `processJob` records the check as awaiting manual review (`verdict: review` semantics, case stays/moves to `in_review`) and stops. No failing HTTP calls, no circuit-breaker noise, no dropped jobs.
- [ ] Ownership claims: in manual mode every claim resolves to `method: manual_review` (the domain already models it — `domain.MethodManualReview`).
- [ ] Repository: `ListCasesAwaitingReview(status, type, limit, cursor)` + same for claims. Index check on `(status, created_at)`.

**Done when:** with no provider URLs set, uploading a document moves the case to `in_review` (not an error log), and it appears in the repository listing. Worker tests cover both modes.

### Phase 2 — Admin review API

- [ ] `GET /admin/kyc/queue` — unified list: case/claim id, user id, check type, status, submitted-at, waiting-duration. Filter by `type` and `status`, cursor-paginated, oldest first (it's a work queue).
- [ ] `GET /admin/kyc/cases/:id` — full case + `document_url`: a vault `SignedURL` with **10-minute TTL**, generated on demand, never stored. If the document was purged (retention TTL), say so explicitly.
- [ ] `POST /admin/kyc/cases/:id/verdict` — body `{verdict: "approved"|"rejected", risk_score?: 0-100, note?: string}`. Maps to `ApplyVerdict` with `ProviderEventID: "admin:" + uuid`. The `note` goes to the audit log only — never to the user-facing case.
- [ ] Ownership claims: `GET /admin/kyc/ownership-claims/:id` + `POST .../verdict` following the worker's existing claim-resolution transitions.
- [ ] Validation + errors through the standard `apperrors` envelope; a verdict on an already-terminal case → 409 `CONFLICT`.
- [ ] **Audit every action**: `admin_viewed_document`, `admin_verdict` with `{admin_user_id, case_id, verdict}`. The audit trail must answer "who approved this identity and when" — that is the compliance story replacing the vendor's.
- [ ] OpenAPI: add the `/admin/kyc/*` paths to `docs/api/openapi.yaml`; extend `swagger_sync_test.go` coverage to the admin group.

**Done when:** the full loop works over curl against a dev stack: submit as user → appears in queue → admin GET shows a working signed document URL → verdict approved → user's case `verified`, tier advanced, audit rows present — all with the FE polling picking it up unchanged.

### Phase 3 — Admin console UI (pillow-FE)

- [ ] Route `/dashboard/admin/verifications` behind the dashboard session guard + client-side role check (decode `roles` from the access-token payload for display gating; the server 403 remains the authority).
- [ ] Queue table (oldest first, waiting-duration badge), detail pane with the document rendered inline (image/PDF) from the signed URL, and Approve / Reject with a required rejection-reason picker (maps to the audit `note`).
- [ ] Verdict actions use the existing FE mutation/error conventions (root alert on failure, toast on success, TanStack invalidation of the queue).
- [ ] Zero PII in telemetry (existing rule); document URLs never logged or stored client-side.
- [ ] Follows pillow-FE rules: RHF+Zod for the verdict form, feature-first under `src/features/kyc-admin/`, barrel exports.

**Done when:** an admin completes a full review (approve and reject paths) entirely in the browser; a non-admin visiting the route sees a designed 403 state, not a blank page.

### Phase 4 — Hardening & operations

- [ ] Rate-limit verdict posts; require re-auth (fresh token < N min) for verdicts — optional, decide below.
- [ ] Metrics: queue depth, median time-to-verdict (the SLA replacing vendor latency), verdicts/day per admin.
- [ ] Retention: verdicted documents follow the existing vault `RetentionTTL` purge; rejected-case documents purge on the same schedule (no special casing).
- [ ] Runbook in `docs/`: how to grant admin, review etiquette (what counts as a valid ID, common rejection reasons — mirror the FE's user-facing rejection copy so guidance matches), and the incident path for a wrong verdict (re-open = start a fresh case; verdicts are never edited, only superseded — the audit trail stays append-only).
- [ ] Smoke: extend the FE's `kyc-smoke` pattern with an admin-mode script (register user → submit → admin verdict via API → tier advanced) so the manual path is machine-verified too.

**Done when:** `make check` (server) green, the admin smoke passes in CI-able form, and the runbook exists.

## Definition of Done (whole plan)

- [ ] A real user's document, submitted through the production FE, is verified by an admin within the console — no curl, no webhook signing, no server shell.
- [ ] The user experience is byte-identical to provider mode: same statuses, same polling, same tier advancement.
- [ ] Every verdict is attributable: audit answers who/what/when for any tier a user holds.
- [ ] Switching to a paid provider later = set `KYC_REVIEW_MODE=provider` + provider URL. No schema, domain, or FE changes. The admin console keeps working for appeals/fallback.
- [ ] No document bytes ever leave the vault except via short-TTL signed URLs requested by an authenticated admin, and every such access is audited.

## Out of scope

- Paid provider integrations (this plan exists because we can't afford them; the contract stays vendor-ready).
- Admin user management UI (DB/CLI provisioning only for now).
- Sanctions screening quality: in manual mode `sanctions`/`payout_aml` checks are an admin attestation, not a real watchlist screen — documented limitation, revisit when budget allows (ComplyAdvantage-class vendors are the eventual answer here; a human cannot replicate this check).
- Notifications to admins (email/Slack on new queue items) — nice-to-have after Phase 4 metrics show volume.

## Open decisions

- [ ] **Fresh-auth for verdicts** (Phase 4): require a recently-issued token for verdict posts? Recommend yes, cheap and meaningful.
- [ ] **Second-admin rule**: for `license`/`kyb` (T3-granting) verdicts, require a different admin than the one who viewed first? Overkill at current scale — recommend no, revisit at >2 admins.
- [ ] **Selfie (liveness) manual policy**: admin compares selfie to ID photo by eye. Accept as v1 policy? (It is exactly what postal banks did for decades.) Recommend yes, with the runbook naming the comparison criteria.
