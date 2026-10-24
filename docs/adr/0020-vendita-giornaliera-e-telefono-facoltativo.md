# 0020 — Vendita giornaliera con biglietti numerati e telefono facoltativo

- **Stato:** accettato
- **Data:** 2026-10-24
- **Sezioni della specifica:** `docs/spec/14-cassa-e-modulo-fiscale.md` (14.3), `docs/spec/07-motore-di-disponibilita.md` (7.5),
  `docs/spec/12-pagina-pubblica-e-widget-incorporabile.md`, `docs/spec/23-privacy-e-gdpr.md`

## Contesto

Il gestore vende molte giornate al banco a clienti di passaggio: serve una schermata unica e veloce, un numero
progressivo del giorno da stampare sul biglietto, la vendita "per fila" senza scegliere subito l'ombrellone e i
biglietti d'ingresso per spiaggia libera attrezzata e piscina. Il telefono obbligatorio rallentava il banco e il
checkout online senza vantaggi reali.

## Decisione

- Ogni vendita rapida crea una prenotazione confermata (canale `desk`) attraverso `internal/domain/booking` (audit e
  outbox nella stessa transazione) e un biglietto in `tickets` che la possiede (`UNIQUE (booking_id)`).
- Il numero del giorno viene da `daily_ticket_counters` con un solo upsert atomico
  (`INSERT ... ON CONFLICT DO UPDATE SET last_number = last_number + 1 RETURNING`) nella transazione di vendita;
  `UNIQUE (establishment_id, ticket_date, ticket_number)` è la garanzia finale. Un annullamento conserva il numero.
- Vendita per zona: la capacità si consuma con i vincoli esistenti. Nelle zone `zone` la riga resta `pending` sul pool
  implicito (spec 7.5); nelle zone `seat` la riga occupa in modo provvisorio l'ultima postazione libera della zona
  (`tickets.provisional_spot_id`, vincolo unico di `spot_allocations`) e `tickets.needs_assignment` resta vero finché
  la `Move` della prenotazione non la colloca. Nessun controllo applicativo sostituisce i vincoli.
- Assegnazione: un biglietto "da assegnare" si colloca solo con la `Move` della sua prenotazione (`booking.MoveThen`:
  allocazione con la chiave unica di `spot_allocations`, aggiornamento condizionale del biglietto, audit e outbox nella
  stessa transazione; 409 `assignment_conflict` se la postazione è stata presa). L'assegnazione automatica usa il motore
  greedy di spec 7.5 in ordine di numero. La mappa etichetta la postazione con il nome facoltativo del biglietto o con
  il numero ("#037"), mai con il cliente condiviso "Cliente al banco" su cui sono registrate le vendite senza cliente;
  una postazione provvisoria porta l'indicatore "da assegnare".
- La prenotazione di un biglietto appartiene al biglietto: `booking.Cancel`, `Update` e `Move` generici e
  l'assegnazione automatica delle righe in attesa la rifiutano o la saltano (`ErrTicketOwned`, 409 `ticket_owned`);
  solo il `void` (cancellazione, rimborso) e `MoveThen` (assegnazione, spostamento) la cambiano, con il biglietto nella
  stessa transazione. Annullamento e assegnazione prendono i lock nello stesso ordine (prenotazione, poi biglietto).
  Gli endpoint del biglietto richiedono `If-Match` con la versione letta (428/412), come le azioni della mappa
  (ADR 0019 C); l'assegnazione automatica ne è esente perché legge i biglietti nella stessa richiesta e colloca con
  aggiornamenti condizionali.
- Prezzo: il totale di un biglietto è quello incassato al banco e **non cambia** con l'assegnazione o lo spostamento,
  anche su una postazione di un'altra categoria (`MoveInput.KeepPrice`: il prezzo del motore resta nel diff dell'audit
  come `engine_total`, `price_kept`). Una vendita per zona è prezzata sulla zona (tutte le postazioni di una zona hanno
  la sua categoria di prezzo). Decisione del prodotto: un biglietto giornaliero è un prezzo chiuso; chi vuole una
  postazione di categoria superiore paga un nuovo biglietto.
