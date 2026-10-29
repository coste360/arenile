# 0029 — Conflitti da riprovare (`concurrent_update`)

- **Stato:** accettato
- **Data:** 2026-10-29
- **Sezioni della specifica:** `docs/spec/21-tempo-reale-e-funzionamento-offline.md` (21.2),
  `docs/spec/29-edizione-desktop-e-app-mobile.md` (29.2)

## Contesto

Su PostgreSQL una transazione può perdere una gara con un'altra sugli stessi dati: deadlock (40P01), errore di
serializzazione (40001), blocco non disponibile (55P03), attesa oltre 5 secondi in `store.LockBookings`.
`store.RunInTx` e `LockBookings` la trasformavano in `store.ErrConflict`, cioè `409 conflict`: lo stesso tipo dei
conflitti definitivi (codice già usato, cassa già aperta, …). Le code offline del pannello e dell'app mobile non
potevano distinguerli, e il middleware di idempotenza conservava anche questi 409: un nuovo tentativo con la stessa
`Idempotency-Key` riceveva per 24 ore lo stesso errore.

## Decisione

1. Nuovo tipo di problema `concurrent_update` (409) con `retry_after` e l'header `Retry-After` (1 secondo). Il
   dominio lo segnala con `store.ErrBusy`, avvolto sempre insieme a `store.ErrConflict`: i controlli esistenti su
   `ErrConflict` non cambiano.
2. Il middleware di idempotenza non conserva le risposte con `Retry-After` (`concurrent_update`, `rate_limited`),
   come già le 5xx: la stessa richiesta con la stessa chiave viene eseguita di nuovo. Ogni altra risposta sotto 500
   resta definitiva per la chiave.
3. Le code offline ritentano `concurrent_update` e `idempotency_in_progress` con la stessa chiave; gli altri 409
   restano conflitti da mostrare all'utente.

## Conseguenze

- I client distinguono un conflitto transitorio da uno definitivo dal solo `type`, come prevede `ProblemType`.
- Rieseguire dopo un `concurrent_update` è sicuro quanto dopo un 5xx: la transazione fallita è stata annullata.
- L'app mobile adotta il tipo quando fissa il contratto della release che lo contiene (ADR 0002 dell'app).
