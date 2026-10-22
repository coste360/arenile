---
name: pre-pr-check
description: Final verification before opening a pull request - runs make check-full, checks changelog, docs, invariants and generated code, and drafts the PR description from the template. Use before opening or updating a PR.
---
# Pre-PR check

1. `git status` and `git diff main...HEAD --stat`: list changed files; flag anything outside the task scope.
2. Run `make check-full`. If it fails, fix and re-run; do not continue with failures.
3. Confirm `make gen` leaves no diff (included in `make check`; re-check if `api/openapi.yaml` changed).
4. Generated files and merged migrations untouched:
   `git diff main...HEAD --name-only` must not contain `*.gen.go` or `web/shared/api/generated/**`
   unless `api/openapi.yaml` also changed; no modified (only added) files under `internal/migrations/`.
   Every new migration exists in both `sqlite/` and `postgres/`.
5. Tests: new behaviour has tests; a bug fix has a test that fails without the fix. Changes in availability,
   booking or payments include a `TestConcurrency_*` test.
6. Invariants: if the change touches availability, pricing, booking or payment, run the
   `invariants-guardian` subagent and address its findings; update `docs/invariants.md` if needed.
7. Docs: behaviour, config or API changes are reflected in `docs/` and in a `CHANGELOG.md` entry
   under "Unreleased". New runtime dependencies are justified.
8. Run the `reviewer` subagent; resolve every CHANGES REQUESTED item.
9. Draft the PR description from `.github/pull_request_template.md`: what changed, why, how it was
   tested (commands and results), spec sections/ADRs involved, open questions. Title in Conventional
   Commits format.

## Verification criteria
- `make check-full` output is green and quoted (summary) in the PR description.
- Reviewer verdict is APPROVE.
