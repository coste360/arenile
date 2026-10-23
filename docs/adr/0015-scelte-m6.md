# 0015 — Scelte di M6: edizione desktop, token per dispositivo, meta, ponte fiscale

- **Stato:** accettato
- **Data:** 2026-10-23
- **Sezioni della specifica:** `docs/spec/29-edizione-desktop-e-app-mobile.md` (29.0, 29.1, 29.2),
  `docs/spec/28-roadmap-e-criteri-di-accettazione.md` (M6, M7 lato core), `docs/spec/16-utenti-ruoli-e-autenticazione.md`,
  `docs/spec/14-cassa-e-modulo-fiscale.md` (14.2), `docs/spec/22-sicurezza.md`, `docs/spec/24-deploy-configurazione-e-operativita.md`

## Contesto

M6 porta l'edizione desktop (launcher con server su 127.0.0.1, modalità rete locale, modalità remota con
ponte verso le periferiche fiscali) e, lato core, i token per dispositivo che userà anche l'app mobile (M7).
Quattro builder lavorano in parallelo a M4 (su `main`): il contratto va fissato prima e le modifiche ai file
condivisi devono restare minime.

## Decisione

1. **`GET /api/admin/v1/meta` è pubblico.** L'app lo legge prima dell'abbinamento (versione minima, edizione).
   Non contiene dati dello stabilimento né personali (la versione è già esposta da `/healthz`). Il pannello
   rileva da qui l'edizione desktop (`edition`, `desktop.mode` `local|lan`). La modalità remota dell'app
   desktop è un client e non è riportata dal server: la finestra dell'app inietta `window.arenileDesktop`.
2. **Token per dispositivo** `ar_dev_<prefix>_<secret>` (256 bit, SHA-256 in `devices.token_hash`), legato a
   un utente e a uno stabilimento: il principal è un utente con i grant **ristretti allo stabilimento del
   dispositivo** (`auth.NarrowGrants`), quindi un token rubato non apre gli altri stabilimenti dell'organizzazione.
   Vale come `Authorization: Bearer` sulle API admin (strategia sul prefisso: `ar_dev_` → dispositivi,
   altrimenti chiavi API) e sulle nuove API `/api/device/v1`, dove i cookie e le chiavi API non sono accettati.
   Bearer ⇒ nessun CSRF. Utente disattivato o senza ruolo sullo stabilimento ⇒ il token non autentica.
3. **Abbinamento.** Codice di 8 caratteri Crockford (`ABCD-EFGH`, 40 bit), 5 minuti, monouso con UPDATE
   condizionale, `POST /api/device/v1/pair` limitato a 10 tentativi al minuto per IP; codice inesistente,
   scaduto o usato ⇒ sempre `400 pairing_code_invalid`. Il QR contiene `arenile://pair?d=<base64url JSON>` con
   URL dell'istanza, codice e, solo con certificato autogenerato (rete locale), l'impronta SHA-256 che l'app
   usa per il pinning. Nell'edizione desktop senza rete locale ⇒ `409 lan_mode_required`. Né il codice né il
   token passano dalla tabella di idempotenza (`/api/device/` escluso; `createDevicePairingCode` non accetta
   `Idempotency-Key`).
4. **API desktop nel contratto.** Primo avvio, impostazioni, backup e ripristino sono in `api/openapi.yaml`
   (tag `desktop`, `/api/desktop/v1`) perché il pannello li chiama con il client generato, ma esistono solo
   nell'edizione desktop: sul server rispondono `404`. Sul desktop il prefisso è protetto da `LoopbackOnly`
   (peer TCP e `Host` di loopback, contro DNS rebinding), sessione e CSRF. Il primo avvio è l'unico percorso
   non autenticato: richiede il token monouso generato dal launcher e passato nel frammento dell'URL.
   Il ripristino è pianificato al riavvio del server interno (il database è in uso).
