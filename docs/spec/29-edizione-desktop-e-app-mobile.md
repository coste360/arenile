## 29. Edizione desktop e app mobile

Due modi in più per usare Arenile, pensati per chi lavora soprattutto in cassa e per lo staff in spiaggia. Entrambi riusano il core senza duplicarlo.

### 29.0 Modi di installazione

Ogni lido ha **un solo server**, che contiene l'archivio. Le soluzioni si distinguono per dove gira quel server; i client si collegano tutti allo stesso modo.

| Dove gira il server | Internet | Prenotazione online | Note |
|---|---|---|---|
| **Online**: VPS, hosting o Docker | necessaria | sì | La soluzione standard (sezione 24). |
| **Desktop locale**: app desktop sul PC della cassa | non necessaria | no | Sezione 29.1, con modalità rete locale per gli altri dispositivi. |
| **Server in sede**: mini PC, NAS o Raspberry Pi nel lido, con Docker | facoltativa | solo se esposto su internet (dominio o tunnel) | Stessa immagine dell'installazione online. |
| **Hosting gestito da Coste360 (Arenile Pro)** | necessaria | sì | Offerta commerciale futura per chi non vuole gestire un server, riservata al licenziante dalla BSL (sezione 26). |

**Client**, validi per ogni soluzione:
- il browser (pannello web, nessuna installazione);
- l'app desktop in modalità remota (29.1), con il ponte verso stampante e registratore telematico;
- l'app mobile (29.2).

**Esclusi:** i modelli ibridi con sincronizzazione tra due server, come desktop offline più VPS (vedi 29.1, *Limiti voluti*). Per passare da una soluzione all'altra si usano backup e `arenile restore`.

**Arenile Pro.** L'hosting gestito è un prodotto separato, con un repository privato distinto (`coste360/arenile-pro`). Comprende il pannello di controllo per creare e aggiornare le istanze, i backup centralizzati, i domini e la fatturazione. Il pro **non è un fork**: importa il core come modulo Go e lo estende solo attraverso i punti di estensione pubblici (`pkg/ext` e un costruttore pubblico dell'applicazione, da introdurre con un ADR quando servirà). Il core di questo repository resta completo e gratuito per chi gestisce da sé i propri stabilimenti. Nessuna funzione necessaria alla gestione di un lido è riservata al pro.

La documentazione (M4) presenta questa tabella nella pagina "Scegli come installare Arenile".

### 29.1 Edizione desktop (tutto in uno, senza server online)

**Obiettivo:** un lido che oggi usa il quaderno scarica un file, fa doppio clic e gestisce prenotazioni, mappa, abbonati e cassa dal PC della cassa, senza internet, senza VPS e senza configurazione.

**Cosa contiene**
- Lo stesso binario del server (core, migrazioni, SQLite, pannello e pagina pubblica incorporati) più un piccolo *launcher* con icona nella barra di sistema.
- Piattaforme: Windows (`.exe` con installer), macOS (`.app` in `.dmg`, Apple Silicon e Intel), Linux (AppImage).
- Dati nella cartella utente del sistema operativo (`%APPDATA%\Arenile`, `~/Library/Application Support/Arenile`, `~/.local/share/arenile`), non accanto all'eseguibile.

**Comportamento**
- All'avvio il launcher esegue le migrazioni, avvia il server su `127.0.0.1` con una porta libera e apre il pannello nel browser predefinito.
- Menu della barra di sistema: *Apri pannello*, *Backup ora*, *Apri cartella dati*, *Avvio automatico all'accensione*, *Esci*.
- Primo avvio: creazione guidata dell'utente owner dal browser (nessun comando da terminale), poi configurazione guidata (sezione 15.2).
- `ARENILE_SECRET_KEY` generata al primo avvio e salvata nel portachiavi del sistema operativo (fallback: file nella cartella dati con permessi ristretti). `ARENILE_BASE_URL` non è richiesto.
- Backup automatico giornaliero e alla chiusura, con retention configurabile, su una cartella a scelta (anche una chiavetta USB). Ripristino dal menu.
- Aggiornamenti: controllo della nuova versione **solo se il gestore lo abilita** (nessuna chiamata di rete di default, coerente con `ARENILE_TELEMETRY=false`).

**Limiti voluti**
- **Niente prenotazione online:** senza un server raggiungibile da internet la pagina pubblica e il widget non hanno senso. In modalità desktop la sezione "Pagina pubblica" del pannello spiega come passare a un'istanza online.
- **Niente sincronizzazione con un'istanza online.** Un modello ibrido richiederebbe la risoluzione dei conflitti e metterebbe in discussione la garanzia contro le doppie prenotazioni (sezione 7, ADR 0003). È escluso.
- Pagamenti: solo driver `manual`.

**Passaggio a un'istanza online:** il backup del desktop è lo stesso database SQLite dell'edizione server. `arenile restore --in FILE` su una VPS porta tutto online senza conversioni; da lì si attiva la pagina pubblica.

