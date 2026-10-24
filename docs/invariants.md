# Invarianti di dominio

Elenco numerato delle proprietà che devono valere **sempre**, qualunque sia la sequenza di operazioni.
Ogni invariante indica il test che la verifica e lo stato:

- `planned`: test previsto, da scrivere insieme al codice che la riguarda;
- `partial`: una parte è verificata da test esistenti, il resto è indicato tra parentesi;
- `implemented`: test presente e in verde in CI.

Regole:
- Un cambiamento che tocca disponibilità, prezzi, prenotazioni o pagamenti deve lasciare valide tutte le
  invarianti (subagent `invariants-guardian`).
- Una nuova invariante si aggiunge in coda con il numero successivo; i numeri non si riusano.
- Quando il test viene scritto, aggiornare "Verificata da" con il nome esatto e lo stato.
- Un test citato con stato `implemented` o `partial` deve esistere con quel nome esatto
  (`grep -rn "func <Nome>" --include='*_test.go' .`); i nomi dei test `planned` sono quelli da usare.

Convenzioni dei test: unit `TestXxx_Scenario`; integrazione con build tag `integration`
(`go test -tags=integration ./...`, SQLite in memoria sempre, PostgreSQL se `ARENILE_TEST_POSTGRES_URL`);
concorrenza `TestConcurrency_*` con `make test-race`, cioè `go test -race -tags=integration -run ^TestConcurrency ./internal/...`.

## Disponibilità (spec 7)

**INV-01 — Nessuna doppia allocazione di una postazione.**
Per ogni `(spot_id, date, slot)` esiste al più una riga in `spot_allocations`, anche con 100 richieste
parallele sulla stessa postazione, su SQLite e PostgreSQL. È garantita dal vincolo
`unique(spot_id, date, slot)`, non da controlli applicativi.
Verificata da: `TestConcurrency_SpotAllocation`, `TestConcurrency_SpotHold_NoDoubleAllocation`, `TestConcurrency_HoldConversion` (`internal/domain/inventory`), `TestConcurrency_CreateSameSpot`, `TestConcurrency_CreateSameSpot_RealInventory` (`internal/domain/booking`), `TestConcurrency_SeasonReleaseSameDay` (`internal/domain/pass`), tutti con tag `integration` e almeno 100 tentativi paralleli. Stato: implemented.
Verifica di sistema: `TestAcceptance_SimulatedDay` e `TestAcceptance_DemoSeed` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL) controllano questa invariante sul database dopo la giornata simulata di M3.

**INV-02 — Il pool non supera la capacità effettiva.**
Per ogni `(pool_id, date, slot)`: `0 <= used`, `0 <= held` e `used + held <= capacità effettiva`
(capacità del pool corretta da `capacity_overrides`).
Verificata da: `TestConcurrency_PoolConsume_NeverExceedsCapacity`, `TestConcurrency_PoolUsage`, `TestPoolConsume_CapacityOverride` (`internal/domain/inventory`, tag `integration`). Stato: implemented.
Verifica di sistema: `TestAcceptance_SimulatedDay` e `TestAcceptance_DemoSeed` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL) controllano questa invariante sul database dopo la giornata simulata di M3.

**INV-03 — `pool_usage` coincide con il dettaglio.**
Per ogni `(pool_id, date, slot)`: `used` = somma delle `quantity` di `pool_allocations` con `kind=booking`,
`held` = somma di quelle `kind=hold` non scadute (a meno di hold scaduti non ancora ripuliti).
Verificata da: `TestPoolUsage_MatchesAllocations` (`internal/domain/inventory`, property test `rapid`, tag `integration`). Stato: implemented.
Verifica di sistema: `TestAcceptance_SimulatedDay` e `TestAcceptance_DemoSeed` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL) controllano questa invariante sul database dopo la giornata simulata di M3.

**INV-04 — Una postazione è occupata se e solo se esiste un'allocazione valida.**
La disponibilità calcolata per `(spot, date, slot)` è `unavailable` se e solo se esiste una riga in
`spot_allocations` con `kind` ∈ {`booking`, `block`, `season`} oppure `kind = hold` con `expires_at > now`.
Verificata da: `TestAvailabilityQuery_ReflectsAllocations`, `TestOperationsDay_StatesAndTotals` (`internal/domain/inventory`, tag `integration`). Stato: implemented.

**INV-05 — Gli hold scaduti non bloccano mai le vendite.**
Un hold con `expires_at <= now` viene ignorato e ripulito al momento del conflitto, anche se il job di
rilascio non è ancora passato. La pulizia blocca prima la prenotazione proprietaria (lo stesso lock sotto cui
avviene la conferma) e rimuove l'hold per intero, mai una risorsa alla volta: una conferma concorrente trova
tutte le righe dell'hold oppure nessuna e fallisce con `hold_expired` (ADR 0011 §3).
Verificata da: `TestSpotHold_ExpiredHoldIsReclaimed` (con `clock.Fake`), `TestConcurrency_HoldSweeper`, `TestHolds_LifecycleAndSweeper`, `TestInventoryConfirm_ReclaimedHold`, `TestConcurrency_ConfirmVsReclaim` (120 hold confermati a cavallo della scadenza contro vendite concorrenti, `internal/domain/inventory`, tag `integration`). Stato: implemented.
Verifica di sistema: `TestAcceptance_SimulatedDay` e `TestAcceptance_DemoSeed` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL) controllano questa invariante sul database dopo la giornata simulata di M3.

**INV-06 — `expires_at` solo sugli hold.**
`spot_allocations.expires_at` e `pool_allocations.expires_at` sono non nulli se e solo se `kind = hold`;
`bookings.hold_expires_at` è valorizzato solo negli stati `held`, `pending_payment`, `awaiting_approval`.
Verificata da: `TestBookingConfirm_ClearsHoldExpiry` (`internal/domain/booking`), `TestInventory_ConfirmExtendRelease` (`internal/domain/inventory`), tag `integration`. Stato: implemented.
Verifica di sistema: `TestAcceptance_SimulatedDay` e `TestAcceptance_DemoSeed` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL) controllano questa invariante sul database dopo la giornata simulata di M3.

**INV-07 — Giorno intero occupa entrambe le fasce.**
Una riga con `slots = FULL` ha allocazioni sia `AM` sia `PM` per ogni data dell'intervallo
(`starts_on`..`ends_on` inclusivi), escluse le date di chiusura.
Verificata da: `TestSpotHold_FullDayAllocatesBothSlots` (`internal/domain/inventory`, tag `integration`). Stato: implemented.

**INV-08 — Capacità delle zone `zone`.**
Per ogni zona in modalità `zone`, data e fascia, le righe assegnate più quelle `pending` non superano il
numero di postazioni attive della zona.
Verificata da: `TestAutoAssign_NeverExceedsZoneCapacity` (property test `rapid`) e `TestBookingAutoAssign_PendingZoneLines` (`internal/domain/booking`, tag `integration`: solo `booking.Move`, sotto il lock della prenotazione, assegna una riga pending). Stato: partial (le righe `pending` consumano il pool implicito della zona con l'`UPDATE` condizionale; un blocco su una postazione di una zona `zone` non riduce ancora quel pool, quindi una riga pending può restare senza posto assegnabile).

**INV-09 — Quota online rispettata.**
Per le zone con `online_quota`, le vendite dei canali diversi da `desk` (`online` e `api`, ADR 0011 §1) per data e
fascia non superano la quota; solo il canale `desk` la ignora.
Verificata da: `TestAvailability_OnlineQuota` (`internal/domain/availability`), `TestInventory_OnlineQuota`, `TestConcurrency_OnlineQuota` (`internal/domain/inventory`, tag `integration`). Stato: implemented.

**INV-10 — Rilasci degli abbonati coerenti.**
Per ogni `season_release` non revocata, non esiste un'allocazione `season` della stessa riga per quella
data e fascia; `resold_booking_line_id` è valorizzato solo se esiste un'allocazione `booking` di quella riga.
Rilascio e revoca avvengono sotto il lock della prenotazione dell'abbonamento (lo stesso di ogni cambio di stato),
quindi una cancellazione concorrente non lascia mai un'allocazione `season` a un abbonamento cancellato (INV-20).
Verificata da: `TestSeasonRelease_RemovesAndRestoresAllocations` (`internal/domain/inventory` e `internal/domain/pass`), `TestConcurrency_SeasonReleaseSameDay`, `TestPassFlow_RealServices`, `TestConcurrency_RevokeVsCancel` (100 revoche contro cancellazioni con i servizi reali, `internal/domain/pass`), tag `integration`. Stato: implemented.
Verifica di sistema: `TestAcceptance_SimulatedDay` e `TestAcceptance_DemoSeed` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL) controllano questa invariante sul database dopo la giornata simulata di M3.

## Prezzi (spec 8)

**INV-11 — Il motore prezzi è deterministico e puro.**
Lo stesso input produce sempre lo stesso `PriceBreakdown`; il package `internal/domain/pricing` non
importa `store`, `database/sql`, `time.Now` né sorgenti casuali.
Verificata da: `TestQuoteProperty_Deterministic` (property test `rapid`) e `TestPricing_NoForbiddenImports` (import transitivi del modulo limitati a `money` e `calendar`, niente `database/sql`, `net/http`, `os`, `math/rand`, `bun`, né chiamate a `time.Now`/`Since`/`Until`) in `internal/domain/pricing`. Stato: implemented.

