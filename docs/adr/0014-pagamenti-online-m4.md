# 0014 — Pagamenti online di M4: SDK Stripe ufficiale, webhook, pagamenti tardivi, rimborsi tramite provider e riconciliazione

- **Stato:** accettato (sostituisce il punto 7 dell'ADR 0013, "Driver Stripe senza SDK")
- **Data:** 2026-10-23
- **Sezioni della specifica:** `docs/spec/13-pagamenti.md`, `docs/spec/10-flusso-di-prenotazione-e-ciclo-di-vita.md` (10.2),
  `docs/spec/22-sicurezza.md`, `docs/spec/24-deploy-configurazione-e-operativita.md`, `docs/invariants.md` (INV-18, 19, 22, 23, 24, 26, 29)

## Contesto

L'ADR 0013 fissa il contratto di M4 (tabelle `payment_intents` e `webhook_events`, endpoint dei webhook, capacità
facoltative dei driver). Restavano aperte le scelte che decidono se i soldi sono al sicuro: come parlare con
Stripe, cosa fare quando il pagamento arriva dopo la scadenza dell'hold o due volte, quando un rimborso entra nel
registro, come recuperare i webhook persi.

## Decisione

1. **SDK ufficiale `github.com/stripe/stripe-go/v87` (MIT).** Sostituisce il client `net/http` scritto a mano
   previsto dall'ADR 0013 §7: la verifica della firma (HMAC a tempo costante, più valori `v1` per la rotazione del
   segreto), i tipi degli oggetti e la gestione degli errori sono quelli mantenuti da Stripe, e la versione
   dell'API è fissata dall'SDK (`stripe.APIVersion`). I backend dell'SDK sono costruiti una volta con il client
   HTTP e l'URL base configurabili (i test puntano a un `httptest.Server`, `stripetest`), con log e telemetria
   dell'SDK spenti e un tentativo di rete in più al massimo, sempre con la stessa chiave di idempotenza. La
   tolleranza della firma (5 minuti, in entrambe le direzioni) è controllata sul `clock.Clock` iniettato, non su
   `time.Now()` dell'SDK, così i replay sono testabili.
2. **Checkout Session allineata all'hold.** `mode=payment`, importo in centesimi, valuta, `customer_email`,
   `locale`, metadati `establishment_id`/`booking_id`/`booking_code`/`intent_id` sulla sessione e sul suo
   PaymentIntent, chiave di idempotenza `<booking_id>:<tentativo>`, `expires_at` = fine dell'hold limitata alla
   finestra di Stripe (da 30 minuti a 24 ore, con un minuto di margine). Il gateway passa locale e scadenza come
   *hint* nei metadati della richiesta (`locale`, `expires_at`), che il driver legge e non inoltra. Un nuovo
   tentativo chiude a Stripe la sessione precedente ancora aperta (`PaymentCanceller`); se non si riesce, il
   doppio pagamento è gestito al punto 4. Se la transazione del chiamante è stata annullata dopo la chiamata a
   Stripe, la ripetizione può riusare la chiave con parametri diversi: Stripe la rifiuta (`idempotency_error`) e
   il gateway riprova una sola volta con una chiave nuova.
3. **Il webhook è l'unico percorso di conferma online (INV-19).** In una sola transazione: inserimento in
   `webhook_events` (unique provider + id evento: un duplicato, anche in parallelo, non ha effetti), lock
   dell'intent, riga di `payments` con `provider_ref` = riferimento esterno dell'intent (idempotente), aggiornamento
   di `paid_amount`, conferma tramite il servizio prenotazioni (`BookingConfirmer`, la stessa del driver manuale),
   audit e outbox. Gli stati dell'intent vanno solo avanti: un `expired` tardivo non annulla un successo, un
   successo dopo `failed`/`expired`/`cancelled` viene applicato perché il denaro è arrivato. Un evento per un
   riferimento non ancora noto ma atteso (metadati Arenile, o rimborso recente) riceve 503 e Stripe lo ripete; un
   evento estraneo è registrato come `ignored`. `payment_intent.payment_failed` registra solo il motivo: la
   sessione resta pagabile.
