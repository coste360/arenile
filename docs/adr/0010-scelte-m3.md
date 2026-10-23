# 0010 — Scelte di M3: allocazioni, hold del pannello, abbonamenti, pagamenti e flusso SSE

- **Stato:** accettato
- **Data:** 2026-10-23
- **Sezioni della specifica:** `docs/spec/06-schema-dati.md` (6.5-6.7), `docs/spec/07-motore-di-disponibilita.md`,
  `docs/spec/10-flusso-di-prenotazione-e-ciclo-di-vita.md`, `docs/spec/13-pagamenti.md`, `docs/spec/14-cassa-e-modulo-fiscale.md`,
  `docs/spec/18-eventi-webhook-e-punti-di-estensione.md` (18.1), `docs/spec/21-tempo-reale-e-funzionamento-offline.md` (21.1)

## Contesto

M3 porta prenotazioni dal pannello, disponibilità, abbonamenti, pagamenti manuali e cassa. Lo schema della spec 6 lascia
aperti alcuni punti (chi possiede un hold creato dal pannello prima che esista la prenotazione, come una cancellazione
libera la postazione, dove vivono i rimborsi, i movimenti di cassa, le quote online dei pool) e la spec 21.1 non fissa i
nomi dei messaggi SSE. Otto builder lavorano in parallelo sullo stesso contratto: le scelte vanno fissate prima.

## Decisione

1. **Allocazioni cancellate, vincolo semplice.** `spot_allocations` ha `unique(spot_id, date, slot)` senza indice
   parziale: ogni rilascio (cancellazione, scadenza, no-show dai giorni futuri, spostamento, rilascio di un abbonato,
   rimozione di un blocco) **cancella** le righe nella stessa transazione. Il vincolo è identico su SQLite e PostgreSQL
   e una prenotazione cancellata non blocca mai la postazione. Lo storico resta in `audit_log` e negli eventi.
2. **Proprietario unico di ogni allocazione.** Una riga ha esattamente un proprietario: `booking_line_id` (+
   `booking_id` denormalizzato) per `booking`, `season` e gli hold di una prenotazione `held`; `hold_id` per un hold del
   pannello; `block_id` per un blocco manuale (tabella `spot_blocks`). Vincoli `CHECK` garantiscono la coppia
   `kind`/proprietario e `expires_at` solo per gli hold (INV-06). `pool_allocations` segue la stessa regola.
3. **Hold del pannello.** La tabella `holds` raccoglie una selezione occupata con scadenza prima che esista la
   prenotazione (cliente ancora da inserire). `POST /bookings` con `hold_id` converte le righe dell'hold nella stessa
   transazione; il flusso pubblico di M4 userà invece prenotazioni in stato `held` (spec 10.1), con le stesse primitive
   dell'inventario.
4. **Quote online sui contatori.** `pool_usage.online_used` conta le unità vendute dal canale `online`, aggiornato con
   lo stesso `UPDATE` condizionale contro il limite della quota (spec 7.6, INV-09); `desk` e `api` lo ignorano.
5. **Abbonamenti come prenotazioni.** Come da spec 6.6, un abbonamento è una prenotazione con `is_season_pass = true` e
   formula `season`; l'API `/season-passes` ne è una vista con lo stesso id. I rilasci sono righe di `season_releases`
   per mezza giornata (`revoked_at` per la revoca, indice unico parziale sulle righe attive).
6. **Un solo registro per pagamenti e rimborsi.** `payments.direction` è `in` o `refund` con `amount > 0` (spec 6.7,
   INV-22/24): nessuna tabella `refunds` separata. Ogni pagamento congela lo scorporo IVA (`vat_breakdown`) e la data
   contabile (`business_date`, fuso dello stabilimento) per l'esportazione dei corrispettivi. Il metodo `credit` usa il
   saldo crediti del cliente (`customer_credits`, spec 13.4).
7. **Cassa.** `cash_sessions.register` (default `main`) con indice unico parziale sulle sessioni aperte; i versamenti e
   prelievi sono `cash_movements`.
8. **Eventi aggiuntivi.** Oltre alla spec 18.1: `booking.rejected`, `booking.expired`, `hold.created`,
   `hold.released`, `hold.converted`, `spot_block.created`, `spot_block.deleted`, `season.day_release_revoked`,
   `customer.updated`, `cash_session.opened`, `cash_movement.recorded`, `fiscal_document.recorded`,
   `allocation.assigned`. Payload versione 1 in `internal/domain/event/operations.go`, senza dati personali.
9. **Messaggi SSE per topic.** Sul flusso `GET /api/admin/v1/stream` il campo `event:` è il topic
   (`booking.changed`, `allocation.changed`, `hold.changed`, `cash.changed`, `customer.changed`, `config.changed`,
   `event`), il tipo di dominio resta in `data.type`; `data` contiene solo id, date e postazioni (`event.Topic`).
10. **Porte tra pacchetti.** Ogni pacchetto di dominio dichiara le interfacce che consuma (`booking.Inventory`,
    `booking.Pricer`, `payment.BookingLedger`, `payment.CashDesk`, `pass.Bookings`, ...); gli adattatori senza stato
    `booking.Ledger` e `pass.Resale` evitano i cicli di costruzione; `internal/app/m3.go` è l'unico punto che li collega.
    Le porte transazionali ricevono la transazione del chiamante (unità di lavoro unica: allocazioni, righe, pagamenti,
    audit e outbox insieme, INV-18).

## Conseguenze

- Positive: correttezza della disponibilità nel database su entrambi i dialetti; i builder lavorano in parallelo su
  interfacce stabili; M4 si innesta su hold con scadenza, campo `channel`, registro dei driver di pagamento e
  `BookingLedger`.
- Negative: le allocazioni cancellate non restano interrogabili come righe (lo storico va letto da audit ed eventi);
  i flussi pubblico (prenotazione `held`) e del pannello (`holds`) hanno due proprietari diversi per gli hold.
- Verifiche: `TestMigration00003_ModelsRoundtrip` (`internal/store`, tag `integration`, SQLite e PostgreSQL) per i
  vincoli; INV-01..INV-10 e INV-17..INV-25 in `docs/invariants.md` con i test di concorrenza dei builder.
