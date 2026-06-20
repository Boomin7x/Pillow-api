# Observability — KYC Module

## Metrics (emitted via `pkg/metrics.Metrics` interface)

### `RecordVerdict(checkType, verdict)`
- Counter per verification outcome
- Tags: `check_type` (document, liveness, sanctions, ownership, license, kyb), `verdict` (approved, rejected, review)
- Alert: **Pass-rate drop** — if the ratio of `approved` to total drops >20% over 5m, page on-call

### `RecordVendorLatency(provider, duration)`
- Histogram per provider call
- Tags: `provider` (license, business, ownership, document, sanctions)
- Alert: **Sustained vendor circuit-open** — if `IncCircuitOpen` fires >5 times in 10m for one provider, page the vendor ops team

### `SetQueueDepth(n)`
- Gauge for pending verification queue depth
- Alert: **Queue backing up** — if depth >1000 for >5m, page on-call

### `IncCircuitOpen(provider)`
- Counter incremented each time `resilience.ErrCircuitOpen` is returned
- Tags: `provider` (sanctions, license, business, ownership, document)
- Alert: see `RecordVendorLatency` above

## Trace Correlation

Every worker `RunOnce` generates a `trace_id` (crypto-random hex, 16 chars) and includes it in every `slog` line emitted during that run. Traces are JSON-structured for ingestion by log aggregators.

## Tracing IDs

- All worker jobs (`CaseReconciler`, `SanctionsRescreener`, `LicenseExpiryChecker`, `VaultPurger`, `QueueConsumer`) emit `trace_id` on entry and on error
- Trace IDs are not propagated to downstream vendor calls — the vendor is the tracing boundary

## Alert Definitions

### `KYCPassRateDrop`
- Condition: `rate(kyc_verdict_total{verdict="approved"}[5m]) / rate(kyc_verdict_total[5m]) < 0.8`
- For: 5m
- Severity: page

### `KYCVendorCircuitOpen`
- Condition: `increase(kyc_circuit_open_total[10m]) > 5`
- For: 1m
- Severity: page

### `KYCQueueDepthHigh`
- Condition: `kyc_queue_depth > 1000`
- For: 5m
- Severity: warn
