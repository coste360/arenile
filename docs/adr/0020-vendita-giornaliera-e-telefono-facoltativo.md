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
- I biglietti d'ingresso sono pack su zone `quantity` (`entry_ticket_types`): prezzo dal motore per periodo, capienza
  dal pool condiviso con l'online, ingresso registrato con un aggiornamento condizionale di `tickets.entered_at`.
- Il telefono è facoltativo ovunque (checkout pubblico, nuovo cliente, modifica cliente, import): `require_phone` resta
  nel contratto come deprecato e vale sempre false. L'email resta obbligatoria dove serve per conferma e magic link;
  il telefono partecipa alla deduplica solo se presente.

## Conseguenze

- Positive: nessuna nuova logica di concorrenza (riuso di `spot_allocations`, `pool_usage` e `Move`); numeri senza
  duplicati su più casse; il banco non chiede dati personali.
- Negative: nelle zone `seat` una vendita per zona mostra in mappa una postazione occupata in modo provvisorio fino
  all'assegnazione; la migrazione è la 00010 (la 00009 è riservata a un altro ramo).
- Verifiche: `TestMigration00010_ModelsRoundtrip` e `TestConcurrency_TicketNumber` (`internal/store`, SQLite e
  PostgreSQL), test di servizio dei pacchetti `dailysale` e `ticketing`.
