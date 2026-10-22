# Arenile — Specifica tecnica e funzionale

> Nome provvisorio: **Arenile**. Il nome è usato in tutto il documento (modulo Go, binario, immagine Docker, prefissi). Va sostituito globalmente quando il nome definitivo sarà scelto.

Gestionale open source per stabilimenti balneari, sviluppato da Coste360 SRL. Il progetto è completo e installabile in autonomia da chiunque, senza dipendenze da servizi esterni.

---

## Indice

1. Obiettivi e principi
2. Perimetro del progetto
3. Architettura e stack
4. Struttura del repository
5. Modello di dominio
6. Schema dati
7. Motore di disponibilità
8. Motore dei prezzi
9. Policy di prenotazione
10. Flusso di prenotazione e ciclo di vita
11. Mappa: editor e vista operativa
12. Pagina pubblica e widget incorporabile
13. Pagamenti
14. Cassa e modulo fiscale
15. Pannello del gestore
16. Utenti, ruoli e autenticazione
17. API
18. Eventi, webhook e punti di estensione
19. Notifiche
20. Internazionalizzazione
21. Tempo reale e funzionamento offline
22. Sicurezza
23. Privacy e GDPR
24. Deploy, configurazione e operatività
25. Test e qualità
26. Licenza e governance del progetto
27. Sviluppo con agenti AI
28. Roadmap e criteri di accettazione

---

## 1. Obiettivi e principi

**Obiettivi**
- Sostituire quaderno, Excel e gestionali chiusi con uno strumento gratuito, moderno e installabile in pochi minuti.
- Offrire una pagina di prenotazione online incorporabile in qualsiasi sito con un iframe.
- Rendere il motore di prenotazione e dei prezzi interamente configurabile, senza modificare codice.

**Principi di progetto**
- **Un binario, un'immagine, un volume.** Nessun servizio esterno obbligatorio (niente Redis, niente code esterne, niente object storage).
- **Configurazione come dati.** Ogni differenza tra stabilimenti si esprime con risorse, pack, regole di prezzo e policy, mai con condizioni nel codice.
- **Valori predefiniti sensati.** Ogni stabilimento parte da un modello funzionante; le opzioni avanzate restano nascoste finché non servono.
- **API-first.** Pannello e pagina pubblica usano esclusivamente le API documentate.
- **Correttezza prima di tutto sulla disponibilità.** Una doppia prenotazione è un difetto bloccante.
- **Dati del gestore.** Esportazione completa sempre disponibile.

---

## 2. Perimetro del progetto

Il progetto include tutto ciò che serve per gestire uno stabilimento, dalla configurazione della mappa all'incasso, senza componenti a pagamento né servizi obbligatori di terze parti.

| Area | Incluso |
|---|---|
| Mappa, risorse, pack, extra | Editor, vista operativa, inventario a posti, zone e quantità |
| Motore prezzi e policy | Regole configurabili, simulatore, copia di stagione |
| Pagina pubblica e widget | Link diretto, iframe, script di incorporamento, temi |
| Pagamenti | Interfaccia `PaymentProvider`, driver manuale, driver Stripe con l'account del gestore |
| Multi-stabilimento | Più stabilimenti nella stessa installazione |
| Backup | Comando CLI, backup automatico su file |
| Dati della concessione | Interfaccia `ConcessionDataProvider` per importare perimetro e sfondo da fonti esterne (vuota di default) |
| Integrazioni | Webhook in uscita, chiavi API, punti di estensione Go |

Chi vuole estendere il progetto (nuovi provider di pagamento, driver fiscali, canali di notifica, integrazioni) lo fa tramite i punti di estensione descritti nella sezione 18, senza modificare il codice del core.

---

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

## 4. Struttura del repository

```
arenile/
├── cmd/
│   └── arenile/              # main: comandi serve, migrate, admin, seed, backup
├── internal/
│   ├── app/                  # composizione, registry delle estensioni
│   ├── config/
│   ├── http/
│   │   ├── admin/            # handler API pannello
│   │   ├── public/           # handler API booking
│   │   ├── middleware/
│   │   └── sse/
│   ├── domain/
│   │   ├── establishment/
│   │   ├── mapping/          # mappe, zone, postazioni
│   │   ├── catalog/          # pack, extra, componenti
│   │   ├── availability/
│   │   ├── pricing/
│   │   ├── policy/
│   │   ├── booking/
│   │   ├── customer/
│   │   ├── payment/
│   │   ├── cash/             # cassa e chiusure
│   │   ├── fiscal/
│   │   ├── notification/
│   │   ├── event/            # outbox e webhook
│   │   └── auth/
│   ├── store/                # repository Bun
│   ├── jobs/                 # job runner in-process
│   └── migrations/
│       ├── sqlite/
│       └── postgres/
├── pkg/
│   └── ext/                  # interfacce pubbliche per le estensioni
├── api/
│   └── openapi.yaml
├── web/
│   ├── admin/
│   ├── booking/
│   └── shared/
├── deploy/
│   ├── Dockerfile
│   ├── docker-compose.yml
│   ├── docker-compose.postgres.yml
│   └── coolify/ caprover/
├── docs/                     # architettura, invarianti, ADR, specifica (sezione 27)
├── e2e/                      # Playwright
├── scripts/agents/           # script usati dagli hook degli agenti
├── .claude/                  # regole, skill, subagent, impostazioni (sezione 27)
├── .devcontainer/
├── .github/                  # CI, template di issue e pull request
├── AGENTS.md                 # istruzioni per gli agenti di coding
├── CLAUDE.md                 # importa AGENTS.md
├── Makefile                  # unica interfaccia per build, test e verifiche
├── LICENSE                   # AGPL-3.0
├── CLA.md
├── CHANGELOG.md
└── CONTRIBUTING.md
```

Le interfacce in `pkg/ext` sono le **uniche API Go stabili** per chi estende il core. Tutto ciò che sta in `internal/` può cambiare senza preavviso.

---

## 5. Modello di dominio

### 5.1 Glossario

| Termine | Significato |
|---|---|
| **Organizzazione** | Soggetto titolare (la società che gestisce). Può avere più stabilimenti. |
| **Stabilimento** | Un lido. Ha fuso orario, stagioni, mappe, catalogo, listini, policy. |
| **Stagione** | Periodo di apertura annuale (es. 2027-05-01 → 2027-09-30). |
| **Periodo tariffario** | Intervallo di date con nome (bassa, media, alta, Ferragosto) usato dalle regole di prezzo. |
| **Mappa** | Rappresentazione grafica di un'area (spiaggia, piscina). |
| **Zona** | Raggruppamento logico di postazioni (Prima fila, Zona B, Piscina). |
| **Postazione** | Elemento prenotabile con posizione: ombrellone, gazebo, tenda, palma. |
| **Pool** | Inventario a quantità senza posizione: "lettini sfusi", "ombrelloni zona libera". |
| **Elemento decorativo** | Elemento non prenotabile della mappa: passerella, docce, torretta, bar. |
| **Componente** | Unità di inventario consumata da un pack: ombrellone, lettino, sdraio, cabina. |
| **Pack** | Prodotto vendibile composto da componenti (1 ombrellone + 2 lettini). |
| **Extra** | Aggiunta opzionale a un pack (lettino aggiuntivo, cabina, telo, parcheggio). |
| **Fascia** | Unità temporale minima: `AM` (mattina) e `PM` (pomeriggio). Il giorno intero è `AM+PM`. |
| **Formula** | Tipo di durata: mezza giornata, giorno, multi-giorno, settimana, quindicina, mese, stagione. |
| **Prenotazione** | Acquisto di uno o più pack su un intervallo di date, con extra, cliente e pagamenti. |
| **Riga di prenotazione** | Un pack prenotato con la sua assegnazione (postazione, zona o pool). |
| **Allocazione** | Occupazione di una risorsa per data e fascia. È ciò che garantisce l'assenza di doppie prenotazioni. |
| **Abbonamento stagionale** | Prenotazione con formula stagione, con diritto di rinnovo l'anno successivo. |
| **Rilascio** | Giorno in cui un abbonato libera il proprio posto, rivendibile. |
| **Canale** | Origine della prenotazione: `desk` (cassa), `online`, `api`. |

### 5.2 Modalità di assegnazione

Ogni zona ha una `assignment_mode`:

- `seat` — il cliente sceglie la postazione esatta sulla mappa.
- `zone` — il cliente sceglie la zona; la postazione viene assegnata dal gestore o automaticamente (sezione 7.5).
- `quantity` — il cliente indica solo la quantità; il consumo avviene su un pool.

Uno stabilimento può avere zone con modalità diverse. La pagina pubblica si adatta automaticamente: mostra la mappa solo se almeno una zona visibile online è `seat`.

---

## 6. Schema dati

Convenzioni: ogni tabella ha `id` (UUIDv7), `created_at`, `updated_at`. Le tabelle operative hanno `establishment_id` con indice. Soft delete solo dove indicato (`archived_at`).

### 6.1 Organizzazione e stabilimento

**organizations**
| Campo | Tipo | Note |
|---|---|---|
| name | text | |
| vat_number | text null | P.IVA |
| fiscal_code | text null | |
| billing_address | json null | |