**INV-12 — Snapshot riproducibile.**
Ricalcolando gli input salvati della prenotazione (date, fasce, formula, righe ed extra) con la stessa
`engine_version`, il totale coincide con il totale del breakdown corrente (`Booking.CurrentPrice()`:
`price_current` dopo una modifica successiva alla conferma, altrimenti `price_snapshot`, ADR 0011 §2) e con
`bookings.total_amount`.
Verificata da: `TestQuoteProperty_SnapshotRoundTrip` (property test `rapid`: lo snapshot serializzato si rilegge identico e rispetta INV-13), `TestBookingSnapshot_RequoteMatchesTotal` (`internal/domain/booking`, tag `integration`: creazione, modifica dopo la conferma con `price_current`, modifica prima della conferma, ricalcolo dagli input salvati). Stato: implemented.

**INV-13 — Totale non negativo e coerente.**
`total >= 0`; il totale è la somma di righe ed extra, più sconti (negativi) e arrotondamento; ogni
riga è la somma dei giorni. Tutti gli importi sono interi in centesimi.
Verificata da: `TestQuoteProperty_TotalNonNegativeAndConsistent` (property test `rapid`), `TestAllocateProperty_ExactAndBounded`. Stato: implemented.

**INV-14 — Gli sconti non aumentano mai il prezzo.**
Aggiungere una regola `discount`, una convenzione o un codice promozionale non aumenta mai il totale.
Verificata da: `TestQuoteProperty_DiscountNeverIncreasesTotal` (property test `rapid`). Stato: implemented.

**INV-15 — Nessun prezzo, nessuna vendita.**
Se per un giorno non esiste una regola `base` applicabile, la quotazione fallisce con `ErrNoPrice` e non
si crea alcuna prenotazione.
Verificata da: `TestQuote_Errors` (casi `no base rule`, giorno scoperto, extra senza prezzo), `TestQuote_NoPriceErrorDetails`, `TestQuote_HalfDayNoPrice` (`internal/domain/pricing`) e `TestBookingCreate_NoPriceCreatesNothing` (`internal/domain/booking`, tag `integration`: `ErrNoPrice` e nessuna riga in bookings, righe, clienti, allocazioni, audit, outbox). Stato: implemented.

**INV-16 — Scorporo IVA esatto.**
Per ogni aliquota, `taxable + tax` è uguale all'importo IVA inclusa attribuito a quell'aliquota.
Verificata da: `TestVATSplit_SumsToGross`, `TestVATSplit_Table` (`internal/money`) e `TestQuote_VAT` (`internal/domain/pricing`). Stato: implemented.

## Prenotazioni (spec 9, 10)

**INV-17 — Solo transizioni di stato ammesse.**
Lo stato di una prenotazione cambia solo lungo gli archi della spec 10.2; ogni altra transizione
restituisce un errore tipizzato e non modifica nulla.
Verificata da: `TestBookingTransitions_OnlyAllowed` (tabella completa stati × azioni), `TestBookingTransitions_ServiceActions` (ogni azione del servizio contro la macchina a stati, 409 `invalid_transition` senza scritture). Stato: implemented.

**INV-18 — Audit e outbox nella stessa transazione.**
Ogni transizione di stato scrive una riga in `audit_log` e un evento in `outbox_events` nella stessa
transazione: se la transazione fallisce non resta nessuna delle tre scritture.
Verificata da: `TestBookingService_AuditAndOutboxAtomic` (modifica rifiutata prima di ogni scrittura), `TestBookingService_TransitionRollsBackWhole` (cancellazione il cui rimborso fallisce dopo la scrittura dello stato: stato, allocazioni, `paid_amount`, audit e outbox invariati) (`internal/domain/booking`, tag `integration`), `TestPaymentService_AtomicRollback` (`internal/domain/payment`), `TestOutbox_SameTransactionSemantics` (primitiva). Stato: implemented.

**INV-19 — Conferma solo dal pagamento.**
Una prenotazione online passa a `confirmed` solo da un webhook verificato del provider, dalla riconciliazione
(esito letto dall'API del provider e applicato dallo stesso percorso del webhook) o dal driver `manual`;
nessun endpoint pubblico chiamato dal browser può confermarla. Un provider online attivo ma non utilizzabile
(credenziali mancanti) non è il driver `manual`: il pagamento fallisce e la prenotazione resta `held`.
Verificata da: `TestPaymentService_ConfirmsOnDeposit` e `TestBookingRealServices_DeskDay` (conferma dal driver `manual` nella transazione del pagamento), `TestCheckoutFlow_OnlinePaymentNeverConfirmsFromBrowser` (`internal/domain/checkout`: ritorno dal provider e polling non confermano), `TestCheckoutFlow_BrokenOnlineProviderNeverConfirms` (Stripe attivo senza credenziali: nessuna conferma senza pagamento), `TestPaymentWebhook_ConfirmsBooking`, `TestPaymentWebhook_SignatureAndTarget` (`internal/domain/payment/gateway`: solo un webhook con firma valida conferma), `TestReconcile_FixesMissedWebhook` (riconciliazione), tag `integration`. Verifica di sistema: `TestAcceptance_SimulatedDay` (fase online: lo stato pubblico resta non confermato fino al webhook firmato), `e2e/tests/m4-acceptance.spec.ts` (browser reale, iframe su un'altra origine) e `scripts/acceptance/m4-real-binary.sh` (binario reale, webhook contraffatto rifiutato). Stato: implemented.

**INV-20 — Risorse coerenti con lo stato.**
Prenotazioni `draft`, `expired`, `cancelled`, `rejected` non hanno allocazioni; `held`, `pending_payment`,
`awaiting_approval` hanno solo allocazioni `hold`; `confirmed`, `checked_in` hanno solo allocazioni `booking`
(o `season`).
Verificata da: `TestBookingHeld_ExpiresAndReleases`, `TestBookingCancel_RefundTiersAndCredit`, `TestBookingCreate_ConflictLeavesNoRows`, `TestBookingLifecycle_CheckInNoShowMoveNotes`, `TestBookingLifecycle_CheckInOutsidePeriod` (`internal/domain/booking`), `TestConcurrency_ConfirmVsReclaim` (`internal/domain/inventory`: mai una prenotazione confermata senza allocazioni), `TestConcurrency_RevokeVsCancel` (`internal/domain/pass`: mai un'allocazione di un abbonamento cancellato), tag `integration`, e il controllo sul database di `TestAcceptance_SimulatedDay`. Stato: partial (il property test `TestBookingLifecycle_AllocationsMatchStatus` resta planned).
Verifica di sistema: `TestAcceptance_SimulatedDay` e `TestAcceptance_DemoSeed` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL) controllano questa invariante sul database dopo la giornata simulata di M3.

**INV-21 — Snapshot immutabili dopo la conferma.**
`price_snapshot` e `policy_snapshot` non cambiano dopo `confirmed`; una modifica genera un nuovo calcolo, salvato
in `price_current` (ADR 0011 §2), e una differenza da incassare o rimborsare.
Verificata da: `TestBookingModify_KeepsOriginalSnapshot`, `TestBookingSnapshot_RequoteMatchesTotal` (`internal/domain/booking`, tag `integration`). Stato: implemented.

## Pagamenti e cassa (spec 13, 14)

**INV-22 — `paid_amount` coerente con i pagamenti.**
`bookings.paid_amount` = somma dei `payments` con `status = succeeded` e `direction = in` meno la somma di
quelli con `direction = refund` e `status = succeeded`, per la stessa prenotazione.
Verificata da: `TestPayments_PaidAmountMatchesLedger` (`internal/domain/payment`, property test, tag `integration`), `TestConcurrency_PaymentsOnSameBooking`, `TestConcurrency_PaymentIdempotency`. Stato: implemented.
Verifica di sistema: `TestAcceptance_SimulatedDay` e `TestAcceptance_DemoSeed` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL) controllano questa invariante sul database dopo la giornata simulata di M3.

**INV-23 — Webhook elaborati una sola volta.**
Lo stesso evento del provider (stesso ID) produce effetti una sola volta per stabilimento, anche se consegnato
più volte o in parallelo (chiave unica `establishment_id, provider, event_id`: un account Stripe condiviso da due
stabilimenti consegna lo stesso evento a due endpoint, e l'evento di un altro stabilimento viene ignorato senza
essere registrato).
Verificata da: `TestPaymentWebhook_Deduplicated`, `TestPaymentWebhook_OutOfOrder`, `TestPaymentWebhook_CrossTenant` (stesso evento a due stabilimenti), `TestConcurrency_WebhookDuplicate` (100 consegne parallele: un pagamento, una conferma) (`internal/domain/payment/gateway`, tag `integration`); verifica di sistema nella fase online di `TestAcceptance_SimulatedDay` (ogni webhook dei pagati è riconsegnato). Stato: implemented.

**INV-24 — Rimborsi entro il pagato.**
La somma dei rimborsi riusciti di una prenotazione non supera mai la somma dei pagamenti riusciti.
Il tetto conta anche i rimborsi in corso presso il provider (intent `created` o `pending`), sia per i rimborsi
tramite provider sia per quelli manuali e i crediti (`payment.refundInTx`, anteprime di cancellazione).
Verificata da: `TestRefund_NeverExceedsPaid` (`internal/domain/payment`, 100 rimborsi paralleli, tag `integration`), `TestRefundViaProvider_CapsAndIdempotency`, `TestRefund_CreditCancellationCountsRefundsInFlight` (credito con un rimborso automatico in corso) e `TestConcurrency_RefundViaProvider` (`internal/domain/payment/gateway`). Stato: implemented.
Verifica di sistema: `TestAcceptance_SimulatedDay` e `TestAcceptance_DemoSeed` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL) controllano questa invariante sul database dopo la giornata simulata di M3.

