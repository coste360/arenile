# Architettura di Arenile

Questo documento descrive come è organizzato il codice di Arenile, il gestionale open source per
stabilimenti balneari, e come scorrono i dati nei flussi principali. È la mappa da leggere prima di
una modifica che attraversa più package. I dettagli normativi restano nella specifica (`docs/spec/`);
le decisioni non ovvie sono negli ADR (`docs/adr/`); le regole che non si possono violare sono in
`docs/invariants.md`.

## 1. Vista d'insieme

Un solo binario Go (`arenile`) contiene API REST, SSE, job runner, outbox e le due SPA React
incorporate con `embed.FS` (ADR 0001). Il database è SQLite di default, PostgreSQL opzionale (ADR 0002).

```
                 browser gestore                   browser cliente / sito ospite (iframe, embed.js)
                       │                                         │
              /admin/  │  /api/admin/v1/*                 /b/{slug} │ /api/public/v1/{slug}/*
                       ▼                                         ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│ binario arenile                                                                     │
│                                                                                     │
│  internal/http/static      internal/http/middleware (log, recover, auth, CSRF,     │
│  (SPA admin + booking)     rate limit, establishment scope, security headers)      │
│                                     │                                               │
│               ┌─────────────────────┼─────────────────────┐                         │
│               ▼                     ▼                     ▼                         │
│      internal/http/admin    internal/http/public   internal/http/sse               │
│      (handler sottili, interfacce generate in internal/http/api)                    │
│               │                     │                     ▲                         │
│               └──────────┬──────────┘                     │ broker in memoria       │
│                          ▼                                │                         │
│     internal/domain/* (availability, pricing, policy, booking, payment, cash,      │
│     fiscal, notification, event, auth, customer, catalog, mapping, establishment)  │
│                          │                                │                         │
│                          ▼                                │                         │
│                   internal/store (Bun) ───────► outbox_events ──► internal/jobs     │
│                          │                                    (dispatcher, hold     │
│                          │                                     scaduti, riconcil.)  │
└──────────────────────────┼──────────────────────────────────────────────────────────┘
                           ▼
              SQLite (WAL, /data/arenile.db)  |  PostgreSQL (DATABASE_URL)
```

## 2. Mappa dei package

| Percorso | Responsabilità | Dipende da |
|---|---|---|
| `cmd/arenile` | `main`, CLI: `serve`, `migrate up\|status`, `admin create`, `seed demo`, `backup --out`, `restore --in`, `version` (versione iniettata con `-ldflags "-X main.version=..."`) | `internal/app`, `internal/config` |
| `internal/app` | Composition root: costruisce store, servizi, router, job runner; registry delle estensioni (`pkg/ext.Extensions`) | tutto `internal/` |
| `internal/config` | Lettura e validazione delle variabili d'ambiente (spec 24.4: `ARENILE_ADDR`, `ARENILE_BASE_URL`, `ARENILE_DATA_DIR`, `DATABASE_URL`, `ARENILE_SECRET_KEY`, `ARENILE_AUTO_MIGRATE`, `ARENILE_TLS_AUTOCERT_DOMAINS`, `ARENILE_SMTP_URL`, `ARENILE_MAIL_FROM`, `ARENILE_HOLD_TTL`, `ARENILE_LOG_LEVEL`, `ARENILE_TRUSTED_PROXIES`, `ARENILE_REQUIRE_TOTP_FOR_OWNERS`, `ARENILE_TELEMETRY`) | stdlib |
| `internal/http/api` | **Generato** da `api/openapi.yaml` con oapi-codegen (`api.gen.go`): modelli e interfacce server chi. Non si modifica a mano | — |
| `internal/http/admin` | Handler di `/api/admin/v1`: decodifica, chiamata al servizio, codifica; errori `application/problem+json` | `http/api`, `domain/*` |
| `internal/http/public` | Handler di `/api/public/v1/{slug}` e dei webhook di pagamento `/webhooks/payments/{provider}/{establishment_id}` | `http/api`, `domain/*` |
| `internal/http/middleware` | Logging slog, recover, request ID, sessione e chiavi API, CSRF double-submit, rate limit, risoluzione dello stabilimento, header di sicurezza | `domain/auth` |
| `internal/http/sse` | `GET /api/admin/v1/stream`: broker in memoria, messaggi leggeri `{type, spot_ids, dates}` | `domain/event` |
| `internal/http/static` | `embed.FS` di `static/admin` e `static/booking` (copiati da `web/*/dist` in build); pagina segnaposto se manca `index.html`. Serve `/admin/`, `/b/{slug}`, `/embed.js` | stdlib |
| `internal/domain/establishment` | Organizzazioni, stabilimenti, stagioni, periodi tariffari, chiusure | `store` (interfacce) |
| `internal/domain/mapping` | Mappe, zone, postazioni, decorazioni, pool | |
| `internal/domain/catalog` | Componenti, pack, extra | |
| `internal/domain/availability` | Allocazioni, hold, pool, quote online, rilasci, assegnazione automatica | `store`, `clock` |
| `internal/domain/pricing` | Motore prezzi **puro** `Quote(input, catalogo, regole) → PriceBreakdown` (ADR 0004) | `money`, `calendar` |
| `internal/domain/policy` | Risoluzione della policy applicabile e validazione dei vincoli | `calendar`, `clock` |
| `internal/domain/booking` | Ciclo di vita della prenotazione (spec 10.2); unico punto che cambia lo stato; audit + outbox nella stessa transazione | `availability`, `pricing`, `policy`, `event` |
| `internal/domain/customer` | Clienti, consensi, crediti, esportazione e anonimizzazione GDPR | |
| `internal/domain/payment` | Pagamenti, rimborsi, driver `manual` e `stripe`, deduplica dei webhook, riconciliazione | `pkg/ext` |
| `internal/domain/cash` | Sessioni di cassa, incassi, chiusure | |
| `internal/domain/fiscal` | Documenti commerciali tramite `ext.FiscalDriver` (`none`, `export`) | `pkg/ext` |
| `internal/domain/notification` | Template, invio email (SMTP), `ext.Notifier` | |
| `internal/domain/event` | Tipi di evento, scrittura nell'outbox, dispatcher verso sink, webhook in uscita e SSE | |
| `internal/domain/auth` | Utenti, membership, ruoli e permessi (spec 16), sessioni, TOTP, chiavi API | `x/crypto` |
| `internal/store` | Repository Bun; **unico posto con SQL**; ogni query operativa filtra per `establishment_id` | `bun` |
| `internal/jobs` | Job runner in-process: rilascio hold scaduti (30 s), dispatch outbox, riconciliazione pagamenti (15 min), webhook in uscita con backoff, backup | `clock` |
| `internal/migrations/sqlite`, `internal/migrations/postgres` | Migrazioni goose per dialetto, stessa numerazione | — |
| `internal/clock` | `Clock` iniettabile (`System`, `Fake`) | — |
| `internal/money` | `Cents` (`int64`), arrotondamento half-even, scorporo IVA | — |
| `internal/calendar` | `Date` (`YYYY-MM-DD`), intervalli, fasce `AM`/`PM`/`FULL` | — |
| `pkg/ext` | Interfacce pubbliche stabili: `PaymentProvider`, `FiscalDriver`, `EventSink`, `ConcessionDataProvider`, `TenantResolver`, `AuthProvider`, `Notifier`, `BotProtection`, `EventBus`, `Extensions` | — |
| `api/openapi.yaml` | Contratto API OpenAPI 3.1, fonte di verità (ADR 0005) | — |
| `web/admin`, `web/booking`, `web/shared` | SPA del pannello, pagina pubblica (budget 150 KB gzip), pacchetto condiviso con client generato in `web/shared/api/generated/` | — |