**establishments**
| Campo | Tipo | Note |
|---|---|---|
| organization_id | uuid FK | |
| slug | text unique | Usato nell'URL pubblico |
| name | text | |
| timezone | text | Default `Europe/Rome` |
| currency | text | Default `EUR` |
| default_locale | text | Default `it` |
| enabled_locales | json | Es. `["it","en","de","fr"]` |
| contact_email, contact_phone | text | |
| address | json | |
| geo | json null | lat/lon |
| concession_ref | text null | Riferimento a un sistema esterno (es. identificativo della concessione) |
| booking_enabled | bool | Pagina pubblica attiva |
| allowed_embed_origins | json | Domini autorizzati all'iframe |
| theme | json | Vedi 12.5 |
| settings | json | Impostazioni minori versionate |
| archived_at | timestamp null | |

**seasons**
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| year | int | |
| opens_on, closes_on | date | |
| status | enum | `draft`, `active`, `closed` |
| unique(establishment_id, year) | | |

**closure_days** — giorni di chiusura straordinaria (maltempo, eventi).
| Campo | Tipo |
|---|---|
| establishment_id | uuid |
| date | date |
| slots | json (`["AM","PM"]`) |
| reason | text |

**rate_periods**
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| season_id | uuid | |
| code | text | `low`, `mid`, `high`, `peak` o libero |
| name | json i18n | |
| color | text | Per il calendario |
| ranges | json | Lista di `{from, to}`; i periodi non si sovrappongono nella stessa stagione |

### 6.2 Mappa e inventario

**maps**
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| name | json i18n | |
| width, height | int | Unità logiche della griglia |
| grid_size | int | Passo di snapping |
| background | json null | Immagine o perimetro GeoJSON (dal `ConcessionDataProvider`) |
| sea_side | enum | `top`, `bottom`, `left`, `right` |
| sort_order | int | |

**zones**
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| map_id | uuid null | Null per zone solo a quantità |
| code | text | |
| name | json i18n | |
| assignment_mode | enum | `seat`, `zone`, `quantity` |
| online_visible | bool | |
| online_quota | json null | `{type: "percent"|"count", value}` |
| price_category | text | Chiave usata dalle regole di prezzo (es. `row1`, `row2`, `pool`) |
| sort_order | int | |

**spots** (postazioni)
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| map_id | uuid | |
| zone_id | uuid | |
| label | text | Es. `A12` |
| row_label | text | Es. `A` |
| number | int | |
| kind | enum | `umbrella`, `gazebo`, `tent`, `palm`, `custom` |
| x, y, rotation | numeric | Posizione sulla mappa |
| shape | json | `{type: "circle"|"rect", w, h}` |
| capacity | json | Componenti inclusi di default (es. `{umbrella:1, sunbed:2}`) |
| max_extra | json | Limiti di extra fisici sulla postazione (es. `{sunbed:2}`) |
| price_category_override | text null | Sovrascrive quella della zona |
| attributes | json | Tag liberi: `accessible`, `shade_morning`, ecc. |
| active | bool | Postazioni disattivate non sono prenotabili |
| unique(map_id, label) | | |

**map_decorations**
| Campo | Tipo |
|---|---|
| map_id | uuid |
| kind | enum: `walkway`, `shower`, `lifeguard`, `bar`, `entrance`, `cabin_block`, `text`, `custom` |
| x, y, rotation | numeric |
| shape | json |
| label | json i18n null |

**pools** (inventario a quantità)
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| zone_id | uuid null | Pool legato a una zona `quantity` |
| component_code | text | Cosa contiene (es. `sunbed`, `umbrella`, `cabin`) |
| name | json i18n | |
| capacity | int | Quantità totale disponibile per fascia |
| capacity_overrides | json | `[{from, to, capacity}]` per variazioni stagionali |

### 6.3 Catalogo

**components** — dizionario dei componenti fisici.
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| code | text | `umbrella`, `sunbed`, `deckchair`, `cabin`, `parking` |
| name | json i18n | |
| tracked | bool | Se `true`, la disponibilità è controllata tramite un pool |
| pool_id | uuid null | Pool da cui consumare se `tracked` |

**packs**
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| code | text | |
| name, description | json i18n | |
| image | text null | |
| composition | json | `{umbrella:1, sunbed:2}` |
| requires_spot_kind | json null | Tipi di postazione compatibili (es. `["umbrella"]`) |
| allowed_zone_ids | json null | Null = tutte |
| allowed_formulas | json | `["half_day","day","multi_day","week","fortnight","month","season"]` |
| channels | json | `["desk","online"]` |
| max_per_booking | int | |
| requires_approval | bool | Per pack speciali (gazebo) |
| sort_order | int | |
| active | bool | |

**extras**
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| code | text | |
| name, description | json i18n | |
| component_code | text null | Se consuma un componente tracciato |
| pricing_unit | enum | `per_booking`, `per_day`, `per_slot` |
| max_quantity | int | Per riga di prenotazione |
| attach_to | enum | `line` (a un pack), `booking` (alla prenotazione) |
| allowed_pack_ids | json null | |
| channels | json | |
| active | bool | |

### 6.4 Prezzi e policy

**price_rules** — sezione 8 per la semantica.
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| season_id | uuid | |
| name | text | Etichetta interna |
| target_type | enum | `pack`, `extra` |
| target_ids | json null | Null = tutti i target del tipo |
| conditions | json | Vedi 8.2 |
| action | json | Vedi 8.3 |
| priority | int | Più alto = valutato dopo, può sovrascrivere |
| stage | enum | `base`, `modifier`, `discount` |
| stackable | bool | Per `modifier` e `discount` |
| active | bool | |

**fixed_duration_prices** — prezzi a pacchetto di durata (settimana, mese, stagione).
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| season_id | uuid | |
| pack_id | uuid | |
| formula | enum | `week`, `fortnight`, `month`, `season` |
| price_category | text | |
| rate_period_code | text null | Null = valido in ogni periodo |
| amount | int64 | Centesimi |

**promo_codes**
| Campo | Tipo |
|---|---|
| establishment_id | uuid |
| code | text (case-insensitive unique) |
| action | json (come `price_rules.action`) |
| conditions | json |
| valid_from, valid_to | date |
| max_uses, max_uses_per_customer | int null |
| uses | int |
| active | bool |

**agreements** (convenzioni) — come i codici, ma assegnate a un cliente o a un'organizzazione partner (hotel), con codice o selezione dal pannello.

**booking_policies**
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| season_id | uuid | |
| scope | json | Filtri: canali, zone, pack, periodi tariffari |
| rules | json | Vedi sezione 9 |
| priority | int | |

### 6.5 Clienti

**customers**
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| first_name, last_name | text | |
| email | text null | Indicizzato, lowercase |
| phone | text null | E.164 |
| locale | text | |
| notes | text | Solo interne |
| tags | json | `vip`, `storico`, ecc. |
| agreement_id | uuid null | |
| marketing_consent | json null | `{given_at, source, text_version}` |
| anonymized_at | timestamp null | |

### 6.6 Prenotazioni

**bookings**
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| code | text unique | Codice leggibile, es. `AR-7K3Q9` |
| customer_id | uuid | |
| channel | enum | `desk`, `online`, `api` |
| status | enum | Vedi 10.2 |
| starts_on, ends_on | date | Inclusivi |
| slots | enum | `AM`, `PM`, `FULL` |
| formula | enum | |
| is_season_pass | bool | |
| renewal_of_booking_id | uuid null | |
| price_snapshot | json | Dettaglio completo del calcolo (sezione 8.6) |
| total_amount | int64 | |
| paid_amount | int64 | Aggiornato dai pagamenti |
| deposit_due | int64 | |
| policy_snapshot | json | Policy applicate al momento della creazione |
| promo_code_id | uuid null | |
| hold_expires_at | timestamp null | Solo in stato `held` |
| manage_token_hash | text | Per il link magico del cliente |
| notes_customer, notes_internal | text | |
| created_by_user_id | uuid null | |
| cancelled_at, cancel_reason | | |

**booking_lines**
| Campo | Tipo | Note |
|---|---|---|
| booking_id | uuid | |
| pack_id | uuid | |
| zone_id | uuid | |
| spot_id | uuid null | Null finché non assegnato (modalità `zone`) o se `quantity` |
| assignment_status | enum | `assigned`, `pending`, `not_applicable` |
| line_total | int64 | |

**booking_line_extras**
| Campo | Tipo |
|---|---|
| booking_line_id | uuid null |
| booking_id | uuid |
| extra_id | uuid |
| quantity | int |
| total | int64 |

**spot_allocations** — chiave della correttezza (sezione 7).
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| spot_id | uuid | |
| date | date | |
| slot | enum | `AM`, `PM` |
| booking_line_id | uuid | |
| kind | enum | `booking`, `hold`, `block` (blocco manuale), `season` |
| expires_at | timestamp null | Solo per `hold` |
| **unique(spot_id, date, slot)** | | |

**pool_usage**
| Campo | Tipo | Note |
|---|---|---|
| pool_id | uuid | |
| date | date | |
| slot | enum | |
| used | int | |
| held | int | |
| **unique(pool_id, date, slot)** | | |

**pool_allocations** — dettaglio per prenotazione, usato per ricostruire `pool_usage` e per i rilasci.
| Campo | Tipo |
|---|---|
| pool_id, date, slot | |
| booking_id | uuid |
| quantity | int |
| kind | `booking`, `hold` |
| expires_at | timestamp null |

