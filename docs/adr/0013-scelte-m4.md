# 0013 — Scelte di M4: contratto della pagina pubblica, sessione di checkout, cliente provvisorio, link magici, webhook e driver Stripe

- **Stato:** accettato
- **Data:** 2026-10-23
- **Sezioni della specifica:** `docs/spec/10-flusso-di-prenotazione-e-ciclo-di-vita.md` (10.1), `docs/spec/12-pagina-pubblica-e-widget-incorporabile.md`,
  `docs/spec/13-pagamenti.md`, `docs/spec/17-api.md` (17.2), `docs/spec/19-notifiche.md`, `docs/spec/23-privacy-e-gdpr.md`,
  `docs/spec/24-deploy-configurazione-e-operativita.md` (24.4), `docs/spec/28-roadmap-e-criteri-di-accettazione.md` (M4)

## Contesto

M4 porta la pagina pubblica, i pagamenti online, le email transazionali e la documentazione. Sei builder lavorano
in parallelo sullo stesso contratto: i punti che la specifica lascia aperti vanno fissati prima.

## Decisione

1. **Percorsi della spec 17.2.** Il flusso pubblico usa `/api/public/v1/{slug}/bookings[/{id}]` (non un
   percorso `checkout`); la creazione restituisce `checkout_token` (256 bit, salvato come hash in
   `checkout_sessions`) che il client conserva in `sessionStorage` (fallback in memoria) e invia
   nell'header `X-Arenile-Checkout`: nessun cookie di terze parti (12.2). Token errato, di un'altra
   prenotazione o di un altro stabilimento → `404 checkout_not_found`, senza distinzioni. `GET` non conferma mai
   (INV-19). Aggiunte alla 17.2: `qr`, `ics`, `manage/{token}/{qr,ics,pay,modify}`, `manage-links` (richiesta di
   un nuovo link, sempre 202). `POST quote` conta come lettura nei limiti per IP (non scrive nulla).
2. **Webhook come da spec 13.3 e ADR 0006**: `POST /webhooks/payments/{provider}/{establishment_id}`, fuori da
   `/api/` (nessun middleware di sessione, CSRF, JSON o limite pubblico), corpo grezzo fino a 256 KB, firma
   verificata dal driver con le credenziali decifrate dello stabilimento della URL, deduplica nella tabella
   `webhook_events` (unique provider + id evento) nella stessa transazione che applica l'evento.
3. **Cliente provvisorio.** `bookings.customer_id` è obbligatorio ma il cliente scrive i suoi dati solo al passo
   4: la prenotazione `held` punta al cliente segnaposto dello stabilimento (tag `_online_placeholder`, escluso
   da elenchi, ricerche, esportazioni e conservazione) finché `AttachCustomer` non la collega al cliente
   creato per quel checkout (mai un cliente esistente trovato per email o telefono: chi conosce un recapito
   leggerebbe nome, email e telefono del cliente dalla sessione e prenoterebbe a suo nome con i suoi prezzi; i
   doppioni si uniscono dal pannello). Le transizioni del canale pubblico sono metodi del
   servizio prenotazioni con `audit.Actor` (cliente o sistema) invece di un `*auth.Principal`
   (`internal/domain/booking/online.go`).
4. **Link magici in una tabella propria.** `magic_link_tokens` (hash, scadenza = fine prenotazione + 30 giorni,
   revoca) sostituisce `bookings.manage_token_hash` (00003, non più usato): un link rigenerato revoca i
   precedenti e lo storico resta interrogabile. Il link nasce all'invio dell'email di conferma: il token in
   chiaro esiste solo nell'email (e una volta nella risposta di `regenerateMagicLink` per il banco).
5. **Pagamenti del provider separati dal registro.** `payment_intents` traccia sessioni Checkout, payment
   intent e rimborsi del provider (riferimento esterno unico per provider, tentativo, chiave di idempotenza
   `<booking_id>:<tentativo>`); la riga di `payments` nasce solo alla conferma del provider (webhook o
   riconciliazione) tramite `payment.Service.RecordProviderPayment`, che riusa la conferma del driver manuale
   (`BookingConfirmer`, INV-19). Un pagamento fallito lascia la prenotazione in `pending_payment` fino alla
   scadenza dell'hold: la pagina può riprovare.
6. **`pkg/ext` invariato.** La riconciliazione richiede di interrogare un pagamento per riferimento, che
   `ext.PaymentProvider` non prevede: le capacità facoltative `gateway.PaymentStatusChecker` e
   `gateway.CredentialsVerifier` usano solo tipi di `pkg/ext`, quindi un driver esterno le implementa senza
   importare pacchetti interni. Un driver senza di esse compare come `provider_unreachable` nel report.
7. **Driver Stripe senza SDK.** `net/http` con richieste form e `Stripe-Version` fissata, URL base
   configurabile per i test con `httptest.Server`, verifica HMAC della firma con tolleranza di 5 minuti sul
   clock iniettato: nessuna nuova dipendenza Go. Le chiavi (`secret_key`, `webhook_secret`,
   `publishable_key`) sono per stabilimento, cifrate con il key ring (AAD `<establishment_id>:<provider>:<mode>`),
   mai restituite né registrate nei log; un campo segreto assente in un aggiornamento lascia il valore salvato.
