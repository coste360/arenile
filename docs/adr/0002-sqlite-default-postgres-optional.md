# 0002 — SQLite di default, PostgreSQL opzionale

- **Stato:** accettato
- **Data:** 2026-10-22
- **Sezioni della specifica:** `docs/spec/03-architettura-e-stack.md` (3.4), `docs/spec/06-schema-dati.md`

## Contesto

La maggior parte delle installazioni gestisce uno o pochi stabilimenti con traffico modesto (picchi
durante l'apertura delle prenotazioni estive). Un database server separato aumenta la complessità di
installazione e backup. Chi ospita molti stabilimenti, o vuole più istanze in parallelo, ha invece
bisogno di PostgreSQL.

## Decisione

- **SQLite** in modalità WAL, file `${ARENILE_DATA_DIR}/arenile.db`, driver puro Go `modernc.org/sqlite`
  (nessun CGO), è il database predefinito.
- **PostgreSQL** si attiva valorizzando `DATABASE_URL` con `postgres://` (driver `pgdriver` di Bun).
- Accesso dati con `uptrace/bun` usando solo il sottoinsieme SQL comune; le differenze (tipi, indici
  parziali) stanno **solo** nelle migrazioni per dialetto: `internal/migrations/sqlite` e
  `internal/migrations/postgres`, gestite con goose e numerate in parallelo.
- ID UUIDv7 (testo in SQLite, `uuid` in PostgreSQL); date `YYYY-MM-DD`; istanti UTC; importi `int64`.

## Conseguenze

- Positive: installazione standard con un solo volume; backup consistente con `VACUUM INTO`; lo stesso
  codice gira su entrambi i database.
- Negative: ogni migrazione va scritta due volte; niente funzionalità specifiche di PostgreSQL nel codice
  applicativo; i test di integrazione vanno eseguiti su entrambi i dialetti.
- Verifiche: test con build tag `integration` su SQLite in memoria sempre e su PostgreSQL quando
  `ARENILE_TEST_POSTGRES_URL` è impostata (`make test-db`); regola `.claude/rules/migrations.md`; hook che
  impedisce di modificare migrazioni già presenti su `main`.