Regole di dipendenza:
- `internal/domain/*` non importa `net/http`, `internal/http/*` né driver SQL.
- `internal/http/*` non contiene SQL né logica di business.
- `pkg/ext` non importa nulla di `internal/`.
- `internal/domain/pricing` non importa `store`, `clock` né `time.Now`.

## 3. Flusso di una prenotazione pubblica (spec 10.1)

```
 Cliente (/b/{slug} o iframe)          API pubblica                    Dominio / DB                     Provider
 ───────────────────────────          ─────────────                   ─────────────                    ────────
 1 sceglie date e fasce ──GET  /availability──► availability.Query ─► spot_allocations, pool_usage
                                                (cache 10 s per stabilimento+intervallo)
 2 sceglie pack/posto   ──POST /quote─────────► pricing.Quote (puro) + policy.Resolve
                                                (nessuna scrittura)
 3 conferma il carrello ──POST /bookings──────► booking.CreateHeld          ┐ una transazione:
                                                availability.Hold           │ INSERT spot_allocations kind=hold
                                                                            │ UPDATE pool_usage held+=qty
                                                                            │ bookings status=held
                                                                            │ audit_log + outbox booking.held
                                                ◄── booking_id, hold_expires_at, session token
 4 dati e consensi      ──PATCH /bookings/{id}─► customer + consensi versionati
 5 paga                 ──POST /bookings/{id}/pay► payment.Start ──────────────────────────────► CreatePayment
                                                status=pending_payment, hold esteso (max 30 min)      │
                         ◄── redirect Stripe Checkout (target=_top) / client_secret                    │
                                                                                                       ▼
 6                                               POST /webhooks/payments/{provider}/{establishment_id}
                                                ParseWebhook (firma) ─► deduplica per ID evento
                                                una transazione: payments succeeded, paid_amount,
                                                bookings confirmed, allocazioni hold→booking
                                                (pool held→used), audit + outbox booking.confirmed
 7 polling stato        ──GET /bookings/{id}───► finché status = confirmed
```

Punti chiave:
- La conferma avviene **solo** dal webhook (o dal driver `manual`), mai dal ritorno del browser.
- Se l'hold scade (`ARENILE_HOLD_TTL`, default 10 minuti) il job lo rilascia e porta la prenotazione in
  `expired`; anche senza job, gli hold scaduti vengono ripuliti al momento del conflitto.
