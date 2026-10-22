# 0003 — Disponibilità garantita dal vincolo unique sulle allocazioni

- **Stato:** accettato
- **Data:** 2026-10-22
- **Sezioni della specifica:** `docs/spec/07-motore-di-disponibilita.md`, `docs/spec/06-schema-dati.md` (6.6)

## Contesto

Una doppia prenotazione della stessa postazione (stesso ombrellone, stessa data e fascia) è un difetto
bloccante: cassa, pagina pubblica e API vendono in parallelo sulle stesse risorse. Controlli applicativi
del tipo "leggi, verifica, scrivi" sono soggetti a race condition, soprattutto con più istanze, e il
lock pessimistico non è portabile allo stesso modo tra SQLite e PostgreSQL.

## Decisione

- Ogni occupazione di una postazione è una riga in `spot_allocations` per `(spot_id, date, slot)`, con
  vincolo **`unique(spot_id, date, slot)`**; `kind` ∈ `hold`, `booking`, `block`, `season`.
  Una giornata intera occupa `AM` e `PM`.
- La violazione del vincolo è tradotta in `ErrSpotUnavailable` con le date in conflitto.
- I pool a quantità usano `pool_usage` con `unique(pool_id, date, slot)` e un `UPDATE` condizionale
  (`used + held + :qty <= capacità`); meno righe aggiornate del previsto ⇒ rollback.
- Gli hold scaduti vengono cancellati nella stessa transazione prima dell'inserimento, così un job in
  ritardo non blocca mai le vendite.
- È vietato sostituire questi vincoli con controlli applicativi.

## Conseguenze

- Positive: correttezza identica su SQLite e PostgreSQL e con qualsiasi numero di istanze; logica
  applicativa semplice; disponibilità calcolabile con query dirette.
- Negative: volume di righe proporzionale a giorni × fasce (un abbonamento di 120 giorni = 240 righe,
  trascurabile); spostamenti e rilasci richiedono di cancellare e ricreare righe nella stessa transazione.
- Verifiche: `TestConcurrency_*` con almeno 100 tentativi paralleli sulla stessa risorsa su entrambi i
  dialetti (INV-01, INV-02 in `docs/invariants.md`); regola `.claude/rules/availability.md`.