**INV-25 — Chiusura di cassa coerente.**
Per una sessione chiusa, `difference = counted_totals - expected_totals` (per metodo e in totale) e
`expected_totals` = fondo cassa + incassi della sessione.
Verificata da: `TestCashSession_CloseComputesDifference`, `TestConcurrency_CloseWhilePaying`, `TestConcurrency_OpenCashSession` (`internal/domain/cash`, tag `integration`), `TestSummarize_Scenarios`. Stato: implemented.
Verifica di sistema: `TestAcceptance_SimulatedDay` e `TestAcceptance_DemoSeed` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL) controllano questa invariante sul database dopo la giornata simulata di M3.

## Multi-stabilimento, sicurezza, privacy (spec 16, 22, 23)

**INV-26 — Isolamento per stabilimento.**
Nessuna query operativa viene eseguita senza `establishment_id` preso dal contesto autenticato; un
utente o una chiave API di uno stabilimento non legge né modifica dati di un altro.
Verificata da: `TestEstablishments_RoundtripAndCrossTenant` (`internal/store`, tag `integration`), `TestService_APIKeysAndCrossTenant` (`internal/domain/auth`, tag `integration`), `TestHTTP_CrossTenantDenied` e `TestHTTPIntegration_CrossTenant` (`internal/http`); per M1/M2 `TestSeasonService_PermissionsAndTenancy`, `TestMappingIntegration_CrossTenantAndPermissions`, `TestMapsHTTP_CrossTenantAndPools`, `TestCatalogIntegration_CrossTenant`, `TestCrossTenant_Isolation` (`internal/domain/pricingconfig`), `TestApply_Permissions` (`internal/domain/setup`), tutti con tag `integration`. Per M3 `TestInventoryService_PermissionsAndTenancy`, `TestBookingService_CrossTenantAndPermissions`, `TestCustomerService_UpdateOptimisticAndPermissions`, `TestPassService_CreateListAndUpdate` (tag `integration`). Stato: implemented (ogni tabella operativa di M3 è letta e scritta solo con `EstablishmentScope`).

**INV-27 — Permessi verificati nel dominio.**
Ogni operazione di amministrazione verifica il permesso del ruolo (spec 16.1) nel servizio di dominio,
indipendentemente dall'handler HTTP.
Verificata da: `TestCan_Matrix`, `TestPrincipal_CanCombinesGrants` (`internal/domain/auth`, tabella ruoli × permessi). Per M1/M2 anche `TestPermissions_Spec16` (`internal/domain/pricingconfig`), `TestCatalogIntegration_Permissions`, `TestTeamService_Permissions`, `TestSeasonService_PermissionsAndTenancy`, `TestMappingIntegration_CrossTenantAndPermissions`, `TestApply_Permissions` (tag `integration`). Per M3 `TestCashService_Permissions`, `TestPaymentService_DeskFlow`, `TestBookingService_CrossTenantAndPermissions`, `TestInventoryService_PermissionsAndTenancy` (tag `integration`: il cassiere prenota, incassa e gestisce la cassa, il viewer legge soltanto). Stato: implemented.

**INV-28 — API pubblica senza dati dei clienti.**
Le risposte di disponibilità, calendario, mappa e catalogo non contengono dati personali; i dati di una
prenotazione si leggono solo con il token di sessione o il manage token.
Verificata da: `TestPublicAPI_NoCustomerData` (`internal/http/public`, tag `integration`: prenotazione dal banco e online con recapiti reali, poi profilo, calendario, mappa, disponibilità, catalogo e preventivo non contengono nome, email, telefono, id cliente né codice; la sessione legge solo il proprio cliente), `TestCheckoutFlow_NeverLinksExistingCustomer` (`internal/domain/checkout`: la pagina pubblica non collega mai una prenotazione a un cliente esistente per email o telefono, quindi non ne rivela i dati). Stato: implemented. Nota: la vista operativa del pannello non trasporta recapiti (la ricerca sulla mappa non cerca per telefono, ADR 0011 §5).

**INV-29 — Segreti mai in chiaro.**
Credenziali dei provider, segreti dei webhook e TOTP sono salvati cifrati (AES-GCM); email, telefoni e
token non compaiono nei log.
Verificata da: `TestKeyRing_TamperAndAAD` (`internal/crypto`), `TestService_TOTPEnrolmentAndLogin` (segreto TOTP cifrato con AAD = id utente), `TestDeliverer_SignedDeliveryRetryAndDisable` e `TestWebhookEndpoint_SecretBoundToRow` (segreto webhook cifrato con AAD = id endpoint). Stato: partial (cifratura implementata; `TestLogging_RedactsPersonalData` planned).

**INV-30 — Audit log solo inserimento.**
`audit_log` non viene mai aggiornato né cancellato dal codice applicativo (l'anonimizzazione GDPR agisce
sui dati del cliente, non sulle righe di audit).
Verificata da: `TestAuditLog_InsertOnly` (`internal/store`, tag `integration`: UPDATE e DELETE rifiutati dal database su SQLite e PostgreSQL). Stato: implemented.

**INV-31 — Idempotenza delle creazioni.**
Due `POST` con la stessa `Idempotency-Key` creano una sola risorsa e restituiscono la stessa risposta.
Verificata da: `TestIdempotency_ReplayMismatchAndRetry`, `TestConcurrency_IdempotencyKey`, `TestConcurrency_IdempotencyKey_ExpiredReclaim` (`internal/http/middleware`), `TestIdempotencyKeys_Claim` e `TestConcurrency_IdempotencyKeyClaim` (`internal/store`, tag `integration`). Stato: implemented.

## Eventi (spec 18)

**INV-32 — Ogni evento dell'outbox viene consegnato almeno una volta.**
Un evento con `dispatched_at` nullo viene ripreso dal dispatcher dopo un riavvio; i consumatori sono
idempotenti sull'`id` dell'evento.
Verificata da: `TestDispatcher_RedeliversAfterRestart`, `TestOutbox_SameTransactionSemantics`, `TestConcurrency_OutboxDispatch`, `TestConcurrency_WebhookDelivery_ClaimedOnce` (`internal/domain/event`, tag `integration`). Stato: implemented.

## Primitive condivise

**INV-33 — Aritmetica monetaria intera con arrotondamento half-even.**
Le operazioni di `internal/money` non usano float e arrotondano al centesimo con half-even.
Verificata da: `TestDivRoundHalfEven_Table`, `TestMulRat_Table`, `TestMulRat_MatchesBigRatReference`, `TestVATSplit_SumsToGross` (`internal/money`). Stato: implemented.

**INV-34 — Date di calendario canoniche.**
`calendar.Date` si serializza sempre come `YYYY-MM-DD` (JSON e SQL) e un `Range` è inclusivo agli estremi.
Verificata da: `TestDate_JSONRoundTrip`, `TestDate_SQLRoundTrip`, `TestRange_Inclusive` (`internal/calendar`). Stato: implemented.

## Configurazione (spec 6, 8, 9, 11, 15)

**INV-35 — Periodi tariffari senza sovrapposizioni.**
I periodi tariffari di una stagione sono contenuti nelle date della stagione e non si sovrappongono, anche
con scritture concorrenti.
Verificata da: `TestConcurrency_RatePeriodsNeverOverlap`, `TestSeasonService_RatePeriods` (`internal/domain/season`, tag `integration`), `TestValidateRatePeriod_Scenarios`. Stato: implemented.

**INV-36 — Sempre almeno un owner attivo.**
L'ultimo owner attivo di un'organizzazione non può essere declassato, disattivato o rimosso.
Verificata da: `TestConcurrency_LastOwnerNeverRemoved`, `TestTeamService_RolesDeactivationAndLastOwner` (`internal/domain/team`, tag `integration`), `TestCheckLastOwner_Scenarios`. Stato: implemented.

**INV-37 — Salvataggio della mappa con versione ottimistica.**
Un salvataggio del documento mappa riesce solo se la versione in `If-Match` è quella corrente; tra
salvataggi concorrenti uno solo vince, gli altri ricevono 412 con `current_version`.
Verificata da: `TestConcurrency_SaveMap`, `TestMappingIntegration_SaveDiffAndVersioning` (`internal/domain/mapping`, tag `integration`), `TestMapsHTTP_DocumentFlow` (`internal/http/admin`, tag `integration`). Stato: implemented.

**INV-38 — Pool implicito allineato alle postazioni.**
Ogni zona in modalità `zone` ha un solo pool di sistema con capacità pari alle sue postazioni attive.
Verificata da: `TestMappingIntegration_ImplicitZonePool` (`internal/domain/mapping`, tag `integration`). Stato: implemented.

**INV-39 — Modifiche di configurazione con audit ed evento atomici.**
Ogni modifica di listino o mappa scrive l'audit log e l'evento `config.pricing_changed` o
`config.map_changed` nella stessa transazione.
Verificata da: `TestPriceRule_VersionSeqAuditOutbox` (`internal/domain/pricingconfig`), `TestMappingIntegration_SaveDiffAndVersioning` (`internal/domain/mapping`), tag `integration`. Stato: partial (verificato per regole di prezzo e mappe; per le altre entità di configurazione il codice segue lo stesso schema senza test dedicato).

**INV-40 — Matrice e simulatore coerenti.**
Ogni cella della matrice di listino ha lo stesso prezzo che il simulatore calcola con gli stessi input.
Verificata da: `TestScenario_SimulateMatrixConsistency` (`internal/domain/pricingconfig`, tag `integration`). Stato: implemented.