- Stampa: HTML stampabile dal browser (scontrino 80/58 mm, A5, A4) con utility `print:` di Tailwind, QR generato nel
  frontend senza nuove dipendenze; nessun PDF lato server. La postazione o la zona è in grande sotto il numero; il
  biglietto porta "Non valido ai fini fiscali" (il documento fiscale è quello del pagamento). Il QR contiene il
  `gate_token` del biglietto (casuale, 128 bit, base32, `UNIQUE`), mai il codice della prenotazione, che è progressivo
  e indovinabile e resta stampato in chiaro sotto il QR.
- I biglietti d'ingresso sono pack su zone `quantity` (`entry_ticket_types`): prezzo dal motore per periodo, capienza
  dal pool condiviso con l'online, ingresso registrato con un aggiornamento condizionale di `tickets.entered_at`. Una
  vendita è un gruppo che entra insieme da una sola zona: un biglietto, un QR, una scansione per il gruppo (tipi di
  zone diverse si vendono separati). Al varco solo il token fa entrare da solo; il numero del giorno o il codice della
  prenotazione scritti a mano passano come inserimento manuale confermato dallo staff (`manual`, nell'audit). Safari su
  iPhone e Firefox non hanno `BarcodeDetector`: lì il varco è l'inserimento manuale del numero (nessuna libreria di
  decodifica aggiunta: una dipendenza in più per un caso che il numero a due cifre copre). Gli ingressi scrivono
  l'audit, non l'outbox: non sono trasmessi (la prenotazione non cambia).
- Il riepilogo di "Ticket del giorno" è per giorno di validità (`ticket_date`) e non riconcilia la cassa: incassi,
  rimborsi e corrispettivi seguono la data contabile dei pagamenti (una vendita anticipata sta nel giorno di utilizzo
  dei biglietti e nella cassa del giorno dell'incasso).
- Il cliente di sistema "Cliente al banco" (tag `walk-in`) resta fuori dalla classifica dei migliori clienti; non ha
  abbonamenti, quindi nemmeno campagne di rinnovo.
- Il telefono è facoltativo ovunque (checkout pubblico, nuovo cliente, modifica cliente, import): `require_phone` resta
  nel contratto come deprecato e vale sempre false. L'email resta obbligatoria dove serve per conferma e magic link;
  il telefono partecipa alla deduplica solo se presente.

## Conseguenze

- Positive: nessuna nuova logica di concorrenza (riuso di `spot_allocations`, `pool_usage` e `Move`); numeri senza
  duplicati su più casse; il banco non chiede dati personali.
- Negative: nelle zone `seat` una vendita per zona occupa in mappa una postazione provvisoria (segnata "da
  assegnare") fino all'assegnazione; la migrazione è la 00010, dopo la 00009 di ADR 0019: i database migrati dal ramo
  prima dell'allineamento con `main` vanno ricreati (`make seed`). Le prenotazioni dei biglietti non si toccano dalle
  azioni generiche delle prenotazioni: lo staff passa dai biglietti (Ticket del giorno, mappa).
- Verifiche: invarianti INV-81 (numerazione), INV-82 (capacità della vendita per zona), INV-83 (assegnazione,
  prezzo conservato, prenotazione di proprietà del biglietto), INV-84 (un solo ingresso per biglietto, QR non
  indovinabile), INV-85 (telefono facoltativo); `TestConcurrency_DailyTicketNumbering`,
  `TestConcurrency_ZoneFirstCapacity`, `TestConcurrency_AssignSameSpot`, `TestConcurrency_VoidVsAssign`,
  `TestConcurrency_EntryCheckInOnce` (SQLite e PostgreSQL), `TestDailySaleHTTP_CrossTenantAndIfMatch` ed e2e
  `e2e/tests/daily-sale.spec.ts`.