**season_releases** — giorni liberati da un abbonato.
| Campo | Tipo | Note |
|---|---|---|
| booking_line_id | uuid | Riga dell'abbonamento |
| date | date | |
| slots | enum | |
| resold_booking_line_id | uuid null | |
| credit_amount | int64 | Eventuale credito riconosciuto |

**waitlist_entries**
| Campo | Tipo |
|---|---|
| establishment_id, customer_id | uuid |
| starts_on, ends_on, slots | |
| pack_id, zone_id, spot_id | null ammessi |
| status | `waiting`, `notified`, `converted`, `expired` |

### 6.7 Pagamenti e cassa

**payments**
| Campo | Tipo | Note |
|---|---|---|
| establishment_id | uuid | |
| booking_id | uuid null | Null per vendite di cassa non legate a prenotazione |
| provider | text | `stripe`, `manual`, … |
| method | enum | `card`, `cash`, `bank_transfer`, `wallet`, `other` |
| direction | enum | `in`, `refund` |
| amount | int64 | |
| status | enum | `pending`, `succeeded`, `failed`, `cancelled` |
| provider_ref | text null | Es. PaymentIntent ID |
| idempotency_key | text unique | |
| cash_session_id | uuid null | |
| received_by_user_id | uuid null | |
| metadata | json | |

**cash_sessions**
| Campo | Tipo |
|---|---|
| establishment_id | uuid |
| opened_by, closed_by | uuid |
| opened_at, closed_at | timestamp |
| opening_float | int64 |
| expected_totals | json (per metodo) |
| counted_totals | json |
| difference | int64 |
| notes | text |

**fiscal_documents**
| Campo | Tipo | Note |
|---|---|---|
| payment_id | uuid | |
| driver | text | `none`, `export`, driver RT specifico |
| status | enum | `pending`, `issued`, `failed`, `void` |
| document_number | text null | |
| payload | json | Righe, aliquote, totali |
| response | json null | Risposta del dispositivo |

### 6.8 Utenti, audit, eventi

**users**: email unique, password_hash, name, locale, totp_secret (cifrato), active, last_login_at.
**memberships**: user_id, organization_id, establishment_id null (null = tutti), role.
**sessions**: token_hash, user_id, expires_at, ip, user_agent.
**api_keys**: establishment_id, name, key_hash, prefix, scopes, last_used_at, revoked_at.
**audit_log**: establishment_id, actor_type (`user`, `customer`, `system`, `api_key`), actor_id, action, entity_type, entity_id, diff (json), ip, created_at. Solo inserimento.
**outbox_events**: id, establishment_id, type, payload, created_at, dispatched_at.
**webhook_endpoints**: establishment_id, url, secret, events (json), active.
**webhook_deliveries**: endpoint_id, event_id, attempt, status_code, response_excerpt, next_attempt_at, delivered_at.
**notification_templates**: establishment_id, code, locale, subject, body (Markdown con variabili), active.
**message_log**: destinatario, canale, template, stato, errore.

---

## 7. Motore di disponibilità

### 7.1 Principio

La disponibilità è garantita **dai vincoli del database**, non dal solo codice applicativo. Questo funziona identico in SQLite e PostgreSQL.

- Per le postazioni: una riga in `spot_allocations` per ogni `(spot, data, fascia)`. Il vincolo `unique(spot_id, date, slot)` rende impossibile una doppia occupazione.
- Per i pool: un contatore per `(pool, data, fascia)` aggiornato con un `UPDATE` condizionale.

Una prenotazione a giornata intera occupa entrambe le fasce. Un abbonamento stagionale da 120 giorni genera 240 righe di allocazione: volume trascurabile anche con centinaia di postazioni.

### 7.2 Occupazione di una postazione

```
BEGIN
  DELETE FROM spot_allocations
    WHERE kind = 'hold' AND expires_at < now()
      AND spot_id = :spot AND date BETWEEN :from AND :to;   -- pulizia locale
  INSERT INTO spot_allocations (spot_id, date, slot, booking_line_id, kind, expires_at)
    VALUES (...per ogni data e fascia...);
  -- violazione del vincolo unique => postazione non disponibile => rollback
COMMIT
```

L'errore di vincolo viene tradotto in `ErrSpotUnavailable` con l'elenco delle date in conflitto, in modo che l'interfaccia possa proporre alternative.

### 7.3 Consumo di un pool

```
UPDATE pool_usage
   SET held = held + :qty
 WHERE pool_id = :pool AND date = :d AND slot = :s
   AND used + held + :qty <= :capacity_effettiva
```

Se il numero di righe aggiornate è minore delle righe attese, la transazione viene annullata. Le righe di `pool_usage` mancanti vengono create con `INSERT ... ON CONFLICT DO NOTHING` prima dell'aggiornamento. La capacità effettiva tiene conto di `capacity_overrides`.

### 7.4 Blocchi temporanei (hold)

- Durante il checkout pubblico le risorse vengono occupate con `kind = 'hold'` e `expires_at = now + HOLD_TTL` (default 10 minuti, configurabile).
- Alla conferma del pagamento, gli hold diventano `booking` (aggiornamento di `kind` ed `expires_at = null`; per i pool, `held` → `used`).
- Un job ogni 30 secondi rilascia gli hold scaduti e porta la prenotazione in stato `expired`.
- Anche senza job, gli hold scaduti vengono ignorati e ripuliti al momento del conflitto (7.2), quindi un job in ritardo non blocca mai le vendite.

### 7.5 Assegnazione automatica (modalità `zone`)

Quando una zona è in modalità `zone`, la prenotazione occupa **capacità di zona** e la postazione viene assegnata in seguito.

- Ogni zona `zone` ha un pool implicito di capacità uguale al numero di postazioni attive (creato e mantenuto dal sistema).
- La prenotazione consuma il pool di zona; la riga ha `assignment_status = pending`.
- L'assegnazione avviene in tre modi: manuale dal pannello (trascinamento), automatica alla conferma se `auto_assign = true`, oppure in blocco con il comando "assegna tutte le prenotazioni di domani".
- **Algoritmo automatico (greedy con preferenze):**
  1. ordina le righe da assegnare per durata decrescente (le più lunghe sono le più difficili da collocare);
  2. per ciascuna, scegli tra le postazioni libere per tutte le date la migliore secondo un punteggio: continuità con la prenotazione precedente dello stesso cliente, vicinanza ad altre righe della stessa prenotazione, preferenze indicate dal cliente (attributi), riempimento compatto delle file;
  3. se nessuna postazione è libera per l'intero periodo, consenti lo spezzamento solo se lo permette la policy (`allow_spot_change_during_stay`), altrimenti segnala la riga al gestore.
- Invariante: la somma delle righe assegnate e non assegnate di una zona non supera mai le postazioni attive per ciascuna data e fascia.

### 7.6 Quote online

Per le zone con `online_quota`, la disponibilità mostrata al canale `online` è:

```
disponibili_online = min(liberi_totali, quota - venduti_online_nello_slot)
```

Le prenotazioni dal canale `desk` ignorano la quota. Le quote sono calcolate per data e fascia.

### 7.7 Abbonati e rilasci

- Le postazioni degli abbonati stagionali sono occupate con `kind = 'season'` per tutte le date della stagione.
- Un rilascio trasforma le allocazioni dei giorni indicati in disponibilità rivendibile: le righe `season` vengono rimosse per quelle date e viene creata una `season_release`.
- Se il giorno viene rivenduto, `resold_booking_line_id` viene valorizzato e, secondo la policy, viene maturato un credito per l'abbonato.
- Se l'abbonato revoca il rilascio prima della rivendita, le allocazioni vengono ricreate.

### 7.8 Query di disponibilità

`GET /public/v1/{slug}/availability?from=&to=&slots=&pack=` restituisce, per ogni zona visibile:
- la modalità di assegnazione;
- per le zone `seat`: l'elenco delle postazioni con stato `available | unavailable` per l'intero intervallo (una postazione è disponibile solo se libera in tutte le date e fasce richieste);
- per le zone `zone` e `quantity`: la quantità disponibile minima nell'intervallo.

La risposta non espone mai dati sui clienti. È cacheabile per 10 secondi in memoria per stabilimento e intervallo, invalidata da ogni evento di allocazione.

---

## 8. Motore dei prezzi

### 8.1 Pipeline di calcolo

Il calcolo è una **funzione pura**: `Quote(input, catalogo, regole) → PriceBreakdown`. Non accede al database durante l'esecuzione: le regole vengono caricate prima. Questo la rende testabile in modo esaustivo.

```
input: stabilimento, canale, date, fasce, formula, righe [pack, zona/postazione],
       extra, cliente (convenzione), codice promozionale

per ogni riga:
  1. FORMULA A DURATA FISSA
     se formula ∈ {week, fortnight, month, season}
     e esiste fixed_duration_price(pack, formula, price_category, periodo)
       → base_riga = quel prezzo
     altrimenti passa al punto 2 sommando i giorni

  2. BASE PER GIORNO
     per ogni giorno dell'intervallo:
       applica le regole stage=base valide quel giorno
       (vince quella con priorità più alta tra quelle applicabili)
       → prezzo_giorno
     se slots ∈ {AM, PM}: usa la regola con condizione slot corrispondente,
       oppure il prezzo giornaliero × half_day_factor (impostazione stabilimento)

  3. MODIFICATORI (stage=modifier)
     per ogni giorno: applica in ordine di priorità
     le regole non stackable sostituiscono le precedenti dello stesso gruppo

  4. EXTRA
     per ogni extra: stesso procedimento (base + modificatori) con target extra,
     moltiplicato per quantità e per giorni o fasce secondo pricing_unit

somma righe + extra a livello prenotazione

  5. SCONTI (stage=discount): convenzione, poi codice promozionale, poi
     sconti automatici (es. durata lunga); stackable=false ⇒ si applica il migliore

  6. ARROTONDAMENTO: secondo l'impostazione dello stabilimento
     (nessuno, all'euro, ai 50 centesimi), applicato solo al totale finale

  7. ACCONTO: calcolato dalla policy (sezione 9)
```

