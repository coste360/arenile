## 27. Sviluppo con agenti AI

Il repository è progettato per essere sviluppato anche da agenti di coding (Claude Code e strumenti compatibili con `AGENTS.md`). Tutto ciò che un agente deve sapere si trova nel repository, e ogni verifica che conta è automatica: un agente non deve mai dover indovinare come si compila, come si testa o dove va una modifica.

Questi file fanno parte del deliverable **M0** e vanno creati prima di qualsiasi codice di dominio.

### 27.1 File e cartelle

```
arenile/
├── AGENTS.md                     # istruzioni condivise per tutti gli agenti (fonte unica)
├── CLAUDE.md                     # importa AGENTS.md + note specifiche per Claude Code
├── .claude/
│   ├── settings.json             # permessi e hook condivisi (versionato)
│   ├── rules/                    # regole caricate solo quando si toccano certi percorsi
│   │   ├── go-backend.md
│   │   ├── frontend.md
│   │   ├── api-contract.md
│   │   ├── migrations.md
│   │   ├── availability.md
│   │   └── pricing.md
│   ├── skills/                   # procedure ripetibili, invocabili come /nome
│   │   ├── new-endpoint/SKILL.md
│   │   ├── new-migration/SKILL.md
│   │   ├── new-price-rule-test/SKILL.md
│   │   ├── new-payment-driver/SKILL.md
│   │   ├── new-domain-event/SKILL.md
│   │   └── pre-pr-check/SKILL.md
│   └── agents/                   # subagent specializzati
│       ├── reviewer.md
│       ├── test-writer.md
│       └── invariants-guardian.md
├── .devcontainer/                # ambiente riproducibile (Go, Node, golangci-lint, jq)
├── .github/
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug.yml
│   │   ├── feature.yml
│   │   └── agent-task.yml        # task pronto per un agente
│   └── pull_request_template.md
├── docs/
│   ├── architecture.md           # mappa dei package e flussi principali
│   ├── domain-glossary.md        # glossario (sezione 5.1), fonte unica dei termini
│   ├── invariants.md             # invarianti di dominio verificabili
│   ├── adr/                      # Architecture Decision Records
│   │   ├── 0001-go-monobinary.md
│   │   ├── 0002-sqlite-default-postgres-optional.md
│   │   ├── 0003-allocations-unique-constraint.md
│   │   ├── 0004-pricing-pure-function.md
│   │   └── 0005-openapi-spec-first.md
│   └── spec/                     # questa specifica, divisa per sezione
└── CLAUDE.local.md               # in .gitignore, preferenze personali
```

`CLAUDE.local.md` va aggiunto a `.gitignore` fin dal primo commit.

### 27.2 Il Makefile come unica interfaccia

Agenti e persone usano gli stessi comandi. Nessuna istruzione nei file per agenti deve contenere comandi lunghi o varianti: si richiamano solo target del Makefile.

| Target | Cosa fa | Tempo indicativo |
|---|---|---|
| `make setup` | Installa dipendenze Go e Node, strumenti di generazione | una tantum |
| `make dev` | Backend con ricarica automatica + Vite per le due SPA | — |
| `make gen` | Rigenera server Go e client TS da `api/openapi.yaml` | < 10 s |
| `make lint` | `golangci-lint`, ESLint, Prettier in verifica, `tsc --noEmit` | < 60 s |
| `make test` | Test unitari Go e Vitest | < 60 s |
| `make test-db` | Test di integrazione su SQLite e PostgreSQL (container) | < 3 min |
| `make test-race` | Test di concorrenza con `-race` su disponibilità e pagamenti | < 3 min |
| `make e2e` | Playwright con server di prova e dati demo | < 5 min |
| `make check` | `gen` + verifica che il codice generato non sia cambiato + `lint` + `test` | < 2 min |
| `make check-full` | `check` + `test-db` + `test-race` + `e2e` | < 10 min |
| `make seed` | Database locale con lo stabilimento dimostrativo | < 10 s |
| `make fmt` | Formatta Go (`gofumpt`, `goimports`) e frontend (Prettier) | < 10 s |

Regola: `make check` deve restare sotto i due minuti, perché un agente lo esegue a ogni iterazione. Ciò che è più lento va in `check-full`, eseguito prima di aprire la pull request e in CI.

### 27.3 AGENTS.md

