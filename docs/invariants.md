# Invarianti di dominio

Elenco numerato delle proprietà che devono valere **sempre**, qualunque sia la sequenza di operazioni.
Ogni invariante indica il test che la verifica e lo stato:

- `planned`: test previsto, da scrivere insieme al codice che la riguarda;
- `implemented`: test presente e in verde in CI.

Regole:
- Un cambiamento che tocca disponibilità, prezzi, prenotazioni o pagamenti deve lasciare valide tutte le
  invarianti (subagent `invariants-guardian`).
- Una nuova invariante si aggiunge in coda con il numero successivo; i numeri non si riusano.
- Quando il test viene scritto, aggiornare "Verificata da" con il nome esatto e lo stato.

Convenzioni dei test: unit `TestXxx_Scenario`; integrazione con build tag `integration`
(`go test -tags=integration ./...`, SQLite in memoria sempre, PostgreSQL se `ARENILE_TEST_POSTGRES_URL`);
concorrenza `TestConcurrency_*` con `make test-race`, cioè `go test -race -tags=integration -run ^TestConcurrency ./internal/...`.

## Disponibilità (spec 7)

**INV-01 — Nessuna doppia allocazione di una postazione.**
Per ogni `(spot_id, date, slot)` esiste al più una riga in `spot_allocations`, anche con 100 richieste
parallele sulla stessa postazione, su SQLite e PostgreSQL. È garantita dal vincolo
`unique(spot_id, date, slot)`, non da controlli applicativi.
Verificata da: `TestConcurrency_SpotHold_NoDoubleAllocation` (`internal/domain/availability`, tag `integration`). Stato: planned.

**INV-02 — Il pool non supera la capacità effettiva.**
Per ogni `(pool_id, date, slot)`: `0 <= used`, `0 <= held` e `used + held <= capacità effettiva`
(capacità del pool corretta da `capacity_overrides`).
Verificata da: `TestConcurrency_PoolConsume_NeverExceedsCapacity`, `TestPoolConsume_CapacityOverride`. Stato: planned.

**INV-03 — `pool_usage` coincide con il dettaglio.**
Per ogni `(pool_id, date, slot)`: `used` = somma delle `quantity` di `pool_allocations` con `kind=booking`,
`held` = somma di quelle `kind=hold` non scadute (a meno di hold scaduti non ancora ripuliti).
Verificata da: `TestPoolUsage_MatchesAllocations` (property test `rapid`, tag `integration`). Stato: planned.

**INV-04 — Una postazione è occupata se e solo se esiste un'allocazione valida.**
La disponibilità calcolata per `(spot, date, slot)` è `unavailable` se e solo se esiste una riga in
`spot_allocations` con `kind` ∈ {`booking`, `block`, `season`} oppure `kind = hold` con `expires_at > now`.
Verificata da: `TestAvailabilityQuery_ReflectsAllocations`. Stato: planned.

**INV-05 — Gli hold scaduti non bloccano mai le vendite.**
Un hold con `expires_at <= now` viene ignorato e ripulito al momento del conflitto, anche se il job di
rilascio non è ancora passato.
Verificata da: `TestSpotHold_ExpiredHoldIsReclaimed` (con `clock.Fake`). Stato: planned.

**INV-06 — `expires_at` solo sugli hold.**
`spot_allocations.expires_at` e `pool_allocations.expires_at` sono non nulli se e solo se `kind = hold`;
`bookings.hold_expires_at` è valorizzato solo negli stati `held`, `pending_payment`, `awaiting_approval`.
Verificata da: `TestBookingConfirm_ClearsHoldExpiry`. Stato: planned.

**INV-07 — Giorno intero occupa entrambe le fasce.**
Una riga con `slots = FULL` ha allocazioni sia `AM` sia `PM` per ogni data dell'intervallo
(`starts_on`..`ends_on` inclusivi), escluse le date di chiusura.
Verificata da: `TestSpotHold_FullDayAllocatesBothSlots`. Stato: planned.

**INV-08 — Capacità delle zone `zone`.**
Per ogni zona in modalità `zone`, data e fascia, le righe assegnate più quelle `pending` non superano il
numero di postazioni attive della zona.
Verificata da: `TestAutoAssign_NeverExceedsZoneCapacity` (property test `rapid`). Stato: implemented (parte pura; il vincolo DB è planned).

