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
Per ogni cliente la somma dei movimenti non scaduti di `customer_credits` è `>= 0`, anche con redemption
concorrenti su prenotazioni diverse o vendite libere: ogni redemption blocca la riga del cliente
(`store.LockCustomer`, UPDATE no-op nella transazione del pagamento) prima di leggere il saldo, così due
pagamenti non possono spendere due volte lo stesso credito (lo schema ha solo `CHECK amount <> 0`).
Verificata da: `TestConcurrency_CreditRedemption_NeverNegative` (`internal/domain/payment`, tag `integration`,
100 tentativi, SQLite e PostgreSQL). Stato: implemented.

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

