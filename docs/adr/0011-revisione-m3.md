# 0011 — Revisione di M3: quota online sul canale `api`, prezzo corrente delle prenotazioni modificate, hold confermati sotto lock, deviazioni dichiarate dalla specifica

- **Stato:** accettato (sostituisce il §4 di [0010](0010-scelte-m3.md))
- **Data:** 2026-10-23
- **Sezioni della specifica:** `docs/spec/07-motore-di-disponibilita.md` (7.2, 7.6), `docs/spec/08-prezzi.md`,
  `docs/spec/10-flusso-di-prenotazione-e-ciclo-di-vita.md` (10.2, 10.3), `docs/spec/11-pannello-admin.md` (11.3),
  `docs/spec/13-pagamenti.md` (13.4), `docs/spec/23-privacy-e-gdpr.md`

## Contesto

La revisione di M3 ha trovato quattro punti in cui il codice, la documentazione e la specifica non dicevano la
stessa cosa:

1. L'ADR 0010 §4 afferma che la quota online è ignorata da `desk` e `api`; il motore puro `availability` e
   `internal/domain/inventory` la applicano invece a ogni canale diverso da `desk` (INV-09 lo registrava come
   "decisione aperta"). La spec 7.6 parla solo del canale `online` e dell'esenzione di `desk`.
2. INV-12 chiede che `bookings.total_amount` coincida con `price_snapshot.total`, ma INV-21 impone che
   `price_snapshot` resti immutabile dopo la conferma: una prenotazione confermata e poi modificata viola per
   costruzione l'una o l'altra. Inoltre i pagamenti scorporavano l'IVA sull'istantanea originale, quindi un extra
   aggiunto con un'altra aliquota finiva nei corrispettivi con il mix di aliquote vecchio.
3. La conferma di un hold (`inventory.Confirm`) e la pulizia degli hold scaduti da parte di una vendita concorrente
   (INV-05) valutano "scaduto" ciascuna al proprio istante. Su PostgreSQL (READ COMMITTED) la pulizia poteva
   cancellare le righe di un hold mentre la conferma le dava per valide: prenotazione confermata senza allocazioni e
   postazione venduta a un altro cliente. Su SQLite il lock di scrittura unico lo impedisce.
4. Alcune scelte del pannello si discostano dalla spec 11.3 senza essere dichiarate: la ricerca sulla mappa non cerca
   per telefono, mancano la selezione multipla per i gruppi, il trascinamento della coda da assegnare e il blocco
   dalla scheda rapida.

## Decisione

1. **La quota online vincola ogni canale diverso da `desk`.** `online` e `api` consumano `pool_usage.online_used` e il
   conteggio delle postazioni vendute online delle zone `seat`; solo il banco la ignora. Un'integrazione via API
   key è un canale esterno come la pagina pubblica, e la spec 7.6 esenta esplicitamente solo `desk`. Il §4 dell'ADR
   0010 è sostituito da questo punto; INV-09 descrive il comportamento effettivo.
2. **Istantanea immutabile più prezzo corrente.** `price_snapshot` resta l'istantanea della creazione e non cambia
   dopo `confirmed` (INV-21). Una modifica successiva alla conferma salva il nuovo `PriceBreakdown` in
   `bookings.price_current` (NULL finché non serve) e `total_amount` è sempre il totale del breakdown corrente
   (`Booking.CurrentPrice()`: `price_current` se presente, altrimenti `price_snapshot`). I pagamenti e i rimborsi
   scorporano l'IVA sul breakdown corrente; l'API espone `price_current` accanto a `price_snapshot`. INV-12 è
   riformulata su questo: ricalcolando gli input salvati (date, fasce, formula, righe ed extra della prenotazione)
   con lo stesso motore si ottiene il totale di `CurrentPrice()`, uguale a `total_amount`. Prima della conferma la
   modifica sostituisce l'istantanea stessa, come già faceva.
