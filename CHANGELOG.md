# Changelog

Tutte le modifiche rilevanti di Arenile sono documentate in questo file.

Il formato segue [Keep a Changelog](https://keepachangelog.com/it-IT/1.1.0/) e il progetto adotta il [Versionamento Semantico](https://semver.org/lang/it/). I rilasci sono generati a partire dai messaggi [Conventional Commits](https://www.conventionalcommits.org/it/v1.0.0/). Ogni versione minore include, quando necessario, una sezione **Note di aggiornamento**.

## [Unreleased]

Milestone **M0 — Fondamenta** (in sviluppo).

### Added

- Specifica tecnica e funzionale completa in `docs/spec/` (un file per sezione).
- Struttura del repository: monobinario Go (`cmd/arenile`), SPA pannello e booking in `web/` (npm workspaces), contratto API in `api/openapi.yaml`.
- Librerie di base: `internal/clock` (orologio iniettabile), `internal/money` (importi in centesimi, arrotondamento half-even, scorporo IVA), `internal/calendar` (date di calendario, intervalli, fasce AM/PM).
- CLI con i comandi `serve`, `migrate up|status`, `admin create`, `seed demo`, `backup --out`, `restore --in`, `version`.
- Configurazione da variabili d'ambiente (`ARENILE_*`, `DATABASE_URL`).
- Migrazioni per SQLite e PostgreSQL, autenticazione e ruoli, audit log, outbox eventi, job runner in-process.
- Motore prezzi e motore delle policy come librerie pure con test table-driven e di proprietà.
- Dockerfile multi-stage, `docker-compose.yml` e variante PostgreSQL.
- Makefile come unica interfaccia (`setup`, `dev`, `gen`, `lint`, `test`, `test-db`, `test-race`, `e2e`, `check`, `check-full`, `seed`, `fmt`) e CI su GitHub Actions.
- Supporto agli agenti AI: `AGENTS.md`, `CLAUDE.md`, `.claude/` con regole, skill e subagent.
- README tecnico in italiano, guide per gestori in `docs/guida/` (installazione, privacy e GDPR, nota fiscale).
- File di governance: licenza AGPL-3.0, CLA (bozza da validare), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1), `SECURITY.md`, `TRADEMARK.md`, modelli di issue e pull request.

[Unreleased]: https://github.com/coste360/arenile/commits/main
