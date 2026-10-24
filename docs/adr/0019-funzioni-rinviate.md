# 0019 — Funzioni rinviate: modalità prova, rimborsi garantiti, azioni dalla mappa, casse, importazioni

- **Stato:** accettato e realizzato (A–F)
- **Data:** 2026-10-24
- **Sezioni della specifica:** `docs/spec/11-mappa-editor-e-vista-operativa.md` (11.3), `docs/spec/12-pagina-pubblica-e-widget-incorporabile.md`,
  `docs/spec/13-pagamenti.md`, `docs/spec/14-cassa-e-modulo-fiscale.md` (14.1, 14.2), `docs/spec/15-pannello-del-gestore.md` (15.4),
  `docs/spec/18-eventi-webhook-e-punti-di-estensione.md`, `docs/spec/23-privacy-e-gdpr.md`

## Contesto

L'elenco di funzioni rinviate concordato con il proprietario (A–F) tocca prenotazioni, pagamenti, cassa e integrazioni.
Per far lavorare sei costruttori in parallelo servono prima un contratto API unico, una sola migrazione (00009, entrambi
i dialetti) e le scelte trasversali scritte.

## Decisione

- **A. Modalità prova** (`test_mode_settings`, `bookings.test`, `payments.test`, `refund_requests.test`): il flag è letto
  nella transazione che crea la prenotazione o il pagamento e copiato sulla riga; un pagamento eredita il flag della sua
  prenotazione. Le righe di prova sono escluse da report, KPI, corrispettivi, emissione fiscale ed esportazioni (filtro
  `test = false`, indici parziali sulle righe di prova). Con Stripe si usa la configurazione `mode = test` quando c'è.
  La pagina pubblica accetta prenotazioni in prova solo con il link di anteprima (token firmato, 2 ore, header
  `X-Arenile-Preview`), salvo `public_enabled`. Eliminazione in blocco in un'unica transazione; l'audit resta.
  Realizzazione: servizio `internal/domain/testmode` (interruttore versionato con `If-Match`, link di anteprima,
  `Purge` con anteprima `dry_run`); la purga rilascia prima i contatori dei pool (`inventory.Release`), poi elimina
  prenotazioni, pagamenti, intent, documenti fiscali, richieste di rimborso, messaggi e righe di credito generate
  dalle prove (saldo crediti ricalcolato, mai negativo) e non tocca mai una riga reale. Gli eventi delle righe di
  prova restano sul bus in tempo reale (il pannello le mostra) ma non escono verso i webhook
  (`event.testModeEvent`). Avviso fisso nel pannello (`TestModeBanner`, aggiornato ogni minuto). Test:
  `TestAcceptance_TestMode` (SQLite e PostgreSQL).
- **B. Rimborso garantito**: la cancellazione (link del cliente o pannello) inserisce `refund_requests` nella stessa
  transazione; un job invia al provider con chiave di idempotenza fissa e backoff fino alla conferma del webhook. Indice
  unico `(booking_id, payment_intent_id)` sulle righe non annullate: mai perso, mai doppio. Tentativi esauriti → `failed`
  con notifica al gestore e riprova manuale.
  Realizzazione: `internal/domain/refund` con le porte `booking.CancelRefunds` e `checkout.RefundSender`; la
  cancellazione chiama `Reserve` nella sua transazione (importo limitato a quanto incassato online e non ancora
  rimborsato), il job `refund.send` reclama le richieste con aggiornamento condizionale (un solo invio per volta
  anche con più istanze) e le attese crescono 1 min, 5 min, 30 min, 2 h, 6 h, 12 h fino a
  `ARENILE_REFUND_MAX_ATTEMPTS` (default 10). La chiave di idempotenza verso il provider è derivata dalla richiesta
  e dal ciclo: **Riprova** dopo `failed` apre un nuovo ciclo con una chiave nuova, mentre i ritentativi dello
  stesso ciclo riusano la stessa (il provider non esegue due volte). Il webhook di conferma porta la richiesta a
  `succeeded`. Una seconda cancellazione o un rimborso manuale mentre ce n'è uno automatico in corso è rifiutato
  (409 `refund_in_progress`). Test: `TestRefund_ProviderDownThenUp`, `TestRefund_DeadLetterThenRetry`,
  `TestConcurrency_CancelRefundOnce`, `TestRefund_PanelCancellation`.