5. **Ponte fiscale con long polling in uscita.** L'app desktop in modalità remota, abbinata come dispositivo,
   registra il ponte (`PUT /api/device/v1/fiscal-bridge`, uno attivo per stabilimento) e attende i lavori con
   `GET .../jobs?wait=25` (sotto il `WriteTimeout` di 60 s, attraversa proxy e VPN senza tarature), li esegue sul
   `FiscalDriver` locale e invia l'esito (idempotente). Consegna almeno una volta con lease di 60 s; il driver
   locale deduplica con `idempotency_key`. Lato istanza il driver fiscale `bridge` (registry) accoda in modo
   idempotente e risponde con un errore *deferred*: `fiscal.IssuePending` lascia il documento `pending` finché
   l'esito non arriva (capacità `Deferred() bool`, senza modificare `pkg/ext`).
6. **Launcher senza CGO di default.** `cmd/arenile-desktop` compila senza tag con un'interfaccia headless
   (CI, `go build ./...`); la tray e la finestra stanno dietro il tag `desktop` (CGO, runner nativi). Il runtime
   condiviso è `internal/desktop` (dati per OS, portachiavi con fallback su file 0600, impostazioni, backup,
   TLS della rete locale con listener che rifiuta i peer non privati, primo avvio); il core riceve solo la porta
   `internal/desktop/control` con `app.WithDesktop`. Il listener della rete locale è servito con `App.ServeListener`.
7. **Migrazione `00006_devices_fiscal_bridge`** (entrambi i dialetti): `devices`, `device_pairing_codes`,
   `fiscal_bridges` (indice unico parziale sull'attivo), `fiscal_bridge_jobs` (unico su stabilimento, tipo,
   chiave di idempotenza).

## Conseguenze

- Nuovi tipi di errore: `pairing_code_invalid`, `lan_mode_required`, `loopback_required`, `setup_token_invalid`,
  `setup_completed`. `Me` ha il campo facoltativo `device`.
- I pacchetti firmati (Authenticode, Developer ID e notarizzazione) richiedono certificati e runner nativi: il job
  di packaging va nell'artefatto `portal-bundle-<tag>`, mai come asset GitHub.
- Le operazioni M6 sono implementate (nessun `501` residuo); il token per dispositivo fallisce chiuso.

## Implementazione (integrazione M6, 2026-10-23)

Decisioni prese durante l'implementazione, a complemento dei punti sopra.

8. **Launcher nello stesso modulo Go.** `cmd/arenile-desktop` usa lo stesso bootstrap `app.New` di
   `arenile serve` (migrazioni comprese): un solo archivio SQLite, un solo formato di backup, nessun fork del
   core. Porte e adattatori: `internal/desktop` definisce le porte `UI`, `LANMode` e `RemoteMode`;
   `internal/desktop/lan` e `internal/desktop/remote` le implementano e `main.go` le collega
   (`NewLAN: lan.New`, `Remote: remote.New(...)`), così `desktop` non importa gli adattatori.
9. **Tag di build e librerie native.** Senza tag: UI headless, nessun CGO (CI, `go build ./...`). Con
   `-tags desktop`: icona nella barra di sistema `fyne.io/systray` (Apache-2.0) e, su macOS e Windows (CGO,
   runner nativi), finestra della modalità remota con il webview di sistema `github.com/webview/webview_go`
   (MIT; WKWebView, WebView2). Su Linux il webview è escluso (`!linux`): collegherebbe WebKitGTK 4.0, che Ubuntu
   24.04, Debian 13 e Fedora non distribuiscono più, e l'AppImage fallirebbe nel loader anche in modalità locale;
   la build tray Linux è quindi senza CGO (binario statico), la modalità remota usa il browser e l'abbinamento
   avviene con `-pair`. I pacchetti del portale usano la build tray su tutti e tre i sistemi (`desktop.yml`,
   `DESKTOP_TAGS=desktop`).
