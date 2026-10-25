# Changelog

Tutte le modifiche rilevanti di Arenile sono documentate in questo file.

Il formato segue [Keep a Changelog](https://keepachangelog.com/it-IT/1.1.0/) e il progetto adotta il [Versionamento Semantico](https://semver.org/lang/it/). I rilasci sono generati a partire dai messaggi [Conventional Commits](https://www.conventionalcommits.org/it/v1.0.0/). Ogni versione minore include, quando necessario, una sezione **Note di aggiornamento**.

## [Unreleased]

### Modificato
- `arenile seed demo`: la stagione dimostrativa è sempre quella dell'anno in corso ed è prenotabile tutti i giorni
  (dall'1 gennaio al 31 dicembre; fuori da maggio-settembre vale la bassa stagione), così vendita rapida, cassa e
  prenotazioni si provano il giorno stesso. Negli ultimi due mesi dell'anno la stagione si sposta in avanti e copre
  sempre i 60 giorni successivi. Prima, dopo il 30 settembre, creava solo la stagione dell'anno dopo. La stagione
  successiva non viene creata: si prova con **Passaggio di stagione**.

### Corretto
- Mappe: **Crea e apri l'editor** apriva l'editor con "Risorsa non trovata" (si apriva un indirizzo senza l'id della
  mappa); la mappa era creata e compariva solo tornando all'elenco. Ora l'editor si apre subito sulla nuova mappa.
- Editor della mappa: nel **Generatore di file** un nome di zona lungo copriva il campo **Tipo** e il campo
  **Illustrazione** usciva dalla finestra. Ora i valori lunghi sono troncati con i puntini.
- Calendari delle date: cambiando mese il calendario cambiava altezza e si spostava sopra o sotto il campo. Ora
  mostra sempre sei settimane e resta fermo.
- Scelta di un periodo (stagioni, periodi tariffari, chiusure, prenotazioni, report e gli altri campi con due
  date): con un periodo già impostato il primo clic spostava la data finale. Ora il primo clic sceglie sempre
  l'inizio e il secondo la fine; in alto ci sono i campi **Dal** e **Al** per scrivere le date a mano
  (gg/mm/aaaa).
- Prezzo mancante (vendita rapida, prenotazioni, simulatore): il messaggio mostrava il testo tecnico interno
  ("pricingconfig: simulate: pricing: no price for pack…"). Ora dice in italiano cosa manca, per esempio
  "Nessun prezzo per il pack "base" nella categoria di prezzo "row5" il 25/10/2026, giornata intera: aggiungi una
  regola base o un prezzo fisso in Prezzi."

## [1.0.3] - 2026-10-25

### Aggiunto
- Pagina **Informazioni su Arenile** (`/admin/informazioni`, dal menu dell'utente, dall'avviso di nuova versione, dal
  menu dell'icona e dal menu **Arenile** dell'app desktop): versione in uso, edizione, stato del controllo degli
  aggiornamenti con l'ora dell'ultimo controllo, ultima versione con le **Novità**, **Verifica aggiornamenti** e
  l'azione per aggiornare: **Scarica Arenile X per &lt;sistema&gt;** (il pacchetto giusto, il download si apre nel
  browser) con **Tutte le versioni e i sistemi**, oppure **Come aggiornare** con Docker. Le azioni sono per titolari
  e gestori.
- **Verifica aggiornamenti**: nuovo `POST /api/admin/v1/system/update/check` (titolari e gestori) che controlla subito
  e restituisce lo stato aggiornato; il server esegue al massimo un controllo al minuto e nessuna chiamata di rete con
  `ARENILE_UPDATE_CHECK=false`. `SystemUpdate` ha i nuovi campi `pending`, `last_attempt_at`, `os` e `arch`.
- App desktop: **Verifica aggiornamenti** e **Informazioni su Arenile** nel menu dell'icona (e nel menu **Arenile** su
  macOS); il risultato della verifica arriva come notifica ("Arenile è aggiornato", "È disponibile la versione X",
  "Impossibile cercare automaticamente gli aggiornamenti.").

### Corretto
- App desktop: **Apri la pagina pubblica** (e ogni altro link che esce dal pannello, anche sullo stesso indirizzo come
  `/b/<slug>`) apriva la pagina dentro la finestra dell'app, senza modo di tornare al pannello. Ora la finestra mostra
  solo il pannello: link (anche con il clic centrale), `window.open` e navigazioni avviate dalla pagina verso altri
  siti o fuori da `/admin` si aprono nel browser predefinito, in modalità locale e remota, su macOS e Windows (su
  macOS anche i reindirizzamenti del server; il pannello non ne fa verso l'esterno). I download continuano a salvarsi
  nella cartella Download; un documento del server aperto in una nuova finestra viene salvato lì invece di prendere
  il posto del pannello, e nella finestra dell'app la chiusura Z si stampa dal pulsante (senza **Apri in una nuova
  scheda**).
