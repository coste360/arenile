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

### Changed

- `pkg/ext`: `PaymentProvider.ParseWebhook` riceve anche le credenziali decifrate dello stabilimento (`ParseWebhook(ctx, r, creds)`), diversamente dalla spec 13.1 ([ADR 0006](docs/adr/0006-deviazioni-m0-dalla-specifica.md)).
- `make test-race` compila con il tag `integration` e fallisce se non trova test `TestConcurrency_*`.

### Added (deviazioni e integrazioni alla specifica, ADR 0006)

- Modalità sviluppo `ARENILE_DEV=true` / `arenile serve --dev`: URL di base facoltativo e chiave segreta generata e salvata in `dev-secret.key`.
- Endpoint `POST /api/admin/v1/auth/totp/enroll` e `POST /api/admin/v1/auth/totp/confirm` per attivare la verifica in due passaggi.
- Creazione del primo owner all'avvio da `ARENILE_BOOTSTRAP_OWNER_EMAIL`, `_NAME`, `_ORG` e `ARENILE_ADMIN_PASSWORD` (solo se non esistono utenti), necessaria con l'immagine distroless senza shell.
- `compose.yaml` nella radice e `make up`: istanza locale vuota compilata dai sorgenti; i compose in `deploy/` hanno una sezione `build`.

### Fixed

- La consegna dei webhook rivendica ogni tentativo con un lease: più istanze sullo stesso database non inviano due volte lo stesso webhook.
- Il recupero di una `Idempotency-Key` scaduta non può più cancellare la chiave appena rivendicata da una richiesta concorrente.
- Il segreto di firma degli endpoint webhook è cifrato con l'id dell'endpoint come dato associato (AAD).

[Unreleased]: https://github.com/coste360/arenile/commits/main