### 8.2 Condizioni di una regola

Tutte opzionali; una regola si applica se **tutte** le condizioni presenti sono vere.

```json
{
  "rate_periods": ["high", "peak"],
  "date_ranges": [{"from": "2027-08-10", "to": "2027-08-20"}],
  "weekdays": [6, 7],
  "holidays": true,
  "slots": ["AM"],
  "formulas": ["day", "multi_day"],
  "price_categories": ["row1", "row2"],
  "zone_ids": ["..."],
  "spot_attributes": ["accessible"],
  "channels": ["online"],
  "min_days": 3,
  "max_days": 6,
  "lead_time_days": {"min": 0, "max": 1},
  "occupancy_percent": {"min": 80},
  "customer_tags": ["storico"],
  "agreement_ids": ["..."]
}
```

- `holidays` usa un calendario delle festività nazionali italiane incluso nel binario, con festività locali aggiungibili per stabilimento (es. santo patrono).
- `occupancy_percent` è valutata sulla zona per il singolo giorno. È opzionale e disattivata di default.

### 8.3 Azioni

```json
{"type": "set",         "amount": 2500}
{"type": "add",         "amount": 500}
{"type": "multiply",    "factor": 1.2}
{"type": "percent_off", "percent": 10}
{"type": "amount_off",  "amount": 1000, "per": "booking"}
{"type": "free_days",   "every": 7, "free": 1}
```

- `set` è valida solo nello stage `base`.
- `multiply` e `percent_off` usano aritmetica intera con arrotondamento al centesimo (half-even), mai float nel calcolo finale.
- `free_days` implementa offerte del tipo "7 giorni al prezzo di 6".

### 8.4 Risoluzione dei conflitti

- Nello stage `base` vince una sola regola per giorno: la priorità più alta; a parità, la più specifica (più condizioni); a parità, la più recente.
- Negli stage `modifier` e `discount`, le regole `stackable = true` si sommano in ordine di priorità; tra le regole `stackable = false` si applica solo quella con il risultato più favorevole al cliente.
- Se per un giorno non esiste nessuna regola `base`, il pack **non è vendibile** quel giorno: la riga viene rifiutata con `ErrNoPrice`. Il pannello segnala in configurazione i buchi di listino (vedi 8.7).

### 8.5 Prezzi IVA

- I prezzi configurati sono **IVA inclusa**.
- Ogni pack ed extra ha un `vat_rate` (configurabile, con valore predefinito a livello di stabilimento). Lo scorporo è calcolato solo per i documenti e i report.
- Il codice non contiene aliquote fisse: le imposta il gestore.

### 8.6 Snapshot del prezzo

Ogni prenotazione salva in `price_snapshot` il dettaglio completo:

```json
{
  "engine_version": 1,
  "currency": "EUR",
  "lines": [{
    "pack": "base", "zone": "row1", "spot": "A12",
    "days": [
      {"date": "2027-08-14", "slots": "FULL", "base": 3500,
       "applied": [{"rule": "r_peak", "stage": "base", "effect": "set 3500"},
                   {"rule": "r_weekend", "stage": "modifier", "effect": "+500"}],
       "total": 4000}
    ],
    "extras": [{"code": "sunbed", "qty": 1, "total": 800}],
    "total": 4800
  }],
  "discounts": [{"source": "promo:ESTATE10", "amount": -480}],
  "rounding": 0,
  "total": 4320,
  "vat": [{"rate": 22, "taxable": 3541, "tax": 779}]
}
```

Lo snapshot non cambia mai dopo la conferma. Le modifiche alla prenotazione generano un nuovo calcolo e una differenza da incassare o rimborsare.

### 8.7 Strumenti del pannello per i prezzi

- **Simulatore:** stessa API di quotazione con canale selezionabile e dettaglio passo per passo.
- **Matrice di listino:** vista tabellare pack × fila × periodo con il prezzo risultante, generata dal motore.
- **Verifica di copertura:** segnala i giorni della stagione in cui un pack attivo non ha prezzo per una categoria.
- **Copia di stagione:** duplica regole, periodi e prezzi fissi sull'anno successivo, spostando le date con lo stesso giorno della settimana dove ha senso.

---

## 9. Policy di prenotazione

Le policy sono regole con un ambito (`scope`) e un insieme di vincoli (`rules`). Per una prenotazione si applica la policy con priorità più alta tra quelle il cui ambito corrisponde; i campi non specificati ereditano dalla policy di default dello stabilimento.

```json
{
  "min_lead_time":   {"days": 0, "cutoff_time": "20:00"},
  "max_lead_time_days": 120,
  "same_day_booking": {"enabled": true, "until": "13:00", "only_slot": "PM_after"},
  "duration": {
    "allowed_formulas": ["day", "multi_day", "week"],
    "min_days": 1,
    "max_days": 30,
    "fixed_start_weekday": null
  },
  "payment": {
    "mode": "full | deposit | none",
    "deposit": {"type": "percent", "value": 30},
    "balance_due": "on_arrival | days_before:7"
  },
  "cancellation": [
    {"until_days_before": 14, "refund_percent": 100, "as": "refund"},
    {"until_days_before": 3,  "refund_percent": 50,  "as": "credit"},
    {"until_days_before": 0,  "refund_percent": 0}
  ],
  "modification": {"allowed_until_days_before": 2, "fee": 0},
  "weather_policy": {"enabled": true, "as": "credit"},
  "approval_required": false,
  "max_spots_per_booking": 4,
  "allow_spot_change_during_stay": false,
  "customer_can_release_season_days": true,
  "season_release_credit": {"enabled": false, "percent": 50}
}
```

Esempi tipici configurabili senza codice:
- ad agosto online solo settimane da sabato a sabato: policy con `scope.rate_periods = ["peak"]`, `fixed_start_weekday = 6`, `allowed_formulas = ["week"]`;
- i gazebo richiedono approvazione: policy con `scope.packs = ["gazebo"]`, `approval_required = true`;
- online pagamento totale, alla cassa nessun vincolo: due policy distinte per canale.

Le policy applicate vengono salvate in `policy_snapshot` per rendere verificabili cancellazioni e rimborsi anche se il gestore cambia le regole dopo.

---

## 10. Flusso di prenotazione e ciclo di vita

### 10.1 Flusso pubblico

```
1. GET  availability            → date, fasce, zone, postazioni
2. POST quote                   → prezzo e policy (nessuna scrittura)
3. POST bookings (hold)         → crea prenotazione in stato held, occupa con hold,
                                  restituisce booking_id, hold_expires_at, client_secret
4. PATCH bookings/{id}/customer → dati cliente, consensi
5. POST bookings/{id}/pay       → crea il pagamento sul provider (o conferma senza pagamento)
6. webhook provider             → pagamento riuscito ⇒ status confirmed, hold → booking
7. GET  bookings/{id}           → la pagina interroga lo stato fino a confirmed
```

La conferma avviene **solo** dal webhook (o dal driver manuale), mai dal ritorno del browser.

### 10.2 Stati della prenotazione

```
          ┌──────────── expired (hold scaduto)
          │
draft ─▶ held ─▶ pending_payment ─▶ confirmed ─▶ checked_in ─▶ completed
                     │                 │
                     │                 ├──▶ cancelled (con rimborso/credito da policy)
                     │                 └──▶ no_show
                     └──▶ awaiting_approval ─▶ confirmed | rejected
```

- `draft`: solo dal pannello, prenotazione in compilazione (non occupa risorse).
- `held`: risorse occupate temporaneamente.
- `pending_payment`: pagamento avviato, hold esteso fino all'esito (max 30 minuti).
- `awaiting_approval`: risorse occupate con hold lungo (configurabile), in attesa del gestore.
- `confirmed`: risorse occupate stabilmente.
- `checked_in`: presenza registrata (scansione QR o tocco sulla mappa).
- `completed`: chiusura automatica a fine periodo.

Ogni transizione è un metodo del servizio `booking`, scrive l'audit log e pubblica un evento nell'outbox nella **stessa transazione**.

### 10.3 Modifiche

- Cambio di postazione o date: nuova allocazione nella stessa transazione, rilascio della vecchia, ricalcolo del prezzo, differenza registrata come importo da incassare o rimborsare.
- Aggiunta o rimozione di extra: stessa logica.
- Il cliente può modificare dal link magico solo entro i limiti della policy; il pannello può sempre forzare, con registrazione nell'audit.

### 10.4 Rinnovo degli abbonamenti

- Alla creazione della nuova stagione il gestore lancia "proponi rinnovi": per ogni abbonamento della stagione precedente viene creata una prenotazione in `held` con hold lungo (es. fino a una data scelta) sulla stessa postazione, con prezzo del nuovo listino.
- Il cliente riceve un link per confermare e pagare; alla scadenza gli hold non confermati vengono rilasciati.
- Diritto di prelazione esplicito, visibile in mappa con un colore dedicato.