- Il job di riconciliazione interroga il provider per i pagamenti `pending` più vecchi di 10 minuti.

Stati (spec 10.2):

```
          ┌──────────── expired (hold scaduto)
          │
draft ─▶ held ─▶ pending_payment ─▶ confirmed ─▶ checked_in ─▶ completed
                     │                 │
                     │                 ├──▶ cancelled
                     │                 └──▶ no_show
                     └──▶ awaiting_approval ─▶ confirmed | rejected
```

## 4. Flusso di eventi e outbox (spec 18)

```
 servizio di dominio                          job runner (internal/jobs)
 ───────────────────                          ──────────────────────────
 BEGIN
   modifica (bookings, allocazioni, ...)
   INSERT audit_log
   INSERT outbox_events (type, payload, v1)
 COMMIT ───────────────────────────────────►  dispatcher: SELECT outbox_events
                                              WHERE dispatched_at IS NULL ORDER BY id
                                                 │
                    ┌────────────────────────────┼──────────────────────────┬────────────────────┐
                    ▼                            ▼                          ▼                    ▼
           ext.EventSink (interni)     webhook in uscita            notifiche (email,     broker SSE
                                       firma Arenile-Signature,     ext.Notifier)          {type, spot_ids,
                                       backoff fino a 24 h,         message_log            dates} → il pannello
                                       webhook_deliveries                                  invalida le query
                    │
                    ▼
           UPDATE outbox_events SET dispatched_at = now
```

- L'evento esiste se e solo se la modifica è stata confermata: niente eventi "fantasma" né modifiche
  senza evento.
- La consegna è almeno una volta: i consumatori sono idempotenti sull'`id` dell'evento.
- Per più istanze in parallelo, `ext.EventBus` permette di usare `LISTEN/NOTIFY` di PostgreSQL al
  posto del broker in memoria.

## 5. Motore di disponibilità (spec 7, ADR 0003)

- **Postazioni:** una riga in `spot_allocations` per `(spot_id, date, slot)`; il vincolo
  `unique(spot_id, date, slot)` rende impossibile la doppia occupazione. `kind` ∈ `hold`, `booking`,
  `block`, `season`. Giorno intero = `AM` + `PM`.
- **Pool:** contatori `pool_usage(used, held)` aggiornati con
  `UPDATE ... WHERE used + held + :qty <= capacità_effettiva`; righe mancanti create con
  `INSERT ... ON CONFLICT DO NOTHING`. Meno righe aggiornate del previsto ⇒ rollback.
- **Zone `zone`:** pool implicito di capacità pari alle postazioni attive; assegnazione successiva
  (manuale, automatica greedy, in blocco).
- **Quote online:** `disponibili_online = min(liberi_totali, quota - venduti_online)` per data e fascia.
- **Abbonati:** allocazioni `season`; i rilasci le rimuovono per le date indicate (`season_releases`).
- La violazione del vincolo è tradotta in `ErrSpotUnavailable` con le date in conflitto.

## 6. Motore dei prezzi (spec 8, ADR 0004)

Funzione pura, senza database né orologio: regole, catalogo e data di riferimento arrivano come input.

```
per riga: 1 durata fissa? ─sì─► prezzo fisso
                 │no
                 ▼
          2 base per giorno (vince una regola: priorità, poi specificità, poi recenza;
            nessuna regola ⇒ ErrNoPrice; AM/PM: regola per fascia o half_day_factor)
          3 modificatori (stackable si sommano; non stackable: il più favorevole)
          4 extra (base + modificatori × quantità × unità)
totale:   5 sconti (convenzione → promo → automatici)
          6 arrotondamento sul totale finale
          7 acconto da policy
```

- Aritmetica intera in centesimi, arrotondamento half-even (`internal/money`).
- L'output è un `PriceBreakdown` salvato come `price_snapshot` con `engine_version`; ricalcolare con la
  stessa versione deve dare lo stesso totale. Si incrementa `engine_version` quando l'output per un
  input esistente può cambiare.
- Prezzi IVA inclusa; lo scorporo per aliquota serve solo a documenti e report.

## 7. Multi-stabilimento e sicurezza

- Ogni tabella operativa ha `establishment_id`; lo store lo richiede come parametro preso dal contesto
  autenticato, mai dal corpo della richiesta.
- I permessi (spec 16.1) sono costanti nel codice e verificati nel servizio di dominio.
- Segreti (chiavi provider, segreti webhook, TOTP) cifrati con AES-GCM con chiave da `ARENILE_SECRET_KEY`.

## 8. Build e distribuzione

```
api/openapi.yaml ──make gen──► internal/http/api/api.gen.go
                         └───► web/shared/api/generated/schema.d.ts
web/ ──npm --prefix web run build──► web/admin/dist, web/booking/dist (+ embed.js)
     ──copia──► internal/http/static/{admin,booking}
go build ./cmd/arenile ──► binario unico ──► immagine distroless (deploy/Dockerfile)
```
