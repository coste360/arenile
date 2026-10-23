<div align="center">

# Arenile

**Il gestionale balneare gratuito per stabilimenti balneari e lidi, con codice sorgente disponibile.**
Mappa ombrelloni interattiva, prenotazione ombrelloni online e prenotazioni senza commissioni, listini prezzi, cassa e abbonamenti stagionali: tutto in un unico binario self-hosted.

[![Licenza: BSL 1.1](https://img.shields.io/badge/licenza-BSL%201.1-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-TypeScript-61DAFB?logo=react&logoColor=black)](https://react.dev)
[![Docker](https://img.shields.io/badge/Docker-ghcr.io%2Fcoste360%2Farenile-2496ED?logo=docker&logoColor=white)](https://github.com/coste360/arenile/pkgs/container/arenile)
[![SQLite | PostgreSQL](https://img.shields.io/badge/DB-SQLite%20%7C%20PostgreSQL-003B57?logo=sqlite&logoColor=white)](#architettura)
[![CI](https://github.com/coste360/arenile/actions/workflows/ci.yml/badge.svg)](https://github.com/coste360/arenile/actions)
[![Stato: M0-M3 completate, M4 in arrivo](https://img.shields.io/badge/stato-M0%E2%80%93M3%20prenotazioni%20e%20cassa-orange.svg)](#roadmap)

[Perché Arenile](#gestionale-per-stabilimenti-balneari-gratuito-e-a-codice-sorgente-disponibile) ·
[Funzionalità](#funzionalità) ·
[Installazione](#installazione-in-5-minuti) ·
[Widget](#incorporare-il-widget-nel-tuo-sito) ·
[Architettura](#architettura) ·
[Sviluppo](#sviluppo) ·
[Roadmap](#roadmap) ·
[FAQ](#domande-frequenti)

</div>

---

> [!IMPORTANT]
> **Stato del progetto: fondamenta (M0), inventario e mappa (M1), prezzi e policy (M2), prenotazioni dal pannello e cassa (M3) completati nel ramo di sviluppo (non ancora pubblicato); prossima milestone M4 (pagina di prenotazione online e pagamenti con Stripe). Nessuna versione ancora rilasciata.**
> Arenile non è ancora pronto per la produzione. Le funzionalità descritte in questa pagina sono la roadmap della [specifica tecnica](docs/spec/README.md): la [tabella di stato](#stato-delle-funzionalità) indica con precisione cosa è disponibile, cosa è in sviluppo e cosa è pianificato. Se gestisci un lido e vuoi provarlo per la stagione, segui il repository (*Watch → Releases*) per essere avvisato al primo rilascio.

**Arenile** è un **software gratuito per la gestione di stabilimenti balneari, con codice sorgente disponibile**, lidi, spiagge attrezzate e piscine. Nasce per sostituire quaderno, fogli Excel e gestionali chiusi con uno strumento moderno che il gestore installa sul proprio server in pochi minuti, con i propri dati e senza pagare commissioni a nessuno su ogni prenotazione.

A regime un solo programma comprenderà il **pannello del gestore** (mappa operativa della spiaggia, prenotazioni, cassa, clienti, abbonamenti, report) e la **pagina di prenotazione online di ombrelloni e lettini**, incorporabile in qualsiasi sito web con un `iframe` o con uno script di una riga. Cosa è già disponibile oggi è indicato nella [tabella di stato](#stato-delle-funzionalità).

È sviluppato da [Coste360 SRL](https://coste360.it), la piattaforma italiana per il monitoraggio delle coste e delle concessioni demaniali marittime.

---

## Gestionale per stabilimenti balneari gratuito e a codice sorgente disponibile

Perché scegliere Arenile come software di gestione dello stabilimento balneare, al posto di un gestionale lido a canone o di un portale che trattiene una percentuale su ogni prenotazione:

| | |
|---|---|
| **Gratuito, davvero** | Gratuito per gli stabilimenti che lo usano per sé: nessun canone, nessuna versione "Pro" a pagamento, nessun limite di postazioni, prenotazioni o utenti. Codice sorgente disponibile con licenza BSL 1.1, che diventa AGPL-3.0 dopo 3 anni. |
| **Zero commissioni sulle prenotazioni** | A differenza dei portali di prenotazione a commissione, Arenile non trattiene nulla. I pagamenti online passano dal **tuo account Stripe**: paghi solo le commissioni del circuito di pagamento, nessuna percentuale alla piattaforma. |
| **I dati sono del gestore** | Clienti, prenotazioni e incassi stanno nel tuo database. Esportazione completa sempre disponibile, backup con un comando. |
| **Installabile in 5 minuti** | Un binario, un'immagine Docker, un volume. Gira su un VPS da pochi euro al mese, su un mini PC o su un Raspberry Pi. |
| **Nessun servizio esterno obbligatorio** | Niente Redis, code esterne o object storage. SQLite incluso; PostgreSQL facoltativo. Anche email e pagamenti online sono opzionali. |
| **Configurabile senza codice** | File, zone, pack, listini, periodi tariffari e policy di cancellazione si esprimono come dati, non come condizioni nel codice. Ogni lido è diverso, il software si adatta. |
| **Il tuo sito, il tuo marchio** | Il widget di prenotazione si integra nel sito del lido (WordPress, Wix, sito statico) con i tuoi colori e il tuo logo. Il cliente prenota da te, non su un portale di terzi. |
| **Aperto ed estendibile** | API REST documentate in OpenAPI, webhook firmati e punti di estensione Go per nuovi driver di pagamento, fiscali e di notifica. |

---

## Funzionalità

Panoramica delle aree funzionali previste dalla specifica. Lo stato di ciascuna è nella [tabella di stato](#stato-delle-funzionalità).

### Mappa ombrelloni interattiva
- **Editor della mappa della spiaggia** con griglia e snapping, generatore di file ("8 file da 15 ombrelloni, numerazione A–Z"), rotazione, allineamento, annulla/ripeti.
- Postazioni di ogni tipo: ombrelloni, gazebo, tende, palme; elementi decorativi (passerelle, docce, bar, torretta).
- Tre modalità di assegnazione per zona: **posto esatto** scelto sulla mappa, **zona** con assegnazione automatica o manuale, **quantità** per inventari senza posizione (es. lettini sfusi).
- **Vista operativa del giorno** con colori per stato (libero, prenotato online, prenotato in cassa, abbonato, abbonato assente, da incassare, presente, bloccato), ricerca per cliente o codice, trascinamento delle prenotazioni da un posto all'altro.
- Aggiornamento in tempo reale tra cassa, spiaggia e prenotazioni online (Server-Sent Events).
- Sfondo opzionale: planimetria caricata dal gestore o perimetro della concessione demaniale marittima importato da un `ConcessionDataProvider`.

### Prenotazioni online di ombrelloni e lettini
- Pagina pubblica di **booking ombrelloni e lettini** a `https://tuo-dominio/b/{slug}`, ottimizzata per smartphone (budget < 150 KB di JavaScript, funzionante da 320 px).
- Flusso in quattro passi: quando, dove (mappa o zone), cosa (pack ed extra), chi e pagamento. Nessun account richiesto al cliente.
- Mezza giornata (mattina/pomeriggio), giornaliero, multi-giorno, settimana, quindicina, mese, stagione.
- Blocco temporaneo del posto durante il checkout: **nessuna doppia prenotazione** tra cassa e online, garantita da vincoli del database.
- Link magico per il cliente: visualizza la prenotazione, paga il saldo, modifica o cancella secondo la policy, file `.ics` per il calendario, QR code.
- Lista d'attesa con notifica quando un posto si libera.

### Prezzi e listini
- **Motore dei prezzi** come funzione pura: periodi tariffari (bassa, media, alta, Ferragosto), giorni della settimana, festività italiane incluse più patrono locale, fila e categoria di prezzo, canale, durata, anticipo.
- **Listino prezzi ombrelloni** a durata fissa per settimana, mese e stagione; offerte come "7 giorni al prezzo di 6".
- Codici promozionali, convenzioni (hotel, associazioni), sconti automatici.
- Prezzi IVA inclusa, aliquote impostate dal gestore, arrotondamento configurabile.
- **Simulatore** e **matrice di listino** per verificare i prezzi prima di pubblicarli, controllo dei "buchi" di listino, copia di stagione.

### Policy di prenotazione
- Anticipo minimo e massimo, orario limite per prenotare in giornata.
- Formule consentite per periodo (es. ad agosto online solo settimane da sabato a sabato).
- Pagamento totale, acconto o nessun pagamento online; scadenza del saldo.
- **Cancellazione a scaglioni** con rimborso o credito, policy meteo, approvazione manuale per gazebo o richieste particolari.
- Le policy applicate vengono salvate con la prenotazione: rimborsi verificabili anche se le regole cambiano dopo.

### Pagamenti
- Driver **manuale**: nessun pagamento online, saldo alla cassa.
- Driver **Stripe con il proprio account**: Stripe Checkout (robusto anche dentro un iframe) o Payment Element incorporato. Le chiavi sono del gestore e cifrate a riposo.
- Conferma solo da webhook firmati, deduplica degli eventi, riconciliazione automatica dei pagamenti in sospeso, rimborsi e crediti cliente.
- Interfaccia `PaymentProvider` aperta per altri circuiti (es. Satispay) come contributi.

### Cassa per lido e modulo fiscale
- **Cassa** con apertura (fondo cassa), incassi per metodo, chiusura con conteggio e differenza, report PDF.
- Incasso rapido dalla mappa, vendita libera senza prenotazione ("lettino extra al volo").
- Modulo fiscale con interfaccia `FiscalDriver`: driver `none` (usi il tuo registratore telematico) ed `export` dei corrispettivi giornalieri per aliquota e metodo in CSV e PDF per il commercialista. Vedi la [nota fiscale](#nota-fiscale).

### Gestione abbonamenti stagionali
- Abbonamenti stagionali con diritto di prelazione sul posto l'anno successivo.
- **Rilascio dei giorni** da parte dell'abbonato (anche dal link magico), con rivendita del posto ed eventuale credito.
- **Rinnovi** proposti in blocco a fine stagione, con hold sulla stessa postazione e prezzo del nuovo listino.

### Multi-stabilimento e utenti
- **Più stabilimenti nella stessa installazione** (organizzazioni con più lidi), con isolamento dei dati verificato da test automatici.
- Ruoli: titolare, responsabile, cassiere, personale di spiaggia, sola lettura.
- Password Argon2id, TOTP opzionale (obbligatorio per i titolari se richiesto), chiavi API con scope.
- Lingue: italiano, inglese, tedesco, francese.

### Notifiche
- Email via il tuo server SMTP (nessun provider imposto): conferma, promemoria saldo, promemoria del giorno prima, cancellazione, rimborso, proposta di rinnovo, posto libero dalla lista d'attesa.
- Template modificabili in Markdown con variabili, in tutte le lingue attive.
- Interfaccia `Notifier` per canali aggiuntivi (WhatsApp, SMS) come estensioni.

### API e webhook
- **API REST** `/api/admin/v1` e `/api/public/v1` descritte in `api/openapi.yaml` (OpenAPI 3.1), errori RFC 9457, `Idempotency-Key`, paginazione a cursore.
- **Webhook in uscita** firmati HMAC-SHA256 (`Arenile-Signature`), con tentativi e log delle consegne.
- Punti di estensione Go stabili in `pkg/ext`: provider di pagamento, driver fiscali, notifiche, autenticazione SSO, protezione anti-bot, plugin UI del pannello.

### Privacy, sicurezza e offline
- **GDPR**: dati minimi, consensi versionati, esportazione JSON dei dati del cliente, anonimizzazione su richiesta e automatica dopo N anni. Vedi la [guida privacy](docs/guida/privacy-gdpr.md).
- Segreti cifrati con AES-GCM, CSP e `frame-ancestors` per stabilimento, limitazione delle richieste, log senza email e telefoni in chiaro.
- **PWA offline** del pannello: consultazione della mappa del giorno e operazioni a basso rischio (check-in, note, incassi in contanti) anche senza connessione, sincronizzate al ritorno della rete.

---

## Stato delle funzionalità

Legenda: **Disponibile** = rilasciato e utilizzabile · **Completato** = implementato e coperto da test nel ramo di sviluppo, non ancora pubblicato né rilasciato · **In sviluppo** = lavori in corso nella milestone indicata · **Pianificato** = previsto dalla specifica, non ancora iniziato.

Ad oggi **nessuna versione è stata rilasciata**. Nel ramo di sviluppo sono completate le fondamenta (M0), l'inventario con l'editor della mappa e la configurazione guidata (M1), la configurazione di prezzi e policy (M2) e il **gestionale operativo del lido** (M3): dal pannello si prenotano ombrelloni e lettini sulla mappa operativa o sul calendario planning, si gestiscono clienti, abbonamenti stagionali e rilasci dei giorni, si incassa in contanti o con carta nella cassa con apertura e chiusura, e si esportano i corrispettivi per il commercialista. La mappa si aggiorna in tempo reale su tutte le postazioni di lavoro. La prenotazione online da parte dei clienti e i pagamenti con Stripe arrivano con M4.

| Area | Funzionalità | Milestone | Stato |
|---|---|:-:|---|
| Progetto | Specifica tecnica completa, licenza, governance, template issue/PR | M0 | Disponibile |
| Fondamenta | Struttura del repository, Makefile, CI, Dockerfile, docker compose | M0 | Completato |
| Fondamenta | Configurazione da variabili d'ambiente, CLI (`serve`, `migrate`, `admin`, `seed`, `backup`, `restore`, `version`) | M0 | Completato |
| Fondamenta | Migrazioni SQLite e PostgreSQL, autenticazione e ruoli, audit log, outbox eventi, job runner | M0 | Completato |
| Fondamenta | Contratto OpenAPI iniziale, SPA pannello e booking di base incorporate nel binario | M0 | Completato |
| Agenti AI | `AGENTS.md`, `CLAUDE.md`, `.claude/` (regole, skill, subagent), devcontainer | M0 | Completato |
| Prezzi | Motore prezzi come libreria pura (regole, condizioni, azioni, IVA, arrotondamento) | M0/M2 | Completato |
| Policy | Motore delle policy come libreria pura (anticipo, durate, acconti, cancellazioni) | M0/M2 | Completato |
| Inventario | Più stabilimenti dal pannello (creazione e selettore), impostazioni e tema, stagioni, periodi tariffari, chiusure, utenti e ruoli | M1 | Completato |
| Mappa | Editor della mappa (annulla/ripeti, allineamento, rinumerazione, conflitti di versione), generatore di file, zone, postazioni, decorazioni, pool | M1 | Completato |
| Catalogo | Componenti, pack, extra; configurazione guidata con 4 modelli e listino rapido | M1 | Completato |
| Prezzi | Regole di prezzo, prezzi a durata fissa, codici promo, convenzioni, policy di prenotazione, simulatore, matrice di listino (esportazione CSV), verifica di copertura, copia di stagione | M2 | Completato |
| Prenotazioni | Motore di disponibilità con allocazioni, hold, pool e quote; prenotazioni dal pannello con anteprima del prezzo, modifiche, spostamenti, cancellazioni con rimborso o credito | M3 | Completato |
| Pannello | Pagina "Oggi", vista operativa della mappa, calendario planning, clienti (ricerca, esportazione e anonimizzazione GDPR), abbonamenti e rilasci con rivendita | M3 | Completato |
| Cassa | Pagamenti manuali, rimborsi, vendite di cassa, cassa con apertura/chiusura e riepilogo Z stampabile, esportazione corrispettivi CSV/JSON | M3 | Completato |
| Tempo reale | Aggiornamento della mappa via SSE con ripresa dopo la riconnessione (`Last-Event-ID`) | M3 | Completato |
| Pagina pubblica | Booking online (posto, zona, quantità), widget `embed.js`, tema | M4 | Pianificato |
| Pagamenti | Driver Stripe (Checkout), webhook, riconciliazione, rimborsi | M4 | Pianificato |
| Clienti | Link magico, email transazionali, QR, `.ics`, assegnazione automatica | M4 | Pianificato |
| Stagione | Rinnovi abbonamenti, lista d'attesa, crediti | M5 | Pianificato |
| Report | Report completi, confronto anni, esportazioni CSV/XLSX | M5 | Pianificato |
| Offline | PWA del pannello con offline limitato | M5 | Pianificato |
| Integrazioni | Webhook in uscita configurabili, chiavi API | M5 | Pianificato |
| Lingue | Traduzioni inglese, tedesco, francese | M5 | Pianificato |
| Dopo l'MVP | Satispay, registratori telematici, WhatsApp/SMS, app staff, bar con addebito sulla postazione | — | Pianificato |

---

## Installazione in 5 minuti

> [!NOTE]
> Le istruzioni seguenti descrivono l'installazione prevista al termine di M0. Fino al primo rilascio l'immagine `ghcr.io/coste360/arenile` potrebbe non essere ancora pubblicata: in quel caso compila dai sorgenti (vedi [Sviluppo](#sviluppo)).

**Requisiti:** un server Linux (`amd64` o `arm64`) con Docker e Docker Compose, un dominio che punti al server. Bastano 1 vCPU e 512 MB di RAM per un lido.

**Prova in locale in un minuto** (compila l'immagine dai sorgenti, nessuna configurazione):

```bash
git clone https://github.com/coste360/arenile.git && cd arenile
ARENILE_BOOTSTRAP_OWNER_EMAIL=titolare@lido.test ARENILE_BOOTSTRAP_OWNER_NAME="Mario Rossi" \
ARENILE_BOOTSTRAP_OWNER_ORG="Lido Prova" ARENILE_ADMIN_PASSWORD=password-di-prova \
  docker compose up --build        # oppure: make up
```

Poi apri `http://localhost:8080/admin/` e accedi con le credenziali indicate. Il file `compose.yaml` nella radice usa la modalità sviluppo: non usarlo in produzione.

**Installazione su un server**

**1. Crea il file `docker-compose.yml`** (oppure usa direttamente [`deploy/docker-compose.yml`](deploy/docker-compose.yml), che compila l'immagine dai sorgenti se non è ancora pubblicata)

```yaml
services:
  arenile:
    image: ghcr.io/coste360/arenile:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - arenile-data:/data
    environment:
      ARENILE_BASE_URL: "https://prenota.mio-lido.it"
      ARENILE_SECRET_KEY: "${ARENILE_SECRET_KEY}"
      ARENILE_SMTP_URL: "${ARENILE_SMTP_URL}"
      # Primo utente owner, creato all'avvio solo se il database non ha utenti
      ARENILE_BOOTSTRAP_OWNER_EMAIL: "${ARENILE_BOOTSTRAP_OWNER_EMAIL}"
      ARENILE_BOOTSTRAP_OWNER_NAME: "${ARENILE_BOOTSTRAP_OWNER_NAME}"
      ARENILE_BOOTSTRAP_OWNER_ORG: "${ARENILE_BOOTSTRAP_OWNER_ORG}"
      ARENILE_ADMIN_PASSWORD: "${ARENILE_ADMIN_PASSWORD}"
volumes:
  arenile-data:
```

**2. Genera la chiave di cifratura e avvia**

```bash
echo "ARENILE_SECRET_KEY=$(openssl rand -base64 32)" > .env
echo "ARENILE_SMTP_URL=" >> .env          # facoltativo: smtp://utente:password@host:587
cat >> .env <<'FINE'
ARENILE_BOOTSTRAP_OWNER_EMAIL=titolare@mio-lido.it
ARENILE_BOOTSTRAP_OWNER_NAME=Mario Rossi
ARENILE_BOOTSTRAP_OWNER_ORG=Lido Esempio
ARENILE_ADMIN_PASSWORD=scegli-una-password-lunga
FINE
docker compose up -d
```

> [!WARNING]
> Conserva `ARENILE_SECRET_KEY` insieme ai backup: cifra le chiavi Stripe e gli altri segreti. Senza di essa i segreti salvati non sono recuperabili.

**3. Primo utente (titolare).** Con le variabili `ARENILE_BOOTSTRAP_OWNER_*` del passo 2 l'owner viene creato al primo avvio (solo se il database non ha ancora utenti; dopo il primo accesso puoi togliere `ARENILE_ADMIN_PASSWORD` da `.env`). In alternativa crealo a mano: l'immagine è distroless e non ha una shell, ma `exec` lancia direttamente il binario.

```bash
docker compose exec -e ARENILE_ADMIN_PASSWORD='scegli-una-password-lunga' arenile \
  /arenile admin create --email titolare@mio-lido.it --name "Mario Rossi" --org "Lido Esempio"
```

**4. Accedi al pannello** su `https://prenota.mio-lido.it/admin/`. Oggi il pannello offre l'accesso (con verifica in due passaggi TOTP facoltativa), la creazione e la scelta degli stabilimenti, la configurazione guidata, l'editor della mappa, il catalogo, i listini, le policy e il simulatore prezzi; prenotazioni, cassa e pagina pubblica completa arrivano con le milestone successive (vedi [Roadmap](#roadmap)).

Per provare con dati dimostrativi: `docker compose exec arenile /arenile seed demo`.

Per HTTPS puoi mettere Arenile dietro un reverse proxy (Caddy, Traefik, Nginx) oppure far gestire i certificati al binario con `ARENILE_TLS_AUTOCERT_DOMAINS`: in questo caso Arenile ascolta direttamente sulle porte 80 e 443, che vanno pubblicate nel compose (`"80:80"` e `"443:443"`, righe già presenti e commentate in `deploy/docker-compose.yml`). Guide dettagliate per VPS, Raspberry Pi, Coolify, CapRover, backup e aggiornamenti in [docs/guida/installazione.md](docs/guida/installazione.md).

### Comandi del binario

```text
arenile serve [--dev]         # avvia il server (migrazioni automatiche se ARENILE_AUTO_MIGRATE=true)
arenile migrate up|status     # applica o mostra lo stato delle migrazioni
arenile admin create --email E --name N --org O
                              # crea il primo utente owner (password da ARENILE_ADMIN_PASSWORD o richiesta)
arenile seed demo             # carica uno stabilimento dimostrativo completo
arenile backup --out FILE     # backup consistente (SQLite: VACUUM INTO; con PostgreSQL usa pg_dump)
arenile restore --in FILE     # ripristino da backup
arenile version               # versione del binario
```

### Aggiornamento

```bash
docker compose pull && docker compose up -d
```

Le migrazioni sono sempre compatibili in avanti e vengono applicate all'avvio. Leggi il [CHANGELOG](CHANGELOG.md) prima di ogni aggiornamento di versione minore.

---

## Configurazione

Tutta la configurazione avviene tramite variabili d'ambiente, compatibili con Docker e con qualsiasi orchestratore.

| Variabile | Default | Descrizione |
|---|---|---|
| `ARENILE_ADDR` | `:8080` | Indirizzo di ascolto |
| `ARENILE_BASE_URL` | — | URL pubblico, obbligatorio in produzione |
| `ARENILE_DATA_DIR` | `/data` | Database SQLite, file caricati, backup |
| `DATABASE_URL` | vuoto | Se valorizzato con `postgres://`, usa PostgreSQL al posto di SQLite |
| `ARENILE_SECRET_KEY` | — | Chiave di cifratura (32 byte in base64); in modalità sviluppo, se assente, viene generata, salvata in `ARENILE_DATA_DIR/dev-secret.key` e mostrata al primo avvio |
| `ARENILE_AUTO_MIGRATE` | `true` | Applica le migrazioni all'avvio di `serve` |
| `ARENILE_TLS_AUTOCERT_DOMAINS` | vuoto | Domini per i certificati TLS automatici |
| `ARENILE_SMTP_URL` | vuoto | Server email, es. `smtp://user:pass@host:587` |
| `ARENILE_MAIL_FROM` | — | Mittente delle email |
| `ARENILE_HOLD_TTL` | `10m` | Durata del blocco temporaneo del posto durante il checkout |
| `ARENILE_LOG_LEVEL` | `info` | Livello di log (`debug`, `info`, `warn`, `error`) |
| `ARENILE_TRUSTED_PROXIES` | vuoto | CIDR dei reverse proxy fidati, per ricavare l'IP reale |
| `ARENILE_REQUIRE_TOTP_FOR_OWNERS` | `false` | Rende obbligatoria l'autenticazione a due fattori per i titolari |
| `ARENILE_TELEMETRY` | `false` | Nessuna telemetria di default |
| `ARENILE_DEV` | `false` | Modalità sviluppo (equivale a `arenile serve --dev`); mai in produzione |
| `ARENILE_BOOTSTRAP_OWNER_EMAIL`, `ARENILE_BOOTSTRAP_OWNER_NAME`, `ARENILE_BOOTSTRAP_OWNER_ORG` | vuoto | Primo utente owner creato all'avvio solo se il database non ha utenti (servono tutte e tre più `ARENILE_ADMIN_PASSWORD`) |
| `ARENILE_ADMIN_PASSWORD` | vuoto | Password del primo owner, o per `arenile admin create` senza prompt |

**SQLite o PostgreSQL?** SQLite (modalità WAL, file `/data/arenile.db`) è il default ed è ideale per uno o pochi stabilimenti. PostgreSQL è consigliato per installazioni con molti stabilimenti o più istanze: basta impostare `DATABASE_URL`. Una variante `docker-compose.postgres.yml` è in `deploy/`: la password di PostgreSQL viene inserita in `DATABASE_URL` senza codifica, quindi usa solo caratteri URL-safe (ad esempio `openssl rand -hex 24`); in un `DATABASE_URL` scritto a mano i caratteri speciali vanno codificati (`@` diventa `%40`).

Endpoint operativi: `/healthz` (processo vivo), `/readyz` (database raggiungibile e migrazioni allineate).

---

## Incorporare il widget nel tuo sito

La pagina di prenotazione è raggiungibile direttamente su `https://{host}/b/{slug}` e accetta parametri opzionali: `?lang=en&from=2027-07-10&to=2027-07-12&pack=base&embed=1`.

**Script (consigliato)**: adatta l'altezza automaticamente e inoltra i parametri.

```html
<div data-arenile="lido-x" data-lang="it"></div>
<script src="https://{host}/embed.js" async></script>
```

**Iframe prenotazioni**: funziona ovunque sia consentito incollare HTML.

```html
<iframe src="https://{host}/b/lido-x?embed=1"
        style="width:100%;border:0" allow="payment" loading="lazy"
        title="Prenota il tuo posto"></iframe>
```

- Funziona con **WordPress** (blocco "HTML personalizzato"), Wix, Squarespace, Joomla, siti statici e qualsiasi CMS.
- `embed.js` pesa meno di 3 KB e rilancia sulla pagina ospitante gli eventi `arenile:resize`, `arenile:booked` e `arenile:navigate` come `CustomEvent`, utili per tracciare le conversioni.
- Per sicurezza, l'incorporamento è consentito solo dai domini indicati dal gestore (`frame-ancestors` per stabilimento) e non usa cookie di terze parti.
- Colori, logo, caratteri e testi si personalizzano dal pannello; il contrasto WCAG AA del testo è calcolato automaticamente.

---

## Architettura

Arenile è un **monobinario Go** che incorpora le due applicazioni web e l'API. Nessun servizio esterno obbligatorio.

```text
┌─────────────────────────────────────────────────────────┐
│                     binario arenile                     │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ SPA pannello │  │ SPA booking  │  │  API REST v1  │  │
│  │ (embed FS)   │  │ (embed FS)   │  │  + SSE        │  │
│  └──────────────┘  └──────────────┘  └───────┬───────┘  │
│                                              │          │
│  ┌───────────────────────────────────────────▼──────┐   │
│  │ Servizi di dominio                               │   │
│  │ availability · pricing · booking · payments ·    │   │
│  │ fiscal · notifications · events                  │   │
│  └──────────────────────────┬───────────────────────┘   │
│                             │                           │
│  ┌──────────────┐  ┌────────▼───────┐  ┌────────────┐   │
│  │ Job runner   │  │ Repository     │  │ Outbox     │   │
│  │ in-process   │  │ (Bun)          │  │ eventi     │   │
│  └──────────────┘  └────────┬───────┘  └────────────┘   │
└─────────────────────────────┼───────────────────────────┘
                              │
             SQLite (default) │ PostgreSQL (opzionale)
```

### Backend

| Componente | Scelta |
|---|---|
| Linguaggio | Go, binario statico senza CGO |
| HTTP | `chi`, `net/http` standard |
| Contratto API | OpenAPI 3.1 spec-first, codice generato con `oapi-codegen` |
| Accesso dati | `uptrace/bun` (stesso codice per SQLite e PostgreSQL) |
| Database | `modernc.org/sqlite` (puro Go) · PostgreSQL via `pgdriver` |
| Migrazioni | `pressly/goose`, una serie per dialetto |
| Sicurezza | Argon2id, AES-GCM per i segreti, TOTP |
| Email, PDF, QR | `wneessen/go-mail`, `go-pdf/fpdf`, `skip2/go-qrcode` |
| Log | `log/slog` in JSON |

### Frontend

| Componente | Scelta |
|---|---|
| Build | Vite + React + TypeScript strict, npm workspaces in `web/` |
| Applicazioni | `web/admin` (pannello, PWA), `web/booking` (pagina pubblica e `embed.js`), `web/shared` (client API, mappa, temi) |
| Dati | TanStack Query + client tipizzato generato (`openapi-typescript`, `openapi-fetch`) |
| UI | Tailwind CSS + shadcn/ui (Radix), react-hook-form + zod |
| Mappa | SVG renderizzato da React, pan e zoom touch |
| i18n | i18next, italiano come lingua sorgente |

### Principi non negoziabili

- **Nessuna doppia prenotazione**: la disponibilità è garantita da vincoli del database (chiave unica sulle allocazioni per postazione, data e fascia), verificata da test di concorrenza con `-race` su entrambi i dialetti.
- **Importi in centesimi interi** (`int64`), mai virgola mobile.
- **Motore prezzi come funzione pura**: niente database, orologio o casualità durante il calcolo; snapshot del prezzo ricalcolabile.
- **Ogni cambio di stato** di una prenotazione scrive audit log ed evento outbox nella stessa transazione.
- **API-first**: pannello e pagina pubblica usano solo le API documentate.

Approfondimenti: [specifica tecnica](docs/spec/README.md) · [architettura](docs/architecture.md) · [invarianti](docs/invariants.md) · [glossario](docs/domain-glossary.md) · [decisioni architetturali (ADR)](docs/adr/README.md).

---

## Sviluppo

**Prerequisiti:** Go 1.27+, Node.js 22+, npm 10+, Docker (per i test su PostgreSQL ed end-to-end), `make`. In alternativa, apri il repository nel **devcontainer** incluso.

```bash
git clone https://github.com/coste360/arenile.git
cd arenile
make setup     # dipendenze Go e Node, strumenti di generazione
make seed      # database locale con lo stabilimento dimostrativo
make dev       # backend con ricarica automatica + Vite per pannello e booking
```

Il `Makefile` è l'**unica interfaccia** per build, test e verifiche, per le persone e per gli agenti AI.

| Target | Cosa fa |
|---|---|
| `make setup` | Installa dipendenze Go e Node e gli strumenti di generazione (una tantum) |
| `make dev` | Backend con ricarica automatica + Vite per le due SPA |
| `make gen` | Rigenera server Go (`internal/http/api`) e tipi TypeScript da `api/openapi.yaml` |
| `make lint` | `golangci-lint`, ESLint, Prettier in verifica, `tsc --noEmit` |
| `make test` | Test unitari Go e Vitest (senza Docker né rete) |
| `make test-db` | Test di integrazione (`-tags=integration`) su SQLite e PostgreSQL in container |
| `make test-race` | Test `TestConcurrency_*` con `-race` (idempotenza, outbox, webhook; poi disponibilità e pagamenti) |
| `make up` | Istanza locale vuota con `docker compose up --build` (`compose.yaml`) |
| `make e2e` | Playwright con server di prova e dati demo |
| `make check` | `gen` + verifica che il codice generato sia aggiornato + `lint` + `test` (< 2 minuti) |
| `make check-full` | `check` + `test-db` + `test-race` + `e2e` (prima di aprire una pull request) |
| `make seed` | Database locale con lo stabilimento dimostrativo |
| `make fmt` | Formatta Go (`gofumpt`, `goimports`) e frontend (Prettier) |

Build manuale del binario con versione:

```bash
npm --prefix web ci && npm --prefix web run build
cp -r web/admin/dist/. internal/http/static/admin/
cp -r web/booking/dist/. internal/http/static/booking/
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=dev" -o bin/arenile ./cmd/arenile
```

Senza build del frontend il binario si compila comunque e mostra una pagina segnaposto al posto delle SPA.

### Struttura del repository

```text
arenile/
├── cmd/arenile/          # entrypoint e CLI
├── internal/
│   ├── domain/<area>/    # logica di dominio, un package per area (niente HTTP, niente SQL)
│   ├── store/            # accesso al database (Bun), unico posto con SQL
│   ├── http/             # handler sottili, codice generato da OpenAPI, file statici
│   ├── migrations/       # sqlite/ e postgres/
│   ├── jobs/             # job runner in-process
│   └── clock/ money/ calendar/   # tempo iniettabile, centesimi, date di calendario
├── pkg/ext/              # interfacce pubbliche per le estensioni (API Go stabile)
├── api/openapi.yaml      # contratto API, unica fonte di verità
├── web/                  # admin/, booking/, shared/ (npm workspaces)
├── deploy/               # Dockerfile, docker compose, Coolify, CapRover
├── docs/                 # specifica, architettura, invarianti, ADR, guide per gestori
├── e2e/                  # test Playwright
├── .claude/              # regole, skill e subagent per gli agenti AI
├── AGENTS.md  CLAUDE.md  Makefile
└── LICENSE  CLA.md  CONTRIBUTING.md  SECURITY.md  TRADEMARK.md  CHANGELOG.md
```

---

## Roadmap

Il dettaglio con i criteri di accettazione è nella sezione 28 della [specifica](docs/spec/28-roadmap-e-criteri-di-accettazione.md).

- [x] **M0 — Fondamenta**: repository, CI, Docker, OpenAPI iniziale, configurazione, migrazioni per SQLite e PostgreSQL, CLI, autenticazione e ruoli, audit log, outbox, job runner, supporto agli agenti AI. *Accettazione:* `docker compose up` avvia un'istanza vuota e `arenile admin create` permette il login.
- [x] **M1 — Inventario e mappa** *(implementata; accettazione parziale: il percorso completo, dal nuovo stabilimento al listino, è verificato da un test end-to-end Playwright, mentre il tempo impiegato da un gestore reale non è ancora stato misurato)*: stagioni e periodi, editor della mappa con generatore di file, catalogo pack ed extra, configurazione guidata. *Accettazione:* un lido di 8 file da 15 posti configurato in meno di 15 minuti.
- [x] **M2 — Prezzi e policy** *(completata: oltre 400 casi di test del motore prezzi)*: motore prezzi completo, prezzi a durata fissa, promo e convenzioni, simulatore e matrice di listino, policy. *Accettazione:* almeno 150 casi di test del motore prezzi.
- [x] **M3 — Prenotazioni dal pannello e cassa** *(completata: 28 test di concorrenza da almeno 100 tentativi paralleli e una giornata simulata da oltre 450 operazioni, su SQLite e PostgreSQL, senza incoerenze tra allocazioni, prenotazioni e incassi; manca ancora la prova sul campo in uno stabilimento reale)*: motore di disponibilità, vista operativa, planning, clienti, abbonamenti e rilasci, cassa, corrispettivi, SSE. *Accettazione:* test di concorrenza superati, giornata simulata da 300 operazioni senza incoerenze.
- [ ] **M4 — Pagina pubblica e pagamenti online (MVP pubblico)** *(prossima)*: booking online, `embed.js`, tema, Stripe, link magico, email, QR e `.ics`. *Accettazione:* prenotazione completa da smartphone in un iframe su dominio esterno in meno di 2 minuti.
- [ ] **M5 — Completamento della stagione**: rinnovi, lista d'attesa, crediti, report, PWA offline, webhook, chiavi API, traduzioni.
- **Dopo l'MVP**: Satispay e altri provider, driver per registratori telematici, WhatsApp e SMS, app per lo staff in spiaggia, bar e ristorante con addebito sulla postazione.

---

## Sviluppo con agenti AI

Arenile è progettato per essere sviluppato anche da agenti di coding (Claude Code e strumenti compatibili con `AGENTS.md`): tutto ciò che serve per compilare, testare e capire dove va una modifica è nel repository, e ogni verifica che conta è automatica.

- [`AGENTS.md`](AGENTS.md): istruzioni condivise per tutti gli agenti (comandi, mappa del repository, regole non negoziabili, definition of done).
- [`CLAUDE.md`](CLAUDE.md): importa `AGENTS.md` e aggiunge le note specifiche di Claude Code.
- `.claude/`: regole caricate per percorso (`rules/`), procedure ripetibili (`skills/`, es. `/new-endpoint`, `/new-migration`, `/pre-pr-check`), subagent di revisione (`agents/`), impostazioni condivise.
- Template di issue **Task per agente** con obiettivo, perimetro e comando di verifica: un task piccolo, verificabile con `make check`.

---

## Contribuire

I contributi sono benvenuti: codice, traduzioni, driver di pagamento o fiscali, documentazione, segnalazioni dai gestori sul campo.

1. Leggi [CONTRIBUTING.md](CONTRIBUTING.md) e il [Codice di condotta](CODE_OF_CONDUCT.md).
2. Apri o scegli una issue (bug, funzionalità, nuovo driver, task per agente).
3. Usa Conventional Commits ed esegui `make check` (e `make check-full` per modifiche a disponibilità, prezzi, pagamenti, migrazioni o flusso pubblico).
4. Alla prima pull request un bot ti chiederà di firmare il [CLA](CLA.md).

I contributi vengono distribuiti con la stessa licenza del progetto (BSL 1.1, poi AGPL-3.0). Contribuire non dà diritto a rivendere Arenile o a offrirlo come servizio a terzi: per questo serve una licenza commerciale di Coste360 SRL.

---

## Sicurezza

Non aprire issue pubbliche per le vulnerabilità. Scrivi a **security@coste360.it** seguendo la [policy di segnalazione](SECURITY.md).

---

## Nota fiscale

Arenile **non è un registratore telematico** e, nell'MVP, non trasmette corrispettivi all'Agenzia delle Entrate. Il driver predefinito `none` presuppone che il gestore emetta il documento commerciale con il proprio registratore; il driver `export` produce il riepilogo giornaliero dei corrispettivi per aliquota e metodo di pagamento da consegnare al commercialista. Le ricevute generate da Arenile sono **non fiscali**.

La conformità normativa della configurazione (aliquote IVA, emissione dei documenti, conservazione) **resta responsabilità del gestore e del suo consulente**. Dettagli in [docs/guida/fiscale.md](docs/guida/fiscale.md).

---

## Licenza e marchio

Arenile è **source available** (codice sorgente disponibile) con licenza **[Business Source License 1.1](LICENSE)** (BSL 1.1, identificativo SPDX `BUSL-1.1`), lo stesso modello adottato da progetti come Sentry, CockroachDB e Terraform. In pratica:

- **Gratuito per gli stabilimenti.** Puoi installare e usare Arenile in produzione, senza pagare nulla, per gestire gli stabilimenti balneari, i lidi, i beach club e le piscine che possiedi o gestisci direttamente, anche pubblicando la pagina di prenotazione e il widget per i tuoi clienti.
- **Libero per provarlo e migliorarlo.** Puoi leggere il codice, copiarlo, modificarlo, testarlo e contribuire al progetto.
- **Non rivendibile.** Non puoi usare Arenile, o una sua versione modificata, per offrire servizi a terzi a pagamento o con altro vantaggio commerciale: hosting, servizio gestito o SaaS, installazione, configurazione, personalizzazione, manutenzione, assistenza, rivendita, né includerlo in un prodotto che fa concorrenza a Coste360. Per questi usi serve una **licenza commerciale** di Coste360 SRL: scrivi a **info@coste360.it**.
- **Diventa AGPL-3.0 dopo 3 anni.** Ogni versione di Arenile passa automaticamente alla licenza open source **[GNU AGPL-3.0 o successiva](https://www.gnu.org/licenses/agpl-3.0.html)** tre anni dopo il suo primo rilascio pubblico (la *Change Date* della BSL).

Il testo vincolante è quello del file [LICENSE](LICENSE); questa sintesi non lo sostituisce. Le scelte e le alternative valutate sono spiegate in [docs/adr/0007-licenza-bsl.md](docs/adr/0007-licenza-bsl.md).

> [!NOTE]
> Testo della licenza e della clausola d'uso da validare con un legale prima dell'apertura del repository.

Il nome **Arenile** e il relativo logo sono marchi di Coste360 SRL: i fork devono usare un nome e un logo diversi. Vedi [TRADEMARK.md](TRADEMARK.md).

---

## Domande frequenti

<details>
<summary><strong>Esiste un gestionale lido gratuito?</strong></summary>

Sì, Arenile è un gestionale lido gratuito con codice sorgente disponibile: un software per la gestione di stabilimenti balneari che installi sul tuo server senza canoni né licenze a pagamento. Comprende (o comprenderà, secondo la [roadmap](#roadmap)) mappa ombrelloni, prenotazione ombrelloni online, listini, cassa e abbonamenti stagionali.
</details>

<details>
<summary><strong>Arenile è davvero gratuito?</strong></summary>

Sì, per chi gestisce il proprio stabilimento. Arenile è un gestionale per stabilimenti balneari gratuito, con codice sorgente disponibile e licenza BSL 1.1: se lo usi per i lidi che possiedi o gestisci direttamente non ci sono canoni, versioni a pagamento o limiti sul numero di ombrelloni, prenotazioni o utenti. L'unico costo è l'hosting (un VPS da pochi euro al mese o un computer che hai già). Una licenza commerciale serve solo a chi vuole offrire Arenile come servizio a terzi.
</details>

<details>
<summary><strong>Sono un'agenzia o un tecnico: posso installarlo per i miei clienti?</strong></summary>

Non senza una licenza commerciale. La licenza gratuita copre solo gli stabilimenti che possiedi o gestisci direttamente. Installare, configurare, personalizzare, ospitare (hosting o SaaS), mantenere o assistere Arenile per conto di altri stabilimenti a pagamento, o rivenderlo, richiede una licenza commerciale di Coste360 SRL: scrivi a **info@coste360.it**. Se invece sei un gestore, puoi far installare Arenile dai tuoi dipendenti o installarlo tu stesso su un server che hai noleggiato.
</details>

<details>
<summary><strong>Perché non è open source in senso stretto?</strong></summary>

Perché vogliamo che Arenile resti gratuito per i gestori e che nessuno possa prenderlo e rivenderlo come servizio a pagamento agli stessi stabilimenti a cui lo diamo gratis. La Business Source License 1.1 protegge questo uso gratuito dai rivenditori, ma lascia il codice leggibile, modificabile e aperto ai contributi. Ogni versione diventa poi open source a tutti gli effetti con licenza **AGPL-3.0** tre anni dopo il suo rilascio.
</details>

<details>
<summary><strong>Ci sono commissioni sulle prenotazioni online?</strong></summary>

No: con Arenile le prenotazioni sono senza commissioni. A differenza dei portali di prenotazione a commissione, i pagamenti online passano dal tuo account Stripe e l'incasso arriva direttamente a te: paghi solo le commissioni del circuito di pagamento, nessuna percentuale alla piattaforma. Puoi anche non attivare i pagamenti online e incassare tutto alla cassa.
</details>

<details>
<summary><strong>Posso usarlo con il mio sito WordPress?</strong></summary>

Sì. Il widget di prenotazione ombrelloni si incolla in un blocco "HTML personalizzato" di WordPress, oppure in Wix, Joomla, Squarespace o qualsiasi sito, con uno script di una riga o un iframe. Puoi anche usare il link diretto alla pagina di prenotazione sui social o su Google Business. Vedi [Incorporare il widget](#incorporare-il-widget-nel-tuo-sito).
</details>

<details>
<summary><strong>Serve un registratore telematico?</strong></summary>

Arenile non sostituisce il registratore telematico: nell'MVP il documento commerciale si emette con il tuo dispositivo, mentre Arenile gestisce cassa, incassi ed esportazione dei corrispettivi per il commercialista. L'architettura prevede driver per registratori telematici come contributi futuri. Vedi la [nota fiscale](#nota-fiscale).
</details>

<details>
<summary><strong>Funziona senza internet?</strong></summary>

Il server può girare anche in rete locale (ad esempio un mini PC o un Raspberry Pi nel lido), ma le prenotazioni online richiedono che sia raggiungibile da internet. Il pannello è previsto come PWA: se la connessione cade puoi consultare la mappa del giorno e registrare check-in, note e incassi in contanti, che vengono sincronizzati al ritorno della rete. Le nuove prenotazioni richiedono invece la connessione, per garantire che non ci siano doppie vendite.
</details>

<details>
<summary><strong>Che differenza c'è tra un gestionale balneare e un portale di prenotazione?</strong></summary>

Un portale di prenotazione ombrelloni vende i tuoi posti sul proprio sito e trattiene una percentuale su ogni prenotazione. Un gestionale balneare come Arenile è il software con cui gestisci lo stabilimento (spiaggia, listini, cassa, clienti) e pubblichi la prenotazione ombrelloni online sul tuo sito: le prenotazioni sono senza commissioni e il rapporto con il cliente resta tuo.
</details>

<details>
<summary><strong>Posso gestire più lidi?</strong></summary>

Sì. Un'unica installazione gestisce più stabilimenti balneari, ciascuno con la propria mappa, i propri listini, le proprie policy e la propria pagina di prenotazione. Per molte strutture è consigliato PostgreSQL.
</details>

<details>
<summary><strong>Quali tipi di stabilimento supporta?</strong></summary>

Lidi con ombrelloni a file e posti numerati, spiagge attrezzate a quantità (es. lettini sfusi), zone con assegnazione del posto da parte del gestore, gazebo, tende, palme e aree piscina. Le modalità si possono combinare nello stesso stabilimento.
</details>

<details>
<summary><strong>I dati dei miei clienti sono al sicuro e conformi al GDPR?</strong></summary>

I dati restano sul server che scegli tu: nessuno, nemmeno Coste360, vi accede. Arenile raccoglie solo i dati necessari, versiona i consensi, consente esportazione e anonimizzazione dei dati del cliente e non registra email o telefoni in chiaro nei log. Il titolare del trattamento resta il gestore: trovi un modello di informativa in [docs/guida/privacy-gdpr.md](docs/guida/privacy-gdpr.md).
</details>

---

## Crediti

Arenile è ideato e sviluppato da **[Coste360 SRL](https://coste360.it)**, la piattaforma italiana per il monitoraggio delle coste e delle concessioni demaniali marittime, insieme alla community dei contributori.

Se Arenile ti è utile, lascia una stella al repository e parlane con altri gestori: è il modo migliore per farlo crescere.

---

## In English

**Arenile** is a free, self-hosted, source-available management system for Italian beach establishments (*stabilimenti balneari*, lidos and beach clubs). It combines an interactive beach umbrella map, a cash desk, seasonal passes, a configurable pricing and booking-policy engine, and a mobile-first online booking page for umbrellas and sunbeds that can be embedded in any website via iframe or a one-line script. There are no booking commissions: online payments go through the operator's own Stripe account. It ships as a single Go binary and Docker image (`ghcr.io/coste360/arenile`) with SQLite by default and optional PostgreSQL, supports multiple establishments per installation, and requires no external services. Milestones M0 (foundations), M1 (inventory, map editor, guided setup) and M2 (pricing and booking policies) are complete on the development branch (not yet published), with no release yet; the project is under active development by [Coste360 SRL](https://coste360.it); see the [roadmap](#roadmap) for status. It is source-available under the [Business Source License 1.1](LICENSE): free for beach establishments that run it for themselves; offering it to third parties as a paid service (hosting, SaaS, installation, customisation, support, resale) requires a commercial license from Coste360 SRL. Each version converts to AGPL-3.0 or later three years after its first public release (the license text and use clause are still pending legal review). Contributions are welcome under the [CLA](CLA.md): see [CONTRIBUTING.md](CONTRIBUTING.md).
