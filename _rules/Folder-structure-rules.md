# Folder-structure-rules.md — Pillow Platform

> **This document is law.**
> Every developer working on the Pillow platform must read, understand, and follow every rule in this document before writing a single line of code. These rules exist to maintain scalability, testability, and long-term team ownership. They are not suggestions. Violations are caught by CI and block merges.

---

## Table of Contents

1. [The Prime Directive](#1-the-prime-directive)
2. [The Folder Map — At a Glance](#2-the-folder-map--at-a-glance)
3. [The Dependency Rule — Non-Negotiable](#3-the-dependency-rule--non-negotiable)
4. [Rule Set A — Domain Rules](#rule-set-a--domain-rules)
5. [Rule Set B — File Rules](#rule-set-b--file-rules)
6. [Rule Set C — Layer Rules](#rule-set-c--layer-rules)
   - [C1. Handler rules](#c1-handler-rules-internaldomain-handlergo)
   - [C2. Service rules](#c2-service-rules-internaldomain-servicego)
   - [C3. Domain layer rules](#c3-domain-layer-rules-internaldomainentitygo)
   - [C4. Repository rules](#c4-repository-rules-internaldomain-repositorygo)
   - [C5. Infrastructure rules](#c5-infrastructure-rules-internalinfrastructuredriver)
7. [Rule Set D — Infrastructure Rules](#rule-set-d--infrastructure-rules)
8. [Rule Set E — Migration Rules](#rule-set-e--migration-rules)
9. [Rule Set F — Testing Rules](#rule-set-f--testing-rules)
10. [Rule Set G — Configuration Rules](#rule-set-g--configuration-rules)
11. [Rule Set H — Naming and Package Rules](#rule-set-h--naming-and-package-rules)
12. [Rule Set I — Dependency Injection Rules](#rule-set-i--dependency-injection-rules)
13. [Rule Set J — Error Handling Rules](#rule-set-j--error-handling-rules)
14. [Adding a New Domain — Checklist](#14-adding-a-new-domain--checklist)
15. [Adding a New Endpoint — Checklist](#15-adding-a-new-endpoint--checklist)
16. [What Goes Where — Quick Reference](#16-what-goes-where--quick-reference)
17. [Violations and Enforcement](#17-violations-and-enforcement)
18. [The Smell Test](#18-the-smell-test)

---

## 1. The Prime Directive

> **Organize by business domain, not by technical concern. Enforce strict inward dependencies. Keep the domain layer pure.**

Everything else in this document is a specific expression of that statement. When you face a decision not explicitly covered here, ask:

1. Does this break a domain boundary?
2. Does this introduce an outward dependency from an inner layer?
3. Does this pollute the domain layer with infrastructure concerns?

If the answer to any of those is yes, your decision is wrong. Rethink it.

---

## 2. The Folder Map — At a Glance

```
pillow/
├── cmd/                          ← Binary entrypoints only. No logic.
│   ├── api/main.go               ← API server entrypoint (< 30 lines)
│   ├── worker/main.go            ← Background worker entrypoint
│   └── migrate/main.go           ← Migration runner entrypoint
│
├── internal/                     ← ALL application code lives here
│   ├── app/                      ← Dependency graph assembly + routing
│   ├── config/                   ← Typed config struct. One place.
│   ├── domain/                   ← Pure types and interfaces. ZERO external imports.
│   ├── apperrors/                ← Typed error taxonomy. One place.
│   ├── middleware/               ← Reusable cross-domain Fiber middleware
│   │
│   ├── auth/                     ← Auth vertical slice (handler/service/repo/dto)
│   ├── property/                 ← Property vertical slice
│   ├── user/                     ← User profile vertical slice
│   ├── upload/                   ← File upload vertical slice
│   └── [new-domain]/             ← Every new capability = new folder here
│
│   └── infrastructure/           ← All external system drivers
│       ├── postgres/             ← GORM setup + model structs
│       ├── redis/                ← Redis client
│       ├── storage/              ← S3 / object storage
│       ├── mailer/               ← Email driver
│       └── tokenutil/            ← JWT signing/verification
│
├── migrations/                   ← Versioned SQL files. Append-only.
├── pkg/                          ← Exportable domain-free utilities only
├── test/                         ← Integration tests + fixtures
├── docs/                         ← Architecture documents
├── Makefile                      ← Single developer interface
├── Dockerfile
├── docker-compose.yml
└── .env.example                  ← Config contract. All vars documented here.
```

---

## 3. The Dependency Rule — Non-Negotiable

This is the architectural spine of the entire codebase. Every other rule derives from it.

```
Transport → Service → Domain ← Repository ← Infrastructure
                        ↑
                   (the core)
               All arrows point inward.
               Nothing inside points outward.
```

Encoded as a table:

| Layer | File location | May import | Must NEVER import |
|---|---|---|---|
| **Domain** | `internal/domain/` | Standard library only | GORM, Fiber, Redis, any 3rd party |
| **Service** | `internal/<domain>/service.go` | `domain`, `apperrors`, `pkg/` | Fiber, GORM, Redis directly |
| **Repository** | `internal/<domain>/repository.go` | `domain`, GORM, Redis, `apperrors` | Fiber, the service layer |
| **Transport** | `internal/<domain>/handler.go` | `domain` (interfaces), `apperrors`, `dto` | GORM, Redis directly |
| **Infrastructure** | `internal/infrastructure/` | GORM, Redis, AWS SDK, etc. | `domain`, `service`, `transport` |

**Enforcement:** `golangci-lint` with `depguard` runs on every PR. A handler file importing `gorm.io/gorm` is a CI failure. A domain file importing `github.com/gofiber/fiber` is a CI failure. There are no exceptions.

**Why:** When an inner layer imports an outer layer, you cannot unit test without spinning up infrastructure. You cannot swap Postgres without touching business logic. You cannot extract a domain into a microservice without untangling a dependency web. Strict inward dependencies prevent all of these problems permanently.

---

## Rule Set A — Domain Rules

These rules govern how business capabilities are organized into domains.

---

### A1 — One folder per business capability

Every distinct business capability gets its own folder under `internal/`. A capability is something a product manager would describe as a feature area.

```
CORRECT:
internal/auth/
internal/property/
internal/user/
internal/notification/

WRONG:
internal/handlers/          ← technical concern, not a domain
internal/services/          ← technical concern, not a domain
internal/models/            ← technical concern, not a domain
```

**Why:** When code is organized by technical concern, understanding one feature requires jumping across four folders. When organized by domain, the entire auth system is in `internal/auth/`. A new developer reads five files and owns the feature.

---

### A2 — A domain folder is a complete vertical slice

Every domain folder must contain all the code needed to deliver its feature: handler, service, repository, and DTOs. A domain is self-contained.

```
internal/auth/
├── handler.go        ← HTTP boundary
├── service.go        ← Business logic
├── repository.go     ← Data access
├── dto.go            ← Request/response structs
├── handler_test.go
└── service_test.go
```

If you find yourself putting auth logic in `internal/user/`, stop. Either it belongs in `internal/auth/`, or it is shared domain logic that belongs in `internal/domain/`.

---

### A3 — Domains must not import each other directly

`internal/auth/` must never import `internal/property/`. `internal/user/` must never import `internal/auth/`.

Cross-domain communication happens in exactly two ways:

1. **Via service interfaces** — one service accepts another service's interface as a constructor dependency, wired in `internal/app/app.go`
2. **Via domain types** — shared pure types live in `internal/domain/` and are imported by both domains

```go
// WRONG: auth importing property directly
import "github.com/pillow/internal/property"

// CORRECT: auth depends on an interface from domain/
type authService struct {
    userRepo domain.UserRepository  // interface — not a concrete type from another domain
}
```

**Why:** Direct inter-domain imports create coupling that makes extraction to microservices impossible and causes circular import errors as the codebase grows.

---

### A4 — Domains own their data

A domain's repository only queries the tables owned by that domain. `internal/property/repository.go` queries the `properties` table. It does not query `users` directly.

If a property repository needs user data, it receives a `domain.UserRepository` interface via constructor injection and calls that. It never constructs its own user query.

---

### A5 — New domains do not modify existing domains

Adding a new domain — say `internal/notification/` — must not require changes to `internal/auth/` or any other existing domain. The new domain registers itself in `internal/app/app.go` and `internal/app/router.go`. That is the only footprint.

If adding a feature requires modifying three existing domains, the feature boundary is wrong. Rethink the domain model.

---

## Rule Set B — File Rules

These rules govern what files exist in each domain and what they are named.

---

### B1 — Required files in every domain

Every domain folder under `internal/` must contain these files:

| File | Purpose |
|---|---|
| `handler.go` | Fiber route handlers — HTTP boundary only |
| `service.go` | Business logic — implements the domain interface |
| `repository.go` | Data access — implements the repository interface (omit only if domain has no persistence) |
| `dto.go` | Request and response structs for the HTTP boundary |
| `service_test.go` | Unit tests for the service layer |

These are mandatory. A PR that adds a domain without `dto.go` or `service_test.go` is rejected.

---

### B2 — Banned file names

The following filenames are banned anywhere under `internal/`:

| Banned name | Why | Correct approach |
|---|---|---|
| `utils.go` | A catch-all that grows without limit | Name files by what they contain: `tokenutil.go`, `hashutil.go` |
| `helpers.go` | Same problem as utils.go | Same solution |
| `common.go` | Vague; becomes a dumping ground | Explicit names only |
| `misc.go` | Same problem | Same solution |
| `base.go` | Implies inheritance — Go does not have inheritance | Use composition and interfaces |

**Why:** Vaguely named files attract code that has no natural home. After 12 months, `utils.go` has 800 lines of unrelated functions. Named files create a forcing function: if you cannot name the file by what it contains, your abstraction is wrong.

---

### B3 — One primary struct per file, named after the file

Each file has one primary type that gives the file its name.

```
handler.go     → type propertyHandler struct
service.go     → type propertyService struct
repository.go  → type propertyRepository struct
```

Secondary types (small helpers, option types) may coexist in the file if they serve only that primary type. If a secondary type is used by more than one file, it moves to its own file.

---

### B4 — Test files live beside their source

Unit test files live in the same folder as the code they test.

```
internal/auth/
├── service.go
├── service_test.go    ← same folder, same package or _test package
├── handler.go
└── handler_test.go
```

Integration tests that require real infrastructure live in `test/integration/`. Nothing else lives there.

---

### B5 — No test utilities in source files

Test helper types, mock implementations, and fixture builders must not be defined in `service.go` or any other source file. They belong in `*_test.go` files or in `test/testhelpers/`.

---

## Rule Set C — Layer Rules

Each layer has a strict contract. Breaking the contract is a violation, not a style choice.

---

### C1. Handler rules (`internal/<domain>/handler.go`)

A handler's only jobs are:
1. Bind the request body / path params / query params
2. Validate the input format (not business rules — that is the service's job)
3. Call the service via its interface
4. Map the result to a DTO
5. Write the HTTP response

**Handlers must never:**

- Contain an `if` statement that enforces a business rule
  ```go
  // WRONG — business rule in handler
  if req.Price < 0 {
      return apperrors.BadRequest("price must be positive")
  }
  // CORRECT — business rule in service; handler validates format only
  if err := validate.Struct(req); err != nil {
      return apperrors.Validation(err)
  }
  ```

- Import `gorm.io/gorm` or `github.com/redis/go-redis`
- Import another domain's concrete types
- Call a repository directly — always go through the service
- Self-register routes (routes are registered in `internal/app/router.go`)
- Hold state — handler structs are stateless; all state is passed via context or returned from services

**Handler struct declaration:**
```go
// CORRECT — depends on interface, not concrete type
type propertyHandler struct {
    service domain.PropertyService
}

// WRONG — depends on concrete implementation
type propertyHandler struct {
    service *propertyService  // never reference a concrete service type
}
```

---

### C2. Service rules (`internal/<domain>/service.go`)

A service is the brain of the domain. All business rules, decision logic, and cross-repository orchestration live here.

**Services must never:**

- Accept or return `*fiber.Ctx` — services are transport-agnostic
  ```go
  // WRONG — service coupled to HTTP transport
  func (s *authService) Login(c *fiber.Ctx) error

  // CORRECT — service is transport-agnostic
  func (s *authService) Login(ctx context.Context, input domain.LoginInput) (*domain.AuthResult, error)
  ```

- Import `gorm.io/gorm` or `github.com/redis/go-redis` directly
- Return raw GORM errors to callers
- Call another domain's repository directly (accept the other domain's service interface via constructor)
- Know what HTTP verb triggered them

**Services must always:**

- Accept `context.Context` as the first argument to every method
- Implement the interface defined in `internal/domain/`
- Depend on repository interfaces, not concrete repository types
- Return `*apperrors.AppError` for all failure cases — never raw errors

---

### C3. Domain layer rules (`internal/domain/<entity>.go`)

The domain layer is untouchable by infrastructure. It defines the language of the system.

**Absolute rules — zero exceptions:**

- No import of any package outside the Go standard library
  ```go
  // WRONG — any of these in internal/domain/ is a build-breaking violation
  import "gorm.io/gorm"
  import "github.com/gofiber/fiber/v2"
  import "github.com/redis/go-redis/v9"
  import "github.com/google/uuid"  // even uuid — use string or define your own type
  ```

- No GORM struct tags (`gorm:"primarykey"`) on domain structs
- No JSON struct tags beyond what the domain genuinely needs
- No HTTP status codes, no HTTP concepts of any kind
- Repository interfaces are defined here and implemented elsewhere
- Service interfaces are defined here and implemented elsewhere

**What belongs in the domain layer:**
- Entity structs with pure Go field types
- Value objects (Money, Coordinates, Address, PropertyStatus)
- Repository interfaces
- Service interfaces
- Input/output types for service methods (e.g. `CreatePropertyInput`, `PropertyFilter`)
- Domain-level constants and enums
- Business rule methods on entity types (`func (p *Property) IsAvailable() bool`)

---

### C4. Repository rules (`internal/<domain>/repository.go`)

The repository is the only place in a domain folder that talks to the database or cache.

**Repository must never:**

- Import `github.com/gofiber/fiber/v2`
- Contain business logic — the service decides what to do; the repository decides how to store it
- Expose GORM model types to callers — always map to/from domain types before returning
- Return `gorm.ErrRecordNotFound` — always translate to `apperrors.NotFound()`
  ```go
  // WRONG — leaks infrastructure error type
  return nil, result.Error  // could be gorm.ErrRecordNotFound

  // CORRECT — maps to domain error
  if errors.Is(result.Error, gorm.ErrRecordNotFound) {
      return nil, apperrors.NotFound("property not found")
  }
  ```

**Repository must always:**

- Accept `context.Context` as the first argument
- Implement the interface defined in `internal/domain/`
- Map GORM models to domain types before returning
- Map domain types to GORM models before writing
- Use `db.WithContext(ctx)` on every query — never omit context

---

### C5. Infrastructure rules (`internal/infrastructure/<driver>/`)

Infrastructure packages initialize and configure external systems. They are drivers, not business logic.

**Infrastructure must never:**

- Contain business rules
- Import domain service interfaces and implement them — only repository-level interfaces live here
- Be called directly from handlers or services (only from repositories via constructor injection)

**Infrastructure must always:**

- Return errors wrapped with `fmt.Errorf("driver: %w", err)`
- Be initializable in a single `New*()` constructor function
- Expose typed clients, not raw connections

---

## Rule Set D — Infrastructure Rules

---

### D1 — GORM models are infrastructure, not domain

GORM model structs live exclusively in `internal/infrastructure/postgres/models.go`. Domain structs live in `internal/domain/`. They are two separate types.

```go
// WRONG — GORM tag on a domain struct
// internal/domain/user.go
type User struct {
    ID    string `gorm:"primarykey;type:uuid"`  // VIOLATION
    Email string `gorm:"uniqueIndex"`           // VIOLATION
}

// CORRECT — GORM model in infrastructure, domain struct in domain/
// internal/infrastructure/postgres/models.go
type UserModel struct {
    ID    string `gorm:"primarykey;type:uuid"`
    Email string `gorm:"uniqueIndex;not null"`
}

// internal/domain/user.go
type User struct {
    ID    uuid.UUID
    Email string
}
```

Every GORM model must have two mapping methods:

```go
func (m *UserModel) ToDomain() *domain.User       // model → domain
func UserModelFrom(u *domain.User) *UserModel     // domain → model
```

These methods live on the model struct in `models.go`. No mapping logic belongs in `repository.go` beyond calling these methods.

---

### D2 — Redis is infrastructure, not a repository implementation detail

The Redis client is initialized once in `internal/infrastructure/redis/client.go` and injected into repositories that need it. Repositories that use Redis must declare `*redis.Client` as a constructor parameter.

No code outside of `internal/infrastructure/redis/` and repositories may construct a Redis client.

---

### D3 — External API clients are infrastructure

Any client for a third-party API (Stripe, Twilio, Google Maps, AWS S3) lives in `internal/infrastructure/`. It is never instantiated inside a domain service or handler.

---

## Rule Set E — Migration Rules

---

### E1 — Never use AutoMigrate in production

`db.AutoMigrate()` is permitted only when `APP_ENV=development`. A startup check enforces this. Any code path that calls `AutoMigrate()` without an environment guard is a critical violation.

**Why:** AutoMigrate cannot drop columns, rename constraints, reorder indexes, or execute data migrations. It silently fails to apply destructive changes, causing silent schema drift between environments.

---

### E2 — All production migrations are versioned SQL files

Every schema change is expressed as two SQL files:

```
migrations/
  000001_create_users.up.sql      ← forward migration
  000001_create_users.down.sql    ← rollback
  000002_create_properties.up.sql
  000002_create_properties.down.sql
```

Migrations are run via `make migrate-up`, which calls `cmd/migrate/main.go`. The migration binary runs as a separate process before the API starts in CI/CD.

---

### E3 — Never edit an applied migration

Once a migration file has been applied to any environment (development, staging, or production), it is immutable. To change the schema, add a new migration file with the next sequence number.

```
WRONG: edit 000003_add_price_index.up.sql after it has been applied

CORRECT: create 000007_add_price_index_v2.up.sql
```

Editing an applied migration causes `golang-migrate` to detect a checksum mismatch and refuse to run subsequent migrations, breaking all deployments.

---

### E4 — Never gap the migration sequence

Migration files must be sequentially numbered with no gaps.

```
WRONG: 000001, 000002, 000004   ← gap at 000003
CORRECT: 000001, 000002, 000003, 000004
```

---

### E5 — Every up migration has a down migration

Every `.up.sql` file must have a corresponding `.down.sql` file that fully reverses the change. A PR adding a migration without a rollback file is rejected.

---

## Rule Set F — Testing Rules

---

### F1 — Unit tests live beside the source file

Unit tests for `service.go` live in `service_test.go` in the same folder. The test file is in the `auth_test` package (external test package) to test only the public API.

```
internal/auth/
├── service.go
└── service_test.go    ← package auth_test
```

---

### F2 — Services are tested with mock repositories

Unit tests for services inject mock implementations of repository interfaces. No real database. No real Redis. If your service test requires a real database, the service has too much database knowledge — move it to the repository.

A mock is a struct implementing the domain repository interface with function fields:

```go
type mockUserRepository struct {
    createFn      func(ctx context.Context, u *domain.User) error
    findByEmailFn func(ctx context.Context, email string) (*domain.User, error)
}

func (m *mockUserRepository) Create(ctx context.Context, u *domain.User) error {
    return m.createFn(ctx, u)
}
```

---

### F3 — Integration tests use real infrastructure via testcontainers

Integration tests under `test/integration/` spin up real Postgres and Redis using `testcontainers-go`. They do not use SQLite, in-memory databases, or mocks.

```go
// test/integration/auth_test.go — CORRECT
db := testhelpers.NewPostgres(t, ctx)   // real Postgres

// WRONG — SQLite is a different dialect
db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
```

**Why:** Postgres-specific behavior — partial indexes, JSONB operators, `ON CONFLICT DO UPDATE`, array operators — is not reproducible in SQLite. Tests that pass against SQLite can fail against Postgres.

---

### F4 — Test coverage minimums are enforced by CI

| Layer | Minimum coverage |
|---|---|
| Domain | 100% |
| Service | 90% |
| Handler | 70% |
| Repository | 60% (integration tests supplement) |

CI runs `go test -cover` and fails the build if any layer falls below its minimum.

---

### F5 — Table-driven tests are mandatory for functions with multiple input cases

Any function tested with more than two input scenarios must use a table-driven test structure.

```go
// CORRECT
tests := []struct {
    name     string
    input    domain.RegisterInput
    wantErr  apperrors.Code
}{
    {"valid input", validInput, ""},
    {"duplicate email", duplicateInput, apperrors.CodeConflict},
    {"empty password", emptyPwInput, apperrors.CodeValidation},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) { ... })
}
```

---

## Rule Set G — Configuration Rules

---

### G1 — All configuration is typed and loaded once

Configuration lives in a single typed struct in `internal/config/config.go`. It is loaded once at startup and passed through the application via constructor injection.

```go
// WRONG — reading env vars anywhere outside config/
os.Getenv("DATABASE_URL")  // called inside a repository

// CORRECT — config is injected
type userRepository struct {
    db  *gorm.DB
    cfg config.DatabaseConfig
}
```

---

### G2 — `.env.example` is the configuration contract

Every environment variable the application reads must be documented in `.env.example` with a comment explaining its purpose and valid values. `.env.example` is committed to version control. A PR that adds a new env var without adding it to `.env.example` is rejected.

```bash
# .env.example

# Postgres connection string (required)
# Format: postgres://user:password@host:port/dbname?sslmode=disable
DATABASE_URL=postgres://pillow:secret@localhost:5432/pillow_dev?sslmode=disable

# JWT RS256 private key path (required)
# Generate: openssl genrsa -out private.pem 4096
JWT_PRIVATE_KEY_PATH=./secrets/private.pem
```

---

### G3 — Required config vars fail fast on startup

Config fields tagged `required` cause an immediate startup failure with a clear error message if missing. Never allow the application to start with incomplete configuration and fail later at runtime.

---

## Rule Set H — Naming and Package Rules

---

### H1 — Package name matches folder name

```
internal/auth/          → package auth
internal/property/      → package property
internal/infrastructure/postgres/  → package postgres
pkg/pagination/         → package pagination
```

No exceptions. Mismatched package names break IDE navigation and `go doc`.

---

### H2 — All package names are lowercase, no underscores

```
WRONG:  package authService
WRONG:  package auth_service
CORRECT: package auth
```

---

### H3 — No `I` prefix on interface names

```
WRONG:  type IUserRepository interface
CORRECT: type UserRepository interface
```

The `I` prefix is a Java convention. Go does not use it. Interfaces are named by what they represent (`UserRepository`, `PropertyService`) or by what they do (`Issuer`, `AuditLogger`).

---

### H4 — Context is always the first parameter for I/O functions

Every method that touches a database, cache, external API, or any blocking I/O must take `context.Context` as its first parameter.

```go
// CORRECT
func (s *propertyService) Search(ctx context.Context, f domain.PropertyFilter) ([]domain.Property, error)

// WRONG — cannot be cancelled, cannot carry trace IDs
func (s *propertyService) Search(f domain.PropertyFilter) ([]domain.Property, error)
```

---

### H5 — Constructors are named `New<Type>`

```go
// CORRECT
func NewPropertyHandler(service domain.PropertyService) *propertyHandler
func NewPropertyService(repo domain.PropertyRepository) *propertyService
func NewPropertyRepository(db *gorm.DB, redis *redis.Client) *propertyRepository

// WRONG
func CreateHandler(...)
func MakeService(...)
func Init(...)
```

---

### H6 — DTOs are never reused as domain types

Request DTOs (`CreatePropertyRequest`) and response DTOs (`PropertyResponse`) are distinct from domain types (`domain.Property`). They are defined in `dto.go` and exist only at the HTTP boundary.

```go
// WRONG — returning a domain type directly from a handler
return c.JSON(property)  // where property is *domain.Property

// CORRECT — map to DTO before returning
return c.JSON(dto.PropertyFromDomain(property))
```

**Why:** A field added to `domain.Property` must not automatically appear in the API response. DTOs decouple the API contract from the internal model.

---

## Rule Set I — Dependency Injection Rules

---

### I1 — All wiring happens in `internal/app/app.go`

The entire dependency graph — every repository, service, and handler — is assembled in one place: `internal/app/app.go`. This is the only place where concrete types are instantiated from infrastructure.

No domain file may call `NewDB()`, `NewRedisClient()`, or any infrastructure constructor. Those are called in `app.go` and injected downward.

---

### I2 — No global variables for dependencies

```go
// WRONG — global database variable
var DB *gorm.DB

// CORRECT — database injected via constructor
type propertyRepository struct {
    db *gorm.DB
}
func NewPropertyRepository(db *gorm.DB) *propertyRepository {
    return &propertyRepository{db: db}
}
```

The only acceptable global variables are: constants, and `slog.SetDefault()` called once in `main.go`.

---

### I3 — Constructors accept interfaces, not concrete types

```go
// WRONG — handler depends on concrete service type
func NewPropertyHandler(svc *propertyService) *propertyHandler

// CORRECT — handler depends on domain interface
func NewPropertyHandler(svc domain.PropertyService) *propertyHandler
```

**Why:** When handlers depend on interfaces, they can be tested with a mock service. When they depend on concrete types, the entire service and repository stack must be initialized to test a single handler.

---

### I4 — No init() functions

`init()` functions are banned throughout the codebase. They execute implicitly, cannot accept parameters, cannot return errors, and make startup order unpredictable. All initialization happens explicitly in `app.go` with proper error handling.

---

## Rule Set J — Error Handling Rules

---

### J1 — All errors flow through `apperrors`

Every error returned across layer boundaries must be either an `*apperrors.AppError` or wrapped with `fmt.Errorf("context: %w", err)` to preserve the error chain.

Raw errors from GORM, Redis, or the standard library must be translated at the repository boundary before they leave the repository layer.

---

### J2 — Never return `gorm.ErrRecordNotFound` from a repository

```go
// WRONG — leaks infrastructure error type
return nil, result.Error

// CORRECT — translates to domain error at the boundary
if errors.Is(result.Error, gorm.ErrRecordNotFound) {
    return nil, apperrors.NotFound("user not found")
}
```

---

### J3 — Never swallow errors

```go
// WRONG — error silently discarded
result, _ := s.userRepo.FindByEmail(ctx, email)

// CORRECT — every error is handled
result, err := s.userRepo.FindByEmail(ctx, email)
if err != nil {
    return nil, fmt.Errorf("find user by email: %w", err)
}
```

The only exception: errors from non-critical side effects (e.g. audit log write failures) may be logged and discarded if failing them would degrade the primary operation. This must be explicitly commented.

---

### J4 — HTTP status mapping happens in one place

Services and repositories return `*apperrors.AppError`. Handlers never call `c.Status(404)` manually for business errors. The global error middleware in `internal/app/middleware.go` maps `AppError.Code` to HTTP status.

```go
// WRONG — handler manually maps errors to HTTP status
if errors.Is(err, apperrors.CodeNotFound) {
    return c.Status(404).JSON(...)
}

// CORRECT — handler returns the error; middleware maps it
return err
```

---

## 14. Adding a New Domain — Checklist

When adding a new business capability to Pillow, follow this checklist exactly. Every step is required.

```
□ 1. Define the domain types in internal/domain/<entity>.go
      - Entity struct (pure Go, no GORM tags)
      - Repository interface
      - Service interface
      - Input/output types for service methods

□ 2. Create the domain folder: internal/<domain>/

□ 3. Create internal/<domain>/dto.go
      - Request structs with validation tags
      - Response structs
      - ToDomain() and FromDomain() mapping methods

□ 4. Create internal/<domain>/repository.go
      - Struct with *gorm.DB (and *redis.Client if needed)
      - Implements domain.<Entity>Repository interface
      - All GORM errors translated to apperrors at this boundary

□ 5. Add GORM model to internal/infrastructure/postgres/models.go
      - GORM struct tags on model, not on domain struct
      - ToDomain() and ModelFrom() mapping methods

□ 6. Create internal/<domain>/service.go
      - Struct with repository interface as field
      - Implements domain.<Entity>Service interface
      - All business rules enforced here
      - context.Context as first param on every method

□ 7. Create internal/<domain>/handler.go
      - Struct with service interface as field
      - Constructor: NewHandler(svc domain.<Entity>Service)
      - HTTP binding, validation, service call, DTO mapping only

□ 8. Create the migration files
      - migrations/NNNNNN_create_<entity>s.up.sql
      - migrations/NNNNNN_create_<entity>s.down.sql
      - Run make migrate-up in development

□ 9. Wire in internal/app/app.go
      - Instantiate repository: <domain>.NewRepository(db)
      - Instantiate service: <domain>.NewService(repo)
      - Instantiate handler: <domain>.NewHandler(service)

□ 10. Register routes in internal/app/router.go
       - Add route group for the domain
       - Apply appropriate middleware (auth, rate limit)

□ 11. Write service_test.go
       - Mock repository implementing domain.<Entity>Repository
       - Test all service methods including error paths

□ 12. Update .env.example if any new config is required

□ 13. Update docs/api/openapi.yaml with new endpoints
```

---

## 15. Adding a New Endpoint — Checklist

When adding an endpoint to an existing domain:

```
□ 1. Add the method signature to the service interface in internal/domain/

□ 2. Implement the method in internal/<domain>/service.go

□ 3. Add any new repository methods to the repository interface in internal/domain/

□ 4. Implement new repository methods in internal/<domain>/repository.go

□ 5. Add request/response types to internal/<domain>/dto.go if needed

□ 6. Add the handler method to internal/<domain>/handler.go

□ 7. Register the route in internal/app/router.go

□ 8. Write or update service_test.go for the new service method

□ 9. Add migration if new columns/tables are required (follow Rule Set E)

□ 10. Update openapi.yaml
```

---

## 16. What Goes Where — Quick Reference

| Scenario | Correct location |
|---|---|
| Business rule: "a property cannot be listed below minimum price" | `internal/property/service.go` |
| HTTP request binding and validation | `internal/property/handler.go` |
| SQL query to find properties by city | `internal/property/repository.go` |
| GORM struct tags for the properties table | `internal/infrastructure/postgres/models.go` |
| `PropertyRepository` interface definition | `internal/domain/property.go` |
| `Property` struct definition | `internal/domain/property.go` |
| Response shape sent to the API client | `internal/property/dto.go` |
| Redis client initialization | `internal/infrastructure/redis/client.go` |
| JWT token generation | `internal/infrastructure/tokenutil/jwt.go` |
| Wiring repository + service + handler | `internal/app/app.go` |
| Route registration | `internal/app/router.go` |
| Auth middleware (JWT verification) | `internal/middleware/auth.go` |
| Typed config struct | `internal/config/config.go` |
| Typed error taxonomy | `internal/apperrors/errors.go` |
| Cursor pagination helper | `pkg/pagination/pagination.go` |
| Schema migration SQL | `migrations/` |
| Integration test with real Postgres | `test/integration/` |
| Unit test with mocked dependencies | `internal/<domain>/service_test.go` |
| Shared test setup (testcontainers) | `test/testhelpers/setup.go` |
| New environment variable | `internal/config/config.go` + `.env.example` |

---

## 17. Violations and Enforcement

### Automated enforcement

The following violations cause an automatic CI failure and block the PR from merging:

| Violation | Enforced by |
|---|---|
| Domain file imports non-stdlib package | `golangci-lint depguard` |
| Handler imports `gorm.io/gorm` | `golangci-lint depguard` |
| Service imports `github.com/gofiber/fiber` | `golangci-lint depguard` |
| Unhandled error | `golangci-lint errcheck` |
| Service coverage below 90% | `go test -cover` threshold |
| Domain coverage below 100% | `go test -cover` threshold |
| Missing migration rollback file | Custom CI check |
| `os.Getenv()` outside `internal/config/` | `golangci-lint` custom rule |
| Gapped migration sequence | Custom CI check |

### Code review enforcement

Reviewers are required to reject PRs that contain:

- Business logic in a handler
- Cross-domain imports (one domain importing another)
- GORM errors returned from a service
- `utils.go` or `helpers.go` files
- Test files that use SQLite as a Postgres stand-in
- `init()` functions
- New env vars not documented in `.env.example`
- GORM struct tags on domain types in `internal/domain/`

### The rule for reviewers

If you are unsure whether a change violates these rules, the question to ask is:

> Does this change make the system harder to test, harder to extract into a microservice, or harder for a new developer to navigate?

If yes, it violates the spirit of these rules even if no specific rule covers it.

---

## 18. The Smell Test

Before submitting a PR, run this smell test against your changes. If any answer is "yes", your change likely violates these rules.

| Question | If yes, the problem is |
|---|---|
| Does my handler have an `if` that checks a business condition? | Business logic in transport layer |
| Does my service method accept `*fiber.Ctx`? | Transport concern leaking into service |
| Does my service file have `import "gorm.io/gorm"`? | Infrastructure leaking into service |
| Does my domain file have any non-stdlib import? | Domain layer contaminated |
| Is my test file spinning up a real database for a unit test? | Wrong test type; use mock repository |
| Does understanding my feature require reading files in 3+ folders? | Domain boundary in wrong place |
| Is there a `utils.go` or `helpers.go` in my PR? | Vague abstraction; name files by content |
| Does my PR touch more than 2 domain folders? | Either coupling problem or wrong domain boundary |
| Am I calling `os.Getenv()` outside `internal/config/`? | Config leak; add to config struct |
| Is my constructor accepting `*concreteType` instead of an interface? | Breaks testability and DI contract |
| Did I add a migration without a `.down.sql`? | Missing rollback |
| Did I add a new env var without updating `.env.example`? | Undocumented configuration |

---

> **This document is a living standard.** If a rule needs updating because the platform has evolved, open a PR against this file with a justification. Rules are not changed casually — every change must be discussed and approved by at least two senior engineers.

---

*End of Folder-structure-rules.md — Pillow Platform*
