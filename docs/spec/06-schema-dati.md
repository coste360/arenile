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
| kind | enum | `umbrella`, `gazebo`, `tent`, `palm`, `sunbed`, `daybed`, `cabana`, `hammock`, `custom`. Tipo semantico (pack `requires_spot_kind`, assegnazione); le famiglie del kit sono aggiunte dall'ADR 0012 |
| asset | text null | Illustrazione del kit della mappa (id del manifest, solo asset prenotabili; ADR 0012). Null = illustrazione predefinita del tipo |
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
| asset | text null: illustrazione del kit (qualsiasi asset del manifest; mai su `text`). Null = illustrazione predefinita del tipo (`custom`: nessuna) |
| variant | text null: variante di colore prevista dall'asset (`bandiera_mare`: `green`, `yellow`, `red`) |
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
| phone | text null | E.164; sempre facoltativo (ADR 0020), usato per la deduplica solo se presente |
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
**notification_templates**: establishment_id, code, locale, subject, body (Markdown con variabili), active. Senza API dalla 1.0: i testi si modificano con `email_template_overrides`.
**email_template_overrides** (ADR 0022): establishment_id, kind, locale, subject, opening, closing, signature, active, version; unique (establishment_id, kind, locale).
**message_log**: destinatario, canale, template, stato, errore.

---