**INV-41 — Una zona a quantità non ha mai postazioni.**
Il salvataggio della mappa e la modifica di una zona si serializzano sulle righe delle zone coinvolte
(`store.LockZones`): anche con scritture concorrenti su PostgreSQL, una zona `quantity` non ha postazioni
e il pool implicito di una zona `zone` conta le sue postazioni attive.
Verificata da: `TestConcurrency_ZoneUpdateVsSaveMap` (`internal/domain/mapping`, tag `integration`, SQLite e PostgreSQL). Stato: implemented.

**INV-42 — Nessuna cancellazione silenziosa nella configurazione.**
Una zona referenziata da regole di prezzo, codici promozionali, convenzioni, pack o policy, o con pool
creati a mano, non si elimina né da sola né con la sua mappa: la richiesta fallisce con 409 e l'errore
elenca i riferimenti.
Verificata da: `TestMappingIntegration_ReferencedZoneCannotBeDeleted`, `TestMappingIntegration_DeleteMapRefusesBoundPools` (`internal/domain/mapping`, tag `integration`). Stato: implemented.

**INV-48 — Illustrazioni della mappa solo dal kit, sanificate al build.**
`asset` di una postazione è vuoto o un id prenotabile del kit (`KitBookableAssets`), `asset` di una decorazione
è vuoto o un id del kit, `variant` è ammessa solo se l'illustrazione la prevede (`KitAssetVariants`); le liste
sono generate da `assets/beach-map-kit/manifest.json`, quindi backend e renderer conoscono lo stesso catalogo.
Il frontend inietta solo i corpi `<symbol>` generati, che il generatore ha già filtrato con una allowlist (la
generazione fallisce su script, handler, riferimenti esterni). ADR 0012.
Verificata da: `TestValidateDocument_Cases`, `TestKitAssets_GeneratedListsAreSortedAndDisjoint`,
`TestMappingIntegration_KitAssets` (`internal/domain/mapping`), `web/scripts/gen-map-kit.test.mjs` e `gen-check`
in `make check`. Stato: implemented.

## Operatività del pannello e cassa (spec 14, 21, 28 — M3)

**INV-43 — Giornata simulata coerente.**
Dopo una giornata di almeno 300 operazioni miste dal pannello (prenotazioni, hold e conversioni, scadenze,
modifiche e spostamenti, cancellazioni con rimborso, abbonamenti con rilasci e rivendite, incassi in contanti
e con carta, vendite libere, movimenti, apertura e chiusura di cassa, raffiche concorrenti su una postazione,
su un pool e sulla stessa `Idempotency-Key`) valgono insieme: nessuna postazione allocata due volte per
`(date, slot)`; `pool_usage` uguale al dettaglio ed entro la capacità; ogni prenotazione confermata ha
esattamente le allocazioni delle sue righe (meno i giorni rilasciati), le cancellate nessuna; `paid_amount` =
pagamenti − rimborsi; i totali attesi di ogni sessione di cassa = fondo + incassi ± movimenti e la differenza
di chiusura = contato − atteso; ogni prenotazione ha audit ed eventi, l'outbox è svuotato e senza dati personali.
Verificata da: `TestAcceptance_SimulatedDay` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL;
seme casuale ripetibile con `ARENILE_ACCEPTANCE_SEED`). Stato: implemented (contano solo le mutazioni accettate
con 2xx: 324 con il seme predefinito, più 152 rifiuti attesi; ogni rifiuto deve appartenere all'elenco ammesso per
quel passo dello scenario, un'operazione che risponde 501 o un rifiuto non previsto fanno fallire il test; il
registratore SSE si riconnette con `Last-Event-ID` come il browser, così un client disconnesso perché in ritardo
non perde eventi sotto carico).

**INV-44 — Corrispettivi uguali agli incassi.**
L'export dei corrispettivi di un periodo ha totale uguale alla somma dei pagamenti riusciti meno i rimborsi
riusciti con `business_date` nel periodo (rimborsi come credito inclusi, metodo `credit`); per ogni riga e per
ogni aliquota `taxable + tax = gross` e la somma delle righe è il totale.
Lo scorporo avviene per aliquota e metodo sia nell'export sia nel riepilogo Z della cassa (`cash.Summarize`), così
imponibile e imposta di una giornata con una sola sessione coincidono al centesimo tra i due.
Verificata da: `TestAcceptance_SimulatedDay` (`checkCorrispettivi`), `TestSummarize_MatchesCorrispettivi` (`internal/domain/cash`). Stato: implemented
(`calendar.Date` implementa il `Binder` di oapi-codegen, verificato da `TestDate_BindQueryParameter`).

**INV-45 — Flusso SSE dopo il commit e senza dati personali.**
Gli eventi arrivano al flusso `GET /api/admin/v1/stream` solo dal dispatcher dell'outbox (dopo il commit), solo
allo stabilimento del messaggio e a chi può leggerlo; il nome SSE è il topic e `data` contiene solo id, tipo,
date e postazioni; con `Last-Event-ID` il client riceve una sola volta gli eventi persi (se ancora nella
cronologia), un client che resta indietro viene disconnesso invece di perdere aggiornamenti in silenzio.
Verificata da: `TestServe_TopicsAndNoPersonalData`, `TestServe_ResumeFromLastEventID`,
`TestServe_ResumeUnknownIDStartsLive`, `TestServe_DisconnectsLaggingClient`, `TestReplayBus_Since`,
`TestConcurrency_ReplayBusPublishSince` (`internal/http/sse`), `TestHTTPIntegration_CrossTenant`
(`internal/http`, tag `integration`) e il controllo del flusso in `TestAcceptance_SimulatedDay`. Stato: implemented
(la composizione in `internal/app` avvolge sempre il bus in `sse.NewReplayBus`).

**INV-46 — Dati dimostrativi coerenti e idempotenti.**
`seed demo` crea clienti con contatti fittizi (`@example.com`, `+39000…`), abbonamenti, prenotazioni, rilasci,
incassi e sessioni di cassa solo attraverso i casi d'uso di M3, quindi rispetta INV-43; una seconda esecuzione
non cambia nulla. IVA dimostrativa 22% su stabilimento, pacchetti ed extra.
Verificata da: `TestSeedDemoOperations_VolumesVATAndIdempotency`, `TestSeedDemoOperations_SkipsMissingUseCases`
(`internal/domain/setup`, tag `integration`) e `TestAcceptance_DemoSeed` (`internal/acceptance`). Stato: implemented
(`arenile seed demo` usa lo stesso adattatore `App.DemoOperations`; `TestCLI_Lifecycle` in `cmd/arenile` lo esegue).

**INV-47 — Credito cliente mai negativo.**
Per ogni cliente la somma dei movimenti di `customer_credits` è `>= 0`, anche con redemption concorrenti su
prenotazioni diverse, dal link magico o da vendite libere. Dalla M5B la garanzia è quella di INV-70: ogni
redemption passa dal servizio pagamenti al registro `credit.Service` (via `customer.UseCreditLedger`), che blocca la
riga del cliente e quella di `customer_credit_balances` e applica l'UPDATE condizionale del saldo; il
`CHECK (balance >= 0)` è l'ultima difesa. Verificata da: `TestConcurrency_CreditRedemption_NeverNegative` (100
vendite libere parallele con lo stesso credito sul registro reale: una sola riesce) e
`TestConcurrency_PayWithCredit_RealLedger` (100 pagamenti dal link magico su dieci prenotazioni: esattamente cinque,
saldo = somma del registro = 0), entrambi in `internal/domain/payment`, tag `integration`, SQLite e PostgreSQL.
Stato: implemented.

## Pagina pubblica e pagamenti online (spec 7.5, 7.6, 12, 13, 19 — M4)

**INV-49 — Nessuna doppia vendita tra cassa e online.**
Prenotazioni della cassa e checkout della pagina pubblica sulla stessa postazione e sullo stesso giorno, o sullo
stesso pool di una zona `zone`, non vendono mai più di quanto esiste: decidono i vincoli del database
(`spot_allocations`, `pool_usage`), anche sotto carico parallelo; la quota online di una zona non viene mai
superata e vincola i canali `online` e `api`.
Verificata da: `TestConcurrency_OnlineVsDesk` (120 tentativi alternati, una vendita), `TestConcurrency_ZoneAutoAssign` (`internal/domain/checkout`, tag `integration`, `make test-race`); verifica di sistema: fasi `onlineVsDesk`, `onlineZoneBurst` e `onlineZoneQuota` di `TestAcceptance_SimulatedDay` (`internal/acceptance`, SQLite e PostgreSQL). Stato: implemented.

**INV-50 — Assegnazione automatica alla conferma, mai doppia.**
Una riga di una zona `zone` occupa il pool della zona finché la prenotazione è in hold; alla conferma (webhook,
driver `manual` o approvazione) riceve una postazione libera della zona e due conferme parallele non ricevono
mai la stessa postazione.
Verificata da: `TestCheckoutZone_AutoAssignAtConfirmation`, `TestConcurrency_ZoneAutoAssign` (`internal/domain/checkout`, tag `integration`); verifica di sistema: `onlineZoneQuota` di `TestAcceptance_SimulatedDay` (vincitori confermati in parallelo su postazioni distinte, poi la cassa vende solo quelle rimaste). Stato: implemented.