- **C. Azioni dalla mappa**: il blocco usa `spot_blocks` (00003) e `POST /spot-blocks`; la prenotazione di gruppo è
  `POST /bookings` con più righe (già atomica, prezzo dal motore, `quote` per l'anteprima): nessun nuovo endpoint.
  `If-Match` diventa obbligatorio (428 `precondition_required`) su check-in, no-show, cancel e move e sull'eliminazione
  della mappa; i client (anche la coda offline) inviano sempre la versione letta.
  Dettagli realizzati: un'anteprima (`dry_run: true` su cancel e move) non scrive nulla e accetta l'assenza di
  `If-Match`; un check-in messo in coda offline dietro un'altra operazione sulla stessa prenotazione (o accodato prima
  di questa regola) invia `If-Match: *`, perché la versione che troverà il server non è nota e la transizione di stato
  (`confirmed → checked_in`, altrimenti 409) lo protegge. Sulla mappa operativa la selezione multipla (pulsante
  "Seleziona" oppure Maiusc/Ctrl-clic su una postazione libera; tocco, intervallo con Maiusc, rettangolo trascinato)
  accetta solo postazioni prenotabili nella fascia mostrata; "Prenotazione di gruppo" apre il foglio di nuova
  prenotazione con tutte le postazioni (una prenotazione, una riga per postazione) e "Blocca" crea un blocco per
  postazione, tutto o niente. Test: `TestGroupBooking_AllOrNothing`, `TestConcurrency_GroupBookingOverlap`.
- **D. Chiusura Z stampabile: HTML pronto per la stampa, nessuna libreria PDF.** `GET /cash-sessions/{id}/z-report`
  restituisce un documento HTML autonomo con stili di stampa (`@page` A4 e scontrino 80 mm), logo Arenile mono (ADR 0018)
  e intestazione dello stabilimento; il PDF è la stampa del browser. Motivo: nessuna nuova dipendenza runtime da
  mantenere (le librerie PDF pure Go mature sono archiviate o poco attive), resa tipografica uguale a quella delle
  ricevute, testo selezionabile e accessibile; il contenuto resta generato lato server (unica fonte dei totali).
  Realizzazione (costruttore D): pacchetto `internal/printout` (funzione pura `RenderZReport`, `html/template`,
  testi in it/en/de/fr nella lingua dell'utente, logo `outlined/arenile-logo-horizontal-navy.svg` incorporato come
  `data:` e verificato identico al kit da un test); la risposta ha una CSP propria (`default-src 'none'`, stili inline,
  immagini `data:`, nessuno script, `frame-ancestors 'self'`). Il pannello stampa il documento da un iframe nascosto
  dello stesso dominio (pulsante "PDF chiusura Z": la finestra di stampa del browser offre "Salva come PDF") e lo
  apre anche in una nuova scheda; la stampa rapida dal pannello porta lo stesso logo mono. I pagamenti in modalità
  prova non entrano nel riepilogo Z (INV-74). Le alternative valutate: le librerie della famiglia fpdf
  (`jung-kurt/gofpdf` archiviata, il fork `go-pdf/fpdf` poco attivo) e `signintech/gopdf` richiedono di incorporare un font TTF per il testo accentato e di impaginare a mano
  tabelle, salti pagina e formato scontrino; il rendering con Chromium headless è una dipendenza esterna che il binario
  unico non può dare per scontata.
- **D. Più casse** (`cash_registers`, `cash_sessions.register_id`): la migrazione crea "Cassa principale" (`main`,
  predefinita) per ogni stabilimento e una cassa per ogni altro codice già usato; una sola sessione aperta per cassa
  (indice unico parziale). Gli stabilimenti nuovi ricevono la cassa predefinita in modo pigro dal servizio cassa.
  Regole del servizio: si apre solo una cassa attiva (`register_id`, altrimenti il codice deprecato `register`,
  altrimenti la predefinita; un codice sconosciuto è un errore di validazione, non crea casse); l'apertura tiene il lock
  della riga della cassa, la modifica che la disattiva aggiorna prima la riga e poi verifica che non ci siano sessioni
  aperte (409 `cash_session_already_open`); la predefinita non si disattiva né si elimina; una cassa con sessioni nello
  storico si può solo disattivare. Il codice si deriva dal nome (minuscole ASCII, `-`, suffisso numerico se occupato) ed
  è immutabile. Le modifiche alle casse scrivono l'audit (`cash_register.*`) ma nessun evento outbox: non sono eventi
  di dominio esposti ai webhook. I corrispettivi con `register_id` contano solo i pagamenti registrati nelle sessioni
  di quella cassa (gli incassi online senza sessione restano solo nel totale).
- **E. Prezzo manuale degli abbonamenti**: colonne `bookings.price_override_*`, permesso `establishment.configure`,
  audit con prima/dopo e motivo; `price_snapshot` resta immutabile (INV-21). `price_current` riporta la riga di
  override e il totale del motore al momento della modifica, così "Torna al listino" ripristina esattamente il
  calcolo precedente; il nuovo totale non può scendere sotto l'incassato (409, prima un rimborso). Il pannello
  mostra la sezione "Prezzo" nella scheda dell'abbonamento; la cassa la vede in sola lettura. Test:
  `TestPriceOverride_AuditedPermissionGatedSnapshotImmutable`, `TestApplyOverride_Scenarios`,
  `TestOverride_RoundTripRestoresEngine`.