### 10.5 Lista d'attesa

- Quando una richiesta non trova disponibilità, il cliente può iscriversi alla lista d'attesa.
- Ogni rilascio di allocazioni pubblica un evento; un job verifica le iscrizioni compatibili in ordine cronologico e invia una notifica con un link che crea un hold riservato per un tempo limitato.

---

## 11. Mappa: editor e vista operativa

### 11.1 Rendering

- SVG con coordinate logiche (`maps.width × maps.height`) e `viewBox`, scalato al contenitore.
- Pan e zoom con gesti touch e rotella; pulsante "adatta alla vista".
- Fino a 1.000 elementi devono restare fluidi su un tablet di fascia media. Oltre questa soglia si usa la virtualizzazione per settori, non prevista nell'MVP.
- Ogni postazione è un `<g>` accessibile: `role="button"`, `aria-label` con etichetta e stato, focus da tastiera.
- La pagina pubblica offre sempre un'alternativa a elenco (per fila) per lettori di schermo e schermi piccoli.

### 11.2 Editor (solo pannello)

- Griglia con snapping (`grid_size`).
- **Generatore di file:** "crea N file da M posti, distanza X, numerazione A–Z / 1–N, a partire da…", come punto di partenza.
- Trascinamento singolo e multiplo, rotazione, allineamento e distribuzione, duplicazione.
- Palette di elementi: postazioni (tipi) ed elementi decorativi.
- Pannello proprietà: etichetta, zona, tipo, capacità, attributi, categoria di prezzo, stato attivo.
- Rinumerazione automatica di una fila selezionata.
- Annulla e ripeti (stack di comandi lato client), con salvataggio esplicito.
- **Salvataggio transazionale con controllo di versione:** `PUT /maps/{id}` con `If-Match` sulla versione; in caso di conflitto viene mostrata la differenza.
- Non è possibile eliminare una postazione con allocazioni future: si può solo disattivarla (`active = false`), e il sistema elenca le prenotazioni da spostare.
- Sfondo opzionale: immagine caricata dal gestore oppure perimetro della concessione fornito da `ConcessionDataProvider`, con scala e rotazione regolabili.

### 11.3 Vista operativa

- Selettore della data sempre visibile, con frecce giorno precedente e successivo e "oggi".
- Selettore della fascia: giorno intero, mattina, pomeriggio.
- **Colori per stato** (personalizzabili nel tema del pannello, con legenda sempre visibile):

| Stato | Significato |
|---|---|
| Libero | Nessuna allocazione |
| Prenotato online | Confermato, canale online |
| Prenotato cassa | Confermato, canale desk |
| Abbonato | Allocazione `season` |
| Abbonato assente oggi | Rilascio del giorno |
| In attesa / hold | Checkout o approvazione in corso |
| Da incassare | `paid_amount < total_amount` |
| Presente | Check-in effettuato |
| Bloccato | Blocco manuale |
| Prelazione rinnovo | Hold di rinnovo |

- Indicatori sovrapposti: icona extra, nota presente, cliente VIP.
- **Tocco su una postazione:** scheda rapida con cliente, periodo, pack, extra, saldo; azioni: nuova prenotazione, check-in, incassa, sposta, rilascia giorno, blocca, apri dettaglio.
- **Trascinamento** di una prenotazione su un'altra postazione: anteprima del conflitto, conferma, ricalcolo prezzo se cambia la categoria.
- **Selezione multipla** per prenotazioni di gruppo.
- **Ricerca** per nome cliente, telefono o codice: evidenzia le postazioni corrispondenti.
- Coda delle righe da assegnare (modalità `zone`) in un pannello laterale, da trascinare sulla mappa.
- Aggiornamento in tempo reale via SSE (sezione 21).

---

## 12. Pagina pubblica e widget incorporabile

### 12.1 URL e modalità di accesso

- Pagina: `https://{host}/b/{slug}` con parametri opzionali `?lang=en&from=2027-07-10&to=2027-07-12&pack=base&embed=1`.
- **Iframe:**
  ```html
  <iframe src="https://{host}/b/lido-x?embed=1"
          style="width:100%;border:0" allow="payment" loading="lazy"
          title="Prenota il tuo posto"></iframe>
  ```
- **Script** (consigliato, gestisce altezza e parametri):
  ```html
  <div data-arenile="lido-x" data-lang="it"></div>
  <script src="https://{host}/embed.js" async></script>
  ```
  `embed.js` (meno di 3 KB) crea l'iframe, ascolta i messaggi `postMessage` di tipo `arenile:resize`, `arenile:booked`, `arenile:navigate`, e rilancia un `CustomEvent` sulla pagina ospitante per chi vuole tracciare le conversioni.

### 12.2 Sicurezza dell'incorporamento

- Header `Content-Security-Policy: frame-ancestors` costruito da `allowed_embed_origins` dello stabilimento (vuoto = solo stesso dominio e link diretto).
- I messaggi `postMessage` in uscita usano come destinazione l'origine verificata del contenitore; quelli in ingresso vengono accettati solo da origini autorizzate.
- Nessuna dipendenza da cookie di terze parti: lo stato del checkout è legato a un token di sessione salvato in `sessionStorage` (con fallback in memoria) e inviato come header.

### 12.3 Flusso utente

**Passo 1 — Quando**
- Calendario con disponibilità sintetica (giorni pieni disabilitati, periodi tariffari evidenziati se il gestore lo abilita).
- Scelta della formula consentita e della fascia.

**Passo 2 — Dove**
- Se almeno una zona online è `seat`: mappa con postazioni disponibili evidenziate, legenda dei prezzi per fila.
- Se solo `zone`: schede per zona con prezzo "da" e disponibilità indicativa ("ultimi 3 posti").
- Se solo `quantity`: selettore di quantità.
- Le modalità miste mostrano mappa e schede insieme.

**Passo 3 — Cosa**
- Pack compatibili con la scelta, composizione chiara (icone), extra con quantità.
- Riepilogo del prezzo sempre visibile e aggiornato a ogni modifica (chiamata `quote` con debounce).
- Campo codice promozionale.

**Passo 4 — Chi e pagamento**
- Nome, cognome, email, telefono; nessun account.
- Accettazione delle condizioni (testo dal gestore, versionato) e consenso marketing separato e facoltativo.
- Countdown visibile della validità del blocco.
- Pagamento o conferma.

**Conferma**
- Codice prenotazione, QR, riepilogo, pulsante "aggiungi al calendario" (file `.ics`), link di gestione inviato via email.

### 12.4 Area gestione prenotazione del cliente

Raggiungibile dal link magico (`/b/{slug}/m/{token}`):
- visualizza dettagli e QR;
- paga il saldo;
- modifica o cancella entro i limiti della policy, con anteprima del rimborso o credito;
- per gli abbonati: rilascia singoli giorni.

Il token è casuale (256 bit), salvato come hash, con validità fino alla fine della prenotazione più 30 giorni. È possibile richiedere un nuovo link inserendo email e codice prenotazione.

### 12.5 Tema

```json
{
  "logo_url": "...",
  "primary_color": "#0E6BA8",
  "accent_color": "#F2B134",
  "font_family": "Inter | system | serif",
  "radius": "none | sm | md | lg",
  "show_prices_on_map": true,
  "hide_header_in_embed": true,
  "custom_texts": {"it": {"intro": "...", "terms": "..."}}
}
```

Il tema viene applicato con variabili CSS; l'anteprima nel pannello è la pagina reale in modalità prova.

### 12.6 Requisiti non funzionali della pagina pubblica

- Primo caricamento: meno di 150 KB di JS gzip; LCP sotto 2,5 secondi su 4G.
- Funzionamento completo da 320 px di larghezza.
- Contrasto WCAG AA con qualsiasi colore primario scelto: il sistema calcola automaticamente il colore del testo sopra il colore primario.
- Pagina indicizzabile quando non in modalità `embed` (titolo, descrizione, dati strutturati `LocalBusiness`).

---

## 13. Pagamenti

### 13.1 Interfaccia

```go
package ext

type PaymentProvider interface {
    Code() string
    Capabilities() PaymentCapabilities // online, refunds, partial refunds, wallets

    // Crea un'intenzione di pagamento e restituisce i dati per il client.
    CreatePayment(ctx context.Context, req CreatePaymentRequest) (*CreatePaymentResult, error)

    // Verifica e interpreta un webhook del provider.
    ParseWebhook(ctx context.Context, r *http.Request) (*PaymentEvent, error)

    Refund(ctx context.Context, req RefundRequest) (*RefundResult, error)
}

type CreatePaymentRequest struct {
    EstablishmentID string
    BookingID       string
    Amount          int64
    Currency        string
    Description     string
    CustomerEmail   string
    ReturnURL       string
    IdempotencyKey  string
    Metadata        map[string]string
}

type CreatePaymentResult struct {
    ProviderRef  string
    Mode         string // "redirect" | "embedded"
    RedirectURL  string
    ClientSecret string
}
```

I provider si registrano nel registry dell'applicazione; ogni stabilimento sceglie il provider attivo e salva le credenziali cifrate (sezione 22).

### 13.2 Driver inclusi nel core

