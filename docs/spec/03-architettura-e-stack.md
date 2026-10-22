## 3. Architettura e stack

### 3.1 Panoramica

```
┌─────────────────────────────────────────────────────────┐
│                     binario arenile                     │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ SPA pannello │  │ SPA booking  │  │  API REST v1  │  │
│  │ (embed FS)   │  │ (embed FS)   │  │  + SSE        │  │
│  └──────────────┘  └──────────────┘  └───────┬───────┘  │
│                                              │          │
│  ┌──────────────────────────────────────────▼───────┐  │
│  │ Servizi di dominio                               │  │
│  │ availability · pricing · booking · payments ·    │  │
│  │ fiscal · notifications · events                  │  │
│  └──────────────────────────┬───────────────────────┘  │
│                             │                          │
│  ┌──────────────┐  ┌────────▼───────┐  ┌────────────┐  │
│  │ Job runner   │  │ Repository     │  │ Outbox     │  │
│  │ in-process   │  │ (Bun)          │  │ eventi     │  │
│  └──────────────┘  └────────┬───────┘  └────────────┘  │
└─────────────────────────────┼───────────────────────────┘
                              │
                 SQLite (default) │ PostgreSQL (opzionale)
```

### 3.2 Backend

| Componente | Scelta | Motivo |
|---|---|---|
| Linguaggio | Go (ultima stabile) | Binario statico, basso consumo, semplice da leggere |
| Router HTTP | `chi` | Standard `net/http`, middleware semplici |
| Contratto API | OpenAPI 3.1 spec-first + `oapi-codegen` | Tipi generati per server e client TS |
| Accesso dati | `uptrace/bun` | Supporta SQLite e PostgreSQL con lo stesso codice |
| Driver SQLite | `modernc.org/sqlite` | Puro Go, nessun CGO, binario statico |
| Driver PostgreSQL | `jackc/pgx` (via `pgdriver` di Bun) | Standard di fatto |
| Migrazioni | `pressly/goose` con migrazioni Go/SQL per dialetto | Eseguite all'avvio |
| Validazione | Validazione esplicita nei servizi + vincoli DB | Nessuna magia |
| Log | `log/slog` JSON | Standard library |
| Configurazione | Variabili d'ambiente (`envconfig` o parsing manuale) | Compatibile con Docker |
| Password | Argon2id (`golang.org/x/crypto/argon2`) | |
| Email | SMTP (`wneessen/go-mail`) | Nessun provider obbligatorio |
| PDF / QR | `go-pdf/fpdf`, `skip2/go-qrcode` | Ricevute non fiscali e voucher |
| Pagamenti | `stripe-go` | Primo driver |

### 3.3 Frontend

Due applicazioni distinte nello stesso monorepo, con un pacchetto condiviso.

| Componente | Scelta |
|---|---|
| Build | Vite + React + TypeScript (strict) |
| Routing | TanStack Router |
| Dati server | TanStack Query, client generato da OpenAPI (`openapi-typescript` + `openapi-fetch`) |
| UI | Tailwind CSS + componenti shadcn/ui (Radix) |
| Form | react-hook-form + zod |
| i18n | i18next |
| Mappa | SVG renderizzato da React, pan e zoom con `@use-gesture/react`; editor con snapping a griglia |
| Date | `date-fns` con fuso `Europe/Rome` configurabile per stabilimento |
| PWA | `vite-plugin-pwa` (solo pannello) |

- `web/admin`: pannello del gestore. Nessun vincolo di peso.
- `web/booking`: pagina pubblica. **Budget: meno di 150 KB gzip di JS al primo caricamento.** Niente librerie pesanti.
- `web/shared`: client API, tipi, componenti della mappa in sola lettura, temi.

Entrambe vengono compilate e incorporate nel binario Go con `embed.FS`.

### 3.4 Database

- **SQLite** di default, in modalità WAL, file in `/data/arenile.db`. Adatto a uno o pochi stabilimenti.
- **PostgreSQL** opzionale via `DATABASE_URL`. Consigliato per installazioni con molti stabilimenti o più istanze.
- Il codice usa solo il sottoinsieme SQL comune. Le differenze (tipi, indici parziali) sono confinate nelle migrazioni per dialetto.
- Tutti gli ID sono **UUIDv7** (ordinabili nel tempo), salvati come testo in SQLite e `uuid` in PostgreSQL.
- Date di calendario come `DATE` (stringa `YYYY-MM-DD` in SQLite); istanti come timestamp UTC.
- Importi in **centesimi interi (`int64`)**, valuta EUR di default.

---