- **E. Regola di approvazione con forzatura dal banco** (spec 9 `approval_required`, spec 10.2
  `awaiting_approval`; `internal/domain/booking/approval.go`). Una prenotazione richiede approvazione quando la sua
  policy ha `approval_required` o un suo pacchetto `requires_approval`. Regole: (R1) **la forzatura scavalca solo le
  violazioni di policy** (anticipo, durata, numero di postazioni...), **mai l'approvazione**: una prenotazione
  forzata che richiede approvazione la attende comunque; (R2) una prenotazione che richiede approvazione arriva a
  `confirmed` solo con **Approva**: creazione, conferma manuale di una bozza/blocco/attesa di pagamento e incasso al
  banco (`ConfirmFromPayment`) la portano in `awaiting_approval` con il blocco lungo; (R3) approvare è decisione del
  gestore (`establishment.configure`: titolare e gestore, mai la sola cassa); la conferma manuale di una
  prenotazione `awaiting_approval` è un'approvazione e segue la stessa regola, mentre **Rifiuta** resta con
  `bookings.write` (libera solo risorse, come una cancellazione); (R4) una modifica (PATCH o spostamento, forzati o
  no) di una prenotazione confermata o presente che prima non richiedeva approvazione e ora sì è rifiutata
  (`ErrApprovalRequired`, 409) se chi la fa non può approvare; fatta da chi può approvare è registrata con
  `approved = true`; (R5) le modifiche di prenotazioni non ancora confermate non sono decise qui (l'approvazione è
  ancora davanti, R2). Prima di questa regola il banco poteva confermare con la forzatura una prenotazione che
  richiedeva approvazione. Test: `TestDecideApproval_Rules`, `TestApprovalRule_DeskCannotBypass`.
- **F. Integrazioni**: notifiche nel pannello (`admin_notifications`, chiave di deduplica, parametri senza dati
  personali); storico completo dei tentativi webhook (`webhook_delivery_attempts`, conservazione configurabile);
  esportazione XLSX **senza nuove dipendenze**: invece di `excelize` (ipotizzata nel contratto) il pacchetto
  `internal/xlsx` scrive in streaming il minimo SpreadsheetML (una sola foglia, stringhe inline, numeri, date e
  importi con formato) con `archive/zip` ed `encoding/xml` della libreria standard; `FromCSV` converte
  l'esportazione CSV esistente riga per riga, quindi XLSX e CSV hanno per costruzione le stesse colonne e nessun
  dato personale in più. Motivo: excelize porta con sé diverse dipendenze transitive e funzioni (lettura,
  formule, grafici) che non servono; il formato scritto è piccolo, coperto da test che rileggono il pacchetto
  (`TestWriter_ValidPackage`, `TestFromCSV_RoundTrip`, `TestExportsHTTP_XLSX`);
  importazione CSV (`import_jobs`: carica, anteprima, applica tutto o niente, stesso file applicato una sola volta);
  meta Open Graph della pagina pubblica generate dal server (nessuna API: `static/meta.go` sostituisce i valori
  predefiniti dell'`index.html` della SPA con titolo, descrizione e logo dello stabilimento, solo `https`, testo
  escapato; immagine Arenile di ripiego); le richieste annullate dal client (`context.Canceled`) non sono errori
  500 nei log (`problem.Error` risponde 499 senza log di errore, il log di accesso le registra come annullate).
  Le notifiche del pannello sono lette dal client ogni minuto (nessun nuovo argomento SSE); l'email al titolare
  per l'endpoint disattivato rispetta l'impostazione "errori dei webhook" delle email al gestore. La mappa
  operativa usa tutta la larghezza (e fino a 78vh di altezza) da 1280 px.

## Conseguenze

- La guardia `TestReportQueries_TestBookingsFlagNotYetFiltered` (ADR 0016) è sostituita da
  `TestReportQueries_TestBookingsExcluded`: `store/report.go` filtra già le righe di prova.
- Nuovi tipi di problema: `test_mode_preview_required`, `import_invalid`, `import_already_applied`, `refund_in_progress`.
- `CashSessionOpen.register` resta per compatibilità; i client usano `register_id`.
- Nuove invarianti INV-75 (rimborso garantito), INV-76 (purga di prova), INV-77 (prezzo manuale), INV-78
  (approvazione non scavalcabile), INV-79 (importazione tutto o niente, una volta), INV-80 (If-Match obbligatorio
  sulle azioni della mappa).
- `If-Match` obbligatorio sulle azioni della mappa è un cambiamento per le integrazioni via API (CHANGELOG,
  sezione Changed).