**INV-51 — Pagamento tardivo rimborsato per intero.**
Un pagamento che il provider conferma dopo la scadenza dell'hold viene registrato e rimborsato per intero
automaticamente (evento `payment.refund_scheduled`): la prenotazione resta `expired`, la postazione libera e
`paid_amount` torna a zero (ADR 0014). Vale anche quando l'hold è scaduto ma il job non è ancora passato (il
gateway scade la prenotazione nella transazione del webhook) e quando un hold scade con denaro sopra (pagamento
sotto la soglia di conferma perché il totale è cambiato): il job di scadenza programma il rimborso nella stessa
transazione e la riconciliazione lo invia. L'importo chiesto al provider è ricalcolato sotto il lock della
prenotazione all'avvio del pagamento.
Verificata da: `TestPaymentWebhook_LatePayment`, `TestPaymentWebhook_DoublePaymentRefundsExcess`, `TestPaymentWebhook_AfterHoldElapsedBeforeSweeper`, `TestPaymentWebhook_BelowThresholdThenExpiry`, `TestConcurrency_WebhookVsHoldExpiry` (100 prenotazioni: webhook e job di scadenza in parallelo, ognuna confermata con le risorse o scaduta e rimborsata) (`internal/domain/payment/gateway`, tag `integration`), `TestCheckoutFlow_PayUsesCurrentTotal` (`internal/domain/checkout`); verifica di sistema: `onlineLatePayments` di `TestAcceptance_SimulatedDay`. Stato: implemented.

**INV-52 — Link magico e token del checkout mai in chiaro.**
Il token del checkout (`X-Arenile-Checkout`) e il token del link magico sono di 256 bit, salvati solo come hash,
mai nei log né negli eventi; un token sconosciuto, scaduto, revocato o di un altro stabilimento risponde 404 senza
distinguere i casi, e "inviami un nuovo link" risponde sempre 202.
Verificata da: `TestNewToken_Format`, `TestHashToken_Stable` (`internal/domain/checkout`), `TestManage_MagicLinkLifecycle`, `TestManage_RequestLinkNoEnumeration` (`internal/domain/checkout`, tag `integration`), `TestNotify_MagicLinks`, `TestNotify_RequestLink` (`internal/domain/notify`, tag `integration`); verifica di sistema: `scripts/acceptance/m4-real-binary.sh` (token assente dal log del server, 404 sui token sconosciuti, 202 sulle richieste). Stato: implemented.

**INV-53 — Ritorno dal widget solo verso i siti autorizzati.**
La pagina di ritorno dal provider indicata dal widget incorporato (`return_to`) è accettata solo se la sua origine
è tra gli `allowed_embed_origins` dello stabilimento (nessun open redirect); `frame-ancestors` della pagina pubblica
elenca solo quelle origini.
Verificata da: `TestEmbedReturnURL_Scenarios` (`internal/domain/checkout`), `TestPublicHTTP_FrameAncestorsAndLimits` (`internal/http/public`, tag `integration`), `TestNormalizeEmbedOrigin_Scenarios` (`internal/domain/establishment`); verifica di sistema: `e2e/tests/m4-acceptance.spec.ts` (il browser blocca l'iframe su un'origine non autorizzata). Stato: implemented.

**INV-54 — Email transazionali una sola volta, senza dati in chiaro nei log.**
Un evento dell'outbox produce al massimo un messaggio per modello (deduplicato anche se riconsegnato); ogni
messaggio è consegnato da un solo worker anche con più istanze; destinatari mascherati nei log e cifrati nel
registro; i dati dimostrativi non generano email.
Verificata da: `TestNotify_OutboxToDelivery`, `TestConcurrency_DeliverClaimsOnce` (`internal/domain/notify`, tag `integration`); verifica di sistema: `scripts/acceptance/m4-real-binary.sh` (nessuna email dopo `seed demo`). Stato: implemented.

## Dispositivi abbinati e app desktop (spec 29 — M6)

**INV-55 — Codice di abbinamento monouso.**
Un codice di abbinamento (spec 29.2) si scambia al più una volta e solo prima di `expires_at`: lo scambio lo
consuma con un UPDATE condizionale (`used_at IS NULL AND expires_at > now`) nella stessa transazione che crea il
dispositivo e scrive l'audit `device.paired`; un codice inesistente, scaduto, già usato o di un utente che nel
frattempo ha perso l'accesso risponde sempre `pairing_code_invalid`. Codici e token sono conservati solo come
SHA-256 e non compaiono mai in log o audit.
Verificata da: `TestConcurrency_PairingCodeSingleUse` (100 scambi paralleli dello stesso codice, un solo token),
`TestDeviceService_CodeSingleUseAndExpiry`, `TestDeviceService_PairAndAuthenticate` (`internal/domain/device`, tag
`integration`, SQLite e PostgreSQL). Stato: implemented.

**INV-56 — Token per dispositivo limitato e revocabile subito.**
Un token `ar_dev_…` autentica come l'utente del dispositivo con i permessi del suo ruolo ristretti allo
stabilimento del dispositivo; non autentica più dalla richiesta successiva alla revoca (nessuna cache), se
l'utente è disattivato o non ha più un ruolo nello stabilimento, né dopo 90 giorni senza uso
(`device.IdleExpiry`, contati da `last_used_at` o dall'abbinamento). Un dispositivo o una chiave API non creano codici
di abbinamento, chiavi API né webhook (`TestAuthorize_RefusesKeysAndDevices` in `internal/domain/apikey` e
`internal/domain/webhook`); con il token non serve il CSRF (niente cookie), con il cookie sì.
Verificata da: `TestDeviceService_RevokeIsImmediate`, `TestDeviceService_PermissionsByRole`, `TestIdleExpired_Scenarios`,
`TestDeviceService_PairAndAuthenticate` (scadenza per inattività),
`TestDeviceService_CrossTenant` (`internal/domain/device`, tag `integration`), `TestAuthenticate_BearerAndCookie`,
`TestAuthenticate_DeviceOnly` (`internal/http/middleware`), `TestHTTPIntegration_DevicePairing` (`internal/http`,
tag `integration`). Stato: implemented.

**INV-57 — Modalità rete locale solo dalla rete locale.**
Nell'edizione desktop il server risponde solo a peer TCP di loopback, RFC 1918, link-local o IPv6 ULA (mai
X-Forwarded-For): il listener della rete locale chiude le altre connessioni prima del TLS
(`desktop.PrivateListener`) e il middleware `LocalPeersOnly` rifiuta con 403 ogni richiesta da un indirizzo pubblico
o CGNAT, anche su un listener aperto senza filtro. Il certificato autogenerato ha un'impronta stabile ai riavvii e ai
cambi di indirizzo. Il server resta uno solo: le garanzie di disponibilità non cambiano.
Verificata da: `TestPeerFilter_Scenarios`, `TestPrivateListener_RejectsPublicPeersBeforeTLS`,
`TestLocalPeersOnly_Middleware`, `TestMode_TLSListener`, `TestCertStore_GenerationAndStableFingerprint`
(`internal/desktop/lan`), `TestM6_ContractRouting` (`internal/app`). Stato: implemented.

**INV-58 — Ponte fiscale: ogni lavoro si chiude una volta sola.**
Al più un ponte fiscale attivo per stabilimento (indice unico parziale). Un lavoro del ponte è accodato una volta per
(stabilimento, tipo, chiave di idempotenza); poll concorrenti non condividono mai un lease; l'esito chiude il lavoro
con un UPDATE condizionale una sola volta (audit `fiscal_bridge_job.*` una volta), un duplicato dello stesso esito
risponde 204 e un esito diverso 409. L'app della cassa memorizza l'esito per chiave di idempotenza prima di inviarlo,
quindi un lavoro riconsegnato (ack perso, lease scaduto, riavvio) non stampa un secondo documento. Dopo 5 consegne
senza esito il lavoro fallisce.
Verificata da: `TestConcurrency_FiscalBridgeAck` (100 invii paralleli dello stesso esito e 100 poll paralleli),
`TestBridge_EndToEnd` (incasso → lavoro → esecuzione sul driver locale → documento emesso, con poll persi, ack persi e
lease scaduto), `TestService_RegisterPollSubmit`, `TestService_RetriesAndLeases` (`internal/fiscalbridge`, tag
`integration`, SQLite e PostgreSQL), `TestAgent_DeduplicatesRedeliveries` (`internal/fiscalbridge`). Stato: implemented.

**INV-59 — Primo avvio dell'app desktop: solo dal PC della cassa, una volta.**
L'utente owner dell'edizione desktop si crea solo con `POST /api/desktop/v1/first-run` da un peer TCP di loopback
con `Host` di loopback (niente X-Forwarded-For, niente DNS rebinding) e con il token monouso a 256 bit che il
launcher genera a ogni avvio finché non esistono utenti e passa al browser nel frammento dell'URL (mai in una
richiesta, mai nei log). I dati sono validati prima di consumare il token; la creazione è condizionata a una tabella
utenti vuota, quindi non nascono mai due owner; dopo il primo owner l'endpoint risponde `setup_completed`. Sul server
l'API desktop non esiste (404).
Verificata da: `TestFirstRun_LoopbackEnforcement`, `TestFirstRun_ServerEditionHidden`,
`TestSmoke_DesktopLauncherHeadless`, `TestSetupToken_SingleUse` (`internal/desktop`), `TestValidateFirstRun_Scenarios`
(`internal/http/desktop`). Stato: implemented.