- **`manual`:** nessun pagamento online; la prenotazione va in `confirmed` (o `awaiting_approval`) e il saldo viene incassato alla cassa.
- **`stripe`:** chiavi dell'account del gestore (`publishable`, `secret`, `webhook secret`).
  - Modalità predefinita: **Stripe Checkout** in redirect a livello di finestra principale (`target=_top` dal widget, oppure nuova scheda se il contenitore lo impedisce). È la soluzione più robusta dentro gli iframe.
  - Modalità opzionale: **Payment Element** incorporato, abilitabile dal gestore, testata su Safari con `allow="payment"`.
  - Webhook gestiti: `checkout.session.completed`, `payment_intent.succeeded`, `payment_intent.payment_failed`, `charge.refunded`.
  - Idempotenza: `IdempotencyKey = booking_id + tentativo`.
  - Metadata: `establishment_id`, `booking_id`, `booking_code`.

### 13.3 Gestione dei webhook

- Endpoint: `POST /webhooks/payments/{provider}/{establishment_id}`.
- Verifica della firma obbligatoria.
- Ogni evento viene registrato con l'ID del provider in una tabella di deduplica: lo stesso evento non viene mai processato due volte.
- L'aggiornamento di `payments`, `bookings` e allocazioni avviene in un'unica transazione.
- Riconciliazione: un job ogni 15 minuti interroga il provider per i pagamenti `pending` più vecchi di 10 minuti, per coprire webhook persi.

### 13.4 Rimborsi e crediti

- I rimborsi calcolati dalla policy vengono eseguiti tramite `Refund` se il provider lo supporta, altrimenti registrati come rimborso manuale da completare.
- I crediti sono un saldo per cliente e stabilimento (`customer_credits`: amount, source, expires_on), utilizzabile come metodo di pagamento in prenotazioni successive.

---

## 14. Cassa e modulo fiscale

### 14.1 Cassa

- Apertura sessione con fondo cassa, registrazione degli incassi per metodo, chiusura con conteggio e differenza.
- Incasso rapido dalla mappa: importo proposto = saldo dovuto, metodo, eventuale nota.
- Vendita libera senza prenotazione (lettino extra al volo, giornaliero senza anagrafica con cliente generico "Cliente cassa").
- Report di chiusura stampabile in PDF.

### 14.2 Modulo fiscale

L'emissione del documento commerciale dipende dal dispositivo e dalla configurazione del gestore. Il core non implementa protocolli specifici dei registratori nell'MVP, ma predispone l'architettura.

```go
type FiscalDriver interface {
    Code() string
    Issue(ctx context.Context, doc FiscalDocumentRequest) (*FiscalDocumentResult, error)
    Void(ctx context.Context, ref string) error
    Status(ctx context.Context) (DeviceStatus, error)
}
```

Driver previsti:
- **`none`:** nessuna emissione, il gestore usa il proprio registratore separatamente (default MVP).
- **`export`:** esportazione giornaliera dei corrispettivi per aliquota e metodo in CSV e PDF, per il commercialista.
- **Driver per registratori telematici:** contributi successivi, uno per famiglia di protocollo.

Le righe del documento derivano dallo snapshot del prezzo (scorporo IVA per aliquota). Ogni emissione o errore è tracciato in `fiscal_documents`. La conformità normativa della configurazione resta responsabilità del gestore e del suo consulente: la documentazione lo indica chiaramente.

---

## 15. Pannello del gestore

### 15.1 Navigazione

1. **Oggi** — mappa operativa del giorno, arrivi, saldi da incassare, righe da assegnare, meteo opzionale.
2. **Mappa** — vista operativa per qualsiasi data.
3. **Prenotazioni** — elenco filtrabile (date, stato, canale, zona, saldo), dettaglio, creazione.
4. **Calendario** — vista a griglia postazioni × giorni (planning), utile per collocare soggiorni lunghi.
5. **Clienti** — anagrafica, storico, crediti, abbonamenti.
6. **Abbonamenti** — elenco stagionali, rilasci, rinnovi.
7. **Cassa** — sessione corrente, incassi, chiusure.
8. **Report** — occupazione, ricavi, canali, confronto con l'anno precedente, esportazioni.
9. **Configurazione** — stabilimento, stagioni e periodi, mappe, zone, catalogo, prezzi, policy, pagina pubblica, pagamenti, notifiche, utenti, integrazioni, webhook.

### 15.2 Configurazione guidata

Al primo avvio:
1. dati dello stabilimento e lingue;
2. scelta di un modello: "Lido classico a file", "Spiaggia attrezzata a quantità", "Lido con piscina", "Vuoto";
3. generatore delle file sulla mappa;
4. listino rapido: prezzo giornaliero per fila e periodo, con generazione automatica delle regole;
5. pagina pubblica: attivazione, tema, codice di incorporamento;
6. pagamenti: manuale o Stripe.

Ogni passo è saltabile e modificabile in seguito.

### 15.3 Report (MVP)

- Occupazione per giorno e per zona (percentuale di fasce occupate).
- Ricavi per periodo, per canale, per pack, per metodo di pagamento.
- Saldi aperti.
- Confronto stagione su stagione.
- Esportazioni CSV e XLSX di prenotazioni, clienti, pagamenti, corrispettivi.

### 15.4 Modalità prova

Interruttore nel pannello: le prenotazioni create sono marcate `test = true`, escluse da report ed esportazioni, eliminabili in blocco. Con Stripe, usa automaticamente le chiavi di test se configurate.

---

## 16. Utenti, ruoli e autenticazione

### 16.1 Ruoli

| Permesso | owner | manager | cashier | beach_staff | viewer |
|---|:-:|:-:|:-:|:-:|:-:|
| Configurazione stabilimento, prezzi, policy | ✔ | ✔ | | | |
| Utenti e ruoli | ✔ | | | | |
| Pagamenti e credenziali provider | ✔ | | | | |
| Creare e modificare prenotazioni | ✔ | ✔ | ✔ | | |
| Incassi e cassa | ✔ | ✔ | ✔ | | |
| Rimborsi | ✔ | ✔ | | | |
| Check-in, rilasci, blocchi postazione | ✔ | ✔ | ✔ | ✔ | |
| Vista mappa e prenotazioni | ✔ | ✔ | ✔ | ✔ | ✔ |
| Report ed esportazioni | ✔ | ✔ | | | ✔ |

I permessi sono definiti come costanti nel codice e verificati nel servizio di dominio, non solo nell'handler HTTP.

### 16.2 Autenticazione

- Login email e password con Argon2id; sessione con cookie `HttpOnly`, `Secure`, `SameSite=Lax`; rotazione del token al login.
- TOTP opzionale, obbligatorio per `owner` se attivato a livello di installazione.
- Reset password via email con token monouso a 30 minuti.
- Limitazione dei tentativi per IP e per account.
- Interfaccia `ext.AuthProvider` per aggiungere SSO tramite estensioni (es. OIDC).
- Chiavi API per integrazioni server-to-server, con scope e prefisso visibile (`ar_live_xxxx`).
- CLI: `arenile admin create --email ... --org ...` per il primo utente.

---

## 17. API

### 17.1 Convenzioni

- REST JSON, versione nel percorso: `/api/admin/v1`, `/api/public/v1`.
- Contratto in `api/openapi.yaml`, unica fonte di verità; codice server e client TypeScript generati.
- Errori in formato `application/problem+json` (RFC 9457) con `type` stabile, ad esempio `spot_unavailable`, `no_price`, `policy_violation`, `hold_expired`.
- Paginazione a cursore (`?cursor=&limit=`).
- `Idempotency-Key` accettato su tutti i `POST` che creano risorse o pagamenti.
- Concorrenza ottimistica con `ETag` e `If-Match` su configurazione e mappe.
- Date di calendario come `YYYY-MM-DD` nel fuso dello stabilimento; istanti in RFC 3339 UTC.

### 17.2 API pubblica (selezione)

| Metodo | Percorso | Descrizione |
|---|---|---|
| GET | `/api/public/v1/{slug}` | Dati dello stabilimento, tema, lingue, configurazione del flusso |
| GET | `/api/public/v1/{slug}/calendar?month=` | Disponibilità sintetica per giorno |
| GET | `/api/public/v1/{slug}/maps/{id}` | Geometria della mappa (senza dati sensibili) |
| GET | `/api/public/v1/{slug}/availability` | Disponibilità dettagliata (7.8) |
| GET | `/api/public/v1/{slug}/catalog` | Pack ed extra vendibili online |
| POST | `/api/public/v1/{slug}/quote` | Calcolo prezzo e policy |
| POST | `/api/public/v1/{slug}/bookings` | Crea prenotazione con hold |
| PATCH | `/api/public/v1/{slug}/bookings/{id}` | Dati cliente, extra, codice promozionale |
| POST | `/api/public/v1/{slug}/bookings/{id}/pay` | Avvia pagamento |
| GET | `/api/public/v1/{slug}/bookings/{id}` | Stato (con token di sessione) |
| DELETE | `/api/public/v1/{slug}/bookings/{id}` | Abbandono, rilascia l'hold |
| GET | `/api/public/v1/{slug}/manage/{token}` | Prenotazione dal link magico |
| POST | `/api/public/v1/{slug}/manage/{token}/cancel` | Cancellazione secondo policy |
| POST | `/api/public/v1/{slug}/manage/{token}/releases` | Rilascio giorni (abbonati) |
| POST | `/api/public/v1/{slug}/waitlist` | Iscrizione lista d'attesa |