File condiviso tra tutti gli strumenti, scritto in inglese per la community. Deve restare sotto le 200 righe; i dettagli vanno nelle regole per percorso e nei documenti in `docs/`.

```markdown
# AGENTS.md

Arenile is an open source management system for beach establishments
(map of umbrellas, bookings, pricing, payments, cash desk).
Go backend + two React SPAs, shipped as a single binary.

## Commands (always use make, never ad-hoc commands)
- `make setup` once, `make dev` to run locally, `make seed` for demo data
- `make check` after every change (must pass before you say you are done)
- `make check-full` before opening a pull request
- `make gen` after any change to `api/openapi.yaml`

## Map of the repository
- `cmd/arenile` entrypoint and CLI
- `internal/domain/<area>` business logic, one package per area; no HTTP, no SQL here
- `internal/store` database access (Bun); the only place with SQL
- `internal/http` handlers; thin, they call domain services
- `pkg/ext` public extension interfaces; changing them is a breaking change
- `api/openapi.yaml` the API contract, source of truth
- `web/admin`, `web/booking`, `web/shared` frontends
- `docs/architecture.md`, `docs/invariants.md`, `docs/adr/` read before large changes

## Non-negotiable rules
- Money is always `int64` cents. Never float, never decimal strings in logic.
- Calendar dates are `YYYY-MM-DD` in the establishment timezone; instants are UTC.
- Availability is enforced by DB constraints (`spot_allocations` unique key,
  conditional updates on `pool_usage`). Never replace them with app-level checks.
- The pricing engine (`internal/domain/pricing`) is a pure function: no DB, no clock,
  no randomness. Pass time and data in.
- Every state change of a booking goes through `internal/domain/booking` and writes
  the audit log and an outbox event in the same transaction.
- Every operational query is scoped by `establishment_id` from the auth context.
- API first: change `api/openapi.yaml`, run `make gen`, then implement.
- Never edit generated files (`*.gen.go`, `web/shared/api/generated/**`).
- Never modify a migration that is already on `main`; add a new one, for both dialects.
- No new runtime dependency (Go module or npm package) without explaining why in the PR.
- User-facing strings go through i18n keys; Italian is the source language.

## Definition of done
- `make check` passes; `make check-full` passes for changes touching
  availability, pricing, payments, migrations or the public booking flow
- New behaviour has tests; bug fixes include a test that fails without the fix
- Docs updated when behaviour, config or API change (`docs/`, `CHANGELOG.md` entry)
- The PR description lists what changed, how it was tested and any open question

## Working style
- Keep changes small and scoped to the task; do not refactor unrelated code.
- If the task is ambiguous or conflicts with an invariant, stop and ask in the
  issue or PR instead of guessing.
- Conventional Commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`).
```

### 27.4 CLAUDE.md

Importa `AGENTS.md` (così le istruzioni restano in un solo file) e aggiunge solo ciò che è specifico di Claude Code.

```markdown
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
- Formatting runs automatically after edits (hook); do not run formatters manually.
```

### 27.5 Regole per percorso (`.claude/rules/`)

Ogni regola ha un frontmatter `paths` e viene caricata solo quando l'agente lavora su file corrispondenti, così il contesto resta leggero.

**`go-backend.md`**
```markdown
---
paths:
  - "**/*.go"
---
- Errors: wrap with `fmt.Errorf("context: %w", err)`; domain errors are typed
  sentinel values in each package (`ErrSpotUnavailable`, `ErrNoPrice`, ...).
- `context.Context` is the first parameter of every function that does I/O.
- Logging only with `log/slog`; never log emails, phones or tokens in clear.
- Tests are table-driven, in the same package, named `TestXxx_Scenario`.
- No global state; dependencies are passed through constructors.
```

**`frontend.md`**
```markdown
---
paths:
  - "web/**/*.{ts,tsx}"
---
- Server data only through TanStack Query hooks built on the generated client.
- Forms: react-hook-form + zod; schemas live next to the form.
- UI primitives from `web/shared/ui` (shadcn/ui); no new component libraries.
- `web/booking` has a 150 KB gzip JS budget: no heavy dependencies, lazy-load the map.
- Every interactive map element needs a role, an aria-label and keyboard support.
- All text through i18next keys; add the Italian string and an English one.
```

**`api-contract.md`**
```markdown
---
paths:
  - "api/**"
  - "internal/http/**"
