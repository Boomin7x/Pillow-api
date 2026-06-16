# devXperience.md — Developer Experience Standard


---

<!-- ## Onboarding

A developer who has never seen this codebase must be able to:

1. Clone the repo
2. Run `make keys` to generate JWT key material
3. Copy `.env.example` to `.env`
4. Run `make docker-up` to start all dependencies
5. Run `make migrate-up` to apply migrations
6. Run `make run` and hit a working API -->

This sequence must never break. If a change breaks any step in this flow, it is a regression and must be fixed before merging. Every new environment variable must be added to `.env.example` with a comment explaining its purpose and a safe default where possible.

---

## Naming

Names must communicate intent without requiring a comment.

- Function names describe what the function does, not how: `FindUserByEmail`, not `QueryDB`.
- Variable names at declaration sites are full words: `accessToken`, not `at`. Short names (`i`, `n`, `err`) are acceptable only in the smallest possible scope.
- Boolean variables and fields are named as statements: `isRevoked`, `hasCredential`.
- Error variables are always named `err`. Shadow `err` per scope — never reuse it across unrelated operations in the same function body.
- Interfaces are named by what they represent or what they do: `TokenIssuer`, `RateLimiter`. Never `Manager`, `Handler` (unless it is literally a Fiber handler), or `Helper`.

---

## Error Messages

Every error that reaches a developer — in logs, in test failures, in API responses — must say exactly what went wrong and where.

- Repository errors wrap with `fmt.Errorf("auth: find user by email: %w", err)`. The prefix follows the pattern `<domain>: <operation>`. A developer reading a log line must know which layer and which operation failed without reading code.
- `AppError.Message` is a client-facing string. It must be human-readable and actionable: `"email already registered"`, not `"constraint violation"`.
- Test failure messages use `t.Errorf("got %v, want %v", got, want)` — never `t.Error("test failed")`.
- Startup failures log the exact missing variable or misconfiguration before exiting. A developer must not have to guess why the process died.

---


## Code Navigation

A developer must be able to understand any feature by reading at most five files.

- All code for a domain lives in its vertical slice under `internal/<domain>/`. A developer working on auth never needs to open a file outside `internal/auth/`, `internal/domain/`, and `internal/infrastructure/` to understand the full flow.
- The dependency graph flows in one direction. A developer tracing a bug starts at the handler and follows the call chain inward without encountering circular dependencies or unexpected detours.
- `internal/app/app.go` is the wiring diagram. Every concrete type in the application is instantiated there. A developer who wants to understand how the system is assembled reads one file.
- `internal/app/router.go` is the route map. Every HTTP route in the application is registered there. A developer who wants to know what endpoints exist reads one file.

---

## Tests as Documentation

Tests are the most reliable form of documentation because they are executed and therefore cannot become stale.

- Test names are full sentences describing behaviour: `TestRefresh_TheftDetected`, not `TestRefresh3`.
- Table-driven tests for any function with more than two input cases. Each case has a `name` field that reads as a sentence.
- A developer reading a service test must be able to understand the business rule being tested without reading the implementation.
- Mock structs use function fields (`createFn func(...)`) so tests can express intent inline rather than in setup boilerplate.
- Integration tests use real infrastructure via `testcontainers-go`. They are labelled with `//go:build integration` so `make test` skips them and `make test-integration` runs them explicitly.

---

## Adding a New Domain

The experience of adding a new domain must be mechanical, not creative. Follow the checklist in `Folder-structure-rules.md §14` exactly. A developer should not need to make architectural decisions when adding a standard domain. The pattern is established — replicate it.

When creating a new domain, copy the structure of `internal/auth/` as the reference implementation. If the auth domain deviates from any rule in `Folder-structure-rules.md`, fix auth first before using it as a template.

---

## What to Avoid

These patterns degrade developer experience and are prohibited:

- **Magic configuration.** If a behaviour changes based on an undocumented environment variable, it will not be discovered until something breaks in production.
- **Silent failures.** An operation that can fail must return an error. Swallowing errors (`_ = someOperation()`) is only permitted for non-critical side effects and must be explicitly noted in the code with the reason.
- **Shared mutable state.** Global variables that are written after startup create race conditions and make tests order-dependent.
- **Long functions.** A function longer than 60 lines is doing too many things. Extract it. The cognitive load of reading a long function compounds — every additional line costs more than the previous one.
- **Overloaded structs.** A struct with more than eight fields is likely representing more than one concept. Split it.
- **Inconsistent conventions.** If the codebase uses one pattern in one place, use the same pattern everywhere. Inconsistency forces developers to hold two mental models simultaneously.