10. **Chiave segreta nel portachiavi del sistema** (`github.com/zalando/go-keyring`, MIT; Keychain, Credential
    Manager, Secret Service) con account derivato dalla cartella dati e fallback su file 0600. Con un archivio
    esistente e nessuna chiave l'avvio si ferma invece di generarne una nuova (INV-60).
11. **Rete locale: TLS autogenerato e doppio filtro sugli indirizzi.** Certificato ECDSA P-256 di 825 giorni
    nella cartella dati, impronta stabile ai riavvii e ai cambi di IP. Il listener chiude prima dell'handshake
    le connessioni da indirizzi non privati (RFC 1918, link-local, ULA, loopback; CGNAT 100.64/10 rifiutato) e il
    middleware `LocalPeersOnly` ripete il controllo per richiesta (INV-57). Niente mDNS (nessuna dipendenza). Il
    QR del pannello (`desktop.lan_qr_svg` in `/meta`) contiene solo l'indirizzo, per la fotocamera dei browser;
    l'impronta viaggia nel QR di abbinamento, dove l'app la usa per il pinning.
12. **Token Bearer accanto ai cookie.** Il middleware sceglie la strategia dal prefisso; CSRF solo per i cookie;
    `last_used_at` scritto al massimo una volta al minuto; revoca efficace dalla richiesta successiva (nessuna
    cache). L'abbinamento crea dispositivo, consuma il codice e scrive l'audit nella stessa transazione
    (INV-55, INV-56).
13. **Ponte fiscale.** Coda `fiscal_bridge_jobs` con lease, long polling risvegliato da un notifier in processo,
    esiti idempotenti, ritentativi con attesa crescente, lease rilasciati alla scadenza o alla revoca del
    dispositivo (INV-58). Lato cassa l'agente salva gli esiti su file per non stampare due volte un lavoro
    riconsegnato. Driver selezionato esplicitamente con `settings.fiscal.driver = "bridge"`.
14. **Abbinamento della modalità remota anche da riga di comando**: `arenile-desktop -pair <QR|URL>
    [-pair-code ...] [-pair-fingerprint ...]` salva token e configurazione e passa la cartella dati in modalità
    remota (utile per installazioni gestite e per i PC senza fotocamera).
15. **Pacchetti**: macOS `.app` + `.dmg` universale, Windows Inno Setup (per utente, senza amministratore),
    Linux AppImage su `ubuntu-22.04` (glibc vecchia). Firme solo se i segreti esistono, altrimenti suffisso
    `-unsigned`; tutto nell'artefatto `portal-bundle-<tag>`, mai come asset di una release GitHub
    (`docs/desktop-build.md`).