3. **Conferma sotto lock delle righe, pulizia per intero.** `Confirm` blocca le righe di hold della prenotazione con
   un `UPDATE` no-op (`store.LockHoldRows`), fallisce con `hold_expired` se non ne trova nessuna o se ne conferma
   meno di quante ne ha bloccate, ed è idempotente su una prenotazione già confermata. La pulizia degli hold
   scaduti (vendita, blocco, ripristino di un giorno di abbonamento) blocca prima le prenotazioni proprietarie in
   ordine di id (lo stesso lock che il servizio prenotazioni prende prima di `Confirm`) e cancella **tutte** le righe
   di hold di quei proprietari, mai una risorsa alla volta: una conferma concorrente trova tutte le righe o nessuna.
   Le funzioni `inventory.AutoAssign` / `AssignPendingInTx`, che scrivevano allocazioni senza il lock della
   prenotazione e non erano usate, sono rimosse: solo `booking.Move` assegna una riga pending.
4. **Check-in e no-show solo nel periodo del soggiorno.** Il check-in è ammesso solo se la data odierna dello
   stabilimento è compresa tra `starts_on` ed `ends_on`, il no-show solo da `starts_on` in poi (prima si cancella, con
   il rimborso della policy): errore tipizzato `422 booking_outside_period` con l'estensione `period`. Le note degli
   operatori (conferma, rifiuto, check-in) non vengono mai copiate nel registro attività (INV-30, spec 23): resta solo
   un flag con la lunghezza. L'anonimizzazione cancella anche `cancel_reason`, le note dei pagamenti e dei crediti.
5. **Deviazioni dichiarate dalla spec 11.3.** La ricerca sulla mappa opera su etichetta, codice e nome del cliente,
   non sul telefono: il payload operativo non trasporta recapiti (INV-28 in anticipo, nessun dato personale oltre il
   nome sulla mappa); la ricerca per telefono passa dalla pagina Clienti. La selezione multipla per i gruppi, il
   trascinamento della coda da assegnare sulla mappa e il blocco dalla scheda rapida sono rinviati (la coda si
   assegna con l'assegnazione automatica e con lo spostamento dalla scheda; i blocchi dall'API `spot-blocks`).

## Conseguenze

- Positive: INV-01 e INV-20 valgono anche su PostgreSQL sotto contesa reale; INV-12 e INV-21 non si contraddicono
  più e i corrispettivi riflettono le aliquote del prezzo corrente; le regole di check-in e no-show sono le stesse
  nel servizio e nel pannello; nessun testo libero finisce nel registro attività.
- Negative: una colonna in più su `bookings` (migrazione 00003, non ancora rilasciata); una conferma può fallire con
  `hold_expired` anche quando il proprio orologio la riteneva valida (comportamento corretto: l'hold è stato
  rivenduto); un hold scaduto viene rimosso per intero alla prima vendita che tocca una sua risorsa, non solo sui
  giorni richiesti (il job di scadenza lo avrebbe fatto comunque).
- Verifiche: `TestConcurrency_ConfirmVsReclaim`, `TestInventoryConfirm_ReclaimedHold` (`internal/domain/inventory`),
  `TestConcurrency_RevokeVsCancel` (`internal/domain/pass`), `TestConcurrency_CreditRedemption_NeverNegative`
  (`internal/domain/payment`), `TestBookingSnapshot_RequoteMatchesTotal`, `TestBookingCreate_NoPriceCreatesNothing`,
  `TestBookingLifecycle_CheckInOutsidePeriod` (`internal/domain/booking`), `TestCustomerService_AnonymizeKeepsAccounting`
  (`internal/domain/customer`), `TestInventory_OnlineQuota` / `TestConcurrency_OnlineQuota` per il canale `api`;
  `docs/invariants.md` INV-05, INV-09, INV-10, INV-12, INV-15, INV-20, INV-21, INV-47.