8. **Email.** Trasporto dalla configurazione del server (`ARENILE_SMTP_URL`, `ARENILE_MAIL_FROM`); senza SMTP le
   email vanno nel log (solo destinatario mascherato) o, con la nuova `ARENILE_MAIL_DIR` (estensione della
   24.4 come `ARENILE_DEV`), in file `.eml` per sviluppo e test end-to-end. Le notifiche nascono dagli eventi
   dell'outbox tramite un `ext.EventSink` interno (`notify`) che accoda righe di `message_log` con una chiave di
   deduplica per evento; un job le consegna con tentativi. I destinatari sono salvati come hash più testo
   cifrato.
9. **Impostazioni online.** `online_settings` (una riga per stabilimento, versione = ETag) tiene la
   configurazione del flusso e delle email; tema e `allowed_embed_origins` restano su `establishments` e il
   servizio `online` li aggiorna nella stessa transazione. Le condizioni sono versioni immutabili
   (`terms_versions`); la prenotazione salva la versione accettata e l'istante (`terms_accepted_at`).
10. **Documentazione** in `docs-site/`, pacchetto npm a sé (come `e2e/`) con Astro Starlight ufficiale,
    italiano come locale radice e inglese, componenti reali da `web/shared` tramite alias (React deduplicato),
    screenshot con l'installazione Playwright di `e2e/`.

11. **Decisioni dell'integrazione** (consolidate al termine di M4; i pagamenti online sono in
    [ADR 0014](0014-pagamenti-online-m4.md)):
    - **Ritorno dal pagamento nel widget.** Lo storage dell'iframe è partizionato per sito ospitante: al ritorno da
      Stripe sulla pagina `/b/{slug}` a tutta finestra il token del checkout non c'è. Il widget passa quindi a
      `pay` l'indirizzo della pagina ospitante (`return_to`, accettato solo se la sua origine è tra gli
      `allowed_embed_origins`, INV-53); Stripe riporta il cliente lì con `arenile_checkout=<id>` ed `embed.js`
      riapre il widget sull'esito, dove il token è ancora nel `sessionStorage` dell'iframe.
    - **Origini locali.** Oltre a `https://`, `allowed_embed_origins` accetta `http://` solo su host di loopback
      (`localhost`, `*.localhost`, `127.0.0.0/8`, `::1`), contesti sicuri per i browser: si prova il widget su un
      sito in locale.
    - **Calendario e policy.** `max_date` del profilo pubblico tiene conto dell'anticipo massimo della policy
      online (il più permissivo tra default e policy del canale online): la pagina non propone date che il
      checkout rifiuterebbe. I dati dimostrativi aprono l'anticipo a 366 giorni, così la demo si prenota anche
      fuori stagione, e pubblicano condizioni d'esempio (`demo-2027`).
    - **Limiti dell'API pubblica.** Contano come "creazioni" (10/min per IP, spec 17.2) solo l'avvio di una
      prenotazione e la richiesta di un nuovo link; i passi successivi della stessa prenotazione contano come
      letture. I limiti sono configurabili (`ARENILE_PUBLIC_READS_PER_MINUTE`,
      `ARENILE_PUBLIC_CREATIONS_PER_MINUTE`) per le reti condivise.
    - **Doppio di Stripe.** `ARENILE_STRIPE_API_URL` (solo test) punta il driver a un doppio dell'API; il finto
      Stripe di `stripetest` gira anche come processo (`internal/testtools/stripefake`) con la pagina di pagamento
      ospitata e consegna webhook firmati: lo usano l'e2e e l'accettazione sul binario reale.
    - **Peso in rete.** I file testuali delle SPA incorporate sono serviti compressi gzip (calcolati una volta in
      memoria, `Vary: Accept-Encoding`): senza, il JavaScript della prima schermata viaggiava non compresso e
      Lighthouse mobile scendeva a 78.
    - **Orologio del countdown.** Il conto alla rovescia dell'hold usa l'orologio del server (header `Date` delle
      risposte), non quello del telefono.
    - **Storia dimostrativa.** Gli eventi dell'outbox creati da `seed demo` sono segnati come già consegnati: il
      primo `serve` non invia decine di email a clienti fittizi.
    - **Email.** Il job di consegna viene svegliato appena un messaggio entra in coda (`jobs.Job.Wake`), oltre al
      giro periodico di 30 secondi.

## Conseguenze

- Positive: i builder lavorano su interfacce stabili dichiarate dai consumatori (`checkout.*`, `gateway.*`,
  `notify.Settings`) e verificate in `internal/app/m4.go`; nessuna modifica a `pkg/ext`; nessun dato personale o
  segreto in chiaro nelle nuove tabelle.
- Negative: un cliente segnaposto per stabilimento da escludere dalle viste; `bookings.manage_token_hash` resta
  come colonna inutilizzata fino a una migrazione di pulizia; il driver Stripe va mantenuto allineato alla
  versione dell'API fissata.
- Verifiche: `TestMigration00005_ModelsRoundtrip` (`internal/store`, SQLite e PostgreSQL) per vincoli e indici
  unici; i test dei builder (firma dei webhook con segreto di prova, deduplica, riconciliazione,
  `TestConcurrency_OnlineVsDesk` per l'assenza di doppie vendite tra cassa e online).