---
- Edit `api/openapi.yaml` first, then `make gen`, then the handler.
- Errors are `application/problem+json` with a stable `type` slug.
- Collections use cursor pagination; creation endpoints accept `Idempotency-Key`.
- Public endpoints never return customer data except via the booking session
  token or the manage token.
```

**`migrations.md`**
```markdown
---
paths:
  - "internal/migrations/**"
  - "internal/store/**"
---
- Every migration exists for both `sqlite/` and `postgres/` with the same number.
- Migrations are forward-only and must not lose data; add, backfill, then switch.
- Never edit a migration already merged on `main`.
- New unique constraints or indexes on hot tables need a note in the PR.
```

**`availability.md`**
```markdown
---
paths:
  - "internal/domain/availability/**"
  - "internal/domain/booking/**"
---
Read `docs/invariants.md` before changing anything here.
- A spot is occupied iff a row exists in `spot_allocations` for (spot, date, slot).
- Holds expire; expired holds must be ignored and cleaned on conflict.
- Any change here needs a concurrency test (`make test-race`) with at least
  100 parallel attempts on the same resource.
```

**`pricing.md`**
```markdown
---
paths:
  - "internal/domain/pricing/**"
---
- The engine is pure: inputs in, `PriceBreakdown` out. No DB, no `time.Now()`.
- Integer arithmetic only, rounding half-even to the cent.
- Every new condition or action needs table-driven cases plus property tests
  (`rapid`): total >= 0, discounts never increase the price, snapshot reproducible.
