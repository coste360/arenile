# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

@AGENTS.md

## Claude Code specifics
- Path-scoped rules live in `.claude/rules/`; they load automatically when you
  touch matching files. Follow them as strictly as AGENTS.md.
- Use the project skills for recurring procedures: `/new-endpoint`, `/new-migration`,
  `/new-price-rule-test`, `/new-payment-driver`, `/new-domain-event`, `/pre-pr-check`.
- Before declaring a task complete, delegate a review to the `reviewer` subagent.
  For changes in availability, pricing, booking or payment packages, also run
  `invariants-guardian`.
- Use `test-writer` to extend table-driven tests instead of writing them inline
  when more than a few cases are needed.
- Formatting runs automatically after edits (hook `scripts/agents/format-file.sh`);
  do not run formatters manually.
- Edits to generated files and to migrations already on `main` are blocked by the
  hook `scripts/agents/guard-generated.sh`. If it blocks you, change the source
  (`api/openapi.yaml`, then `make gen`) or add a new migration instead.
- Personal preferences go in `CLAUDE.local.md` (git-ignored), never in this file.
