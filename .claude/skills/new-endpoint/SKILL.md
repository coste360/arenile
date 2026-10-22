---
name: new-endpoint
description: Add a new REST endpoint end to end (OpenAPI, generated code, handler, domain service, tests). Use when a task asks for a new API operation.
argument-hint: "[METHOD] [path]"
---
# New endpoint: $ARGUMENTS

Context to read first: `docs/spec/17-api.md`, `docs/spec/16-utenti-ruoli-e-autenticazione.md`,
`docs/architecture.md`, `.claude/rules/api-contract.md`.

1. Read `api/openapi.yaml` and find the closest existing operation to copy its style.
2. Add the operation: tags, `operationId` (camelCase, verb first), request/response schemas
   under `components/schemas`, `application/problem+json` errors (400, 401, 403, 404, 409, 422 as relevant),
   cursor pagination for collections, `Idempotency-Key` header for creating `POST`s,
   `ETag`/`If-Match` for configuration resources.
3. Run `make gen`. Do not edit generated files (`internal/http/api/*.gen.go`,
   `web/shared/api/generated/**`). The build must now fail until the new handler method exists.
4. Implement the handler in `internal/http/<admin|public>`; keep it thin: decode, call the service, encode.
   Map domain sentinel errors to problem types (`spot_unavailable`, `no_price`, `policy_violation`, ...).
5. Put logic in the matching `internal/domain/<area>` service; add store methods in `internal/store` if needed.
   Every query is scoped by `establishment_id` from the auth context.
6. Check permissions in the domain service for admin endpoints (spec section 16.1 table).
7. Tests:
   - domain unit test, table-driven, `TestXxx_Scenario`, with `clock.Fake`;
   - handler test (`httptest`) asserting status, problem `type`, and that the response matches the OpenAPI schema;
   - a cross-establishment access test returning 404/403 for admin endpoints;
   - store changes: integration test with `//go:build integration`.
8. If the frontend needs it, add the TanStack Query hook in `web/shared` on top of the generated client.
9. Run `make check`. Report the files changed and the tests added.

## Verification criteria
- `make check` passes and `make gen` produces no diff.
- The operation appears in `api/openapi.yaml` with every response documented.
- No SQL outside `internal/store`, no business logic in the handler.
- At least one test fails if the permission check or the `establishment_id` scope is removed.
