# KYC Architecture & Phase 0 Decisions — Pillow Platform

> **Status:** Accepted (Phase 0)
> **Last updated:** 2026-06-16
> **Companion to:** [`kyc-implementation-plan.md`](../kyc-implementation-plan.md)

This document records the decisions that Phase 0 of the KYC implementation plan requires to be locked before any Go is written. It is the architectural reference for the KYC vertical slice (`internal/kyc/`).

---

## 1. Trust-Tier Model

KYC is mapped to **action risk, not role**. A role only sets a *default ceiling*; the gate is always the specific action.

| Tier | Name | Earned by | Grants |
|---|---|---|---|
| T0 | Anonymous | nothing (device/cookie only) | browse, search, save |
| T1 | Identified | email + phone verified | save searches, contact agents, request tours |
| T2 | Verified | government ID + liveness + sanctions clear | make offers, submit applications |
| T3 | Regulated | license / NMLS / KYB | operate as agent, lender, builder |

On top of a tier, a profile can hold **qualifications** that gate specific high-trust actions independently of tier:

| Qualification | Required for |
|---|---|
| `ownership` | listing a property you claim to own |
| `payout_aml` | receiving money (rent, service payouts) |
| `license` | operating as a regulated agent / lender |
| `kyb` | operating as a builder (business entity) |

## 2. Roles — Locked Canonical Values

The role enum is frozen at these eight string values (typed as `domain.Role` in Phase 1). Today roles are a free-form `[]string` on auth claims (`internal/domain/auth.go:49`); KYC introduces the typed enum.

`buyer`, `renter`, `seller`, `landlord`, `agent`, `lender`, `builder`, `service_pro`

### Action → requirement matrix

| Action | Min tier | Qualifications |
|---|---|---|
| browse | T0 | — |
| save_search / contact_agent / request_tour | T1 | — |
| make_offer / submit_application | T2 | — |
| list_property | T2 | ownership |
| receive_payout | T2 | payout_aml |
| receive_rent | T2 | ownership + payout_aml |
| operate_as_agent / operate_as_lender | T3 | license |
| operate_as_builder | T3 | kyb |

This matrix is implemented as the single source of truth in `domain.RequirementFor(action)` and is covered 100% by unit tests.

## 3. IDV Vendor Strategy

- **v1 ships behind vendor-agnostic interfaces** (`IdentityVerifier`, `SanctionsScreener`, `OwnershipVerifier`, `LicenseVerifier`, `BusinessVerifier`) declared in `internal/domain/kyc.go` and implemented under `internal/infrastructure/`.
- A **single primary IDV vendor** is integrated first; the abstraction allows a second vendor and dynamic routing/failover later without touching the service layer.
- No vendor SDK type ever crosses the domain or service boundary. Adapters map provider responses to `domain.ProviderCheckResult`.

## 4. Document Data-Residency & Retention Policy

- Raw identity-document bytes are **never** stored in Postgres. They go to the `DocumentVault` (object storage), encrypted at rest, addressed by an opaque `DocumentReference`.
- Access is via **short-lived signed URLs** only.
- Provider raw payloads are stored as JSONB for audit/replay; PII document references are tokenised.
- **Retention:** documents are purged on a TTL after a terminal verdict. The `AuditEvent` trail is append-only and survives document purge.
- **GDPR/CCPA delete** is satisfied by tombstoning the profile and purging vault documents while preserving the audit trail (regulatory requirement).

## 5. Migration Numbers Reserved

To avoid sequence gaps (Rule E4), the following numbers are reserved for the KYC module (current head is `000005`):

| Number | Migration |
|---|---|
| `000006` | create_kyc_profiles |
| `000007` | create_verification_cases |
| `000008` | create_kyc_checks |
| `000009` | create_ownership_claims |
| `000010` | create_kyc_audit_events |

## 6. Threat Model

The risk engine (Phase 4) and the verification flows are designed against these vectors:

| Threat | Vector | Mitigation |
|---|---|---|
| **Listing fraud** | listing property you don't own | `ownership` qualification: identity match → public-record match → document → postcard-to-property → manual review |
| **Application fraud** | fake identity to bypass rental screening | T2 (ID + liveness + sanctions) before `submit_application` |
| **Payout / AML** | laundering through rent/service payouts | `payout_aml` qualification + sanctions screening + perpetual re-screen |
| **License fraud** | impersonating a licensed agent/lender | `license` qualification via state/NMLS lookup with expiry re-check |
| **Document tampering** | forged or manipulated ID | document-tamper signal feeds risk score → step-up to active liveness / manual review |
| **Velocity / synthetic identity** | scripted mass account creation | device/velocity/geo signals → risk band → step-up; rate limiting on verification-start |

## 7. Enforcement

Architecture boundaries are enforced in CI by `golangci-lint` `depguard` rules added in [`.golangci.yml`](../.golangci.yml):

- `internal/domain/*.go` may import the standard library only.
- `internal/*/service.go` and `handler.go` may not import GORM / Redis / Fiber directly.
- `internal/kyc/*.go` may not import other domain packages.
- `internal/infrastructure/**` may not import the transport layer.