- App desktop su macOS: le notifiche comparivano come "Script Editor" con la sua icona. Ora sono notifiche di Arenile
  (nome e icona dell'app, visibili anche con la finestra in primo piano) e un clic apre la pagina giusta del pannello:
  **Informazioni su Arenile** per una nuova versione, le impostazioni dell'app per i backup, **Rete locale** per gli
  avvisi della rete locale. Al primo avviso macOS chiede il permesso; se le notifiche di Arenile sono spente nelle
  Impostazioni di Sistema, l'app non ne mostra.
- App desktop: la notifica di nuova versione si ripeteva a ogni avvio. Ora ogni versione è notificata una sola volta
  per installazione; una versione più recente viene notificata di nuovo.
- L'avviso di nuova versione nel pannello compariva solo dopo aver ricaricato la pagina o dopo un'ora: finché il
  server non ha fatto il primo controllo (30 secondi dopo l'avvio) il pannello rilegge lo stato ogni 15 secondi.

### Modificato
- L'avviso di nuova versione usa gli stessi pulsanti della pagina **Informazioni su Arenile** (**Scarica Arenile X per
  &lt;sistema&gt;**, **Novità**) e porta ai **Dettagli**.

## [1.0.2] - 2026-10-25

Versione di sola distribuzione: il codice è identico alla 1.0.1.

### Modificato
- Gli installer Windows (x64 e arm64) e l'eseguibile dell'app desktop sono firmati da **Coste360 SRL** con Azure
  Artifact Signing e marca temporale Microsoft: SmartScreen non mostra più "editore sconosciuto". Documentazione
  dell'app desktop aggiornata.

## [1.0.1] - 2026-10-24

### Aggiunto
- Avviso di nuova versione (ADR 0025): titolari e gestori vedono in cima al pannello "È disponibile Arenile X" con
  **Novità** e **Scarica** (app desktop, il pacchetto per il proprio sistema) o **Come aggiornare** (Docker), chiudibile
  per versione. Il controllo è acceso di default su tutte le edizioni e si spegne con `ARENILE_UPDATE_CHECK=false` o
  dalle impostazioni dell'app desktop: una richiesta al giorno a `https://updates.coste360.it/arenile/latest.json`
  (`ARENILE_UPDATE_URL`) con solo versione, edizione e sistema nell'User-Agent, nessun identificativo né dato del
  lido; non è telemetria. Un controllo fallito non è mai un errore. Nuovo `GET /api/admin/v1/system/update`.

### Corretto
- Configurazione → Tutti gli stabilimenti: tolto il riquadro provvisorio "Sezione in preparazione · Disponibile dalla
  milestone M1" rimasto dalla prima milestone; il titolare trova il pulsante **Nuovo stabilimento** nell'elenco, anche
  quando è vuoto (prima si raggiungeva solo dal selettore in alto).
- Configurazione guidata, passo Pagamenti: Stripe non è più mostrato disattivato con "In arrivo · Disponibile a breve";
  il passo spiega che in cassa è tutto pronto e porta con **Configura i pagamenti online** alla pagina di Stripe.
- Vendita rapida dopo l'orario limite della giornata: invece di "La richiesta non rispetta le regole di prenotazione"
  la cassa dice di scegliere **Pomeriggio** (o un'altra fascia) e dove si cambia la regola.
- Test end-to-end della vendita rapida indipendenti dall'ora in cui girano.

### Modificato
- Repository pubblico: le GitHub Actions non conservano più binari o pacchetti desktop come artefatti (sarebbero
  scaricabili da chiunque). `release.yml` non costruisce più i pacchetti desktop, che si fanno nel repository privato
  delle build; il bundle dei binari server va solo al portale Coste360, quando configurato.
- Documentazione e README: i pacchetti dell'app desktop sono indicati come disponibili, con il link a
  [arenile.app/download](https://arenile.app/download/) (prima "in arrivo" sul portale Coste360).

## [1.0.0] - 2026-10-24

Prima versione stabile. Comprende tutte le milestone della specifica: **M0 — Fondamenta**, **M1 — Inventario e
mappa**, **M2 — Prezzi e policy**, **M3 — Prenotazioni dal pannello e cassa**, **M4 — Pagina pubblica e pagamenti
online**, **M5 — Completamento della stagione** e **M6 — Edizione desktop**; le funzioni rinviate (ADR 0019), la
vendita giornaliera con ticket numerati e i biglietti d'ingresso (ADR 0020), la stampa ESC/POS su stampanti di rete
dall'app desktop (ADR 0021), i testi delle email modificabili e i promemoria automatici (ADR 0022), la revisione
della concessione d'uso (ADR 0023), il kit della mappa v3 "Moderno" (ADR 0024) e il lavoro sull'accessibilità
(WCAG 2.1 AA). Le note di rilascio sono in [`docs/release-notes/1.0.md`](docs/release-notes/1.0.md).

### Added

- **Kit mappa v3 "Moderno"** di Coste360 (ADR 0024): icone piene a un colore in cui il corpo di ogni postazione
  prende il colore del suo stato (tutti gli 11 stati dell'app, tramite `--beach-state`), badge nell'angolo che il
  kit lascia libero, anello di contorno per passaggio del mouse, focus e selezione visibile su ogni colore. Il kit
  v2.1 resta disponibile come **"Classico"** (alone sotto il disegno). Si sceglie per stabilimento in
  **Configurazione → Stabilimento → Mappa**, con l'anteprima dei due kit; vale per pannello e pagina pubblica.
  Contratto: `Theme.map_kit` (`modern` | `classic`, assente = `modern`), nessuna migrazione.
- Barra diagonale sulle postazioni **bloccate** in entrambi i kit, oltre al badge: lo stato non dipende dal colore.
- **Accessibilità della mappa**: lucchetto nell'angolo delle postazioni non disponibili (anche nella legenda, che
  ripete i simboli dei badge) così libero e non disponibile non differiscono solo per il tono; il nome di ogni
  postazione dice fila e zona ("Postazione A1, fila A, Prima fila: Libero, da 30,00 €"); le legende sono elenchi con
  nome. Nell'elenco delle postazioni ogni fila è un solo punto di tabulazione (frecce, Home e Fine al suo interno,
  pattern WAI-ARIA toolbar), le postazioni non sceglibili restano raggiungibili e il nome letto di ogni pulsante è il
  testo visibile: tolta l'ultima eccezione axe (`label-content-name-mismatch`) dai test end-to-end.
- **Accessibilità (WCAG 2.1 AA, European Accessibility Act)**: nuova pagina della guida
  [Accessibilità](https://docs.arenile.app/gestore/accessibilita/) (it/en) con cosa è supportato, limiti noti e un
  modello di dichiarazione di accessibilità per il lido; sezione dedicata nel README. Correzioni: il calendario della
  pagina pubblica è una finestra con nome e legenda, i giorni pieni, chiusi o non prenotabili sono barrati (non solo
  attenuati) e annunciati; pagine di errore con un titolo di primo livello; l'avviso della modalità prova non viene
  più annunciato come allarme; con la preferenza "riduci movimento" animazioni, transizioni e scorrimenti diventano
  istantanei; testo dei toast leggibile (4,5:1) sui colori di stato; i selettori di data del pannello includono la
  data mostrata nel nome accessibile; i grafici nascosti alle tecnologie assistive non ricevono più il focus; barra
  laterale come regione `complementary`; l'elenco delle allocazioni di una prenotazione scorre anche da tastiera. Test
  end-to-end: axe su ogni passo della prenotazione, sull'esito, sull'area del cliente e sulle pagine principali del
  pannello, una prenotazione completata con la sola tastiera, lingua della pagina e movimento ridotto.
- Pagamenti online: se l'URL del webhook è locale (`localhost`, rete privata, `.local`), il pannello avvisa che
  Stripe non può raggiungerlo e suggerisce il comando `stripe listen --forward-to <URL>` per le prove.
- **Testi delle email ai clienti modificabili** ([ADR 0022](docs/adr/0022-modelli-email-e-promemoria.md),
  INV-97): nuova pagina Configurazione → **Email ai clienti**; per ogni email al cliente e lingua (it/en/de/fr)
  si modificano oggetto, testo iniziale, testo finale e firma, con segnaposto dall'elenco (`{nome}`, `{codice}`,
  `{data}`, `{postazione}`, `{stabilimento}`, `{importo}`, `{saldo}`, `{scadenza}`, `{cognome}`), anteprima dal vivo
  con dati di esempio, prova al proprio indirizzo e ripristino del testo predefinito. QR, riepilogo, importi, link di
  gestione e piè di pagina restano fissi. API `email-templates` (ETag/`If-Match`, `422 email_template_invalid`),
  migrazione **00012** (SQLite e PostgreSQL) con la tabella `email_template_overrides`.
- **Promemoria automatici** (ADR 0022, INV-95, INV-96): il promemoria del giorno prima (18:00 predefinite) e quello
  del saldo (alle 10:00, 2 giorni prima della scadenza della policy, configurabili) partono da soli con il job
  `notify.reminders`, nel fuso orario dello stabilimento, una sola volta per prenotazione; mai per prenotazioni di
  prova, cancellate o scadute, clienti senza email o saldo a zero. Nuovi modelli `balance_reminder` e
  `day_before_reminder` in quattro lingue; `email-settings` ha il nuovo campo `reminders`.
- Contratto della **vendita giornaliera** (ADR 0020, spec 14.3): API `daily-sales` (preventivo e vendita su
  postazione o zona, pagamento contanti/carta, nome facoltativo), `tickets` (Ticket del giorno con riepilogo ed
  esportazione CSV, dati di stampa, ristampa, annullamento con rimborso, assegnazione, spostamento, assegnazione
  automatica, ricerca dal QR), `entry-ticket-types`, `entry-sales` ed `entry-gate/check-in`; nuovi tipi di problema
  `zone_full`, `assignment_conflict`, `ticket_number_conflict`, `ticket_voided`, `ticket_already_entered`,
  `price_changed`. Migrazione **00010** (SQLite e PostgreSQL): `daily_ticket_counters` (numero progressivo del
  giorno con upsert atomico), `tickets` con `UNIQUE (establishment_id, ticket_date, ticket_number)`,
  `entry_ticket_types`, `daily_sale_settings`; voci di menu Vendita rapida, Ticket del giorno e Biglietti
  d'ingresso.
- **Assegnazione dei biglietti da assegnare** (ADR 0020, INV-83): lista "Da assegnare" del giorno, assegnazione a
  una postazione libera della zona e spostamento tramite la `Move` della prenotazione (atomici, 409
  `assignment_conflict` se la postazione è stata presa), assegnazione automatica in ordine di numero con il motore di
  spec 7.5; la vista operativa del giorno espone il biglietto (`ticket`: numero, etichetta "#037", da assegnare) e
  l'etichetta dell'occupante diventa il nome facoltativo o il numero.
- **Vendita giornaliera e biglietti** (ADR 0020, INV-81, INV-82, INV-84): vendita rapida su postazione o zona con
  prezzo dal motore, pagamento contanti/carta col driver manuale (contanti solo con cassa aperta, resto calcolato) e
  cliente "al banco" con nome facoltativo; numero progressivo del giorno unico su tutte le casse; vendita per zona che
  consuma subito la capacità; Ticket del giorno con riepilogo per metodo e aliquota ed esportazione CSV; dati di
  stampa, ristampa, annullamento che conserva il numero con rimborso tramite i pagamenti; tipi di biglietto
  d'ingresso, vendita di ingressi sul pool condiviso con l'online e controllo al varco con un solo ingresso per QR. Gli
  incassi entrano nelle sessioni di cassa e nei corrispettivi per aliquota.
- Telefono **facoltativo ovunque** (checkout pubblico e pannello): `require_phone` è deprecato e vale sempre false.
- Pannello: **Vendita rapida** con le schede **Ombrelloni** e **Ingressi** (biglietti d'ingresso con stampa automatica)
  e il collegamento al **Varco**; mappa, Oggi e Cassa collegati alla vendita giornaliera; stampa del biglietto su
  scontrino 80/58 mm, A5 e A4 con QR; pannello **Da assegnare** sulla mappa; **Impostazioni** dei biglietti (formato
  di stampa predefinito, stampa automatica, vendita per zona) in Ticket del giorno. Le postazioni dei biglietti
  mostrano il cognome o il numero anche nel nome accessibile e nel tooltip (non più "Cliente al banco"); la vendita
  rapida propone solo i pack ammessi nella zona scelta (mai quelli degli ingressi). Le impostazioni della pagina
  pubblica non mostrano più "Telefono obbligatorio"; i moduli cliente chiedono solo il cognome.
- Documentazione: guide "Vendita giornaliera e ticket", "Assegnazione ombrelloni" e "Biglietti d'ingresso e controllo
  accessi" (it, en) con screenshot; cassa, clienti, prenotazioni e pagina pubblica aggiornate per il telefono
  facoltativo. Test end-to-end della vendita su postazione e per zona, assegnazione, annullamento, ingressi e varco,
  checkout pubblico e nuovo cliente senza telefono.
- Revisione della vendita giornaliera (ADR 0020, INV-83, INV-84): la prenotazione di un biglietto si cambia solo dalle
  azioni del biglietto (cancellazione, modifica, spostamento e assegnazione automatica generici rispondono 409
  `ticket_owned`); assegnazione e spostamento **conservano il totale incassato** (prezzo del motore solo nell'audit);
  annullamento e assegnazione prendono i lock nello stesso ordine (nessuno stallo su PostgreSQL); `assign`, `move` e
  `void` richiedono **If-Match** con la versione del biglietto (428/412) e rispondono con `ETag`; il QR porta un **gate
  token** casuale (`tickets.gate_token`, 128 bit) e il varco fa entrare solo con quello: numero del giorno o codice
  della prenotazione scritti a mano passano come **inserimento manuale** confermato (`manual`, nell'audit); un
  biglietto la cui prenotazione non è più confermata non entra; una vendita di ingressi riguarda una sola zona (una
  scansione fa entrare il gruppo); `OperationalTicket` espone `id` e `version`; il riepilogo di Ticket del giorno è
  dichiarato per giorno di validità; il cliente "al banco" resta fuori dalla classifica dei migliori clienti. Pannello:
  spostamento dei biglietti dalla mappa con le azioni del biglietto, postazioni provvisorie segnate "da assegnare" (la
  zona piena offre al biglietto la sua stessa postazione, le postazioni libere come pulsanti grandi, mappa portata in
  vista con tocchi da 44 px e legenda ripiegata), scorciatoie C/K attive solo dentro il modulo e mai sotto un altro
  dialogo, riepilogo compatto sul telefono con le zone piene disattivate, postazione in grande e "Non valido ai fini
  fiscali" sul biglietto, stato accanto al numero sui telefoni, varco con numero del giorno e avviso per iPhone e
  Firefox. Guide, spec 14.3 e invarianti allineate (numerazione per giorno di validità).
- Contratto delle funzioni rinviate (ADR 0019): modalità prova (`/test-mode`, anteprima, eliminazione in blocco),
  rimborsi garantiti (`/refund-requests`), casse multiple (`/cash-registers`, `register_id` sulle sessioni) e
  chiusura Z stampabile (`/cash-sessions/{id}/z-report`), prezzo manuale degli abbonamenti
  (`/season-passes/{id}/price-override`), notifiche del pannello (`/notifications`), storico dei tentativi webhook,
  esportazione elenchi CSV/XLSX (`/exports/{list}`, `format=xlsx` su report e corrispettivi), importazione CSV di
  clienti e abbonamenti (`/imports`). Migrazione 00009 (entrambi i dialetti): `bookings.test`, `payments.test`,
  `outbox_events.test`, `test_mode_settings`, `refund_requests`, `cash_registers` con "Cassa principale" per ogni
  stabilimento, `webhook_delivery_attempts`, `import_jobs`, `admin_notifications`. Le singole funzioni sono
  descritte nelle voci che seguono.
- Mappa operativa: selezione multipla delle postazioni (pulsante "Seleziona", Maiusc/Ctrl-clic, rettangolo
  trascinato, anche al tocco) con conteggio e due azioni: **prenotazione di gruppo** (una prenotazione con una riga per
  postazione, prezzo dal motore, atomica) e **blocco** con motivo, periodo e fascia; "Blocca postazione" e "Sblocca"
  anche dalla scheda della postazione (ADR 0019 C).
- **Modalità prova** (spec 15.4, ADR 0019 A): Configurazione → Modalità prova con interruttore, avviso fisso in
  tutto il pannello, link di anteprima firmato (2 ore) per prenotare in prova dalla pagina pubblica (chiusa ai
  clienti reali salvo opzione esplicita), Stripe con la configurazione di test quando presente ed eliminazione in
  blocco delle prenotazioni di prova (un'unica transazione, audit conservato). Le righe di prova sono escluse da
  report, KPI, corrispettivi, chiusura Z, esportazioni e webhook in uscita.
- **Rimborso garantito** alla cancellazione (link di gestione del cliente o pannello, ADR 0019 B): il rimborso è
  riservato nella stessa transazione della cancellazione e inviato da un job con chiave di idempotenza fissa e
  attese crescenti (1 minuto → 12 ore, `ARENILE_REFUND_MAX_ATTEMPTS`, default 10) fino alla conferma del provider;
  mai perso, mai doppio. Sezione "Rimborsi automatici" nel dettaglio della prenotazione con **Riprova**, stato del
  rimborso nel link di gestione del cliente, notifica al titolare quando i tentativi finiscono.
- **Casse multiple** (ADR 0019 D): Configurazione → Casse (crea, modifica, disattiva, elimina se mai usata), scelta
  della cassa all'apertura, una sola sessione aperta per cassa, corrispettivi filtrabili per cassa. **Chiusura Z in
  PDF**: documento HTML pronto per la stampa generato dal server (A4 o scontrino 80 mm) con intestazione dello
  stabilimento e logo Arenile mono, salvabile come PDF dalla stampa del browser (nessuna libreria PDF).
- **Prezzo manuale degli abbonamenti** (ADR 0019 E): sezione "Prezzo" nella scheda dell'abbonamento, solo titolare
  e gestore, motivo obbligatorio, audit con prima/dopo, `price_snapshot` invariato; "Torna al listino".
- **Notifiche del pannello** (ADR 0019 F): campanella nell'intestazione con conteggio da leggere; un endpoint
  webhook disattivato in automatico avvisa il titolare nel pannello e via email.
- **Storico completo dei tentativi webhook**: ogni tentativo (codice HTTP, durata, errore) nel dettaglio della
  consegna, conservato `ARENILE_WEBHOOK_ATTEMPT_RETENTION_DAYS` giorni (default 30).
- **Esportazioni Excel**: "Esporta Excel" su ogni report, "Scarica Excel" nei corrispettivi e "Esporta elenchi"
  (clienti, prenotazioni, abbonamenti, incassi) in XLSX o CSV, scritti in streaming con le stesse colonne dei CSV.
- **Importa dati**: importazione CSV di clienti e abbonamenti con anteprima, rapporto di validazione per riga e
  colonna, applicazione tutto o niente, stesso file applicato una sola volta, consensi GDPR mai presunti.
- Pagina pubblica: meta Open Graph per stabilimento generati dal server (titolo, descrizione, immagine = logo
  dello stabilimento o immagine Arenile) per le anteprime di WhatsApp e social.
- Mappa operativa a tutta larghezza (e più alta) sugli schermi larghi.
- Stampa ESC/POS su stampanti termiche di rete (Wi-Fi/Ethernet, TCP 9100) nell'edizione desktop (ADR 0021):
  Configurazione → **Stampanti** (voce visibile solo nell'edizione desktop; indirizzo solo di rete locale, mai
  loopback né metadati cloud salvo `ARENILE_PRINTERS_ALLOW_LOOPBACK` per sviluppo e test; carta 58/80 mm, codepage
  CP858 predefinita, taglio, avanzamento 3 righe, logo retinato in cache, cassetto pin 2/5, verifica raggiungibilità,
  **Stampa di prova**), instradamento per documento e cassa (chiusura Z, ricevuta di pagamento non fiscale, biglietto
  della vendita giornaliera) e ultime stampe con riprova. Lavori accodati già composti; `Idempotency-Key` legata alla
  richiesta (stessa chiave → stesso lavoro, stessa chiave per un altro documento → `422 idempotency_key_reused`, senza
  chiave ogni richiesta è una ristampa); consegna per stampante in parallelo con lease, timeout e nuovi tentativi (5
  s, 15 s, 1 min, 5 min), "almeno una volta" (un invio interrotto viene rispedito senza riaprire il cassetto);
  contenuto delle stampe concluse cancellato dopo 30 giorni; nessun dato personale nei log. Con 2-3 copie, dalla
  seconda in poi la dicitura **COPIA**. In Cassa **Stampa riepilogo** (anche a cassa aperta, con i totali correnti) e
  **Stampa chiusura Z** usano la stampante termica quando è pronta, altrimenti la stampa del browser con un avviso;
  dopo un incasso si può stampare la ricevuta (solo termica). La **Vendita rapida** (stampa automatica) e il **Ticket
  del giorno** (ristampa) mandano il biglietto alla stampante termica quando l'instradamento del biglietto giornaliero
  è pronto, altrimenti la stampa del browser: numero #NNN, giorno, zona e postazione o "da assegnare", pacchetto,
  importo, nome facoltativo (solo il cognome, come sul biglietto del browser), QR (lo stesso del biglietto stampato
  dal browser: il codice del varco per gli ingressi) e la riga "Non valido ai fini fiscali". Sull'edizione server la pagina spiega che serve l'app desktop, non si aggiungono stampanti
  (`409 printing_unavailable`) e la chiusura si stampa dal browser. Codificatore `internal/escpos` (puro Go, fixture
  di byte), modelli `internal/printing/templates` (ricevuta, riepilogo Z, pagina di prova, biglietto `TicketDoc`
  alimentato dai biglietti della vendita giornaliera), servizio `internal/domain/printing` (interfaccia `Driver`,
  driver `network`), migrazione 00011 (entrambi i dialetti). Guida "Stampanti e ESC/POS" (it/en), invarianti INV-86 e
  INV-87. USB, Bluetooth e ponte per il VPS non sono inclusi.

- App desktop per macOS: oltre al `.dmg` universale, la CI produce i `.dmg` **solo Apple Silicon**
  (`_macos_arm64`) e **solo Intel** (`_macos_x86_64`), circa metà dimensione, ognuno firmato, notarizzato e
  graffettato, tutti nel bundle per il portale. Se la notarizzazione Apple non finisce entro `NOTARY_TIMEOUT`, il
  dmg firmato resta comunque nell'artefatto (nome `-notarization-pending`, id della richiesta in
  `.notarization.txt`) e il job fallisce; come verificare e graffettare a mano in `docs/desktop-build.md`.
- Mappa operativa (Mappa e Oggi): **etichetta dell'occupante** sotto le postazioni occupate, cioè il cognome del
  cliente (il nome se manca il cognome; titolare per gli abbonamenti, cliente principale per i gruppi; "Anonimo"
  per i clienti anonimizzati; mai email o telefono), troncata allo spazio tra le postazioni vicine e nascosta,
  con l'invito a ingrandire, quando non sarebbe leggibile senza sovrapporsi; nome completo nel tooltip e nel nome
  accessibile ("occupato da Rossi M."), anche nella vista Elenco. Selettore **Etichette: Codice / Cliente /
  Entrambi** nella barra della mappa (menu a tendina su schermi stretti), memorizzato per utente. API: campi
  `occupant_label` e `customer_anonymized` in `OperationalBooking` (`GET .../operations/day`, permesso
  `bookings.read`); gli eventi SSE restano senza dati personali. Spec 11.3, guida "Mappa del giorno" (it/en) e
  screenshot `admin-map-labels`.
- Documentazione, guida del gestore in italiano e inglese: nuove pagine **Report** (periodo, report di dettaglio,
  confronto con l'anno precedente, esportazione CSV), **Chiavi API e webhook** (creazione, permessi, scadenza e
  revoca delle chiavi; endpoint, evento di prova, stati, registro consegne e rotazione del segreto),
  **Dispositivi** (abbinamento con QR code, accesso limitato allo stabilimento, revoca), **Pannello senza
  connessione** (installazione come app, cosa funziona offline, coda delle operazioni, dati sul dispositivo) e
  **App desktop e rete locale** (finestra, icona nella barra di sistema, backup e ripristino, modalità rete locale,
  modalità remota; pacchetti indicati come in arrivo); sezione "Spostare il mare" (lato del mare e riorientamento
  della disposizione) in Mappe ed editor; panoramica della guida con le schede di Cambio stagione, Crediti,
  Rinnovi, Lista d'attesa e delle nuove pagine; screenshot `admin-reports`, `admin-api-keys`, `admin-webhooks`,
  `admin-devices`. Home, domande frequenti, Cos'è Arenile, Novità, Scegli come installare e le pagine API su
  autenticazione ed eventi non indicano più come "in arrivo" funzioni già disponibili (rinnovi, lista d'attesa,
  crediti, report, pannello offline, chiavi API, webhook); restano in arrivo i pacchetti dell'app desktop e l'app
  mobile.
- Marchio ufficiale **Arenile — Coste360.it** e dominio **arenile.app** ([ADR 0018](docs/adr/0018-marchio.md)):
  brand kit v1.0 e logo Coste360 in `assets/brand/` (tutti i diritti riservati, esclusi dalla licenza del
  codice, `NOTICE.md` e `TRADEMARK.md` aggiornati), testo dei logotipi in tracciati e PNG derivati con
  `scripts/brand/build-brand.mjs` (Titillium Web, OFL-1.1, usato solo per generare i tracciati: nessuna nuova
  dipendenza di runtime, nessun font caricato dalle app).
- Pannello: tema shadcn con i colori allineati al design system Coste360 (decisione del titolare del
  2026-10-23: navy `#0b2542` primario in chiaro, `#185482` per focus e link, azzurro `#38bdf8` primario in
  scuro su superfici navy, ink e slate per testi e bordi, errore `#b91c1c`, bordi dei campi a 3:1, grafici e
  sidebar coerenti,
  contrasto AA verificato da un test; componenti, layout, raggi e font invariati), icona e
  scritta Arenile nella sidebar, logo orizzontale in accesso e primo avvio con "Un prodotto di" + logo
  Coste360, firma Coste360 discreta nel piè della sidebar, favicon SVG con PNG, icone PWA (192, 512,
  maschera), apple-touch-icon, `theme_color` navy e titoli delle schede "Pagina · Stabilimento · Arenile".
  Componenti condivisi `BrandLogo`, `MadeByCoste360`, `Coste360Signature`.
- Pagina pubblica: colore predefinito `#185482` (primary-lighter Coste360; il tema dello stabilimento resta prioritario;
  anello di focus con variante chiara e scura),
  favicon e immagine social Arenile come default (nel browser sostituiti dal logo dello stabilimento, se c'è;
  i crawler dei social vedono il default), piè "powered by Arenile" con link a https://arenile.app.
- Email: in fondo a ogni messaggio una firma discreta "Arenile — Coste360.it" con il marchio Arenile come
  immagine inline (cid:, nessuna immagine remota né tracciamento) e link a https://arenile.app, anche nella
  parte di testo; il marchio dello stabilimento resta in primo piano. Il colore predefinito delle email senza
  tema è il navy Coste360 `#0b2542` (primario del pannello, testo bianco AAA); la finestra di abbinamento
  dell'app desktop usa gli stessi colori del pannello in chiaro e in scuro.
- Edizione desktop con il marchio ufficiale: icone per macOS (.icns sulla griglia di macOS), Windows (.ico
  16–256 px) e Linux (PNG 512 e SVG) generate da `scripts/desktop/gen-icons.sh`; nella barra dei menu di
  macOS la "A" monocromatica (immagine template), nella tray di Windows e Linux la tile a colori. "Informazioni
  su Arenile" (macOS) mostra "Arenile — Coste360.it", la versione e la licenza; installer e metadati AppStream indicano
  https://arenile.app.
- Versione di build reale (`scripts/version.sh`): il tag se il commit è taggato, altrimenti una versione di
  sviluppo semver come `0.6.0-dev.187+835d645`, mai `0.0.0` (anche in `/meta`); su macOS CFBundleVersion è il
  numero di commit. Il controllo aggiornamenti considera una pre-release inferiore alla release corrispondente.
- Documentazione: logo Arenile chiaro/scuro, favicon, immagine social, piè "Arenile — Coste360.it" con logo
  Coste360 e riquadri nei colori del marchio; indirizzo predefinito `https://docs.arenile.app` (configurabile
  con `ARENILE_DOCS_SITE`). Contatto e documentazione nel contratto OpenAPI, link su docs.arenile.app in
  README, CONTRIBUTING e nei template Docker, Coolify e CapRover.
- Documentazione, home per i gestori: logo Arenile al centro, titolo "Il gestionale per stabilimenti balneari",
  riga "Gratuito, senza commissioni, con il codice sorgente pubblico", pulsanti "Inizia ora" e "Guarda come
  funziona", link per i tecnici, schermata reale del pannello con la pagina di prenotazione sul telefono; poi
  funzioni con schermate, tre passi per iniziare con l'aiuto del team Coste360 per chi non vuole installarlo da
  sé, mappa dal vivo, domande frequenti e un'area separata per tecnici e sviluppatori; stesso impianto in
  inglese. SEO: title e description per "gestionale per stabilimenti balneari gratuito", JSON-LD
  `SoftwareApplication` (gratuito, Coste360 SRL) e `FAQPage`, `robots.txt` con la sitemap. Intestazione: icona
  dell'app con il nome "Arenile" e "di Coste360.it" con il logo originale (solo il simbolo sui telefoni).
- M5, traduzioni inglese, tedesco e francese (spec 20): revisione completa di pannello, pagina pubblica ed email
  con un glossario unico ([docs/i18n-glossary.md](docs/i18n-glossary.md)), registro formale (Sie / vous) e
  controllo automatico `web/scripts/i18n-check.mjs` in `npm run lint` (chiavi mancanti o in più rispetto
  all'italiano, variabili non usate, plurali CLDR, sintassi ICU, registro informale, template email). Importi,
  percentuali e date del pannello seguono la lingua dell'interfaccia (`shared/src/lib/intl.ts`).
- M5 parte B, contratto e schema ([ADR 0017](docs/adr/0017-scelte-m5b.md)): API per passaggio di
  stagione, campagne di rinnovo con diritto di prelazione, lista d'attesa con offerte e crediti dei clienti (pannello
  e link magico); migrazione 00008 (`season_rollovers`, `renewal_campaigns`, `renewal_proposals`, `waitlist_*`,
  estensione di `customer_credits` e saldo `customer_credit_balances` mai negativo); policy `credit_validity_days`;
  nuovi eventi webhook `season.renewal_*`, `season.rolled_over`, `waitlist.*`, `credit.*`.
- M5 parte B, passaggio di stagione e rinnovi degli abbonamenti (spec 8.7, 10.4, 19, 28): anteprima e applicazione
  idempotente del passaggio alla stagione successiva (periodi, regole, prezzi fissi, policy e chiusure spostate
  sull'anno, INV-67); campagne "proponi rinnovi" con una prenotazione `held` per abbonato sulla stessa postazione,
  prezzo del nuovo listino e prelazione fino alla scadenza garantita dalle allocazioni (INV-68); invio e promemoria
  via email con link magico (template it/en/de/fr), accettazione dal pannello o dal link con pagamento online di
  acconto o saldo confermato solo dal webhook (INV-69), rifiuto, azioni di massa, proroga della scadenza, chiusura e
  rilascio automatico alla scadenza (job `renewals.sync`). Test di accettazione M5 `TestAcceptance_SeasonCycle`.
- M5 parte B, lista d'attesa e crediti cliente (spec 9, 10.5, 13.4, 19, 23): lista d'attesa con priorità (ordine
  di iscrizione e riordino manuale), iscrizione dal pannello o tramite l'API pubblica (202 senza dati, link
  personale via email, una sola richiesta aperta per indirizzo, pacchetto e periodo; la pagina di prenotazione non ha
  ancora il modulo), offerta automatica sugli eventi di rilascio con hold riservato e scadenza (impostazioni per
  stabilimento), passaggio all'iscrizione successiva, accettazione che prosegue nel checkout di M4 (INV-72; job
  `waitlist.expire`); registro crediti con saldo mai negativo anche in concorrenza, scadenza materializzata (job
  `credits.expire`), crediti manuali e rettifiche dal pannello, uso del credito dal link magico e nei rinnovi, parte
  pagata con credito restituita come credito nei rimborsi con la regola "prima il denaro", credito speso su una
  prenotazione scaduta o rifiutata prima della conferma restituito nella stessa transazione (INV-70, INV-71); la
  prelazione decade se l'abbonamento di origine viene annullato (INV-68). Email `waitlist_joined` e
  `waitlist_offer` in it/en/de/fr.
- M5 parte B, pannello e pagina pubblica: pagine **Rinnovi** (campagne, proposte, azioni di massa), **Lista
  d'attesa** (coda, offerte, impostazioni), procedura **Nuova stagione** e sezione crediti della scheda cliente;
  proposta di rinnovo e uso del credito nell'area di gestione, pagina personale della lista d'attesa
  `/b/{slug}/m/w/{token}`. Guide in italiano e inglese (Cambio stagione, Rinnovi, Lista d'attesa, Crediti) e test
  end-to-end Playwright `e2e/tests/m5b.spec.ts` (progetto `m5b`).
- M5 parte A, report completi (spec 15.3): riepilogo, occupazione per giorno/zona/fila/fascia, ricavi incassati o
  venduti per periodo/canale/pack/extra/metodo/aliquota, prenotazioni per canale/anticipo/formula, cancellazioni e
  no-show, abbonamenti con rilasci e rivendite, clienti migliori (solo nome), saldi aperti e confronto con l'anno
  precedente allineato per data o per giorno della settimana (nuovo parametro `align=calendar|weekday`). Riepilogo e
  confronto su due sole basi di data (soggiorno per occupazione, prenotazioni, prenotato, abbonamenti, cancellazioni
  e no-show; incasso per l'incassato), stampate sotto ogni scheda; la serie del confronto sovrappone gli anni per
  giorno di calendario (29 febbraio ripiegato sul 28) e `compare_year` deve precedere l'anno del periodo.
  Esportazione CSV italiana di ogni report aggregato (BOM, `;`, virgola decimale). Il ricavo incassato coincide al
  centesimo con l'export giornaliero dei corrispettivi (INV-61); una stagione di un lido da 300 postazioni risponde in
  meno di 0,3 secondi. Non ancora fatte, rimandate a M5 parte B ([ADR 0016](docs/adr/0016-scelte-m5a.md)): le
  esportazioni di elenco (prenotazioni, clienti, pagamenti, corrispettivi) e il formato XLSX richiesti da 15.3.
- M5 parte A, chiavi API e webhook in uscita dal pannello: chiavi con scope, scadenza (rifiutata in autenticazione),
  revoca immediata, limite di 300 richieste al minuto per chiave e canale `api` forzato per prenotazioni e hold creati
  con una chiave (quota online, ADR 0011); endpoint webhook con filtro sugli eventi, evento di prova sincrono,
  registro delle consegne con reinvio, rotazione del segreto con doppia firma nel periodo di grazia, ritenzione di 30
  giorni, limite di 256 KiB sul corpo e protezione SSRF (risoluzione unica e connessione all'IP controllato, link-local
  e metadati sempre bloccati, reti private solo con la nuova variabile `ARENILE_WEBHOOKS_ALLOW_PRIVATE`, nessun
  redirect seguito; `localhost` e loopback rifiutati già al salvataggio quando la variabile è spenta). La query
  string dell'URL (dove spesso viaggia un token del ricevitore) non compare mai nel registro attività né
  nell'elenco del pannello. La disattivazione automatica dopo 20 errori non invia ancora l'email al proprietario
  prevista da 18.2 (registro attività e stato nel pannello; email in M5 parte B, ADR 0016). Guida in
  [`docs/guida/integrazioni.md`](docs/guida/integrazioni.md) con la verifica della firma; invarianti INV-62, INV-63,
  INV-64.
- M5 parte A, pannello PWA con offline limitato (spec 21.2): pannello installabile (manifest e icone Arenile, avviso
  di nuova versione con aggiornamento su conferma), copia offline in IndexedDB solo dello stabilimento corrente
  (utente, mappe, situazione di oggi e domani, scadenza 48 ore, cancellata all'uscita o al cambio utente), indicatore
  di connessione con operazioni in coda, avviso offline, coda di check-in, note e incassi in contanti inviata in
  ordine con `Idempotency-Key` alla riconnessione e conflitti da rivedere (riprova o scarta); creazione e spostamento
  di prenotazioni non disponibili offline. La coda sopravvive al cambio di utente sul dispositivo (incassi davvero
  riscossi: li invia la nuova sessione); un check-in accodato dopo un incasso sulla stessa prenotazione non fallisce
  più per versione superata; gli incassi del giorno dicono «Non disponibile offline». L'avviso «pronto per
  funzionare offline» compare una sola volta per browser, in basso a destra, senza coprire l'indicatore di
  connessione. Il service worker non mette mai in cache le risposte `/api/`. Guida in
  [`docs/guida/pannello-offline.md`](docs/guida/pannello-offline.md); invariante INV-65.
- M5 parte A, contratto e scaffolding ([ADR 0016](docs/adr/0016-scelte-m5a.md)): report completi in
  `api/openapi.yaml` (riepilogo, occupazione per giorno/zona/fila/fascia, ricavi per periodo/canale/pack/extra/metodo
  di pagamento/aliquota IVA su base incassato o venduto, prenotazioni per canale e anticipo, cancellazioni e
  no-show, abbonamenti e rivendite dei rilasci, clienti migliori, saldi aperti, confronto con l'anno precedente;
  esportazione CSV di ciascuno), gestione delle chiavi API (scadenza, segreto mostrato una volta) e dei webhook in
  uscita (eventi sottoscritti, rotazione del segreto con periodo di grazia, pausa, registro delle consegne, reinvio,
  evento di prova `webhook.test`), `POST .../maps/{mapId}/reorient` per spostare il lato mare ribaltando o ruotando
  la disposizione; `Idempotency-Key` documentata su check-in e note (coda offline). Migrazione
  `00007_integrations_reports` (SQLite e PostgreSQL). Pagine Report, Chiavi API e Webhook nel pannello e contratto
  della modalità offline (`web/admin/src/offline`). Le nuove operazioni rispondono `501` finché i builder non le
  completano.
- Mappa con il mare in alto (spec 11.2): nuove mappe, generatore di file, modello classico e demo hanno il mare
  in alto con la fila A più vicina; passerella, ingresso e servizi del kit seguono il lato del mare. Cambiando il
  lato del mare l'editor propone "Ribalta la disposizione" (specchio per lati opposti, rotazione di 90° per lati
  adiacenti, tessere delle passerelle sulla loro griglia, un solo salvataggio versionato con evento
  `config.map_changed` `reoriented`, avviso e conferma se ci sono allocazioni future) oppure "Solo il mare".
  `seed demo` porta una volta il mare in alto nelle mappe del modello di una demo esistente (solo geometria,
  con voce di audit `setup.demo_map_reoriented`); le mappe disegnate a mano non cambiano.
- M5 parte A, integrazione: guida [`docs/guida/webhook-verifica-firma.md`](docs/guida/webhook-verifica-firma.md)
  (formato di `Arenile-Signature`, controlli, esempi Node.js/Python/Go/PHP, vettore di prova verificato da
  `TestSignature_DocumentedVector`); test end-to-end per report con confronto anno su anno e CSV, chiave API creata dal
  pannello e usata sull'API admin, endpoint webhook con consegna firmata verso un ricevitore locale, check-in offline
  inviato una sola volta alla riconnessione; invariante INV-66 (lato del mare: solo geometria).
- M6, launcher dell'app desktop in modalità locale (spec 29.1, `cmd/arenile-desktop`, `internal/desktop`): cartella
  dati per sistema operativo, `ARENILE_SECRET_KEY` generata al primo avvio nel portachiavi del sistema
  (`github.com/zalando/go-keyring`, MIT; fallback file 0600) e mai rigenerata per un archivio esistente (INV-60),
  migrazioni e server su `127.0.0.1` con porta libera (riusata ai riavvii), browser aperto sul pannello, istanza unica
  (il secondo avvio apre il pannello), icona nella barra di sistema con *Apri pannello*, *Backup ora*, *Apri cartella
  dati*, *Avvio automatico all'accensione* (LaunchAgent, chiave Run, voce XDG), *Modalità rete locale*, *Ripristina
  un backup…*, *Esci* (`fyne.io/systray`, Apache-2.0, solo con il tag `desktop`); backup giornaliero, alla chiusura e
  manuale con retention e cartella a scelta, ripristino con riavvio del server; controllo aggiornamenti solo se
  abilitato; log ruotati nella cartella dati. Primo avvio dal browser con token monouso e solo da loopback (INV-59),
  pagina *Configurazione → App desktop* e, nell'edizione desktop, spiegazione del passaggio a un'istanza online al
  posto di *Pagina pubblica* e *Pagamenti online*. `arenile backup|restore` e l'app desktop condividono ora lo stesso
  codice (`store.BackupSQLite`, `store.ValidateSQLiteBackup`, `store.RestoreSQLiteFile`).
- M6, modalità rete locale dell'app desktop (spec 29.1, `internal/desktop/lan`): certificato ECDSA P-256
  autogenerato nella cartella dati (825 giorni, impronta SHA-256 stabile ai riavvii e ai cambi di IP), listener
  HTTPS su tutte le interfacce che chiude prima del TLS le connessioni da fuori della rete locale, e middleware
  `LocalPeersOnly` attivo nell'edizione desktop come seconda difesa (INV-57). Pagina *Configurazione → Dispositivi →
  Rete locale* (scorciatoia `/admin/rete-locale` per la voce del menu "Mostra QR rete locale") con indirizzo,
  impronta, istruzioni per accettare il certificato nel browser e abbinamento.
- M6, modalità remota dell'app desktop (`internal/desktop/remote`): abbinamento a un'istanza con QR o codice
  (controllo di `/meta` e della versione minima, https obbligatorio, impronta del certificato verificata), token per
  dispositivo nel portachiavi, finestra dedicata con il webview di sistema (tag `desktop`, macOS e Windows) o il
  browser predefinito (Linux e build headless), opzione kiosk, avvio automatico, proxy locale verificato per le
  istanze con certificato autofirmato, scollegamento e gestione della revoca. Nuova dipendenza
  `github.com/webview/webview_go` (MIT), compilata solo con il tag `desktop` e CGO su macOS e Windows: su Linux il
  launcher resta senza CGO (nessuna dipendenza da WebKitGTK, AppImage statica).
- M6, ponte fiscale (spec 29.1, 14.2, `internal/fiscalbridge`): coda dei lavori con lease, long polling risvegliato
  all'accodamento, esiti idempotenti, ritentativi con attesa crescente per gli errori temporanei, rilascio dei lease
  scaduti e dei dispositivi revocati; agente sul PC della cassa con registro dei driver locali (driver `noop`) ed
  esiti memorizzati su file per non emettere due volte lo stesso documento (INV-58). Endpoint
  `/api/device/v1/fiscal-bridge*` e `GET .../fiscal-bridge` per il pannello; audit `fiscal_bridge.registered`,
  `fiscal_bridge.unregistered`, `fiscal_bridge_job.*`.

- M6, pacchetti dell'app desktop e CI (`packaging/desktop`, `scripts/desktop`, `.github/workflows/desktop.yml`,
  `docs/desktop-build.md`): `.app` e `.dmg` universali per macOS (firma Developer ID, notarizzazione e stapling se
  ci sono i segreti), installer Inno Setup per Windows (per utente, senza amministratore; firma con PFX o Azure
  Artifact Signing), AppImage per Linux costruita su `ubuntu-22.04`; pacchetti senza firma con suffisso
  `-unsigned`. I pacchetti finiscono solo nell'artefatto `portal-bundle-<tag>` per il futuro portale Coste360,
  mai come asset di una release GitHub. Target `make desktop-build` e `make desktop-package-{macos,windows,linux}`.
- M6, il pannello dell'app desktop gira in una finestra propria, senza browser (spec 29.1 rivista, ADR 0015 punto
  19, `internal/desktop/shell`): webview di sistema (WKWebView su macOS, WebView2 su Windows) con icona nel Dock o
  nella barra delle applicazioni, titolo "Arenile", dimensione minima, posizione, dimensioni e zoom ricordati in
  `window.json`, istanza unica (il secondo avvio porta in primo piano la finestra tramite un listener di attivazione
  su loopback con token in `instance.json`). Su macOS menu dell'app (*Arenile*, *Modifica* con copia e incolla nei
  campi, *Vista* con ricarica e ingrandimento, *Finestra*), chiusura della finestra = l'app resta nella barra dei menu
  e nel Dock, *Esci* ⌘Q = backup di uscita e arresto della rete locale; download (`<a download>`, blob, allegati) nella
  cartella Download con notifica, `window.print` con il pannello di stampa del sistema, link esterni nel browser. Su
  Windows chiudere la finestra chiude l'app salvo la nuova impostazione `keep_running_in_tray` (*Alla chiusura della
  finestra resta attivo nella barra di sistema*, `DesktopSettings`, falsa di default). Voce *Apri nel browser* nella
  barra di sistema e nel menu *Vista*, flag `-browser`; Linux e Windows senza WebView2 restano sul browser. La finestra
  della modalità remota usa lo stesso strato nativo (su macOS copia e incolla prima non funzionavano). `Arenile.app`
  non è più `LSUIElement`.
- M6, pacchetto Windows ARM64 nativo (`Arenile_<tag>_windows_arm64_setup.exe`) accanto a quello x64: job sul runner
  GitHub `windows-11-arm` con CGO tramite llvm-mingw (versione e sha256 fissati), stessi passi di firma, artefatto
  `desktop-windows-arm64` nel `portal-bundle-<tag>`; il bundle accoglie anche i pacchetti costruiti quando un solo job
  desktop fallisce.
- M6, `arenile-desktop -pair <QR|URL> [-pair-code] [-pair-fingerprint]`: abbina il PC a un'istanza in modalità
  remota da riga di comando; la modalità remota è ora collegata al launcher.
- M6, `/meta` riporta `desktop.lan_qr_svg` (solo in modalità rete locale): la pagina *Rete locale* mostra il QR
  dell'indirizzo del pannello da inquadrare con la fotocamera di telefoni e tablet (spec 29.1).
- Test: e2e del pannello nell'edizione desktop simulata (spiegazioni al posto di *Pagina pubblica* e *Pagamenti
  online*, abbinamento in attesa della rete locale, pagina *Rete locale*) e collaudo M6 facoltativo contro l'app
  desktop in esecuzione (`e2e/playwright.desktop.config.ts`: primo avvio, configurazione guidata, prima
  prenotazione con i tempi).
- M6/M7, dispositivi abbinati (spec 29.2): codici di abbinamento monouso di 8 caratteri (5 minuti, conservati
  come hash) con QR in SVG generato dal server (`qr_svg`, nuovo campo opzionale di `DevicePairingCode`),
  scambio con un token per dispositivo a 256 bit (hash, legato a utente e stabilimento, permessi del ruolo),
  autenticazione `Authorization: Bearer ar_dev_…` accanto ai cookie (CSRF solo per i cookie), `last_used_at`
  aggiornato al più una volta al minuto, revoca immediata, scadenza per inattività dopo 90 giorni, audit `device.pairing_code_created`,
  `device.paired`, `device.revoked`; `Me.device` per le richieste con token. Pagina *Configurazione →
  Dispositivi* con QR, codice, conto alla rovescia e revoca (INV-55, INV-56). Nuova dipendenza
  `github.com/skip2/go-qrcode` (MIT), la stessa usata da M4 per il QR delle prenotazioni.
- M6/M7 lato core, contratto e scaffolding ([ADR 0015](docs/adr/0015-scelte-m6.md)): `GET /api/admin/v1/meta`
  pubblico (versione, edizione `server|desktop`, modalità desktop `local|lan`, versioni minime dei client,
  capacità); dispositivi abbinati con codice monouso e QR (`devices/pairing-codes`), elenco e revoca, token per
  dispositivo `ar_dev_...` come `Authorization: Bearer` sulle API admin e sulle nuove API `/api/device/v1`
  (abbinamento, dispositivo corrente, ponte fiscale con long polling); API dell'edizione desktop
  `/api/desktop/v1` solo da loopback (primo avvio, impostazioni, backup e ripristino); nuovi tipi di errore
  `pairing_code_invalid`, `lan_mode_required`, `loopback_required`, `setup_token_invalid`, `setup_completed`.
  Migrazione `00006_devices_fiscal_bridge` (SQLite e PostgreSQL). Driver fiscale `bridge`. Launcher
  `cmd/arenile-desktop` (senza CGO di default, tray con il tag `desktop`) e `make build-desktop`. Pagine
  *Dispositivi* e *App desktop* nel pannello. Le operazioni M6 rispondono `501` finché i builder non le completano.
- **M4 — Pagina pubblica** ([ADR 0013](docs/adr/0013-scelte-m4.md)): prenotazione online in quattro passi
  (Quando, Dove, Cosa, Chi e pagamento) per zone a posto sulla mappa con il kit, a zona con assegnazione
  automatica alla conferma e a quantità, anche miste; calendario con giorni pieni e "ultimi posti", legenda dei
  prezzi per fila, pack con composizione, extra, codice promozionale, riepilogo sempre visibile, conto alla
  rovescia dell'hold sull'orologio del server, condizioni versionate e consenso marketing separato; pagina di
  esito con codice, QR e `.ics`; area di gestione dal link magico (dettagli, saldo, cambio date con anteprima,
  cancellazione con rimborso tramite Stripe o credito, rilascio dei giorni per gli abbonati, nuovo link via
  email). Tema dello stabilimento con contrasto WCAG AA, chiaro/scuro, italiano, inglese, tedesco e francese.
- **Widget `embed.js`** (1,1 KB gzip): iframe con altezza automatica, eventi `arenile:resize`,
  `arenile:booked` (solo codice) e `arenile:navigate` (annullabile con `preventDefault()`: il sito gestisce
  l'apertura del pagamento); `frame-ancestors` per stabilimento; dopo il pagamento Stripe riporta il cliente
  sulla pagina del sito ospitante (`return_to` sull'API `pay`, solo origini autorizzate) e il widget riapre
  l'esito senza cookie di terze parti. Con un `<iframe>` semplice, senza `embed.js`, il ritorno da Stripe non
  può riaprire il widget: l'iframe semplice va bene per gli stabilimenti che incassano in cassa.
- **Pagamenti online** ([ADR 0014](docs/adr/0014-pagamenti-online-m4.md)): driver Stripe Checkout con l'SDK
  ufficiale, webhook firmato come unico percorso di conferma online, deduplica, eventi fuori ordine,
  pagamenti tardivi o doppi rimborsati automaticamente (nuovo evento `payment.refund_scheduled`), rimborsi
  tramite Stripe dal pannello e dall'area di gestione, riconciliazione ogni 15 minuti con report.
- **Notifiche** (spec 19): email di conferma (QR in linea, `.ics`, link magico, condizioni di cancellazione),
  modifica, cancellazione, rifiuto, rimborso, richiesta ricevuta e notifiche al gestore (nuova prenotazione,
  richiesta di approvazione, pagamento fallito, errore di invio), in it/en/de/fr con supporto interno a modelli
  Markdown per stabilimento (modifica dal pannello in arrivo); consegna con claim esclusivo, backoff fino a 6 ore e dead
  letter con evento `message.failed`; trasporti SMTP (TLS obbligatorio con credenziali), file `.eml` e log;
  destinatari cifrati e mascherati, nessun pixel di tracciamento. Mailpit nel `compose.yaml` di sviluppo.
- **Pannello**: pagine Pagina pubblica (flusso, tema con anteprima dal vivo telefono/computer chiaro/scuro,
  condizioni, codice da incorporare), Pagamenti online (chiavi in sola scrittura, URL ed eventi del webhook,
  verifica delle credenziali) ed Email e notifiche (mittente, interruttori, email di prova, registro degli
  invii); reinvio della conferma e rigenerazione del link dalla prenotazione.
- **Documentazione** `docs-site/` (Astro Starlight, italiano e inglese, 325 pagine): guida del gestore
  funzione per funzione, installazione e operatività, API e webhook con riferimento generato da
  `api/openapi.yaml`, sviluppo ed estensioni; componenti reali da `web/shared` (mappa col kit, simulatore
  prezzi su dati demo, badge di stato); 46 screenshot chiari e scuri generati da Playwright sui dati demo
  (`make docs-screenshots`), ricerca integrata, link verificati a ogni build, pubblicazione con ogni rilascio
  (`.github/workflows/docs.yml`).
- **Accettazione M4** automatizzata: `e2e/tests/m4-acceptance.spec.ts` (smartphone, iframe su un'altra
  origine, pagamento con un finto Stripe che firma il webhook, esito con codice e QR: circa 5 s; un'origine non
  autorizzata è bloccata da `frame-ancestors`); fase online della giornata simulata di
  `TestAcceptance_SimulatedDay` (120 tentativi cassa/online su una postazione, gare sul pool di una zona
  `zone`, scadenze, webhook riconsegnati, rimborsi, pagamenti tardivi) su SQLite e PostgreSQL;
  `make acceptance-m4` (binario reale con `curl`, 29 controlli); `make lighthouse` (mobile, rete e CPU
  rallentate: 97 performance, 100 accessibilità, best practices e SEO), anche in CI.
- Configurazione: `ARENILE_PUBLIC_READS_PER_MINUTE` e `ARENILE_PUBLIC_CREATIONS_PER_MINUTE` (limiti per IP
  dell'API pubblica, default 60 e 10 della spec 17.2), `ARENILE_STRIPE_API_URL` (solo test). `embed origins`
  accettano anche `http://localhost` e `http://127.0.0.1` per provare il widget in locale.
- Dati dimostrativi: condizioni d'esempio (`demo-2027`) e prenotazione online aperta fino a 366 giorni prima,
  così la demo si prenota anche fuori stagione.

#### Limiti noti di M4

- Pagamento online dentro un `<iframe>` semplice (senza `embed.js`): dopo Stripe il cliente torna alla pagina
  pubblica a tutta finestra e non vede l'esito nel widget.
- I pagamenti tardivi sono sempre rimborsati (la riconferma automatica quando la postazione è ancora libera non
  è collegata).
- La penale di modifica prevista dalla policy è mostrata ma non addebitata.
- Nessuna cache di 10 secondi della disponibilità pubblica.
- Notifiche al gestore per chiusura di cassa con differenza ed errore di consegna dei webhook: non ancora
  implementate (spec 19); promemoria del saldo e del giorno prima: in arrivo.
- Modifica dei modelli email dal pannello: in arrivo (esiste solo il supporto interno).
- Nessuna prova con un account Stripe di produzione.

### Changed

- Kit della mappa spostati in `assets/map-kits/<chiave>/` (`modern` v3.0, `classic` v2.1, ognuno con `NOTICE.md`);
  il generatore `web/scripts/gen-map-kit.mjs` gestisce tutti i kit, pretende lo stesso catalogo in ognuno e
  applica regole per profilo (v3: solo `g/path/rect/circle/ellipse`, colori solo `var(--beach-*, fallback)`,
  angolo degli indicatori vuoto tranne mare e sabbia, stato con `--beach-state`); simboli `ark-<kit>-<id>` in un
  chunk pigro per kit (ADR 0024).
- Colori degli stati della mappa per tema (ADR 0024): **Libero** diventa un **grigio neutro** (`#71717a` nel chiaro,
  `#d4d4d8` nello scuro, prima il verde `#22c55e`), a 3:1 su ogni sabbia e distinto da Bloccato (ora `#374151` nel
  chiaro) e Non disponibile; **In attesa** `#b45309` nel chiaro (3:1, prima 2,0:1); **Prenotato online** `#0284c7`,
  distinguibile da **Presente** anche in luminosità; nel tema scuro gli stati da gestire hanno valori propri che
  arrivano a 3:1 sulla sabbia scura. Legenda, planning e grafici seguono.
- **Aggiornamento:** gli stabilimenti esistenti non hanno ancora scelto un kit e passano al **Moderno**; le mappe
  salvate non cambiano (stessi id delle illustrazioni). Per tornare al disegno precedente scegliete **Classico** in
  Configurazione → Stabilimento → Mappa.
- Licenza: adottato il testo della concessione d'uso rivisto dal legale (ADR 0023). Restano gratuiti gli
  stabilimenti gestiti direttamente, comprese le piscine all'aperto; sono chiariti widget e link nei siti, installazione
  "in un clic", consorzi e associazioni, hotel e aiuto gratuito; il divieto riguarda i servizi a terzi e i prodotti
  che sostituiscono Arenile. README, FAQ e pagina Licenza della documentazione aggiornati.
- **Promemoria attivi per impostazione predefinita**: gli interruttori dei promemoria del saldo e del giorno prima,
  finora visibili ma senza effetto, erano già accesi nelle impostazioni email predefinite; dall'aggiornamento i
  promemoria partono davvero. Si spengono in Configurazione → Email ai clienti, dove si trovano ora (non più in
  Email e notifiche). Le email ai clienti non usano più il grassetto nel testo iniziale e finale (testo semplice
  modificabile).
- **Approvazione delle prenotazioni** (regola R3, ADR 0019 E, INV-78): **Approva**, e **Conferma** su una
  prenotazione *Da approvare*, richiedono ora `establishment.configure` (titolare e gestore); con il solo
  `bookings.write` (cassa, chiavi API con quel permesso) la risposta è `403`. **Rifiuta** resta con
  `bookings.write`. La forzatura (**Conferma comunque**) scavalca solo le violazioni di policy, mai l'approvazione:
  una prenotazione che la richiede nasce *Da approvare* anche se forzata o pagata subito in cassa. Il pannello
  nasconde Approva e Conferma a chi non può approvare.
- **Pagamenti online e modalità prova**: la configurazione Stripe (live o test) usata per un pagamento segue la
  prenotazione, non l'interruttore della modalità prova: una prenotazione reale paga sempre con le chiavi live,
  anche mentre la modalità prova è accesa, e una di prova sempre con quelle di test, anche dopo averla spenta. I
  webhook sono verificati con entrambe le configurazioni e un evento verificato con un segreto non tocca mai un
  intento dell'altra modalità; riconciliazione e rimborsi usano la modalità dell'intento. Un pagamento preso con
  le chiavi di test è una riga di prova (nessun documento fiscale, fuori dai corrispettivi) qualunque sia la
  prenotazione.
- **Rimborsi garantiti**: la notifica "Rimborso non riuscito" è visibile a chi può rimborsare (titolare e gestore),
  non solo al titolare; una chiave di idempotenza non viene più rinviata al provider oltre 24 ore (la richiesta passa a
  `failed` con `idempotency_window_elapsed` per la riconciliazione manuale prima di Riprova); il controllo
  `refund_in_progress` di un rimborso manuale avviene sotto il lock della prenotazione.
- **Prenotazioni da approvare**: la conferma al banco di un rinnovo segue la regola di approvazione; una prenotazione
  da approvare già pagata al banco non scade più da sola (il blocco viene rinnovato finché il gestore non decide).
- **Modalità prova**: l'eliminazione delle prenotazioni di prova è rifiutata (409, anche in anteprima) se un
  credito generato da una prenotazione di prova è già stato speso su una prenotazione reale; prima veniva
  compensato in automatico lasciando valore di prova nell'incassato reale.
- **Casse**: lo schema esige che ogni sessione di cassa appartenga a una cassa dello stesso stabilimento (`NOT NULL`
  e chiave esterna composta su PostgreSQL, trigger su SQLite).
- **Importa dati**: limite di 2.000 righe per file (500 per gli abbonamenti) al posto di 20.000: anteprima e
  applicazione girano in un'unica transazione dentro la richiesta (misurate ~1,6 ms per cliente e ~4,5 ms per
  abbonamento su SQLite), così un file pieno resta ben sotto il timeout del server e non blocca a lungo la cassa.
  Nessuna importazione con la modalità prova accesa (409); i pacchetti che richiedono approvazione
  sono rifiutati riga per riga; il contenuto del file è cancellato appena l'importazione è applicata, fallita o
  scartata. Le notifiche "Importazione completata / non riuscita" non esistono: l'esito è nello storico.
- **Prezzo manuale degli abbonamenti**: spostare o modificare un abbonamento con prezzo manuale (anche dalla
  cassa) **conserva** il prezzo manuale e aggiorna il prezzo da listino mostrato sotto di esso; prima la modifica
  lo cancellava senza traccia. Solo "Imposta prezzo manuale" e "Torna al listino" (titolare e gestore) lo cambiano.
- `If-Match` è obbligatorio su check-in, no-show, annullamento e spostamento di una prenotazione e sull'eliminazione
  di una mappa: senza l'header la risposta è `428 precondition_required` (le anteprime `dry_run` restano libere). Il
  pannello e la coda offline inviano sempre la versione letta (`*` per un check-in accodato dietro altre operazioni).
  Le integrazioni via API che chiamano queste operazioni devono inviare l'`ETag` letto o `*`.
- Marchio: adottato il **brand kit Arenile v2.0** (decisione del titolare del 2026-10-23, ADR 0018) al posto
  del v1.0 ovunque: pannello, pagina pubblica, email, app desktop (icone `.icns`/`.ico`/Linux, tray e
  template della barra dei menu), sito della documentazione. "A" sabbia con traversa a onda azzurra su duna,
  riquadro navy, palette Coste360 (navy `#0B2542`, sabbia `#FFF0D1`, sabbia calda `#F2C893`, onda `#38BDF8`),
  nessuna byline nei logotipi. Rifinitura del kit approvata: distanza uniforme tra onda e gambe della "A",
  tolta la tacca sul piede destro, simbolo centrato otticamente, varianti per sfondi chiari con la "A" navy,
  testo dei logotipi in tracciati. `BrandLogo` ha `variant="symbol"` e `tone` (`colour` / `white` / `mono`)
  con le regole d'uso nell'ADR 0018; nella sidebar il simbolo sostituisce l'icona, in tema scuro i marchi
  sono bianchi, il piè "powered by Arenile" ha il simbolo monocromatico.

- Pannello: le pagine di M4, M5 parte A e M6 (Pagina pubblica, Pagamenti online, Email e notifiche,
  Dispositivi, Rete locale, App desktop, Report) seguono lo standard UI comune (`Page`, `SectionCard`, `StatCard`,
  `DataTable`, `ResponsiveTabsList`, `SegmentedControl`); niente più barre di schede che vanno a capo: sotto i
  640 px diventano un menu a tendina.
- Le invarianti di M6 sono ora INV-55..INV-60 e quelle di M5 parte A INV-61..INV-66; l'ADR di M5 parte A è
  0016 (0014 resta quello dei pagamenti online di M4).
- I file testuali delle SPA incorporate sono serviti compressi gzip (`Vary: Accept-Encoding`).
- Limiti dell'API pubblica: contano come creazioni solo l'avvio di una prenotazione e la richiesta di un nuovo
  link; i passi successivi della stessa prenotazione contano come letture.
- Il calendario pubblico non propone date oltre l'anticipo massimo della policy online.
- La mappa segna la postazione scelta con una spunta, la legenda dei prezzi unisce le file con lo stesso prezzo
  ("File C–H") e il prezzo "da" di un pack è quello della postazione scelta.

### Fixed

- **Chiavi di test e clienti reali** (INV-88): con il provider di pagamento configurato in modalità test (chiavi
  `sk_test_…`) e la modalità prova spenta, un cliente reale poteva pagare con una carta di test e ottenere una
  prenotazione confermata gratis. Ora nessun pagamento online parte per una prenotazione reale con chiavi di test
  (`409 payment_provider_test_mode`, nuovo tipo di problema) e un pagamento di prova su una sessione aperta prima
  dell'aggiornamento non conferma la prenotazione reale; la pagina pubblica riporta il pagamento online come
  non disponibile (`online.payment.live_payments_blocked`) e blocca il passo di pagamento con un messaggio chiaro;
  `payment-settings` espone `live_payments_blocked` e il pannello mostra l'avviso **Pagamenti online bloccati per
  i clienti reali** in Pagamenti online e Pagina pubblica, con i collegamenti per passare alle chiavi live o
  accendere la modalità prova. Le prenotazioni di prova continuano a usare le chiavi di test; con le chiavi live
  nulla cambia. Test end-to-end e collaudo M4 ora configurano Stripe (finto) in modalità live.
- Area di gestione del cliente: l'avviso del rimborso di una prenotazione cancellata mostrava le chiavi di
  traduzione (`manage.refund.title`, …) invece dei testi; ora usa lo spazio dei testi della pagina pubblica.
- Modalità prova, **Elimina le prenotazioni di prova**: l'anteprima contava anche i pagamenti Stripe in modalità test
  di prenotazioni reali, che l'eliminazione non tocca; ora conta esattamente le righe che verranno eliminate.
- Le richieste annullate dal client (`context.Canceled`, ad esempio una pagina chiusa durante il caricamento) non
  sono più registrate come errori 500 nei log.
- Pannello, **Nuova prenotazione**: gli extra usano il nuovo controllo condiviso `QuantityStepper` (− valore +,
  sempre visibile e a larghezza fissa) dentro `ExtraList`/`ExtraRow` (nome su una riga, unità "per postazione /
  per prenotazione · al giorno" sotto, "max N" accanto al controllo). Prima il campo numerico veniva schiacciato
  dall'etichetta (valore invisibile, ad esempio per "Lettino aggiuntivo") e permetteva di superare il massimo
  dell'extra; ora lo schema del modulo rispetta `max_quantity` come il server. Lo stesso controllo è usato nel
  simulatore prezzi e nella pagina pubblica (extra e quantità di zona).
- Errori di validazione del server: il nuovo `mapProblemToForm` (`@arenile/shared/api`) collega i percorsi del
  problem+json (ad esempio `lines[0].extras[0].quantity`) ai campi del modulo, che vengono evidenziati con il
  messaggio sotto, e il riquadro in alto elenca cosa correggere a parole ("Lettino aggiuntivo: massimo 2"); i
  percorsi tecnici non vengono più mostrati (prenotazioni, configurazione, editor della mappa).

- SQLite: le scritture concorrenti si mettono in coda nel processo (una coda FIFO per database, davanti a
  ogni transazione di scrittura e scrittura autocommit) invece di contendersi il lock di SQLite con
  `busy_timeout`: sotto un picco di scrittori (webhook, scadenza dei blocchi, cassa) un'attesa poteva
  superare i 10 secondi e fallire con `SQLITE_BUSY` (CI rossa su `TestConcurrency_RevokeVsCancel` e
  `TestConcurrency_WebhookVsHoldExpiry`).
- Test di prestazioni (report di stagione, query operative, template classico): verificano il numero di query
  (niente N+1) invece di un limite di tempo fisso, che ora vale solo con `ARENILE_PERF_BUDGETS=1`.
- Sito della documentazione: `ARENILE_DOCS_SITE` e `ARENILE_DOCS_BASE` vuote (variabili del repository non
  impostate in CI) valgono come non impostate invece di rompere la build con "Invalid URL".
- Schede e barre di navigazione a schede (`ResponsiveTabsList`, `NavTabs`) diventano un menu a tendina anche
  quando non entrano nella larghezza disponibile (sidebar aperta su tablet a 768 e 1024 px), non solo sotto
  `sm`; il breadcrumb del pannello mostra il percorso completo solo da `lg` e non va più a capo.
- Pagina pubblica: la lingua scelta nel selettore non viene più riportata subito a quella automatica (browser o
  lingua dello stabilimento).
- Le schede (`SectionCard`) con azioni larghe nell'intestazione non sovrappongono più i pulsanti al titolo sugli
  schermi stretti: le azioni vanno a capo sotto il titolo.
- M5 parte B: il rinnovo pagato online diventa "Rinnovato" appena arriva la conferma del pagamento (evento
  `booking.confirmed`), non al giro successivo del job; il link dell'email della lista d'attesa punta alla pagina
  `/b/{slug}/m/w/{token}`; l'anonimizzazione di un cliente cancella anche il motivo di rifiuto dei rinnovi; ricerca
  attiva nelle tabelle di Rinnovi e Lista d'attesa.
- `seed demo` non fa più partire, al primo `serve`, le email di conferma delle prenotazioni dimostrative.
- Revisione di sicurezza M4: la pagina pubblica non collega più una prenotazione a un cliente esistente con la
  stessa email o telefono (chi conosceva un recapito ne leggeva nome, email e telefono dalla sessione e
  prenotava a suo nome con i suoi prezzi); il token del link magico non viene più inviato a Stripe come URL di
  ritorno del saldo; i token del checkout scadono 24 ore dopo la fine della sessione; l'idempotenza delle
  chiamate pubbliche tiene conto del token; `no-referrer` e `no-store` sull'area di gestione; nomi dello
  stabilimento e valori `.ics` senza caratteri di controllo; schema dei link verificato nei modelli email;
  webhook con firma errata limitati per IP; STARTTLS obbligatorio fuori da localhost (`?tls=none` per
  rinunciare); tentativi di codice promozionale limitati per IP; `ARENILE_STRIPE_API_URL` accettata solo su
  loopback o con `ARENILE_DEV=true`.
- Pagamenti: un provider online attivo ma non utilizzabile non conferma più senza pagamento (INV-19); la
  deduplica dei webhook è per stabilimento (un account Stripe condiviso tra due lidi confermava solo il primo);
  crediti e rimborsi manuali contano i rimborsi in corso presso il provider (INV-24); l'importo chiesto a Stripe
  è ricalcolato sotto il lock all'avvio del pagamento, un hold scaduto con denaro sopra viene rimborsato e un
  webhook arrivato dopo la scadenza dell'hold ma prima del job scade la prenotazione e segue il percorso del
  pagamento tardivo (INV-51).
- Pagina pubblica: contrasto AA dei link e dei prezzi anche in modalità scura; nessuno scorrimento laterale al
  passo 4 a 320 px; una giornata singola è mostrata con una sola data; `arenile:navigate` arriva al sito
  ospitante prima della navigazione.

- M4, contratto e scaffolding ([ADR 0013](docs/adr/0013-scelte-m4.md)): API pubblica completa in
  `api/openapi.yaml` (profilo con configurazione del flusso ed `ETag`, calendario, mappa pubblica,
  disponibilità online, catalogo, preventivo, prenotazione con sessione di checkout `X-Arenile-Checkout`,
  pagamento, QR, `.ics`, area gestione via link magico, richiesta di un nuovo link), webhook
  `POST /webhooks/payments/{provider}/{establishment_id}`, API del pannello per pagina pubblica, condizioni
  versionate, provider di pagamento (chiavi cifrate), email, registro messaggi, riconciliazione, reinvio della
  conferma e rigenerazione del link; nuovi tipi di errore `checkout_not_found`, `payment_provider_unavailable`,
  `webhook_signature_invalid`. Migrazione `00005_online_payments_notifications` (SQLite e PostgreSQL). Nuova
  variabile `ARENILE_MAIL_DIR`. Sito della documentazione `docs-site/` (Astro Starlight, italiano e inglese) con
  `make docs-dev`, `make docs-build`, `make docs-screenshots`. Le operazioni M4 sono implementate (vedi sopra).
- Kit di illustrazioni della mappa ([ADR 0012](docs/adr/0012-kit-mappa.md)): le 107 illustrazioni originali di
  Coste360 (`assets/beach-map-kit`, provenienza in `NOTICE.md`) sostituiscono pallini e rettangoli in editor,
  mappa operativa e mini mappa delle prenotazioni. Un solo sprite `<symbol>` per pagina con i soli disegni usati,
  caricato in modo pigro, e un `<use>` per elemento (1.000 postazioni misurate fluide in Chromium, con margine
  ridotto a CPU rallentata 4×: numeri nell'ADR); lo stato è un anello intorno al disegno più un badge con un
  glifo per ogni stato prenotato (online, cassa, presente, abbonato, assente oggi, in attesa, bloccato, da
  incassare, prelazione), leggibile in chiaro, scuro e in scala di grigi; legenda sopra la mappa; sul telefono
  il disegno mantiene le proporzioni della mappa senza spazio vuoto; sabbia, mare e battigia nello stile del kit.
- API: `SpotKind` aggiunge `sunbed`, `daybed`, `cabana`, `hammock` (anche in `requires_spot_kind` dei pack);
  `asset` opzionale su postazioni, decorazioni e generatore di file (id del kit, validato dal backend; mai su
  un testo), `variant` sulle decorazioni (colore della bandiera `green|yellow|red`). Migrazione
  `00004_map_kit_assets` (SQLite e PostgreSQL).
- Editor: tavolozza con ricerca e categorie per postazioni ed elementi (frecce per riga e colonna,
  illustrazione attuale annunciata), proprietà "Illustrazione" e "Colore della bandiera", illustrazione anche
  nel generatore di file; il tipo resta semantico: un'illustrazione di un'altra famiglia cambia il tipo e il
  pannello lo dice, un cambio di tipo riporta l'illustrazione a quella predefinita; passerelle modulari di 60
  unità che si raccordano (agganciate alla griglia delle tessere quando il passo della griglia divide 60),
  rotazione per ogni asset.
- `make gen` rigenera anche il kit (`web/scripts/gen-map-kit.mjs`: sanifica gli SVG e fallisce su contenuti non
  ammessi) in `web/shared/src/map/kit/generated/` e `internal/domain/mapping/kitassets_gen.go`.
- Modello "Lido classico" e demo con il kit (ombrelloni con lettini, gazebo, passerella, bar, docce, WC, DAE,
  torretta, bandiera, sedia JOB, rampa, palme); `seed demo` aggiorna una demo esistente una sola volta,
  sostituendo solo le quattro decorazioni del modello pre-kit (quelle aggiunte a mano restano) con una voce
  di audit per mappa e il riepilogo di cosa ha tolto e aggiunto.

### Note di aggiornamento

- La migrazione `00004` su SQLite ricostruisce la tabella `spots` (pochi secondi anche con migliaia di
  postazioni); fate un backup prima dell'aggiornamento come per ogni migrazione.

### Fixed

- Report: un indicatore passato da 0 a 0 mostrava "Nuovo" invece di "0,0%".
- Le schede a più righe (Report, Stabilimento) a 390 px si sovrapponevano al contenuto: l'altezza fissa del gruppo di
  schede ora si adatta quando vanno a capo.
- Oggi e Cassa a 390 px scorrevano in orizzontale (griglia senza colonna esplicita): ora restano nella larghezza dello
  schermo.
- Revisione di M3 ([ADR 0011](docs/adr/0011-revisione-m3.md)): la conferma di un hold blocca le sue righe e gli hold
  scaduti vengono rimossi per intero sotto il lock della prenotazione proprietaria (su PostgreSQL una vendita
  concorrente poteva lasciare una prenotazione confermata senza allocazioni); rilascio e revoca dei giorni di un
  abbonamento e redemption del credito cliente serializzano sul lock della prenotazione e del cliente (nuova
  INV-47: credito mai negativo); check-in solo nei giorni del soggiorno e no-show solo dal primo giorno
  (`422 booking_outside_period`, pulsanti nascosti nel pannello); le note degli operatori non finiscono più nel
  registro attività e l'anonimizzazione cancella anche motivo di cancellazione, note dei pagamenti e dei crediti;
  `price_current` per le prenotazioni modificate dopo la conferma (INV-12 riformulata, IVA dei pagamenti sul prezzo
  corrente); la quota online vincola anche il canale `api` (sostituisce ADR 0010 §4); `force` e
  `overpayment_as_credit` dichiarati nel contratto, `existing_customer_id` come estensione del problema;
  le funzioni `inventory.AutoAssign` / `AssignPendingInTx` (senza lock della prenotazione, inutilizzate) sono rimosse.
- `calendar.Date` implementa il `Binder` di oapi-codegen: le date nei parametri di query (`from`, `to`, `date`)
  non rispondono più 400 né vengono ignorate.
- L'anteprima di una prenotazione (`POST /bookings/quote`) verifica la disponibilità anche senza una prenotazione
  esistente, invece di rispondere 400 `owner`.
- Un hold chiuso dal job di scadenza risulta `expired`, non `released`.
- Pannello: i selettori di data non escono dalla loro colonna (filtro "Periodo", barra del giorno della mappa), i
  menu a tendina vuoti mostrano il segnaposto, nomi dei pack e degli extra al posto dei codici, "Cassa principale"
  al posto del codice `main`, calendario planning esteso alla larghezza disponibile con la data iniziale nell'URL.

### Changed

- Pannello: un solo aspetto per ogni componente (standard in `web/shared/src/ui/README.md`, "UI standard"):
  tutte le pagine usano `Page` (intestazione, azioni, avvisi), le tabelle sono sempre `DataTable` (filtri nella
  barra della tabella, numeri e importi allineati a destra, raggruppamento per gli abbonamenti per zona, totali
  in fondo ai corrispettivi, "Carica altri" comune), sezioni in `SectionCard`, KPI in `StatCard`, schede dei
  prezzi come `NavTabs`; schede e controlli segmentati scorrono in orizzontale su telefono e nessuna pagina
  scorre più di lato a 390 px; `Alert` con varianti `success` e `warning` al posto degli stili di pagina.
- Pagina pubblica: il passo "Quando" e il selettore della lingua (NativeSelect) fanno parte del primo caricamento, il corpo del calendario si carica all'apertura del campo; `npm run budget` conta ogni chunk del primo schermo seguendo il manifest di Vite (JS 141 KB gzip su 150, CSS 12,8 KB su 25) e la scansione di Tailwind è limitata ai componenti usati. I passi successivi restano disattivati con "In arrivo".
- Pannello: nomi al posto dei codici per periodi e categorie di prezzo (riepiloghi delle regole, matrice con colonna "Codice" nascosta, simulatore, policy, linea temporale delle stagioni); Policy come scheda della barra prezzi; barra laterale con le sezioni non ancora disponibili raccolte in un gruppo "In arrivo" chiuso e voce "Tutti gli stabilimenti"; mappa condivisa con un solo punto di tabulazione (frecce, PagSu/PagGiù, Home/Fine tra le postazioni) ed etichette mai sotto i 9 px; procedura guidata senza riferimenti alle milestone interne.

### Added

- M3 — motore di disponibilità (`internal/domain/inventory`): allocazioni di postazioni con `INSERT … ON CONFLICT DO
  NOTHING` sul vincolo unico `(spot, date, slot)` e conflitti per ogni mezza giornata rifiutata, pool con `UPDATE`
  condizionale e `capacity_overrides`, quota online, hold del pannello con scadenza e job di pulizia ogni 30 s,
  blocchi di postazioni, assegnazione automatica delle righe di zona, disponibilità, vista operativa del giorno e
  planning (massimo 62 giorni) con un numero fisso di query.
- M3 — prenotazioni dal pannello (`internal/domain/booking`): creazione con cliente esistente o nuovo, anteprima del
  prezzo e delle policy (forzatura registrata nell'audit), codici `LD-2027-000123`, modifica con `If-Match` e
  anteprima, spostamento di una riga, conferma, approvazione, rifiuto, check-in, no-show, note, cancellazione con
  rimborso o credito secondo la policy salvata; ogni cambio di stato passa dalla macchina a stati e scrive audit ed
  evento nella stessa transazione; job di scadenza degli hold e di completamento.
- M3 — clienti (ricerca senza accenti su nome, email, telefono e tag; duplicati 409; esportazione e
  anonimizzazione GDPR che conservano la contabilità), abbonamenti stagionali con rilascio dei giorni, revoca e
  rivendita con credito dall'istantanea della policy.
- M3 — pagamenti manuali con driver `manual`, `Idempotency-Key`, scorporo IVA dall'istantanea del prezzo, rimborsi
  entro il pagato, vendite di cassa; conferma della prenotazione `held` nella transazione del pagamento quando
  l'acconto è raggiunto; cassa con una sola sessione aperta per registratore, movimenti, chiusura con conteggio,
  differenza e riepilogo Z; documenti fiscali numerati per giorno e corrispettivi giornalieri o mensili in JSON e
  CSV (BOM, `;`, virgola decimale, intestazioni in italiano).
- M3 — flusso SSE con ripresa da `Last-Event-ID` (`sse.ReplayBus`, sempre attivo), keep-alive e disconnessione dei
  client in ritardo; `seed demo` aggiunge 40 clienti fittizi, 25 abbonamenti, circa 70 prenotazioni, rilasci con
  rivendite, incassi e due sessioni di cassa attraverso i casi d'uso di M3.
- M3 — pannello: pagina Oggi (occupazione, arrivi, check-in, incassi, saldi, righe da assegnare), mappa operativa con
  stati, ricerca, scheda della postazione e spostamento con anteprima, calendario planning in SVG con trascinamento
  e tastiera, elenco e dettaglio prenotazioni, foglio "Nuova prenotazione" con mini mappa e anteprima del prezzo,
  clienti, abbonamenti con calendario dei rilasci, cassa e corrispettivi; italiano, inglese, tedesco e francese.
- M3 — test di accettazione `internal/acceptance`: giornata simulata da oltre 300 operazioni accettate via API (324 con il seme predefinito; 100 prenotazioni
  parallele sulla stessa postazione, 50 hold paralleli sullo stesso pool, 10 ripetizioni dello stesso pagamento)
  con verifica sul database di allocazioni, pool, prenotazioni, incassi, cassa, corrispettivi, audit, outbox e
  flusso SSE, su SQLite e PostgreSQL; 28 test `TestConcurrency_*`; test Playwright della mappa operativa, del foglio
  "Nuova prenotazione" e della chiusura di cassa.

- M3 — contratto API (clienti, prenotazioni con anteprima e azioni di stato, hold, disponibilità, vista operativa del
  giorno, planning, blocchi, abbonamenti e rilasci, pagamenti manuali e rimborsi, vendite di cassa, sessioni di cassa,
  corrispettivi JSON/CSV) con nuovi tipi di errore `pool_exhausted`, `quota_exceeded`, `invalid_transition`,
  `cash_session_closed`, `cash_session_already_open` e `conflicts`; migrazione 00003 per SQLite e PostgreSQL (ADR
  0010); flusso SSE con nomi di evento per topic (`booking.changed`, `allocation.changed`, `hold.changed`,
  `cash.changed`); pannello con le sezioni Oggi, Mappa, Prenotazioni, Calendario, Clienti, Abbonamenti e Cassa per
  stabilimento.

- Pannello e pagina di prenotazione con UI kit shadcn/ui (ADR 0009): barra laterale con selettore dello stabilimento e menu utente, breadcrumb, palette comandi (Ctrl/⌘+K), tema chiaro/scuro/sistema, toast; tabelle con ordinamento e filtro, pannelli laterali di modifica, calendari e selettori di date gg/MM/aaaa in italiano al posto di `<input type="date">`, importi con `MoneyInput`; editor della mappa con barra degli strumenti a icone, zoom fino al 400%, legenda e pannello proprietà; stagioni con linea temporale e calendario, chiusure su calendario, simulatore con grafico e dettaglio per giorno; pagina pubblica mobile-first con passo "Quando" e selettore del periodo.
- Controllo `web/scripts/css-guard.mjs` in `npm run lint`: rifiuta fogli di stile diversi dal tema (anche in `public/` e `.html`), CSS-in-JS, `style={…}` e stili programmatici fuori dalle esenzioni elencate (geometria della mappa, variabili del tema, iframe dell'embed), valori arbitrari di Tailwind che aggirano il tema (colori, `text-[10px]`, proprietà) e i vecchi token `--ar-*`.
- Test e2e del guscio del pannello (tema scuro persistente, palette comandi) e del selettore di date della pagina pubblica; scansioni axe (WCAG 2.1 AA) del guscio, di una pagina con modulo e dell'editor della mappa in tema chiaro e scuro.
- `GET /api/public/v1/{slug}` restituisce `season` (anno e date della stagione attiva, assente senza stagione attiva): la pagina pubblica limita il calendario alla stagione e mostra città e date sotto il titolo.
- Pattern condivisi `QueryState`, `RowActions` (`EditButton`, `DeleteButton` con conferma e errore del server nel dialogo), `OptionSelect`, `MultiSelect`, `FormDialog`/`FormSheet`/`SheetForm`/`FormActions` in `@arenile/shared/ui/patterns`; un solo modulo degli importi (`@arenile/shared/lib/money`, raggruppamento delle migliaia sempre attivo, importi illeggibili respinti dai moduli invece di essere azzerati).
- M1 — stabilimenti: creazione di nuovi stabilimenti dal pannello (solo owner dell'organizzazione) con selettore, impostazioni con `If-Match` (fuso, lingue, origini embed, tema, impostazioni prezzi, festività locali, policy predefinita), archiviazione e ripristino.
- M1 — stagioni, periodi tariffari (senza sovrapposizioni, serializzati sulla stagione) e giorni di chiusura per fascia AM/PM; copia delle date di stagione.
- M1 — utenti e ruoli (solo owner): aggiunta con password iniziale o temporanea, cambio ruolo e ambito, disattivazione con chiusura delle sessioni; l'ultimo owner attivo non può essere rimosso.
- M1 — mappe: documento versionato salvato in una transazione con `ETag`/`If-Match` (412 con `current_version`, 428 senza header), zone (posto, zona, quantità), pool con capacità per periodo, pool implicito per le zone a zona sincronizzato con le postazioni attive, generatore di file con anteprima, eliminazione rifiutata per postazioni con allocazioni future (verifica predisposta, sempre "nessuna" fino a M3).
- M1 — catalogo: componenti, pack ed extra con archiviazione quando referenziati.
- M1 — configurazione guidata: modelli `classic_rows` (8 × 15), `quantity_beach`, `lido_with_pool`, `empty`, listino rapido per categoria × periodo, stato della configurazione; `seed demo` completo e idempotente (owner dimostrativo, listino, prezzi fissi, policy, codice promo e convenzione).
- M2 — persistenza e API di regole di prezzo, prezzi a durata fissa, codici promozionali, convenzioni e policy di prenotazione collegati ai motori puri; simulatore, matrice di listino, verifica di copertura e copia di stagione del listino.
- Pannello: impostazioni dello stabilimento con anteprima del tema, stagioni con timeline dei periodi, chiusure, zone, pool, catalogo, utenti, procedura guidata in 6 passi, editor della mappa SVG (selezione multipla, trascinamento, tastiera, annulla/ripeti, allinea/distribuisci, rinumerazione, dialogo di conflitto), regole di prezzo con riepilogo in linguaggio naturale, griglia dei prezzi fissi, matrice con esportazione CSV, calendario della copertura, simulatore con dettaglio per regola, policy con esempi della spec 9.
- Eventi `config.map_changed` e `config.pricing_changed` scritti nell'outbox nella stessa transazione delle modifiche, con un unico payload versione 1 (`entity`, `entity_id`, `change`, più `season_id`, `map_id`, `code`, `version` e, per la configurazione guidata, `reason`, `template`, `map_ids`, `zone_ids`, `counts`; spec 18.1).
- `GET /setup/status` restituisce `price_categories`: le categorie di prezzo che il listino rapido deve coprire (zone e override delle postazioni); la griglia del passo 4 della procedura guidata le usa per le righe.
- L'eliminazione di una zona, da sola o con la sua mappa, è rifiutata (409) quando regole di prezzo, codici promozionali o convenzioni (`conditions.zone_ids`), pack (`allowed_zone_ids`) o policy (`scope.zones`) la referenziano; l'errore elenca le entità da modificare prima.
- Test end-to-end Playwright del percorso di accettazione M1/M2: login, nuovo stabilimento dal selettore, modello classico, listino rapido, spostamento e salvataggio di una postazione, simulatore.
- M1/M2: contratto API di amministrazione per stabilimenti, utenti, stagioni, periodi tariffari, chiusure, mappe (documento versionato con `ETag`/`If-Match`, generatore di file), zone, pool, componenti, pack, extra, regole di prezzo, prezzi a durata fissa, codici promozionali, convenzioni, policy, simulatore, matrice di listino, verifica di copertura, copia di stagione e configurazione guidata; migrazione `00002_inventory_catalog_pricing` per SQLite e PostgreSQL; nuovi tipi di errore `version_conflict`, `precondition_required`, `spot_has_future_allocations`, `not_implemented`; pannello con rotte per stabilimento (`/admin/e/{id}/...`), selettore dello stabilimento e menu Configurazione.

- Specifica tecnica e funzionale completa in `docs/spec/` (un file per sezione).
- Struttura del repository: monobinario Go (`cmd/arenile`), SPA pannello e booking in `web/` (npm workspaces), contratto API in `api/openapi.yaml`.
- Librerie di base: `internal/clock` (orologio iniettabile), `internal/money` (importi in centesimi, arrotondamento half-even, scorporo IVA), `internal/calendar` (date di calendario, intervalli, fasce AM/PM).
- CLI con i comandi `serve`, `migrate up|status`, `admin create`, `seed demo`, `backup --out`, `restore --in`, `version`.
- Configurazione da variabili d'ambiente (`ARENILE_*`, `DATABASE_URL`).
- Migrazioni per SQLite e PostgreSQL, autenticazione e ruoli, audit log, outbox eventi, job runner in-process.
- Motore prezzi e motore delle policy come librerie pure con test table-driven e di proprietà.
- Dockerfile multi-stage, `docker-compose.yml` e variante PostgreSQL.
- Makefile come unica interfaccia (`setup`, `dev`, `gen`, `lint`, `test`, `test-db`, `test-race`, `e2e`, `check`, `check-full`, `seed`, `fmt`) e CI su GitHub Actions.
- Supporto agli agenti AI: `AGENTS.md`, `CLAUDE.md`, `.claude/` con regole, skill e subagent.
- README tecnico in italiano, guide per gestori in `docs/guida/` (installazione, privacy e GDPR, nota fiscale).
- File di governance: licenza AGPL-3.0, CLA (bozza da validare), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1), `SECURITY.md`, `TRADEMARK.md`, modelli di issue e pull request.

### Changed

- Interfaccia senza CSS custom: un solo foglio di stile (`web/shared/src/styles/globals.css`, variabili del tema shadcn); rimossi `index.css` delle due app, `tokens.css`, le primitive scritte a mano e i token `--ar-*`. Il tema dello stabilimento sovrascrive le variabili shadcn (`--primary`, `--highlight`, `--radius`, `--app-font`) con lo stesso controllo del contrasto.
- Matrice di listino e linea temporale delle stagioni mostrano il nome del periodo tariffario invece del codice; il simulatore mostra l'effetto delle regole come importo o percentuale ("= 25,00 €", "−10%") invece del formato interno del motore.
- JS iniziale della pagina di prenotazione: 119,7 KB gzip su 150 (prima 118,3).
- Licenza cambiata da AGPL-3.0 a Business Source License 1.1 (BUSL-1.1), con conversione in AGPL-3.0-or-later tre anni dopo il rilascio di ogni versione (ADR 0007); aggiunto `.github/CODEOWNERS`.
- Roadmap estesa con M6 (edizione desktop, locale o client remoto) e M7 (app mobile), sezione 29 della specifica.
- `pkg/ext`: `PaymentProvider.ParseWebhook` riceve anche le credenziali decifrate dello stabilimento (`ParseWebhook(ctx, r, creds)`), diversamente dalla spec 13.1 ([ADR 0006](docs/adr/0006-deviazioni-m0-dalla-specifica.md)).
- `make test-race` compila con il tag `integration` e fallisce se non trova test `TestConcurrency_*`.
- L'editor della mappa usa un _roving tabindex_: la mappa è un solo punto di tabulazione (il primo elemento selezionato, altrimenti la prima postazione); Inizio/Fine e PagSu/PagGiù spostano fuoco e selezione tra gli elementi, le frecce lo fanno in sola lettura.

### Added (deviazioni e integrazioni alla specifica, ADR 0006)

- Modalità sviluppo `ARENILE_DEV=true` / `arenile serve --dev`: URL di base facoltativo e chiave segreta generata e salvata in `dev-secret.key`.
- Endpoint `POST /api/admin/v1/auth/totp/enroll` e `POST /api/admin/v1/auth/totp/confirm` per attivare la verifica in due passaggi.
- Creazione del primo owner all'avvio da `ARENILE_BOOTSTRAP_OWNER_EMAIL`, `_NAME`, `_ORG` e `ARENILE_ADMIN_PASSWORD` (solo se non esistono utenti), necessaria con l'immagine distroless senza shell.
- `compose.yaml` nella radice e `make up`: istanza locale vuota compilata dai sorgenti; i compose in `deploy/` hanno una sezione `build`.

### Fixed

- Selettore di date: l'elenco degli anni non andava oltre l'anno corrente, impedendo di scegliere date della stagione successiva.
- Campo importo: un valore non valido veniva cancellato all'uscita dal campo invece di restare visibile con l'errore.
- Un `PUT` della mappa senza header `If-Match` risponde 428 `precondition_required` invece di 400.
- `If-Match: *` è una precondizione soddisfatta da qualsiasi versione (RFC 9110): il salvataggio della mappa e l'applicazione del generatore la accettano invece di rispondere 428. La forma debole `W/"3"` resta accettata (proxy che comprimono le risposte, ADR 0008).
- Salvataggio della mappa e modifica o eliminazione di una zona si serializzano sulle righe delle zone coinvolte: su PostgreSQL non è più possibile che un salvataggio che aggiunge postazioni a una zona e la modifica della stessa zona in `quantity` vadano entrambi a buon fine (`TestConcurrency_ZoneUpdateVsSaveMap`); la capacità del pool implicito è calcolata dopo il lock.
- L'eliminazione di una mappa è rifiutata, come quella della zona, quando una zona a quantità della mappa ha pool creati a mano: prima i pool venivano cancellati in cascata senza audit, azzerando il `pool_id` dei componenti.
- Il simulatore con `season_id` esplicito rifiuta date fuori dalla stagione (errore sul campo `starts_on`) invece di calcolare un prezzo fuorviante o un 422 `no_price`.
- La verifica delle allocazioni future fallisce se il fuso orario dello stabilimento non è caricabile, invece di ripiegare su UTC (stessa regola delle policy).
- Il simulatore prezzi mostra il nome delle regole applicate invece del loro identificativo.
- La consegna dei webhook rivendica ogni tentativo con un lease: più istanze sullo stesso database non inviano due volte lo stesso webhook.
- Il recupero di una `Idempotency-Key` scaduta non può più cancellare la chiave appena rivendicata da una richiesta concorrente.
- Il segreto di firma degli endpoint webhook è cifrato con l'id dell'endpoint come dato associato (AAD).

[Unreleased]: https://github.com/coste360/arenile/compare/v1.0.3...main
[1.0.3]: https://github.com/coste360/arenile/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/coste360/arenile/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/coste360/arenile/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/coste360/arenile/releases/tag/v1.0.0
