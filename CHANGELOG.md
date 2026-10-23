# Changelog

Tutte le modifiche rilevanti di Arenile sono documentate in questo file.

Il formato segue [Keep a Changelog](https://keepachangelog.com/it-IT/1.1.0/) e il progetto adotta il [Versionamento Semantico](https://semver.org/lang/it/). I rilasci sono generati a partire dai messaggi [Conventional Commits](https://www.conventionalcommits.org/it/v1.0.0/). Ogni versione minore include, quando necessario, una sezione **Note di aggiornamento**.

## [Unreleased]

Milestone **M0 — Fondamenta**, **M1 — Inventario e mappa**, **M2 — Prezzi e policy** e **M3 — Prenotazioni dal
pannello e cassa** (completate, non rilasciate); prossima **M4 — Pagina pubblica e pagamenti online**.

### Added

- M6, launcher dell'app desktop in modalità locale (spec 29.1, `cmd/arenile-desktop`, `internal/desktop`): cartella
  dati per sistema operativo, `ARENILE_SECRET_KEY` generata al primo avvio nel portachiavi del sistema
  (`github.com/zalando/go-keyring`, MIT; fallback file 0600) e mai rigenerata per un archivio esistente (INV-54),
  migrazioni e server su `127.0.0.1` con porta libera (riusata ai riavvii), browser aperto sul pannello, istanza unica
  (il secondo avvio apre il pannello), icona nella barra di sistema con *Apri pannello*, *Backup ora*, *Apri cartella
  dati*, *Avvio automatico all'accensione* (LaunchAgent, chiave Run, voce XDG), *Modalità rete locale*, *Ripristina
  un backup…*, *Esci* (`fyne.io/systray`, Apache-2.0, solo con il tag `desktop`); backup giornaliero, alla chiusura e
  manuale con retention e cartella a scelta, ripristino con riavvio del server; controllo aggiornamenti solo se
  abilitato; log ruotati nella cartella dati. Primo avvio dal browser con token monouso e solo da loopback (INV-53),
  pagina *Configurazione → App desktop* e, nell'edizione desktop, spiegazione del passaggio a un'istanza online al
  posto di *Pagina pubblica* e *Pagamenti online*. `arenile backup|restore` e l'app desktop condividono ora lo stesso
  codice (`store.BackupSQLite`, `store.ValidateSQLiteBackup`, `store.RestoreSQLiteFile`).
- M6, modalità rete locale dell'app desktop (spec 29.1, `internal/desktop/lan`): certificato ECDSA P-256
  autogenerato nella cartella dati (825 giorni, impronta SHA-256 stabile ai riavvii e ai cambi di IP), listener
  HTTPS su tutte le interfacce che chiude prima del TLS le connessioni da fuori della rete locale, e middleware
  `LocalPeersOnly` attivo nell'edizione desktop come seconda difesa (INV-51). Pagina *Configurazione → Dispositivi →
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
  esiti memorizzati su file per non emettere due volte lo stesso documento (INV-52). Endpoint
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
  Dispositivi* con QR, codice, conto alla rovescia e revoca (INV-49, INV-50). Nuova dipendenza
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
- M4, contratto e scaffolding ([ADR 0013](docs/adr/0013-scelte-m4.md)): API pubblica completa in
  `api/openapi.yaml` (profilo con configurazione del flusso ed `ETag`, calendario, mappa pubblica,
  disponibilità online, catalogo, preventivo, prenotazione con sessione di checkout `X-Arenile-Checkout`,
  pagamento, QR, `.ics`, area gestione via link magico, richiesta di un nuovo link), webhook
  `POST /webhooks/payments/{provider}/{establishment_id}`, API del pannello per pagina pubblica, condizioni
  versionate, provider di pagamento (chiavi cifrate), email, registro messaggi, riconciliazione, reinvio della
  conferma e rigenerazione del link; nuovi tipi di errore `checkout_not_found`, `payment_provider_unavailable`,
  `webhook_signature_invalid`. Migrazione `00005_online_payments_notifications` (SQLite e PostgreSQL). Nuova
  variabile `ARENILE_MAIL_DIR`. Sito della documentazione `docs-site/` (Astro Starlight, italiano e inglese) con
  `make docs-dev`, `make docs-build`, `make docs-screenshots`. Le operazioni M4 rispondono `501` finché i builder
  non le completano.
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

[Unreleased]: https://github.com/coste360/arenile/commits/main