- Bump `engine_version` when the output for an existing input can change.
```

### 27.6 Skill di progetto (`.claude/skills/`)

Procedure ripetibili, ognuna con passi numerati e criteri di verifica. Esempio completo:

**`new-endpoint/SKILL.md`**
```markdown
---
name: new-endpoint
description: Add a new REST endpoint end to end (OpenAPI, generated code, handler, domain service, tests). Use when a task asks for a new API operation.
argument-hint: "[METHOD] [path]"
---
1. Read `api/openapi.yaml` and find the closest existing operation to copy its style.
2. Add the operation: tags, operationId, request/response schemas, problem+json errors.
3. Run `make gen`. Do not edit generated files.
4. Implement the handler in `internal/http/<admin|public>`; keep it thin.
5. Put logic in the matching `internal/domain/<area>` service; add store methods if needed.
6. Check permissions in the domain service for admin endpoints (see section 16 of the spec).
7. Tests: domain unit test, handler test against the OpenAPI schema.
8. If the frontend needs it, add the TanStack Query hook in `web/shared`.
9. Run `make check`. Report the files changed and the tests added.
```

Le altre skill seguono lo stesso schema:
- **`new-migration`:** crea la coppia di file con numerazione corretta per SQLite e PostgreSQL, aggiorna i modelli Bun, esegue `make test-db`.
- **`new-price-rule-test`:** aggiunge casi alla suite del motore prezzi a partire da uno scenario descritto a parole (stagione, pack, date, risultato atteso).
- **`new-payment-driver`:** implementa `ext.PaymentProvider` con test sui webhook firmati, registrazione nel registry e documentazione della configurazione.
- **`new-domain-event`:** aggiunge il tipo di evento, il payload versionato, la scrittura nell'outbox nella stessa transazione e la voce nella documentazione dei webhook.
- **`pre-pr-check`:** esegue `make check-full`, verifica changelog e documentazione, compila la descrizione della pull request dal template.

### 27.7 Subagent (`.claude/agents/`)

**`reviewer.md`**
```markdown
---
name: reviewer
description: Reviews a finished change before it is declared done. Use after implementing any task.
tools: Read, Grep, Glob, Bash
model: inherit
---
You review changes in the Arenile repository. Read AGENTS.md and the rules for
the touched paths, then inspect `git diff main...HEAD`.
Report, in order of severity: violations of the non-negotiable rules, missing
or weak tests, behaviour not covered by the task, unnecessary changes, missing
docs. Quote file and line. Do not modify files. End with APPROVE or CHANGES REQUESTED.
```

**`test-writer.md`** — scrive test table-driven e property test seguendo le convenzioni del package, senza toccare il codice di produzione.

**`invariants-guardian.md`** — legge `docs/invariants.md`, verifica che la modifica non le violi, propone o scrive test che ne dimostrino il rispetto (doppia allocazione impossibile, totale coerente con lo snapshot, transizioni di stato ammesse, isolamento per stabilimento).

### 27.8 Impostazioni condivise (`.claude/settings.json`)

Permessi espliciti per i comandi sicuri, divieti per quelli distruttivi, formattazione automatica dopo ogni modifica e blocco delle modifiche ai file generati.

```json
{
  "permissions": {
    "allow": [
      "Bash(make:*)",
      "Bash(go test:*)",
      "Bash(go build:*)",
      "Bash(go vet:*)",
      "Bash(npm run:*)",
      "Bash(npx playwright:*)",
      "Bash(git status)",
      "Bash(git diff:*)",
      "Bash(git log:*)",
      "Bash(git add:*)",
      "Bash(git commit:*)",
      "Bash(git switch:*)",
      "Bash(git checkout -b:*)"
    ],
    "deny": [
      "Bash(git push --force:*)",
      "Bash(git reset --hard:*)",
      "Bash(rm -rf:*)",
      "Read(./.env)",
      "Read(./.env.*)"
    ]
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [{ "type": "command", "command": "scripts/agents/guard-generated.sh" }]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [{ "type": "command", "command": "scripts/agents/format-file.sh" }]
      }
    ]
  }
}
```

- `scripts/agents/guard-generated.sh` legge il JSON in ingresso (`tool_input.file_path`) e termina con codice 2 e un messaggio esplicativo se il percorso è un file generato o una migrazione già presente su `main`.
- `scripts/agents/format-file.sh` applica `gofumpt`/`goimports` ai file Go e Prettier ai file del frontend.
- La sintassi esatta di permessi e hook va verificata sulla documentazione di Claude Code al momento della creazione del file.

### 27.9 Task pronti per un agente

Un agente lavora bene quando il task è piccolo e verificabile. Il template `agent-task.yml` impone questi campi:

| Campo | Contenuto |
|---|---|
| Obiettivo | Una frase sul risultato atteso |
| Contesto | Sezioni della specifica e ADR da leggere |
| Perimetro | Package e file che si prevede di toccare |
| Fuori perimetro | Cosa non va modificato |
| Criteri di accettazione | Elenco verificabile, ognuno traducibile in un test |
| Test richiesti | Unit, integrazione, concorrenza, e2e |
| Comando di verifica | `make check` o `make check-full` |

Regole di scomposizione della roadmap in task:
- un task tocca al massimo un'area di dominio e il relativo endpoint;
- motore prezzi e motore di disponibilità si sviluppano prima come librerie pure con test, poi si collegano a HTTP e database;
- l'interfaccia del frontend arriva dopo che l'endpoint è unito su `main`, usando il client generato;
- ogni milestone della sezione 28 viene divisa in task di questo tipo prima di iniziare.

### 27.10 Ambiente riproducibile e dati di prova

- **Devcontainer** con le versioni esatte di Go, Node, `golangci-lint`, `jq` e Playwright: un agente in un ambiente nuovo esegue `make setup` e tutto funziona.
- **`make seed`** crea uno stabilimento dimostrativo completo (8 file, zone miste, listino di una stagione con tutti i periodi, policy, abbonati, prenotazioni passate e future), usato da test e2e, sviluppo e demo.
- **Pagamenti di prova:** fixture di webhook Stripe firmati con un segreto di test e un provider finto (`fakepay`) attivabile solo in sviluppo, così i test non dipendono dalla rete.
- **Orologio iniettabile** in tutti i servizi (`clock.Clock`), per testare scadenze degli hold, cut-off e stagioni senza attese reali.

### 27.11 Documenti di supporto

- **`docs/invariants.md`:** elenco numerato delle invarianti di dominio, ognuna con il test che la verifica. Esempi: nessuna postazione con due allocazioni per stessa data e fascia; `paid_amount` uguale alla somma dei pagamenti riusciti meno i rimborsi; nessuna query operativa senza `establishment_id`; lo snapshot del prezzo, ricalcolato con la stessa `engine_version`, produce lo stesso totale.
- **`docs/architecture.md`:** mappa dei package, flusso di una prenotazione dalla pagina pubblica al webhook, flusso degli eventi.
- **`docs/adr/`:** ogni scelta non ovvia ha un ADR breve (contesto, decisione, conseguenze). Gli agenti li leggono prima di proporre alternative, e un nuovo ADR è obbligatorio per cambiare una decisione esistente.
- **`docs/spec/`:** questa specifica, divisa in un file per sezione, in modo che un task possa richiamare solo le sezioni pertinenti.

---