**INV-09 — Quota online rispettata.**
Per le zone con `online_quota`, le vendite del canale `online` per data e fascia non superano la quota;
il canale `desk` la ignora.
Verificata da: `TestAvailability_OnlineQuota`, `TestConcurrency_OnlineQuota`. Stato: partial (`TestAvailability_OnlineQuota` implementato; test di concorrenza planned).

**INV-10 — Rilasci degli abbonati coerenti.**
Per ogni `season_release` non revocata, non esiste un'allocazione `season` della stessa riga per quella
data e fascia; `resold_booking_line_id` è valorizzato solo se esiste un'allocazione `booking` di quella riga.
Verificata da: `TestSeasonRelease_RemovesAndRestoresAllocations`. Stato: planned.

## Prezzi (spec 8)

**INV-11 — Il motore prezzi è deterministico e puro.**
Lo stesso input produce sempre lo stesso `PriceBreakdown`; il package `internal/domain/pricing` non
importa `store`, `database/sql`, `time.Now` né sorgenti casuali.
Verificata da: `TestQuote_Deterministic` (property test `rapid`) e controllo degli import `TestPricing_NoForbiddenImports`. Stato: planned.

**INV-12 — Snapshot riproducibile.**
Ricalcolando gli input salvati con la stessa `engine_version`, il totale coincide con `price_snapshot.total`
e con `bookings.total_amount`.
Verificata da: `TestQuote_SnapshotReproducible` (property test `rapid`). Stato: planned.

**INV-13 — Totale non negativo e coerente.**
`total >= 0`; il totale è la somma di righe ed extra, più sconti (negativi) e arrotondamento; ogni
riga è la somma dei giorni. Tutti gli importi sono interi in centesimi.
Verificata da: `TestQuote_TotalsAddUp`, `TestQuote_TotalNonNegative` (property test `rapid`). Stato: planned.

**INV-14 — Gli sconti non aumentano mai il prezzo.**
Aggiungere una regola `discount`, una convenzione o un codice promozionale non aumenta mai il totale.
Verificata da: `TestQuote_DiscountMonotonic` (property test `rapid`). Stato: planned.

**INV-15 — Nessun prezzo, nessuna vendita.**
Se per un giorno non esiste una regola `base` applicabile, la quotazione fallisce con `ErrNoPrice` e non
si crea alcuna prenotazione.
Verificata da: `TestQuote_NoBaseRule_ErrNoPrice`. Stato: planned.

**INV-16 — Scorporo IVA esatto.**
Per ogni aliquota, `taxable + tax` è uguale all'importo IVA inclusa attribuito a quell'aliquota.
Verificata da: `TestVATSplit_SumsToGross` (`internal/money`) e `TestQuote_VATBreakdown`. Stato: planned.

## Prenotazioni (spec 9, 10)

**INV-17 — Solo transizioni di stato ammesse.**
Lo stato di una prenotazione cambia solo lungo gli archi della spec 10.2; ogni altra transizione
restituisce un errore tipizzato e non modifica nulla.
Verificata da: `TestBookingTransitions_OnlyAllowed` (tabella completa stati × azioni). Stato: partial (tabella implementata; verifica nel servizio planned).

**INV-18 — Audit e outbox nella stessa transazione.**
Ogni transizione di stato scrive una riga in `audit_log` e un evento in `outbox_events` nella stessa
transazione: se la transazione fallisce non resta nessuna delle tre scritture.
Verificata da: `TestBookingService_AuditAndOutboxAtomic` (tag `integration`). Stato: planned.

**INV-19 — Conferma solo dal pagamento.**
Una prenotazione online passa a `confirmed` solo da un webhook verificato del provider o dal driver
`manual`; nessun endpoint pubblico chiamato dal browser può confermarla.
Verificata da: `TestPublicBooking_ReturnURLDoesNotConfirm`, `TestPaymentWebhook_ConfirmsBooking`. Stato: planned.

**INV-20 — Risorse coerenti con lo stato.**
Prenotazioni `draft`, `expired`, `cancelled`, `rejected` non hanno allocazioni; `held`, `pending_payment`,
`awaiting_approval` hanno solo allocazioni `hold`; `confirmed`, `checked_in` hanno solo allocazioni `booking`
(o `season`).
Verificata da: `TestBookingLifecycle_AllocationsMatchStatus` (property test `rapid`, tag `integration`). Stato: planned.

**INV-21 — Snapshot immutabili dopo la conferma.**
`price_snapshot` e `policy_snapshot` non cambiano dopo `confirmed`; una modifica genera un nuovo calcolo e
una differenza da incassare o rimborsare.
Verificata da: `TestBookingModify_KeepsOriginalSnapshot`. Stato: planned.