**INV-60 — Chiave segreta e archivio dell'app desktop non si biforcano mai.**
`ARENILE_SECRET_KEY` dell'app desktop nasce al primo avvio e vive nel portachiavi del sistema (fallback: file 0600
nella cartella dati); se l'archivio esiste ma nessun deposito ha la chiave il launcher si ferma invece di generarne
una nuova (le colonne cifrate resterebbero illeggibili). Una cartella dati ha un solo launcher attivo (lock del
sistema operativo, liberato dal kernel anche dopo un crash): il secondo avvio apre il pannello del primo. Il backup è
lo stesso file di `arenile backup` (VACUUM INTO), ripristinabile con `arenile restore` su una VPS; il ripristino
avviene a server fermo e salva prima l'archivio corrente come backup `pre_restore`; la retention elimina solo i
backup automatici.
Verificata da: `TestResolveSecretKey_NeverForksAnExistingDatabase`, `TestKeyringSecretStore_FallbackToFile`,
`TestInstanceLock_SingleInstance`, `TestBackuper_BackupValidateRestore_SQLite`, `TestPruneCandidates_KeepsManualAndNewest`,
`TestSmoke_DesktopLauncherHeadless` (`internal/desktop`). Stato: implemented.

## Report, integrazioni e pannello offline (spec 15.3, 17, 18.2, 21.2 — M5 parte A)

**INV-61 — Report riconciliati con il registro dei pagamenti e i corrispettivi.**
Il ricavo incassato di un periodo (report ricavi `basis=collected`, riepilogo, confronto) è uguale al totale
dell'export dei corrispettivi dello stesso periodo; il riferimento è l'**export giornaliero** (`group_by=day`): lo
scorporo del report avviene sempre per cella (giorno, aliquota, metodo), quindi imponibile e imposta per aliquota
coincidono al centesimo con quell'export qualunque sia il raggruppamento del report (giorno, settimana, mese, metodo,
aliquota), mentre l'export dei corrispettivi raggruppato per mese scorpora per cella mensile e può differire di
qualche centesimo nell'imponibile e nell'imposta (il lordo coincide sempre). Occupazione = unità (postazione, giorno, fascia) allocate a prenotazioni e
abbonamenti sui mezzi giorni aperti diviso le unità disponibili (postazioni attive meno blocchi, esclusi giorni di
chiusura e fuori stagione); i report sono sempre filtrati per stabilimento e non espongono mai contatti dei clienti.
Verificata da: `TestBuildRevenueCollected_ReconcilesWithCorrispettivi` (property test), `TestReportService_ReconcilesWithLedger`
e `TestReportsHTTP_Flow` (tag `integration`, SQLite e PostgreSQL). Stato: implemented.

**INV-62 — Chiavi API: canale `api`, scope limitati, scadenza.**
Una prenotazione o un hold creati con una chiave API sono sempre del canale `api` (`auth.ChannelFor`), qualunque
canale chieda la richiesta, quindi consumano e rispettano la quota online (ADR 0011 §1). Una chiave non riceve mai
`users.manage` e non gestisce chiavi né webhook (sempre `403`, come un token di dispositivo); vale solo per il suo stabilimento; una chiave scaduta
(`expires_at <= now`) o revocata non autentica; il segreto è salvato solo come SHA-256 e mostrato una volta
(`Cache-Control: no-store` e `middleware.NoReplay`); ogni chiave ha un budget di richieste al minuto.
Verificata da: `TestInventory_APIKeyCannotBypassQuota` (`internal/domain/inventory`), `TestAPIKeyService_Lifecycle`,
`TestAPIKeyService_ExpiredKeyRejected`, `TestAPIKeyService_PermissionsAndTenancy` (`internal/domain/apikey`),
`TestIntegrationsHTTP_APIKeys` (`internal/http/admin`), tutti con tag `integration`, `TestChannelFor_APIKeyAlwaysAPI`,
`TestAPIKeyRateLimit_PerKey`, `TestAuthorize_RefusesKeysAndDevices` (`internal/domain/apikey`, `internal/domain/webhook`).
Stato: implemented.

**INV-63 — Webhook in uscita mai verso la rete interna.**
Le consegne passano dal client di `event.NewEgressClient`: l'host è risolto una volta, ogni indirizzo è controllato
da `event.EgressPolicy` e la connessione va all'IP controllato (nessun DNS rebinding); link-local, metadati cloud,
multicast e intervalli riservati sono sempre bloccati, loopback e reti private salvo
`ARENILE_WEBHOOKS_ALLOW_PRIVATE`; i redirect non sono seguiti, nessun proxy d'ambiente.
Verificata da: `TestEgressPolicy_Check`, `TestEgressDialer_PinsResolvedAddress`,
`TestEgressClient_BlocksLoopbackAndRedirects` (`internal/domain/event`), `TestDeliverer_DefaultClientRefusesLoopback`
(tag `integration`), `TestValidateURL_Scenarios` (`internal/domain/webhook`). Stato: implemented.

**INV-64 — Una consegna webhook alla volta, firmata, con segreti mai in chiaro.**
Ogni tentativo di consegna è preso con un lease (`ClaimWebhookDelivery`, `ClaimWebhookDeliveryNow`), quindi più
istanze e il test sincrono del pannello non inviano mai due volte lo stesso tentativo; il corpo è l'envelope
dell'outbox, firmato `t=<ts>,v1=<HMAC-SHA256>` con il segreto corrente e, nel periodo di grazia di una rotazione,
anche con il precedente. I segreti sono cifrati con il portachiavi (AAD `<id>` e `<id>:previous`), mostrati solo alla
creazione e alla rotazione, mai nel registro attività né nei log; l'URL entra nel registro attività e nell'elenco del
pannello senza query string (`webhook.RedactURL`, `redactWebhookUrl`), dove spesso viaggia un token del ricevitore;
una risposta che mostra un segreto una sola volta (chiave API, segreto del webhook, password iniziale, codice di
abbinamento, link magico) è marcata `middleware.NoReplay` e non viene mai salvata da `Idempotency-Key`
(`TestIdempotency_NeverStoresNoStoreResponses`); il solo `Cache-Control: no-store`, presente su ogni risposta
`/api/`, non spegne la ripetizione (`TestIdempotency_NoStoreHeaderStillReplays`). Scostamento dichiarato (ADR 0016): la disattivazione automatica
dopo 20 errori consecutivi scrive registro attività e log ma non invia ancora l'email al proprietario prevista da
18.2.
Verificata da: `TestConcurrency_WebhookDelivery` (100 worker, SQLite e PostgreSQL),
`TestDeliverer_SignsWithPreviousSecretDuringGrace`, `TestDeliverer_RetryBackoffAndDeadLetter`,
`TestDeliverer_AttemptNowTestEvent` (`internal/domain/event`), `TestWebhookService_Flow` (`internal/domain/webhook`),
tutti con tag `integration`, `TestSignAll_VerifiesWithEitherSecret`, `TestSignature_DocumentedVector` (il vettore
di [`docs/guida/webhook-verifica-firma.md`](guida/webhook-verifica-firma.md)) e `e2e/tests/integrations.spec.ts`
(consegna di prova firmata verificata da un ricevitore locale). Stato: implemented.

