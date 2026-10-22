# AGENTS.md

Arenile is a source-available (BSL 1.1) management system for beach establishments
(stabilimenti balneari): map of umbrellas, bookings, pricing, payments, cash desk.
Go backend + two React SPAs, shipped as a single binary. Maintained by Coste360 SRL.
The specification (Italian) lives in `docs/spec/`, one file per section; it is authoritative.

## Commands (always use make, never ad-hoc commands)
- `make setup` once, `make dev` to run locally, `make seed` for demo data
- `make check` after every change (must pass before you say you are done; < 2 min)
- `make check-full` before opening a pull request (adds `test-db`, `test-race`, `e2e`)
- `make gen` after any change to `api/openapi.yaml`
- `make fmt` formats Go (gofumpt, goimports) and frontend (Prettier); in Claude Code a hook does it
- Focused runs: `make test` (Go unit + Vitest), `make test-db` (integration, SQLite + PostgreSQL),
  `make test-race` (concurrency), `make lint`

## Map of the repository
- `cmd/arenile` entrypoint and CLI (`serve`, `migrate up|status`, `admin create`, `seed demo`,
  `backup`, `restore`, `version`)
- `internal/app` composition root and extension registry; `internal/config` env parsing (spec 24.4)
- `internal/domain/<area>` business logic, one package per area; no HTTP, no SQL here
- `internal/store` database access (Bun); the only place with SQL
- `internal/http/{admin,public,middleware,sse}` handlers; thin, they call domain services
- `internal/http/api` generated server code and models (`api.gen.go`)
- `internal/http/static` embedded SPA builds (`admin/`, `booking/`)
- `internal/jobs` in-process job runner; `internal/migrations/{sqlite,postgres}` goose migrations
- `internal/clock`, `internal/money`, `internal/calendar` shared primitives (use them)
- `pkg/ext` public extension interfaces; changing them is a breaking change
- `api/openapi.yaml` the API contract (OpenAPI 3.1), source of truth
- `web/admin`, `web/booking`, `web/shared` frontends (npm workspaces rooted in `web/`)
- `docs/architecture.md`, `docs/invariants.md`, `docs/domain-glossary.md`, `docs/adr/`
  read before large changes

## Generated code (never edit by hand)
- `internal/http/api/*.gen.go` from `api/openapi.yaml` via oapi-codegen (`api/oapi-codegen.yaml`)
- `web/shared/api/generated/**` (openapi-typescript `schema.d.ts`, used with openapi-fetch)
- `make check` regenerates and fails if the generated code differs from what is committed.

## Non-negotiable rules
- Money is always `int64` cents (`money.Cents`). Never float, never decimal strings in logic.
- Calendar dates are `YYYY-MM-DD` (`calendar.Date`) in the establishment timezone;
  instants are UTC. Time comes from an injected `clock.Clock`, never `time.Now()` in domain code.
- Availability is enforced by DB constraints (`spot_allocations` unique key,
  conditional updates on `pool_usage`). Never replace them with app-level checks.
- The pricing engine (`internal/domain/pricing`) is a pure function: no DB, no clock,
  no randomness. Pass time and data in.
- Every state change of a booking goes through `internal/domain/booking` and writes
  the audit log and an outbox event in the same transaction.
- Booking confirmation happens only from the payment webhook or the manual driver,
  never from the browser return URL.
- Every operational query is scoped by `establishment_id` from the auth context.
- API first: change `api/openapi.yaml`, run `make gen`, then implement.
- Never edit generated files (`*.gen.go`, `web/shared/api/generated/**`).
- Never modify a migration that is already on `main`; add a new one, for both dialects.
- No new runtime dependency (Go module or npm package) without explaining why in the PR.
- User-facing strings go through i18n keys; Italian is the source language.
- Frontend UI = shadcn/ui only (`web/shared/src/ui`, ADR 0009): no custom CSS (no new `.css`
  files, CSS modules, CSS-in-JS or `style={{…}}` outside map geometry), only shadcn variants and
  Tailwind utilities on theme tokens; no arbitrary colours or sizes (`bg-[#…]`, `text-[10px]`). Add
  components with `npx shadcn@latest add <name> --cwd shared` (from `web/`); `npm run lint` (CSS
  guard, exemptions listed in `web/scripts/css-guard.mjs` with their reason) enforces it. Details in
  `web/shared/src/ui/README.md`.
- Never log emails, phone numbers, tokens or secrets in clear.
- Code comments and identifiers in English.
- License: the project is BSL 1.1 (SPDX `BUSL-1.1`, converts to AGPL-3.0-or-later three years
  after each release; see `docs/adr/0007-licenza-bsl.md`). Never call it "open source" in new text
  (use "source available"); never edit `LICENSE` or `CLA.md`; new dependencies must have a license
  compatible with both BSL 1.1 and AGPL-3.0 (e.g. MIT, BSD, Apache-2.0).

## Tests
- Unit tests: `go test ./...` must not need Docker or network.
- DB integration tests carry the build tag `integration` (`//go:build integration`):
  SQLite in memory always; PostgreSQL only when `ARENILE_TEST_POSTGRES_URL` is set (skip otherwise).
- Concurrency tests are named `TestConcurrency_*` (run with `-race`), at least 100 parallel
  attempts on the same resource.
- Table-driven tests in the same package, named `TestXxx_Scenario`; property tests with
  `pgregory.net/rapid` for pricing and availability invariants.
- Frontend: Vitest + Testing Library; end-to-end: Playwright in `e2e/`.

## Definition of done
- `make check` passes; `make check-full` passes for changes touching
  availability, pricing, payments, migrations or the public booking flow
- New behaviour has tests; bug fixes include a test that fails without the fix
- Invariants in `docs/invariants.md` still hold; new invariants are added there with their test
- Docs updated when behaviour, config or API change (`docs/`, `CHANGELOG.md` entry)
- The PR description lists what changed, how it was tested and any open question

## Working style
- Keep changes small and scoped to the task; do not refactor unrelated code.
- Read the spec sections and ADRs referenced by the task before writing code.
- If the task is ambiguous or conflicts with an invariant, stop and ask in the
  issue or PR instead of guessing.
- Changing an existing decision requires a new ADR in `docs/adr/`.
- Conventional Commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`).
