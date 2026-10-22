# Glossario di dominio

Fonte unica dei termini usati in Arenile (spec sezione 5.1). Il termine italiano è quello usato nella
specifica e nell'interfaccia; l'identificatore inglese è quello da usare nel codice (tipi Go, tabelle,
campi JSON, chiavi i18n). Se serve un termine nuovo, va aggiunto qui prima di usarlo nel codice.

| Termine | Identificatore nel codice | Significato |
|---|---|---|
| **Organizzazione** | `Organization`, tabella `organizations` | Soggetto titolare (la società che gestisce). Può avere più stabilimenti. |
| **Stabilimento** | `Establishment`, `establishments`, `establishment_id` | Un lido. Ha fuso orario, stagioni, mappe, catalogo, listini, policy. |
| **Stagione** | `Season`, `seasons` | Periodo di apertura annuale (es. 2027-05-01 → 2027-09-30). |
| **Periodo tariffario** | `RatePeriod`, `rate_periods` | Intervallo di date con nome (bassa, media, alta, Ferragosto) usato dalle regole di prezzo. |
| **Giorno di chiusura** | `ClosureDay`, `closure_days` | Data in cui lo stabilimento non vende. |
| **Mappa** | `Map`, `maps` (package `mapping`) | Rappresentazione grafica di un'area (spiaggia, piscina). |
| **Zona** | `Zone`, `zones` | Raggruppamento logico di postazioni (Prima fila, Zona B, Piscina). |
| **Modalità di assegnazione** | `AssignmentMode`: `seat`, `zone`, `quantity` | Come il cliente sceglie: posto esatto, zona, sola quantità (spec 5.2). |
| **Postazione** | `Spot`, `spots` | Elemento prenotabile con posizione: ombrellone, gazebo, tenda, palma. |
| **Categoria di prezzo** | `PriceCategory`, `price_category` | Classe di postazioni con lo stesso listino (es. `row1`, `row2`). |
| **Pool** | `Pool`, `pools`, `pool_usage` | Inventario a quantità senza posizione: "lettini sfusi", "ombrelloni zona libera". |
| **Elemento decorativo** | `Decoration`, `decorations` | Elemento non prenotabile della mappa: passerella, docce, torretta, bar. |
| **Componente** | `Component`, `components` | Unità di inventario consumata da un pack: ombrellone, lettino, sdraio, cabina. |
| **Pack** | `Pack`, `packs` | Prodotto vendibile composto da componenti (1 ombrellone + 2 lettini). |
| **Extra** | `Extra`, `extras` | Aggiunta opzionale a un pack (lettino aggiuntivo, cabina, telo, parcheggio). |
| **Fascia** | `calendar.Slot`: `AM`, `PM`; `calendar.Slots`: `AM`, `PM`, `FULL` | Unità temporale minima: mattina e pomeriggio. Il giorno intero è `AM+PM` (`FULL`). |
| **Formula** | `Formula`: `half_day`, `day`, `multi_day`, `week`, `fortnight`, `month`, `season` | Tipo di durata. |
| **Prenotazione** | `Booking`, `bookings` | Acquisto di uno o più pack su un intervallo di date, con extra, cliente e pagamenti. |
| **Codice prenotazione** | `Booking.Code` (es. `AR-7K3Q9`) | Codice leggibile mostrato al cliente. |
| **Riga di prenotazione** | `BookingLine`, `booking_lines` | Un pack prenotato con la sua assegnazione (postazione, zona o pool). |
| **Stato di assegnazione** | `AssignmentStatus`: `assigned`, `pending`, `not_applicable` | Se la riga ha già una postazione. |
| **Allocazione** | `SpotAllocation` (`spot_allocations`), `PoolAllocation` (`pool_allocations`) | Occupazione di una risorsa per data e fascia. È ciò che garantisce l'assenza di doppie prenotazioni. |
| **Tipo di allocazione** | `AllocationKind`: `hold`, `booking`, `block`, `season` | Blocco temporaneo, prenotazione, blocco manuale, abbonamento. |
| **Blocco temporaneo (hold)** | `hold`, `hold_expires_at`, `ARENILE_HOLD_TTL` | Occupazione a scadenza durante il checkout. |
| **Abbonamento stagionale** | `is_season_pass`, `SeasonPass` | Prenotazione con formula stagione, con diritto di rinnovo l'anno successivo. |
| **Rilascio** | `SeasonRelease`, `season_releases` | Giorno in cui un abbonato libera il proprio posto, rivendibile. |
| **Rinnovo** | `Renewal`, `renewal_of_booking_id` | Proposta di prenotazione per la nuova stagione sulla stessa postazione. |
| **Lista d'attesa** | `WaitlistEntry`, `waitlist_entries` | Iscrizione per una richiesta senza disponibilità. |
| **Canale** | `Channel`: `desk`, `online`, `api` | Origine della prenotazione: cassa, online, API. |
| **Quota online** | `online_quota` | Massimo vendibile online per zona, data e fascia. |
| **Regola di prezzo** | `PriceRule`, `price_rules` (stage `base`, `modifier`, `discount`) | Condizioni + azione valutate dal motore prezzi. |
| **Prezzo a durata fissa** | `FixedDurationPrice`, `fixed_prices` | Prezzo per settimana, quindicina, mese, stagione. |
| **Convenzione** | `Agreement`, `agreements` | Accordo con sconti dedicati (es. hotel, aziende). |
| **Codice promozionale** | `PromoCode`, `promo_codes` | Codice sconto inserito dal cliente. |
| **Dettaglio prezzo / snapshot** | `PriceBreakdown`, `price_snapshot`, `engine_version` | Risultato completo del calcolo salvato sulla prenotazione. |
| **Policy** | `Policy`, `policies`, `policy_snapshot` | Vincoli di prenotazione, pagamento, cancellazione (spec 9). |
| **Acconto / saldo** | `deposit_due`, `balance_due` | Quota da pagare subito e residuo. |
| **Cliente** | `Customer`, `customers` | Persona che prenota. |
| **Credito** | `CustomerCredit`, `customer_credits` | Saldo spendibile del cliente in uno stabilimento. |
| **Pagamento / rimborso** | `Payment` (`direction`: `in`, `refund`), `payments` | Movimento di denaro legato o no a una prenotazione. |
| **Provider di pagamento** | `ext.PaymentProvider` (`manual`, `stripe`) | Driver di pagamento. |
| **Sessione di cassa** | `CashSession`, `cash_sessions` | Apertura, incassi e chiusura della cassa. |
| **Documento commerciale** | `FiscalDocument`, `fiscal_documents`, `ext.FiscalDriver` | Documento fiscale emesso per un pagamento. |
| **Ruolo** | `Role`: `owner`, `manager`, `cashier`, `beach_staff`, `viewer` | Livello di permessi (spec 16.1). |
| **Membership** | `Membership`, `memberships` | Legame utente ↔ organizzazione/stabilimento con un ruolo. |
| **Chiave API** | `APIKey`, `api_keys` (prefisso `ar_live_`) | Credenziale per integrazioni server-to-server. |
| **Registro di audit** | `AuditLog`, `audit_log` | Traccia, solo inserimento, di ogni modifica. |
| **Evento di dominio** | `Event`, `outbox_events` | Fatto registrato nell'outbox (spec 18). |
| **Link magico** | `manage_token`, `manage_token_hash` | Link con cui il cliente gestisce la propria prenotazione. |
| **Check-in** | `checked_in` | Presenza registrata in spiaggia. |