**INV-65 — Pannello offline: niente disponibilità senza server, repliche idempotenti, nessun dato personale oltre la 21.2.**
Offline si accodano solo check-in, nota e incasso in contanti su una prenotazione esistente; creazione e spostamento
non partono mai senza connessione. La coda è inviata in ordine, ognuna con la `Idempotency-Key` generata all'accodamento
(la stessa anche per la richiesta online non arrivata al server), quindi una replica non applica mai due volte
un'operazione; 409/412/422 diventano conflitti mostrati all'utente, 401 sospende la coda. Un check-in accodato dietro
un'altra operazione sulla stessa prenotazione non invia `If-Match` (ogni operazione alza la versione; la versione in
cache segue). Il service worker precarica solo shell e risorse statiche e non mette mai in cache `/api/`; sul
dispositivo restano solo le chiavi dell'allowlist (`admin/src/offline/cache.ts`) dello stabilimento corrente, per 48
ore, cancellate all'uscita e al cambio utente; la coda invece sopravvive al cambio utente (incassi davvero riscossi,
nessun dato personale) e la invia la nuova sessione.
Verificata da: `OperationQueue_Flush` e `OperationQueue_RetryDiscard` (`web/shared/src/offline/queue.test.ts`),
`createPersistPolicy_Allowlist` (`web/admin/src/offline/cache.test.ts`), `OfflineRuntime_Queue` e
`OfflineRuntime_Cache` (`web/admin/src/offline/runtime.test.ts`), `offline-ui.test.tsx` e
`e2e/tests/offline.spec.ts` (Chromium reale: avvio offline dalla copia salvata, cancellazione all'uscita) e
`e2e/tests/offline-queue.spec.ts` (check-in offline inviato una sola volta alla riconnessione, versione della
prenotazione +1; mappa e prenotazione di oggi leggibili dopo un ricaricamento offline; incasso in contanti seguito da
check-in sulla stessa prenotazione, entrambi applicati). Stato: implemented.

**INV-66 — Spostare il lato del mare cambia solo la geometria.**
Ribaltare o ruotare una mappa al cambio del lato del mare (`mapping.ReorientLayout`, `POST .../maps/{id}/reorient`)
conserva id, etichette, file e zone di postazioni e decorazioni: allocazioni, prenotazioni e abbonamenti non cambiano.
Ogni fila resta alla stessa distanza dal mare e l'ordine delle file è conservato (la fila A resta la più vicina); il
passaggio inverso ricostruisce la disposizione originale; le passerelle restano sulla griglia di 60 unità. Il
salvataggio è unico e versionato (If-Match, versione +1, audit `map.reoriented` ed evento `config.map_changed` con
`reoriented` nella stessa transazione); con allocazioni future serve la conferma esplicita (`409` altrimenti).
Lati opposti sono uno specchio e lati adiacenti una rotazione, quindi percorsi diversi tra gli stessi due lati danno
disposizioni diverse (scelta documentata in ADR 0016, `TestReorientLayout_PathsDoNotCompose`).
Verificata da: `TestReorientLayout_PropertyRoundTripIsIdentity`, `TestReorientLayout_PropertyDistanceFromTheSea`,
`TestReorientLayout_AllSideCombinations`, `TestReorientLayout_TilesKeepTheTileGrid` (`internal/domain/mapping`),
`TestMappingIntegration_ReorientMap`, `TestMapsHTTP_Reorient`, `TestSeedDemo_UpgradesSeaToTop` (tag `integration`,
SQLite e PostgreSQL) e `e2e/tests/map-sea.spec.ts`. Stato: implemented.

**INV-67 — Passaggio di stagione atomico e idempotente.**
L'applicazione di `POST /seasons/{id}/rollover` crea in una sola transazione la stagione dell'anno indicato (bozza),
periodi tariffari, regole, prezzi fissi, policy e chiusure spostate sull'anno (stesso giorno della settimana, festività
fisse sulla loro data, duplicati e date fuori stagione scartati con un avviso), la riga `season_rollovers`, l'audit
`season.rolled_over` e l'evento `season.rolled_over`: o tutto o niente. L'anteprima esegue le stesse scritture in una
transazione sempre annullata, quindi riporta esattamente ciò che l'applicazione copierebbe senza lasciare tracce. La
chiave unica `(establishment_id, source_season_id, target_year)` e il lock sulla stagione di origine rendono
l'applicazione idempotente anche in concorrenza: una seconda applicazione (o 99 concorrenti) restituisce il risultato
registrato (`already_applied`, HTTP 200); una stagione di quell'anno non creata da questo passaggio è `409
season_rollover_conflict`.
Verificata da: `TestRollover_PreviewApplyIdempotent`, `TestRollover_Refusals`, `TestConcurrency_RolloverApply`
(`internal/domain/rollover`, 100 applicazioni parallele), `TestAcceptance_SeasonCycle` (`internal/acceptance`), tutti
con tag `integration` su SQLite e PostgreSQL, e le tabelle pure `TestTargetDates_Scenarios`,
`TestPlanClosureDays_Scenarios`. Stato: implemented.

**INV-68 — Diritto di prelazione garantito dal database.**
Una proposta di rinnovo aperta (`proposed`/`accepted`) possiede una prenotazione `held` della nuova stagione sulla
stessa postazione, creata dal servizio prenotazioni, con hold fino alla scadenza della prelazione (creata con l'hold
massimo del pannello e prorogata alla scadenza nella stessa transazione): prima della scadenza nessun altro canale può
vendere quella postazione, perché la decide la chiave unica di `spot_allocations`, mai un controllo applicativo.
L'accettazione trasforma le allocazioni `hold` in allocazioni `season` (mai una seconda riga); rifiuto, scadenza,
rilascio forzato e chiusura della campagna liberano la postazione solo tramite il servizio prenotazioni (annullata o
scaduta, `allocation.released`). Al più una proposta aperta per abbonamento di origine (indice unico parziale) e una
campagna per coppia di stagioni. La prelazione segue l'abbonamento di origine: se quello viene annullato dopo la
creazione della campagna, il pannello e il link rifiutano l'accettazione (`ErrSourceCancelled`, 409) e il sink
`renewal` (evento `booking.cancelled`) o il job `renewals.sync` fanno scadere la proposta e liberano la postazione
anche prima della scadenza (`source_pass_cancelled`); il job blocca anche la riga dell'abbonamento di origine.
Verificata da: `TestConcurrency_RenewalAcceptVsPublicSale` (99 vendite parallele contro l'accettazione: nessuna
vendita; 99 vendite contro il rifiuto: al più una), `TestConcurrency_RenewalProposalAccept` (100 accettazioni
parallele da banco e link: una sola conversione), `TestRenewal_RevokedPassReleasesPreemption`,
`TestRenewal_CampaignLifecycle`, `TestRenewal_SkipsAndBulk` (`internal/domain/renewal`) e
`TestAcceptance_SeasonCycle`, con tag `integration` su SQLite e PostgreSQL. Stato: implemented.

**INV-69 — Rinnovo confermato solo dal webhook o dal driver manuale, ogni passaggio tracciato.**
L'accettazione online di un rinnovo dal link magico porta la prenotazione in `pending_payment` e avvia il pagamento
(acconto o saldo) con il gateway di M4; la conferma arriva solo dal webhook del provider (o dal driver manuale:
accettazione dal pannello con `confirm`, rinnovo a costo zero), mai dal ritorno del browser. Il job `renewals.sync`
allinea la proposta alla prenotazione (`converted` se confermata, `expired` se scaduta o annullata, rilascio alla
scadenza). Ogni passaggio di una proposta scrive audit ed evento outbox (`season.renewal_offered`,
`season.renewal_accepted`, `season.renewal_declined`, `season.renewal_expired`) nella stessa transazione del
cambiamento della prenotazione; i payload portano solo id, stati e canale, e il motivo del rifiuto (testo libero) resta
sulla proposta, mai nel registro attività.
Verificata da: `TestRenewal_CampaignLifecycle` (nessuna conversione prima del webhook, conteggi outbox e audit, motivo
assente dall'audit) e `TestAcceptance_SeasonCycle` (webhook Stripe firmato), con tag `integration`, e le tabelle pure
`TestPublicActions_Scenarios`, `TestAmountDue_Scenarios`. Stato: implemented.

**INV-70 — Saldo crediti = somma del registro, mai negativo, nemmeno in concorrenza.**
Ogni movimento di `customer_credits` passa da `credit.Service` (anche quelli di pagamenti e abbonamenti: il servizio
clienti delega `GrantCredit` / `CreditBalance` al registro): nella stessa transazione blocca la riga di
`customer_credit_balances`, materializza le scadenze prima di un addebito, inserisce la riga e aggiorna il saldo con un
UPDATE condizionale (`balance + importo >= 0`), con audit ed evento `credit.issued` / `credit.consumed` /
`credit.expired`. Zero righe aggiornate = `422 credit_insufficient` e nessuna scrittura; il `CHECK (balance >= 0)` del
database è l'ultima difesa. Uno storno di un credito già speso (`pass.UnmarkResold`, fonti `cancellation`,
`season_release`, `weather` negative) è limitato a quanto resta, senza bloccare l'annullamento che lo causa. La
scadenza è materializzata con una riga `expiry` per credito (`reference_credit_id`, unica), consumo FIFO per data di
scadenza, dove il giorno di un addebito è il giorno di calendario dello stabilimento (mai il giorno UTC);
`available` = saldo meno le scadenze non ancora materializzate. Il job orario `credits.expire` è idempotente, percorre
tutti i clienti candidati a pagine (keyset) considerando solo chi ha saldo positivo (un credito interamente usato non
torna a ogni esecuzione) e un cliente che fallisce viene registrato e saltato senza bloccare gli altri. La migrazione
00008 porta a zero con una riga `adjustment` i registri legacy con somma negativa, così `balance = SUM(registro)` vale
dal primo avvio; i crediti legacy già scaduti restano nel saldo finché il job o il primo addebito li materializzano.
Verificata da: `TestConcurrency_CreditSpend` (100 addebiti paralleli su un saldo da 5: esattamente 5 riusciti),
`TestCreditLedger_GrantSpendExpire`, `TestCreditLedger_NeverNegative`, `TestCreditLedger_ReversalCappedAtBalance`,
`TestCreditLedger_PolicyValidity`, `TestCreditLedger_ExpireAllSkipsConsumedGrants` (`internal/domain/credit`, tag
`integration`, SQLite e PostgreSQL), `TestMigration00008_CreditBackfillAndDown` (`internal/store`) e le proprietà
`TestAllocate_BalanceInvariant`, `TestAllocate_ExpiredLotNeverConsumed` (rapid). Stato: implemented.

**INV-71 — Il credito torna credito; nel pubblico solo con identità verificata.**
Il pagamento con metodo `credit` è registrato dal servizio pagamenti (driver manuale, documento fiscale, `paid_amount`,
audit ed evento nella stessa transazione del movimento `redemption`); dalla pagina pubblica è possibile solo dal link
magico della prenotazione (`/manage/{token}/credits`, `/manage/{token}/apply-credit`, `credit_available`) o
dall'accettazione di un rinnovo (`renewal.CreditApplier`), mai dal checkout anonimo. Un rimborso in denaro di una
prenotazione pagata in parte con credito segue una sola regola (`payment.CreditRefundSplit`, "prima il denaro"):
gli altri metodi coprono il rimborso finché bastano (quanto pagato con contanti, carta o online, al netto dei rimborsi
fatti e di quelli in corso presso il provider) e solo l'eccedenza, fino alla parte pagata con credito, torna come
credito (fonte `cancellation`/`weather`, scadenza da `credit_validity_days` della policy della prenotazione), sia
all'annullamento (pannello e link) sia con `POST .../refunds`; un rimborso mirato a un pagamento con credito torna
interamente come credito `cancellation`. Il credito speso su una prenotazione che scade, viene rifiutata o annullata
prima della conferma (hold scaduto, rinnovo rifiutato o scaduto) torna come credito nella stessa transazione, prima
del rimborso online della parte in denaro (INV-51).
Verificata da: `TestRefund_CreditPaidPartGoesBackAsCredit`, `TestPayWithCredit_ManageLinkPartial`,
`TestApplyCreditToBooking_HeldConfirmsAtDeposit` (`internal/domain/payment`),
`TestBookingCancel_CreditPaidPartReturnsAsCredit`, `TestBooking_ExpiredHeldCreditReturnsCredit`
(`internal/domain/booking`), `TestRenewal_DeclinedAfterCreditReturnsCredit` (`internal/domain/renewal`),
`TestPublicHTTP_CreditsAndWaitlist` (`internal/http/public`), tag `integration`, SQLite e PostgreSQL. Stato:
implemented.

**INV-72 — Lista d'attesa: un posto liberato, una sola offerta, protetta dal database.**
Il sink `waitlist` reagisce agli eventi di rilascio (`booking.cancelled`, `booking.expired`, `hold.released`,
`season.day_released`, `allocation.released`) e offre l'inventario libero alle iscrizioni compatibili in ordine
(`priority`, poi data di iscrizione): ogni offerta è un hold (`holds` + allocazioni, chiave unica di
`spot_allocations` e contatori dei pool, quota online del canale dell'iscrizione) creato nella stessa transazione del
passaggio `waiting -> offered` (condizionale sulla versione), della riga `waitlist_offers` (una sola offerta attiva per
iscrizione e per hold, indici unici), dell'audit e dell'evento `waitlist.offered`. Offerte scadute, rifiutate o
revocate rilasciano l'hold e passano il posto all'iscrizione successiva (mai di nuovo alla stessa per quel rilascio).
L'accettazione converte l'hold in una prenotazione `held` del cliente con sessione di checkout di M4: la conferma resta
al webhook o al driver manuale (INV-19). Il link personale è un token da 256 bit salvato come hash e cifrato nella coda
email; l'anonimizzazione del cliente rimuove le sue iscrizioni e cancella note e link.
Verificata da: `TestConcurrency_WaitlistOffer` (100 consegne concorrenti dello stesso rilascio: una sola offerta, un
solo hold), `TestWaitlist_CancellationOffersToFirstEntryAndAcceptGoesToCheckout`,
`TestWaitlist_ExpiredOfferMovesToNextEntry`, `TestWaitlist_PanelOperations`, `TestWaitlist_PublicJoinAndAnonymisation`
(`internal/domain/waitlist`, tag `integration`, SQLite e PostgreSQL). Stato: implemented.

**INV-73 — Una sola sessione aperta per cassa.**
Ogni stabilimento ha una cassa predefinita attiva (indice unico parziale su `cash_registers.is_default`) e al massimo
una sessione aperta per cassa (`cash_sessions_register_open_uidx` su `register_id` con `closed_at IS NULL`, accanto
all'indice di 00003 sul codice). La migrazione 00009 assegna `register_id` a ogni sessione esistente.
Il servizio cassa apre solo casse attive e tiene il lock della riga della cassa durante l'apertura, così disattivazione
e apertura non si sovrappongono; la cassa predefinita creata in modo pigro è unica anche sotto concorrenza.
Verificata da: `TestMigration00009_CashRegisterBackfill` (`internal/store`), `TestConcurrency_OpenSessionPerRegister`,
`TestCashRegisters_TwoOpenSessionsReconcile`, `TestCashRegisters_CRUD` (`internal/domain/cash`), tutti con tag
`integration`, SQLite e PostgreSQL. Stato: implemented.

**INV-74 — Le righe di prova non entrano nei numeri.**
Prenotazioni e pagamenti creati in modalità prova (`bookings.test`, `payments.test`, spec 15.4) sono esclusi da ogni
query dei report; le loro allocazioni contano come libere. Lo stesso vale per corrispettivi, KPI ed esportazioni.
Verificata da: `TestReportQueries_TestBookingsExcluded` (`internal/store`, tag `integration`, SQLite e PostgreSQL).
Anche il riepilogo Z di una sessione di cassa ignora i pagamenti di prova (`TestSummarize_TestPaymentsExcluded`,
`internal/domain/cash`).
Corrispettivi, riepilogo di "Oggi" ed esportazioni (CSV e XLSX) le escludono: `TestAcceptance_TestMode`
(`internal/acceptance`), `TestExportsHTTP_XLSX` (`internal/http/admin`), tag `integration`, SQLite e PostgreSQL. Gli
eventi delle righe di prova non escono verso i webhook. Stato: implemented.

**INV-75 — Rimborso garantito: mai perso, mai doppio.**
La cancellazione di una prenotazione pagata online (link di gestione del cliente o pannello) inserisce la richiesta
di rimborso (`refund_requests`) nella **stessa transazione** della cancellazione: se la transazione va a buon fine il
rimborso esiste, se fallisce non esiste nemmeno la cancellazione. L'importo non supera mai l'incassato online non
ancora rimborsato. Un indice unico su `(booking_id, payment_intent_id)` delle richieste non annullate impedisce due
richieste vive per lo stesso pagamento; l'invio al provider usa una chiave di idempotenza fissa per ciclo e un
reclamo condizionale della riga, quindi anche con più istanze e ritentativi il provider esegue il rimborso una volta.
Finché una richiesta è viva un rimborso manuale o una seconda cancellazione sono rifiutati (409
`refund_in_progress`). Tentativi esauriti → `failed`, notifica al titolare, riprova manuale.
Verificata da: `TestConcurrency_CancelRefundOnce` (100 tentativi paralleli tra cancellazioni e invii: una richiesta, un rimborso),
`TestRefund_ProviderDownThenUp`, `TestRefund_DeadLetterThenRetry`, `TestRefund_ReserveCapsAtProviderPayments`,
`TestRefund_PanelCancellation` (`internal/domain/refund`, tag `integration`, SQLite e PostgreSQL), `TestBackoff_Steps`,
`TestCycle_RetryStartsNewCycle`, `TestIntentKey_DerivedAfterRefusal`. Stato: implemented.

**INV-76 — La purga di prova non tocca mai una riga reale.**
L'eliminazione in blocco delle prenotazioni di prova avviene in un'unica transazione, rilascia prima i contatori dei
pool e cancella solo righe con `test = true` o appartenenti a una prenotazione di prova (pagamenti, intent, documenti,
richieste di rimborso, messaggi, righe di credito); il saldo crediti dei clienti coinvolti è ricalcolato dal registro
e non diventa mai negativo; il registro attività resta.
Verificata da: `TestAcceptance_TestMode` (`internal/acceptance`, tag `integration`, SQLite e PostgreSQL: la
prenotazione reale e il suo pagamento restano, le postazioni di prova tornano libere). Stato: implemented.

**INV-77 — Prezzo manuale degli abbonamenti: solo il gestore, sempre tracciato, snapshot intatto.**
Solo `establishment.configure` (titolare, gestore) imposta o toglie il prezzo manuale di un abbonamento, con motivo
obbligatorio e `If-Match`; ogni modifica scrive audit (prima/dopo, motivo) ed evento `booking.modified` nella stessa
transazione; `price_snapshot` non cambia mai (INV-21) e "Torna al listino" ripristina il totale del motore; il nuovo
totale non scende sotto l'incassato.
Verificata da: `TestPriceOverride_AuditedPermissionGatedSnapshotImmutable` (`internal/domain/pass`, tag
`integration`, SQLite e PostgreSQL), `TestApplyOverride_Scenarios`, `TestOverride_RoundTripRestoresEngine`,
`TestValidateOverride_Table`. Stato: implemented.

**INV-78 — La forzatura dal banco non scavalca l'approvazione.**
Una prenotazione che richiede approvazione (policy `approval_required` o pacchetto `requires_approval`) arriva a
`confirmed` solo con **Approva** da chi ha `establishment.configure`; la forzatura scavalca solo le violazioni di
policy, mai l'approvazione; una modifica che introduce l'obbligo di approvazione è rifiutata a chi non può approvare
(regole R1–R5, ADR 0019 E).
Verificata da: `TestDecideApproval_Rules` (`internal/domain/booking`), `TestApprovalRule_DeskCannotBypass`
(tag `integration`, SQLite e PostgreSQL). Stato: implemented.

**INV-79 — Importazione CSV: tutto o niente, lo stesso file una volta.**
L'applicazione di un'importazione è un'unica transazione: al primo errore non cambia nessun dato e il lavoro passa a
`failed` con il rapporto. Lo stesso file (SHA-256) dello stesso tipo si applica una sola volta (indice unico, 409
`import_already_applied`); le righe già presenti per il criterio scelto sono aggiornate o saltate, mai duplicate.
Gli abbonamenti importati passano dal servizio prenotazioni (allocazioni protette dal database, audit, outbox). Il
consenso marketing non è mai presunto.
Verificata da: `TestImport_SeasonPasses_RollbackAndIdempotence`, `TestImport_Customers_CreateSkipUpdate`,
`TestImport_Customers_ValidationReport`, `TestImport_RetentionAndDiscard` (`internal/domain/importer`, tag
`integration`, SQLite e PostgreSQL), `TestValidateRows_Scenarios`. Stato: implemented.

**INV-80 — Le azioni della mappa richiedono la versione letta.**
Check-in, no-show, annullamento e spostamento di una prenotazione ed eliminazione di una mappa senza `If-Match`
rispondono `428 precondition_required` (con `*` = qualunque versione; le anteprime `dry_run` non scrivono e restano
libere); con una versione superata `412`. I client del pannello, coda offline compresa, inviano sempre la versione.
Verificata da: `TestMapsHTTP_DocumentFlow`, `TestMapsHTTP_IfMatchWildcard`, `TestBookingsHTTP_FlowAndErrors`
(`internal/http/admin`, tag `integration`, SQLite e PostgreSQL), `e2e/tests/map-actions.spec.ts`. Stato: implemented.
