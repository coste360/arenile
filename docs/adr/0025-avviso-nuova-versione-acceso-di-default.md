# 0025 — Avviso di nuova versione acceso di default

- **Stato:** accettato (la verifica su richiesta è nell'ADR 0026)
- **Data:** 2026-10-24
- **Sezioni della specifica:** `docs/spec/29-edizione-desktop-e-app-mobile.md` (29.1, cambia "controllo della
  nuova versione solo se il gestore lo abilita"), `docs/spec/24-deploy-configurazione-e-operativita.md` (24.4)

## Contesto

Oggi un lido non sa mai che esiste una versione nuova di Arenile: l'app desktop ha un controllo della versione,
ma è spento di default (spec 29.1) e quasi nessuno lo accende; l'installazione Docker o con il binario non ne ha
nessuno. Chi resta indietro perde correzioni di sicurezza e di calcolo proprio in alta stagione.

Vincoli: `ARENILE_TELEMETRY=false` resta il default e significa "nessuna telemetria"; nessun dato del lido deve
uscire dall'installazione; un controllo fallito (rete assente, server giù, risposta non valida) non deve mai
diventare un errore per chi lavora in cassa; gli sviluppatori non devono essere disturbati dalle loro build.

Alternative scartate:

- **Lasciare il controllo spento di default**: è la situazione attuale, e non funziona.
- **Identificativo dell'installazione** (per contare le attivazioni in modo esatto): è un dato personale
  indiretto e renderebbe il controllo una telemetria vera e propria.
- **Leggere le release di GitHub**: dipendenza da un servizio terzo, limiti di frequenza, e nessun modo per
  Coste360 di sapere quante installazioni sono attive nemmeno in aggregato.
- **Chiamata di rete durante la richiesta del pannello**: rallenta la pagina e la fa dipendere da internet.

## Decisione

1. Il controllo della nuova versione è **acceso di default** su tutte le edizioni e si può spegnere:
   `ARENILE_UPDATE_CHECK=false` sui server (Docker o binario); nell'app desktop l'impostazione *Controlla se
   esiste una nuova versione* (`update_check`, ora vera di default). Le versioni precedenti scrivevano sempre
   `"update_check": false` (il vecchio predefinito) appena si salvava una qualsiasi impostazione, quindi un file
   senza `settings_version` vale vero; dopo questo ADR il file porta `settings_version: 2` e un `false` salvato
   resta spento. Nell'app desktop `ARENILE_UPDATE_CHECK=false` nell'ambiente del launcher spegne comunque il
   controllo (collaudi); un `ARENILE_UPDATE_URL` non valido torna al predefinito senza bloccare l'avvio.
2. La fonte è un file JSON pubblico su un server di Coste360, `https://updates.coste360.it/arenile/latest.json`
   (costante unica `updates.DefaultURL`, sostituibile con `ARENILE_UPDATE_URL` su server e desktop), scritto
   dalla pipeline di build con la forma `{version, tag, published_at, release_notes, checksums, files[]}`. Il
   campo `version` mantiene il significato che leggono le vecchie build desktop.
3. La richiesta è un `GET` senza cookie, senza identificativi e senza dati del lido. Rivela soltanto: la
   versione, l'edizione e sistema operativo/architettura nell'User-Agent
   (`arenile/1.0.1 (desktop; windows/arm64)`) e, come ogni richiesta HTTP, l'indirizzo IP al server di
   Coste360. Coste360 usa queste informazioni solo per conteggi aggregati (quante installazioni per versione,
   edizione e sistema). **Non è telemetria**: `ARENILE_TELEMETRY` resta `false` e non cambia significato.
4. Un job in processo (`internal/domain/update`, job `updates.check`) controlla 30 secondi dopo l'avvio e poi
   ogni 24 ore con il clock iniettato; dopo un errore riprova dopo un'ora. Il risultato resta in memoria
   (nessuna migrazione). Con il controllo spento, o con una versione non di release (`dev`, `-dev.` di
   `scripts/version.sh`), **nessuna chiamata di rete**.
5. `GET /api/admin/v1/system/update` (titolari e gestori: permesso `establishment.configure` su almeno uno
   stabilimento; niente chiavi API) risponde dalla copia in memoria con `status` `ok` | `unavailable` |
   `disabled`. Un controllo fallito è `unavailable`, mai un errore: il pannello mostra solo la riga neutra
   "Impossibile cercare automaticamente gli aggiornamenti." e conserva l'ultimo risultato riuscito per 7 giorni.
   Gli errori di rete vanno nel log a livello debug.
6. Il pannello mostra un avviso chiudibile in cima (shadcn `Alert` con `role="status"`, mai una finestra
   modale): desktop → *Scarica* il pacchetto per il sistema in uso (`files[]`, altrimenti
   `https://arenile.app/download/`); Docker → *Come aggiornare* verso la guida (backup, poi
   `docker compose pull && docker compose up -d`); binario → l'archivio per il sistema in uso. La chiusura
   ricorda la versione nel browser, quindi l'avviso torna per la versione successiva. L'app desktop mostra anche
   una notifica di sistema una volta per versione.

## Conseguenze

- Accettato: dopo un ripristino dell'app desktop il server interno riparte e il controllo si ripete dopo 30
  secondi (fino ad allora l'avviso non compare); la notifica di sistema non si ripete per la stessa versione.

- Positive: i lidi scoprono le versioni nuove senza fare nulla; un solo codice (`internal/updates`) per server e
  desktop; Coste360 ha numeri aggregati sull'adozione senza raccogliere dati dei lidi.
- Negative: di default ogni installazione fa una richiesta al giorno verso Coste360, che vede l'IP; chi non lo
  vuole deve spegnerlo. La documentazione (privacy, configurazione, app desktop) lo dice chiaramente.
- Verifiche: `TestRefresh_NoNetworkWhenDisabledOrDev`, `TestRefresh_Schedule`,
  `TestRefresh_FailureIsSilentAndRetried`, `TestStatus_States`, `TestStatus_Permissions`
  (`internal/domain/update`), `TestUpdates_WiringAndNoNetworkWhenDisabled` (`internal/app`),
  `TestChecker_RequestCarriesOnlyTheUserAgent` (`internal/updates`), `TestLoadSettings_UpdateCheckDefault`
  (`internal/desktop`), `TestGetSystemUpdate_Scenarios` (`internal/http/admin`), `UpdateBanner.test.tsx`;
  invariante INV-98. I test end-to-end e la cattura degli screenshot girano con `ARENILE_UPDATE_CHECK=false`.
