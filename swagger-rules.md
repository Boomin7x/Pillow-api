# Swagger Rules

Every endpoint shipped in this project must be documented in `docs/api/openapi.yaml` before the implementing PR is merged. Documentation is not a follow-up task. It ships in the same commit as the code.

---

## What "documented" means

An endpoint is documented when all of the following are present in `docs/api/openapi.yaml`:

1. A path entry under `paths` with the correct HTTP method and path.
2. A populated `summary` and `description` that explain what the endpoint does and any non-obvious behaviour (side effects, token rotation, audit events, etc.).
3. The correct `tags` entry matching the domain folder name (e.g. `auth`, `property`, `user`).
4. A `requestBody` schema if the endpoint accepts a body, defined as a `$ref` to a named schema under `components/schemas`.
5. A response entry for every HTTP status code the handler can return — including all error codes (`400`, `401`, `403`, `404`, `409`, `422`, `429`, `500`).
6. Every response references a named schema via `$ref` — inline schemas are not permitted.
7. If the endpoint is protected, it declares `security: [{ bearerAuth: [] }]`. If it is public, it explicitly declares `security: []`.
8. At least one request body example and at least one success response example.

---

## Schema rules

- Every new domain type that crosses the HTTP boundary gets its own named schema under `components/schemas`.
- Schemas use `$ref` for nested types — they do not inline object definitions.
- Required fields are declared explicitly in `required:` arrays — never rely on nullability to imply optionality.
- Password fields set `format: password` so Swagger UI masks them.
- UUID fields set `format: uuid`.
- Timestamp fields set `format: date-time`.

---

## Adding a new domain

When a new domain folder is created under `internal/`, the following must be added to `docs/api/openapi.yaml` before any endpoint in that domain is merged:

1. A new entry under `tags` with the domain name and a one-sentence description.
2. All path entries for the domain's endpoints.
3. All request and response schemas under `components/schemas`.

---

## Enforcement

A PR that adds or modifies a handler without a corresponding update to `docs/api/openapi.yaml` is rejected in code review. The reviewer must check:

- Does every new route in `internal/app/router.go` have a matching path in the spec?
- Does every new DTO in `dto.go` have a matching schema in `components/schemas`?
- Does every possible error response from the handler appear in the spec?

There are no exceptions. An endpoint with no documentation does not exist as far as any consumer of this API is concerned.