4. **Pagamento tardivo e doppio pagamento.** Il denaro ricevuto entra sempre nel registro (INV-22 rispecchia la
   realtà). Se la prenotazione è `expired`, il gateway chiede alla porta facoltativa `LateConfirmer` (servizio
   prenotazioni) di riallocare le risorse tramite l'inventario, cioè con i vincoli del database, e confermare,
   dentro un savepoint: se riesce la prenotazione è confermata; se manca o fallisce, il pagamento viene
   **rimborsato automaticamente per intero**. Se la prenotazione è `cancelled`/`rejected`/chiusa, rimborso
   automatico. Se un pagamento porta il pagato oltre il totale (due sessioni pagate), si rimborsa l'eccedenza. Ogni
   rimborso automatico crea un intent di rimborso, scrive `payment.refund_scheduled` (payload `PaymentChange`,
   versione 1) da cui `notify` informa il cliente, ed è inviato a Stripe dopo il commit; se Stripe non risponde lo
   reinvia la riconciliazione con la stessa chiave di idempotenza.
5. **Rimborsi tramite provider: registro alla conferma, importo prenotato subito.** Una transazione breve blocca il
   pagamento, verifica il tetto (incassato − rimborsi riusciti − rimborsi in corso, anche per prenotazione: INV-24
   vale con rimborsi in volo) e inserisce l'intent di rimborso; la chiamata a Stripe avviene fuori da ogni lock.
   Se Stripe risponde `succeeded` la riga di rimborso nasce subito; se `pending`, nasce con `charge.refunded`
   (importo cumulativo: si completano gli intent aperti in ordine e un residuo, fatto dal pannello di Stripe, viene
   registrato con riferimento derivato dal cumulativo) o con la riconciliazione; se Stripe rifiuta, l'intent è
   `failed` e l'importo torna disponibile. La riga di registro dei rimborsi Arenile ha `provider_ref` = id
   dell'intent di rimborso, stabile anche prima di conoscere l'id `re_…`. L'API risponde con lo stato attuale
   (`pending` o `succeeded`).
6. **Riconciliazione per intent, non per elenco.** Ogni 15 minuti gli intent aperti da più di 10 minuti vengono
   interrogati uno per uno (`PaymentStatusChecker`: sessione, PaymentIntent o rimborso) e l'esito passa per lo
   stesso codice del webhook con un id evento sintetico per riferimento ed esito: la correzione avviene una volta
   e il webhook vero, se arriva dopo, non cambia nulla. Non elenchiamo le sessioni di Stripe: ogni pagamento che
   interessa ad Arenile ha già un intent, la ricerca puntuale è esatta e limitata e non richiede paginazione. Gli
   intent vengono presi in ordine di ultimo controllo, così uno che fallisce sempre non blocca gli altri. Il report
   mostra `missing_ledger_row`, `amount_mismatch`, `pending_too_long`, `status_mismatch` (verificato in tempo
   reale, massimo 25 interrogazioni) e `provider_unreachable`.
7. **Credenziali.** Restano per stabilimento e modalità, cifrate con il key ring (ADR 0013 §7); il driver le
   riceve decifrate a ogni chiamata, non le conserva, non le scrive nei log; gli errori di Stripe sono ridotti a
   tipo e codice (il messaggio può riportare dati della richiesta). Il webhook senza segreto configurato è
   rifiutato come firma non valida.

## Conseguenze

- Positive: nessuna conferma senza webhook verificato o riconciliazione; nessun doppio effetto con consegne
  duplicate o fuori ordine; nessun rimborso oltre l'incassato anche con 100 richieste parallele; il denaro
  ricevuto è sempre nel registro e quello non dovuto torna al cliente senza intervento del gestore.
- Negative: una dipendenza Go in più (MIT, compatibile con BSL e AGPL) da aggiornare con le versioni dell'API;
  la riallocazione dei pagamenti tardivi richiede che il servizio prenotazioni implementi `LateConfirmer` (finché
  non c'è, ogni pagamento tardivo è rimborsato); la sessione Stripe vive almeno 30 minuti anche se l'hold è più
  breve, quindi i pagamenti tardivi sono possibili per costruzione; i rimborsi fatti dal pannello di Stripe
  vengono registrati solo tramite `charge.refunded`.
- Verifiche: `internal/domain/payment/stripe` (firma valida, errata, rotata, replay oltre tolleranza, mappatura
  degli eventi, client contro `httptest`), `internal/domain/payment/gateway` con tag `integration` su SQLite e
  PostgreSQL (`TestPaymentWebhook_ConfirmsBooking`, `_AtomicRollback`, `_Deduplicated`, `_OutOfOrder`,
  `_LatePayment`, `_DoublePaymentRefundsExcess`, `_CrossTenant`, `TestRefundViaProvider_*`,
  `TestReconcile_FixesMissedWebhook`, `TestConcurrency_WebhookDuplicate`, `TestConcurrency_RefundViaProvider`),
  `internal/domain/payment` (`TestProvider*`), `internal/http/webhooks` (`TestReceivePaymentWebhook_HTTP`).