Limiti: 60 richieste al minuto per IP sulle letture, 10 al minuto sulle creazioni; protezione aggiuntiva opzionale con challenge invisibile (interfaccia `ext.BotProtection`, nessun servizio imposto).

### 17.3 API di amministrazione (gruppi)

- `establishments`, `seasons`, `rate-periods`, `closure-days`
- `maps`, `zones`, `spots`, `decorations`, `pools`
- `components`, `packs`, `extras`
- `price-rules`, `fixed-prices`, `promo-codes`, `agreements`, `pricing/simulate`, `pricing/matrix`, `pricing/coverage`
- `policies`
- `bookings` (+ `assign`, `move`, `check-in`, `cancel`, `approve`, `reject`), `assignments/auto`
- `season-passes`, `releases`, `renewals`
- `customers`, `credits`, `waitlist`
- `payments`, `refunds`, `cash-sessions`, `fiscal-documents`
- `reports/*`, `exports/*`
- `users`, `memberships`, `api-keys`, `audit-log`
- `webhooks`, `notification-templates`
- `stream` (SSE)

---

## 18. Eventi, webhook e punti di estensione

### 18.1 Eventi di dominio

Scritti nell'outbox nella stessa transazione della modifica, poi distribuiti dal job runner.

```
booking.created          booking.held            booking.confirmed
booking.modified         booking.cancelled       booking.checked_in
booking.no_show          booking.completed       booking.approval_requested
allocation.released      season.day_released     season.renewal_offered
payment.succeeded        payment.failed          payment.refunded
customer.created         customer.anonymized
cash_session.closed      config.pricing_changed  config.map_changed
```

Formato:

```json
{
  "id": "evt_01J...",
  "type": "booking.confirmed",
  "establishment_id": "...",
  "occurred_at": "2027-07-01T09:12:44Z",
  "data": { "booking": { "...": "..." } },
  "version": 1
}
```

### 18.2 Webhook in uscita

- Configurabili per stabilimento, con filtro sugli eventi.
- Firma `Arenile-Signature: t=<timestamp>,v1=<HMAC-SHA256>` sul corpo con il segreto dell'endpoint.
- Tentativi con backoff esponenziale fino a 24 ore; disattivazione automatica dopo errori ripetuti, con avviso al proprietario.
- Pagina di log delle consegne con possibilità di reinvio.

### 18.3 Punti di estensione Go (`pkg/ext`)

```go
type Extensions struct {
    PaymentProviders  []PaymentProvider
    FiscalDrivers     []FiscalDriver
    EventSinks        []EventSink            // consumatori interni degli eventi
    ConcessionData    ConcessionDataProvider // perimetro, satellite, scadenze
    TenantResolver    TenantResolver         // risolve lo stabilimento da host/header
    AuthProviders     []AuthProvider
    Notifiers         []Notifier             // canali aggiuntivi (WhatsApp, SMS)
    BotProtection     BotProtection
    HTTPRoutes        func(r chi.Router)     // rotte aggiuntive (es. billing)
    AdminUIPlugins    []UIPluginManifest     // voci di menu e pagine extra nel pannello
}

func main() {
    app := arenile.New(config.FromEnv(), arenile.WithExtensions(ext))
    app.Run()
}
```

Il binario ufficiale usa un `Extensions` con le sole implementazioni del core. Chi vuole aggiungere funzionalità (integratori, fornitori di hosting, sviluppatori di driver) crea un proprio `main` che importa il modulo e registra le proprie implementazioni.

**Plugin UI del pannello:** un manifest dichiara voci di menu e un URL di pagina, caricata in un'area dedicata del pannello (iframe con token di sessione a breve durata). Così un'estensione può aggiungere pagine al pannello senza modificare il frontend del core.

---

## 19. Notifiche

- Canale email nel core (SMTP); interfaccia `Notifier` per WhatsApp o SMS come estensioni.
- Template predefiniti in tutte le lingue abilitate, modificabili dal gestore in Markdown con variabili (`{{booking.code}}`, `{{customer.first_name}}`, `{{manage_url}}`).
- Notifiche al cliente: conferma, promemoria del saldo, promemoria il giorno prima, cancellazione, rimborso, proposta di rinnovo, posto disponibile dalla lista d'attesa.
- Notifiche al gestore: nuova prenotazione online, richiesta di approvazione, pagamento fallito, chiusura cassa con differenza, errore di webhook.
- Invio asincrono tramite outbox, con tentativi e registro in `message_log`.
- Allegati: QR in linea, file `.ics`.

---

## 20. Internazionalizzazione

- Lingue incluse: italiano (fonte), inglese, tedesco, francese. Struttura pronta per contributi della community.
- Testi dell'interfaccia in file JSON per i18next; testi del backend (email, PDF) in file `go-i18n`.
- Contenuti del gestore (nomi di pack, zone, condizioni) come campi `json i18n` con fallback alla lingua predefinita dello stabilimento.
- Formati di data, numero e valuta con `Intl` lato client e `golang.org/x/text` lato server.
- La lingua della pagina pubblica è scelta da parametro, poi dalla lingua del browser, poi dal default dello stabilimento.

---

## 21. Tempo reale e funzionamento offline

### 21.1 Tempo reale

- Endpoint SSE `GET /api/admin/v1/stream?establishment=` autenticato.
- Gli eventi di allocazione e prenotazione vengono inviati come messaggi leggeri (`{type, spot_ids, dates}`); il client invalida le query interessate di TanStack Query.
- Broker in memoria nel processo. Per installazioni con più istanze in parallelo l'interfaccia `ext.EventBus` permette di usare `LISTEN/NOTIFY` di PostgreSQL.

### 21.2 Offline (pannello)

Obiettivo realistico per l'MVP: **continuità della consultazione e delle operazioni a basso rischio**.

- Il pannello è una PWA: shell e risorse in cache.
- Cache locale (IndexedDB) della mappa e delle prenotazioni del giorno corrente e del successivo.
- Operazioni consentite offline e messe in coda: check-in, nota, registrazione di un incasso in contanti su prenotazione esistente.
- Operazioni **non** consentite offline: creazione o spostamento di prenotazioni, perché richiedono la garanzia di disponibilità del server.
- Alla riconnessione la coda viene inviata in ordine con chiavi di idempotenza; eventuali conflitti vengono mostrati all'utente.
- Indicatore di stato della connessione sempre visibile.

---

## 22. Sicurezza

- HTTPS obbligatorio in produzione; il binario può fare da terminatore TLS con certificati automatici (`autocert`) oppure stare dietro un reverse proxy.
- Header di sicurezza: CSP restrittiva sul pannello, `frame-ancestors` per stabilimento sulla pagina pubblica, `X-Content-Type-Options`, `Referrer-Policy`, HSTS.
- CSRF: cookie `SameSite=Lax` più token double-submit sulle richieste che modificano dati del pannello.
- **Segreti cifrati a riposo** (chiavi Stripe, segreti dei webhook, TOTP) con AES-GCM e chiave da `ARENILE_SECRET_KEY`; rotazione supportata con più chiavi attive.
- Isolamento per stabilimento verificato a livello di repository: ogni query operativa richiede `establishment_id` dal contesto autenticato, con test automatici che tentano accessi incrociati.
- Limitazione delle richieste su login, API pubblica e creazione prenotazioni.
- Caricamento file (loghi, sfondi): solo immagini, verifica del tipo reale, ridimensionamento lato server, rimozione dei metadati EXIF.
- Dipendenze controllate in CI (`govulncheck`, `npm audit`), immagine Docker scansionata (Trivy).
- Policy di segnalazione delle vulnerabilità in `SECURITY.md`.

---

## 23. Privacy e GDPR

- Dati minimi per una prenotazione online: nome, cognome, email o telefono.
- Testi delle condizioni e dell'informativa versionati; ogni prenotazione salva la versione accettata.
- Consenso marketing separato, facoltativo, con data, fonte e versione del testo.
- **Esportazione** dei dati di un cliente in JSON dal pannello.
- **Anonimizzazione** su richiesta: i dati personali vengono sostituiti, mentre prenotazioni e pagamenti restano per gli obblighi contabili.
- **Conservazione configurabile:** anonimizzazione automatica dei clienti senza prenotazioni da N anni (default 5, modificabile).
- Log applicativi senza dati personali in chiaro (email e telefoni mascherati).
- Documentazione per il gestore con un modello di informativa da adattare e l'indicazione dei ruoli (titolare: gestore; responsabile: chi ospita il servizio).

---

## 24. Deploy, configurazione e operatività

### 24.1 Comandi del binario

```
arenile serve                 # avvia il server (esegue le migrazioni se ARENILE_AUTO_MIGRATE=true)
arenile migrate up|status
arenile admin create          # crea il primo utente owner
arenile seed demo             # carica uno stabilimento dimostrativo completo
arenile backup --out FILE     # backup consistente (SQLite: VACUUM INTO; Postgres: indica pg_dump)
arenile restore --in FILE
arenile version
```

### 24.2 Dockerfile (multi-stage)

