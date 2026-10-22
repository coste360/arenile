---
name: new-migration
description: Create a new database migration pair for SQLite and PostgreSQL with matching numbers, update Bun models and run integration tests. Use whenever the schema must change.
argument-hint: "[short_snake_case_name]"
---
# New migration: $ARGUMENTS

Context to read first: `docs/spec/06-schema-dati.md`, `.claude/rules/migrations.md`,
`docs/adr/0002-sqlite-default-postgres-optional.md`.

1. List `internal/migrations/sqlite/` and `internal/migrations/postgres/`; find the highest number
   (goose format, e.g. `00007_*.sql`). The new number is highest + 1, identical in both directories.
   Both directories must already have the same highest number; if not, stop and report.
2. Create `internal/migrations/sqlite/NNNNN_<name>.sql` and `internal/migrations/postgres/NNNNN_<name>.sql`
   with `-- +goose Up` and `-- +goose Down` sections.
3. Write the dialect-specific DDL:
   - IDs: `TEXT` in SQLite, `uuid` in PostgreSQL; dates `TEXT` (`YYYY-MM-DD`) vs `date`;
     instants `TEXT`/`timestamp` UTC vs `timestamptz`; money `INTEGER` vs `bigint`; JSON `TEXT` vs `jsonb`.
   - Operational tables get `establishment_id` plus an index on it, and `created_at`, `updated_at`.
   - Keep constraints equivalent across dialects (same unique keys, same checks).
4. Never modify a migration that is already on `main` (the hook blocks it). Changes are additive:
   add column / table, backfill, then switch reads; drop only in a later release.
5. Update or add the Bun models and queries in `internal/store` (common SQL only).
6. Add or extend integration tests (`//go:build integration`) that migrate an empty database up and
   exercise the new columns/constraints on SQLite in memory, and on PostgreSQL when
   `ARENILE_TEST_POSTGRES_URL` is set.
7. Run `make test-db`, then `make check`.
8. If you added a unique constraint or index on a hot table (`spot_allocations`, `pool_usage`,
   `bookings`, `payments`, `outbox_events`), note it in the PR description.

## Verification criteria
- Same number and name exist in both dialect directories.
- `arenile migrate up` on an empty database and `arenile migrate status` show the migration applied on both dialects (covered by `make test-db`).
- Existing migrations on `main` are unchanged (`git diff main -- internal/migrations` only adds files).