## Pagamenti e cassa (spec 13, 14)

**INV-22 — `paid_amount` coerente con i pagamenti.**
`bookings.paid_amount` = somma dei `payments` con `status = succeeded` e `direction = in` meno la somma di
quelli con `direction = refund` e `status = succeeded`, per la stessa prenotazione.
Verificata da: `TestPayments_PaidAmountMatchesLedger` (property test, tag `integration`). Stato: planned.

**INV-23 — Webhook elaborati una sola volta.**
Lo stesso evento del provider (stesso ID) produce effetti una sola volta, anche se consegnato più volte
o in parallelo.
Verificata da: `TestPaymentWebhook_Deduplicated`, `TestConcurrency_PaymentWebhook_Duplicate`. Stato: planned.

**INV-24 — Rimborsi entro il pagato.**
La somma dei rimborsi riusciti di una prenotazione non supera mai la somma dei pagamenti riusciti.
Verificata da: `TestRefund_NeverExceedsPaid`. Stato: planned.

**INV-25 — Chiusura di cassa coerente.**
Per una sessione chiusa, `difference = counted_totals - expected_totals` (per metodo e in totale) e
`expected_totals` = fondo cassa + incassi della sessione.
Verificata da: `TestCashSession_CloseComputesDifference`. Stato: planned.

## Multi-stabilimento, sicurezza, privacy (spec 16, 22, 23)

**INV-26 — Isolamento per stabilimento.**
Nessuna query operativa viene eseguita senza `establishment_id` preso dal contesto autenticato; un
utente o una chiave API di uno stabilimento non legge né modifica dati di un altro.
Verificata da: `TestStore_CrossEstablishmentAccessDenied` (tag `integration`), `TestAdminAPI_CrossTenant`. Stato: planned.

**INV-27 — Permessi verificati nel dominio.**
Ogni operazione di amministrazione verifica il permesso del ruolo (spec 16.1) nel servizio di dominio,
indipendentemente dall'handler HTTP.
Verificata da: `TestPermissions_Matrix` (tabella ruoli × operazioni). Stato: planned.

**INV-28 — API pubblica senza dati dei clienti.**
Le risposte di disponibilità, calendario, mappa e catalogo non contengono dati personali; i dati di una
prenotazione si leggono solo con il token di sessione o il manage token.
Verificata da: `TestPublicAPI_NoCustomerData`. Stato: planned.

**INV-29 — Segreti mai in chiaro.**
Credenziali dei provider, segreti dei webhook e TOTP sono salvati cifrati (AES-GCM); email, telefoni e
token non compaiono nei log.
Verificata da: `TestSecrets_EncryptedAtRest`, `TestLogging_RedactsPersonalData`. Stato: planned.

**INV-30 — Audit log solo inserimento.**
`audit_log` non viene mai aggiornato né cancellato dal codice applicativo (l'anonimizzazione GDPR agisce
sui dati del cliente, non sulle righe di audit).
Verificata da: `TestStore_AuditLogAppendOnly`. Stato: planned.

**INV-31 — Idempotenza delle creazioni.**
Due `POST` con la stessa `Idempotency-Key` creano una sola risorsa e restituiscono la stessa risposta.
Verificata da: `TestIdempotencyKey_SingleResource`, `TestConcurrency_IdempotencyKey`. Stato: planned.

## Eventi (spec 18)

**INV-32 — Ogni evento dell'outbox viene consegnato almeno una volta.**
Un evento con `dispatched_at` nullo viene ripreso dal dispatcher dopo un riavvio; i consumatori sono
idempotenti sull'`id` dell'evento.
Verificata da: `TestDispatcher_RedeliversAfterRestart`. Stato: planned.

## Primitive condivise

**INV-33 — Aritmetica monetaria intera con arrotondamento half-even.**
Le operazioni di `internal/money` non usano float e arrotondano al centesimo con half-even.
Verificata da: `TestDivRoundHalfEven_Table`, `TestMulRat_Table`, `TestVATSplit_SumsToGross` (`internal/money`). Stato: planned.

**INV-34 — Date di calendario canoniche.**
`calendar.Date` si serializza sempre come `YYYY-MM-DD` (JSON e SQL) e un `Range` è inclusivo agli estremi.
Verificata da: `TestDate_JSONRoundTrip`, `TestDate_SQLRoundTrip`, `TestRange_Inclusive` (`internal/calendar`). Stato: planned.