```dockerfile
# 1. Frontend
FROM node:22-alpine AS web
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci
COPY web ./web
COPY api ./api
RUN cd web && npm run build          # produce web/admin/dist e web/booking/dist

# 2. Backend
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/admin/dist   ./internal/http/static/admin
COPY --from=web /src/web/booking/dist ./internal/http/static/booking
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" -o /out/arenile ./cmd/arenile

# 3. Runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/arenile /arenile
VOLUME ["/data"]
ENV ARENILE_DATA_DIR=/data ARENILE_ADDR=:8080
EXPOSE 8080
HEALTHCHECK NONE
USER nonroot
ENTRYPOINT ["/arenile", "serve"]
```

> Le versioni delle immagini di base vanno allineate all'ultima stabile al momento del rilascio. L'healthcheck è demandato all'orchestratore tramite `/healthz` perché l'immagine distroless non contiene `curl`.

### 24.3 docker-compose.yml (installazione standard)

```yaml
services:
  arenile:
    image: ghcr.io/coste360/arenile:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - arenile-data:/data
    environment:
      ARENILE_BASE_URL: "https://prenota.mio-lido.it"
      ARENILE_SECRET_KEY: "${ARENILE_SECRET_KEY}"
      ARENILE_SMTP_URL: "${ARENILE_SMTP_URL}"
volumes:
  arenile-data:
```

Variante `docker-compose.postgres.yml` con servizio PostgreSQL e `DATABASE_URL`.

### 24.4 Variabili d'ambiente

| Variabile | Default | Descrizione |
|---|---|---|
| `ARENILE_ADDR` | `:8080` | Indirizzo di ascolto |
| `ARENILE_BASE_URL` | — | URL pubblico, obbligatorio in produzione |
| `ARENILE_DATA_DIR` | `/data` | Database SQLite, file caricati, backup |
| `DATABASE_URL` | vuoto | Se valorizzato con `postgres://`, usa PostgreSQL |
| `ARENILE_SECRET_KEY` | — | Chiave di cifratura (32 byte base64); generata e mostrata al primo avvio se assente in sviluppo |
| `ARENILE_AUTO_MIGRATE` | `true` | |
| `ARENILE_TLS_AUTOCERT_DOMAINS` | vuoto | Abilita certificati automatici |
| `ARENILE_SMTP_URL` | vuoto | `smtp://user:pass@host:587` |
| `ARENILE_MAIL_FROM` | — | |
| `ARENILE_HOLD_TTL` | `10m` | |
| `ARENILE_LOG_LEVEL` | `info` | |
| `ARENILE_TRUSTED_PROXIES` | vuoto | CIDR dei proxy per l'IP reale |
| `ARENILE_REQUIRE_TOTP_FOR_OWNERS` | `false` | |
| `ARENILE_TELEMETRY` | `false` | Nessuna telemetria di default |

### 24.5 Operatività

- `/healthz` (processo vivo) e `/readyz` (database raggiungibile, migrazioni allineate).
- `/metrics` in formato Prometheus, disattivato di default, protetto da token.
- Backup automatico giornaliero opzionale nel data dir con retention configurabile (SQLite).
- Aggiornamento: `docker compose pull && docker compose up -d`; migrazioni sempre compatibili in avanti; note di aggiornamento nel changelog per ogni versione minore.
- Template one-click per Coolify e CapRover; documentazione per VPS e per Raspberry Pi (build `arm64`).
- Immagini multi-architettura `amd64` e `arm64` pubblicate su GitHub Container Registry.

---

## 25. Test e qualità

| Livello | Strumenti | Copertura richiesta |
|---|---|---|
| Unit motore prezzi | Test table-driven Go | Ogni tipo di condizione e azione, conflitti, arrotondamenti, IVA, durate fisse, mezze giornate |
| Proprietà motore prezzi | `pgregory.net/rapid` | Invarianti: totale ≥ 0, monotonia degli sconti, snapshot ricalcolabile |
| Unit disponibilità | Go | Hold, scadenze, pool, quote, rilasci, assegnazione automatica |
| Concorrenza | Go con N goroutine | Nessuna doppia allocazione con 100 richieste parallele sulla stessa postazione, su SQLite e PostgreSQL |
| Integrazione | Go + database reale (SQLite in memoria, PostgreSQL in container) | Repository e migrazioni su entrambi i dialetti |
| Contratto API | Validazione delle risposte contro `openapi.yaml` | Tutti gli endpoint |
| Frontend | Vitest + Testing Library | Componenti della mappa, form, calcolo locale del riepilogo |
| End-to-end | Playwright | Flusso pubblico completo con Stripe in modalità test e webhook simulati; flusso cassa; editor mappa; iframe su pagina ospite di prova |
| Accessibilità | `axe-core` in Playwright | Pagina pubblica senza violazioni gravi |
| Prestazioni | Lighthouse CI | Budget della pagina pubblica (12.6) |

**CI (GitHub Actions):** lint (`golangci-lint`, ESLint, Prettier), test, build dell'immagine, scansione di sicurezza, verifica che il codice generato da OpenAPI sia aggiornato.

**Rilasci:** versionamento semantico, changelog generato da Conventional Commits, tag che pubblica binari e immagini.

---

## 26. Licenza e governance del progetto

- Licenza del core: **AGPL-3.0**.
- **CLA** obbligatorio per i contributi, firmato tramite bot alla prima pull request: consente a Coste360 SRL, titolare del progetto, di gestire la licenza nel tempo. Testo da far validare a un legale.
- Marchio: il nome e il logo del progetto vanno registrati da Coste360 SRL; `TRADEMARK.md` chiarisce che i fork devono cambiare nome.
- File di progetto: `README.md` (installazione in 5 minuti), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `docs/` con guide per gestori e per sviluppatori.
- Issue template per bug, richieste di funzionalità e nuovi driver (pagamento, fiscale, notifiche).
- Demo pubblica con dati di esempio, azzerata ogni notte.

---

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

## 28. Roadmap e criteri di accettazione

### M0 — Fondamenta
- Repository, struttura, CI, Dockerfile, compose, OpenAPI iniziale.
- Configurazione, migrazioni su entrambi i dialetti, CLI, autenticazione e ruoli, audit log, outbox, job runner.
- Supporto agli agenti (sezione 27): `AGENTS.md`, `CLAUDE.md`, `.claude/` con regole, skill, subagent e impostazioni, script degli hook, devcontainer, template dei task, `docs/architecture.md`, `docs/invariants.md`, primi cinque ADR, Makefile completo, `make seed`.
- **Accettazione:** `docker compose up` avvia un'istanza vuota, `arenile admin create` permette il login, la CI è verde su SQLite e PostgreSQL. Un agente avviato in un devcontainer nuovo, con il solo task "aggiungi l'endpoint `GET /api/admin/v1/establishments`", completa il lavoro usando `/new-endpoint` e supera `make check` senza istruzioni aggiuntive.

### M1 — Inventario e mappa
- Stabilimento, stagioni, periodi, chiusure.
- Editor della mappa con generatore di file, zone, postazioni, decorazioni, pool.
- Catalogo: componenti, pack, extra.
- Configurazione guidata con i modelli.
- **Accettazione:** un gestore configura un lido di 8 file da 15 posti in meno di 15 minuti partendo dal modello.

### M2 — Prezzi e policy
- Motore prezzi completo (8.1–8.6), prezzi a durata fissa, codici promozionali, convenzioni.
- Simulatore, matrice di listino, verifica di copertura, copia di stagione.
- Policy (sezione 9).
- **Accettazione:** suite di test del motore prezzi con almeno 150 casi; la matrice di listino coincide con i calcoli del simulatore.

### M3 — Prenotazioni dal pannello e cassa
- Motore di disponibilità con allocazioni, hold, pool, quote.
- Vista operativa della mappa, calendario planning, prenotazioni, clienti, abbonamenti stagionali e rilasci.
- Pagamenti manuali, cassa con apertura e chiusura, esportazione corrispettivi.
- SSE per l'aggiornamento della mappa.
- **Accettazione:** test di concorrenza superati; una giornata simulata con 300 operazioni non produce incoerenze tra allocazioni, prenotazioni e incassi.

### M4 — Pagina pubblica e pagamenti online (MVP pubblico)
- Pagina pubblica nelle tre modalità (posto, zona, quantità), widget `embed.js`, tema.
- Driver Stripe (Checkout), webhook, riconciliazione, rimborsi.
- Link magico per la gestione del cliente, email transazionali, QR e `.ics`.
- Assegnazione automatica per le zone `zone`.
- **Accettazione:** prenotazione completa da smartphone dentro un iframe su un dominio esterno in meno di 2 minuti; nessuna doppia vendita tra cassa e online in un test con carico parallelo; budget di peso e Lighthouse rispettati.

### M5 — Completamento della stagione
- Rinnovi degli abbonamenti, lista d'attesa, crediti.
- Report completi e confronto anni.
- PWA del pannello con offline limitato (21.2).
- Webhook in uscita configurabili, chiavi API.
- Traduzioni inglese, tedesco, francese.
- **Accettazione:** ciclo completo di fine stagione → nuova stagione (copia configurazione, proposta rinnovi, conferme) eseguito senza interventi manuali sul database.

### Dopo l'MVP
- Driver Satispay e altri provider; driver per registratori telematici.
- Notifiche WhatsApp e SMS tramite estensioni.
- App Expo per lo staff in spiaggia.
- Bar e ristorante con addebito sulla postazione.
- Prezzi per occupazione (regole `occupancy_percent`) abilitati nell'interfaccia.
- Multi-mappa avanzata e virtualizzazione per stabilimenti molto grandi.