16. **Durata del token per dispositivo.** Il token non ha scadenza assoluta (un telefono della cassa resta abbinato
    per la stagione) ma scade dopo **90 giorni senza uso** (`device.IdleExpiry`, contati da `last_used_at` o
    dall'abbinamento): un dispositivo dimenticato in un cassetto non conserva i permessi del titolare per sempre.
    Con il token non si chiede un secondo fattore (spec 29.2: "permessi del suo ruolo"; l'abbinamento nasce da una
    sessione già autenticata con TOTP) e le operazioni sensibili (team, impostazioni, revoca di altri dispositivi)
    non sono per ora ristrette ai dispositivi: le mitigazioni sono il restringimento allo stabilimento, la scadenza
    per inattività e la revoca immediata dal pannello. Un eventuale restringimento si deciderà con M7, quando l'app
    mobile definirà quali operazioni le servono davvero.
17. **Modalità remota: deviazioni dalla sezione 29.1 (da sanare in M7).** La finestra del pannello si autentica con
    la **sessione utente** dell'istanza (login con password e, se attivo, TOTP) attraverso il proxy loopback; il token
    per dispositivo serve all'abbinamento, al ponte fiscale e allo scollegamento, non sostituisce la sessione della
    finestra. La **coda offline** della modalità remota (29.1 e 21.2) non è implementata: senza connessione la
    finestra non funziona e i lavori fiscali restano sul server. Il proxy riscrive `Origin` e `Referer` verso
    l'origine dell'istanza (così i controlli CSRF e di origine dell'istanza vedono il pannello come proprio) e
    riporta `Location` sull'origine del proxy. La guida utente lo dichiara.
18. **Numerazione e ordine di integrazione.** La migrazione di M6 è `00006_devices_fiscal_bridge`; `m5-season` usa
    `00007`. M6 va unita e rilasciata **prima** di M5A: un database già a `00007` incontrerebbe la migrazione
    mancante `00006` (errore di goose); in alternativa M6 va rinumerata al momento del merge. ADR: 0013 e 0014 = M4,
    0015 = M6, 0016 = M5A. Invarianti M6: INV-55..INV-60 (INV-48 è del kit mappa, INV-49..INV-54 di M4).
19. **Il pannello in una finestra propria, non nel browser** (decisione del product owner, 2026-10-23: "tutto dentro
    l'app"; modifica la sezione 29.1, che prevedeva il browser predefinito). In modalità locale e rete locale il
    launcher apre il pannello nel webview di sistema con lo stesso `github.com/webview/webview_go` della modalità
    remota (nessuna nuova dipendenza): WKWebView su macOS, WebView2 su Windows. Scartati Electron/CEF (centinaia di MB,
    un secondo motore da aggiornare) e Wails/Tauri (un altro framework e un altro ciclo di build per una sola finestra).
    `webview_go` non dà menu, chiusura senza distruzione, download né stampa: li aggiunge `internal/desktop/shell`,
    diviso in nucleo puro testato su ogni sistema (stato della finestra, menu come dati e collegamento ai comandi,
    comportamento alla chiusura, nomi dei file scaricati, politica dei link esterni) e strato nativo minimo: su macOS
    uno shim Objective-C di circa 300 righe con sole API pubbliche (menu principale; `windowShouldClose:`,
    `applicationShouldHandleReopen:` e `applicationShouldTerminate:` aggiunti alle classi create da `webview_go`;
    `WKNavigationDelegate` che trasforma allegati e `<a download>` in `WKDownload` verso `~/Downloads`;
    `NSPrintOperation` per `window.print`), su Windows solo Win32 da Go (`golang.org/x/sys`: sottoclasse di
    `WM_CLOSE`, `Get/SetWindowPlacement`, primo piano), perché WebView2 gestisce già download, stampa, zoom e
    scorciatoie. L'icona della barra di sistema resta (`fyne.io/systray` con `RunWithExternalLoop` su macOS, nel ciclo
    di eventi della finestra; su Windows nel proprio thread). Chiusura: su macOS la finestra si nasconde (convenzione
    della piattaforma), su Windows chiude l'app salvo `keep_running_in_tray` (falsa di default, consigliata con la rete
    locale; impostazione desktop nel contratto). Istanza unica: listener di attivazione su loopback con token casuale in
    `instance.json` (0600), il secondo avvio chiede di mostrare la finestra e termina. Prima di chiudere il ciclo di
    eventi la pagina passa ad `about:blank`, così il flusso SSE del pannello non trattiene lo spegnimento del server.
    Linux resta sul browser (punto 9, WebKitGTK 4.0); il browser resta disponibile ovunque (*Apri nel browser*,
    `-browser`) e come ripiego se manca WebView2.
20. **Windows ARM64 nativo.** I PC Snapdragon (Surface Pro) eseguirebbero la build x64 in emulazione; il pacchetto
    `windows_arm64` è costruito sul runner GitHub `windows-11-arm` con CGO tramite **llvm-mingw** (host e target
    aarch64, la stessa toolchain dei builder windows/arm64 di Go; versione e sha256 fissati nel workflow), scelto
    rispetto a MSVC (cgo non lo supporta) e a zig (toolchain meno collaudata per C++ con WebView2). Inno Setup gira in
    emulazione x86 e produce un installer `ArchitecturesAllowed=arm64`; firma con gli stessi passi della build x64.