**Modalità rete locale (M6):** il gestore può esporre il server desktop sulla Wi-Fi del lido, così altri PC, tablet e telefoni dello staff lavorano sullo stesso archivio dal browser (e, da M7, dall'app mobile) senza internet. Esiste un solo server, quindi le garanzie contro le doppie prenotazioni restano quelle della sezione 7. Si attiva esplicitamente dal menu e usa TLS con un certificato autogenerato. Le postazioni sono abbinate tramite un QR code che contiene indirizzo, impronta del certificato e codice monouso (sezione 29.2); i browser lo usano per raggiungere il pannello, l'app per ottenere il token del dispositivo. Il menu mostra l'indirizzo locale e il QR code. Le connessioni dall'esterno della rete locale sono rifiutate.

**Modalità remota (client di un'istanza online o privata)**

Al primo avvio, e in seguito dalle impostazioni, il gestore sceglie tra **Locale** (quanto descritto sopra) e **Remota**. In modalità remota l'app non avvia il server interno: si collega a un'istanza Arenile esistente (VPS, server privato, anche raggiungibile solo via VPN) e mostra il pannello di quell'istanza.

- Collegamento con lo stesso meccanismo dell'app mobile: QR code o codice di abbinamento dal pannello, **token per dispositivo** revocabile (sezione 29.2). Nessun cookie di terze parti, nessun dato verso Coste360.
- Finestra dedicata, avvio automatico, modalità *kiosk* per il PC della cassa.
- **Ponte verso le periferiche locali:** stampante delle ricevute, lettore QR e registratore telematico collegati in USB o in rete locale, che un server remoto non può raggiungere. L'app espone all'istanza un canale autenticato verso il `FiscalDriver` locale (sezione 14.2), così l'emissione dei documenti avviene sul dispositivo della cassa anche quando il server è online.
- Funzionamento offline con le regole del pannello (sezione 21.2): consultazione in cache, check-in, note e incassi in contanti in coda; creazione e spostamento di prenotazioni solo con la connessione.
- Le due modalità sono **alternative, non sincronizzate**: il passaggio da Locale a Remota avviene con backup, `arenile restore` sul server e cambio di modalità nell'app.

**Architettura e build**
- Stesso repository e stesso modulo Go: il launcher è `cmd/arenile-desktop` e importa i package `internal/` del core (che Go non consente di importare da un altro modulo). Il core resta senza CGO; solo il launcher può usare CGO per l'icona nella barra di sistema, ed è compilato sui runner nativi di ciascun sistema operativo.
- Release con lo stesso tag dell'edizione server: binari firmati (Authenticode su Windows; Developer ID e notarizzazione Apple su macOS).
- Distribuzione: i pacchetti pronti si scaricano dal portale Coste360, previo login, da una pagina dedicata ai download per Windows, macOS e Linux. Non sono pubblicati come asset su GitHub: il repository contiene il codice e le istruzioni per compilarlo.
- Il pannello rileva la modalità desktop da un flag esposto dall'API (nessun fork del frontend).

### 29.2 App mobile (si collega a un'istanza Arenile)

**Obiettivo:** un'unica app pubblicata da Coste360 su App Store e Google Play, che ogni gestore collega alla **propria** istanza Arenile, sul modello di Matomo, Nextcloud o Home Assistant. È pensata per lo staff in spiaggia e per il gestore in movimento.

**Collegamento e autenticazione**
- L'app non ha un server predefinito: l'utente scansiona un QR code generato dal pannello (*Configurazione → Dispositivi*) oppure inserisce l'indirizzo dell'istanza e fa login.
- Il QR contiene indirizzo dell'istanza e codice di abbinamento monouso a breve scadenza; l'app lo scambia con un **token per dispositivo** legato all'utente, con i permessi del suo ruolo (sezione 16).
- I token per dispositivo sono elencati e revocabili dal pannello (nome del dispositivo, ultimo utilizzo). Niente cookie: autenticazione con header `Authorization`, quindi nessun requisito CORS né CSRF per l'app.
- L'istanza può essere una VPS del gestore, un hosting gestito, oppure l'edizione desktop in modalità rete locale (29.1).

**Privacy first**
- L'app parla solo con l'istanza del gestore: nessun dato passa da Coste360, nessuna telemetria, nessun SDK di analisi o pubblicità.
- Notifiche push: per vincolo tecnico di Apple e Google passano da un server centrale. Sono **facoltative**: l'istanza può usare un relay push di Coste360 che riceve solo un identificativo opaco del dispositivo e un testo generico ("Nuova prenotazione"), oppure nessuna push (l'app aggiorna i dati all'apertura).

**Funzioni (prima versione)**
- Vista operativa della mappa del giorno con colori per stato (sezione 11.3) e ricerca.
- Check-in con scansione del QR della prenotazione, incasso rapido in contanti, rilascio giorno degli abbonati, note.
- Funzionamento offline con le stesse regole del pannello (sezione 21.2): consultazione in cache, check-in, note e incassi in contanti messi in coda con chiavi di idempotenza; creazione e spostamento di prenotazioni solo online.
- Notifiche al gestore (sezione 19) se le push sono attive.

**Contratto e compatibilità**
- Repository separato (`coste360/arenile-mobile`, Expo + React Native + TypeScript): ciclo di rilascio legato agli store, toolchain diversa.
- Il client API è generato da `api/openapi.yaml`, pubblicato come asset di ogni release del core. L'app non importa codice del pannello.
- Il server espone versione e capacità (`GET /api/admin/v1/meta`); l'app dichiara la versione minima di Arenile supportata e mostra un messaggio chiaro se l'istanza è troppo vecchia.
- Per la revisione degli store si usa la demo pubblica azzerata ogni notte (sezione 26).
- Licenza dell'app: BSL 1.1 come il core (con conversione in AGPL-3.0 dopo 3 anni), salvo diversa decisione di Coste360 prima della pubblicazione (ad esempio proprietaria, consentito dal CLA).
