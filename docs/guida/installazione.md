# Installazione di Arenile

Questa guida spiega come installare il gestionale Arenile sul tuo server, come proteggerlo con HTTPS, come fare backup e ripristino e come aggiornarlo.

> [!NOTE]
> Arenile è in sviluppo (milestone M0). L'immagine `ghcr.io/coste360/arenile` verrà pubblicata al primo rilascio; fino ad allora si può compilare dai sorgenti (vedi il [README](../../README.md#sviluppo)). Le istruzioni seguenti descrivono l'installazione prevista.

## Indice

- [Di cosa hai bisogno](#di-cosa-hai-bisogno)
- [Installazione con Docker Compose](#installazione-con-docker-compose)
- [Installazione su VPS](#installazione-su-vps)
- [Raspberry Pi o mini PC (arm64)](#raspberry-pi-o-mini-pc-arm64)
- [Coolify e CapRover](#coolify-e-caprover)
- [HTTPS](#https)
- [PostgreSQL](#postgresql)
- [Email](#email)
- [Backup e ripristino](#backup-e-ripristino)
- [Aggiornamento](#aggiornamento)
- [Risoluzione dei problemi](#risoluzione-dei-problemi)

## Di cosa hai bisogno

| Requisito | Indicazione |
|---|---|
| Server | Linux `amd64` o `arm64` con Docker e Docker Compose |
| Risorse | 1 vCPU e 512 MB di RAM bastano per un lido; 1 GB di disco più lo spazio per i backup |
| Dominio | Un sottodominio, ad esempio `prenota.mio-lido.it`, che punti all'IP del server (record DNS `A`/`AAAA`) |
| Email (facoltativa) | Credenziali SMTP per inviare conferme e promemoria ai clienti |
| Pagamenti online (facoltativi) | Un account Stripe intestato allo stabilimento |

Nessun altro servizio è necessario: niente Redis, niente code esterne, niente object storage. Il database predefinito è SQLite, salvato nel volume `/data`.

## Installazione con Docker Compose

1. Crea una cartella, ad esempio `/opt/arenile`, con il file `docker-compose.yml`:

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
   volumes:
     arenile-data:
   ```

2. Crea il file `.env` nella stessa cartella con la chiave di cifratura (32 byte casuali in base64):

   ```bash
   echo "ARENILE_SECRET_KEY=$(openssl rand -base64 32)" > .env
   echo "ARENILE_SMTP_URL=" >> .env
   chmod 600 .env
   ```

   > [!WARNING]
   > `ARENILE_SECRET_KEY` cifra le chiavi Stripe, i segreti dei webhook e i codici TOTP. Conservala in un luogo sicuro **separato** dai backup del database ma sempre recuperabile: senza di essa i segreti salvati sono perduti.

3. Avvia:

   ```bash
   docker compose up -d
   docker compose logs -f arenile
   ```

   Le migrazioni del database vengono applicate automaticamente all'avvio (`ARENILE_AUTO_MIGRATE=true`).

4. Crea il primo utente con ruolo titolare (`owner`). Il modo più semplice è impostare in `.env`, prima del primo avvio, `ARENILE_BOOTSTRAP_OWNER_EMAIL`, `ARENILE_BOOTSTRAP_OWNER_NAME`, `ARENILE_BOOTSTRAP_OWNER_ORG` e `ARENILE_ADMIN_PASSWORD` (almeno 10 caratteri): l'owner viene creato all'avvio solo se il database non ha ancora utenti, e dopo il primo accesso puoi rimuovere la password da `.env`. In alternativa, con il container avviato (l'immagine non ha una shell, ma `exec` lancia direttamente il binario):

   ```bash
   docker compose exec -e ARENILE_ADMIN_PASSWORD='scegli-una-password-lunga' arenile \
     /arenile admin create --email titolare@mio-lido.it --name "Mario Rossi" --org "Lido Esempio"
   ```

5. Apri `https://prenota.mio-lido.it/admin/` e accedi. Nella versione attuale (M0) il pannello offre l'accesso e l'elenco degli stabilimenti; la configurazione guidata (dati dello stabilimento, modello di mappa, listino rapido, pagina pubblica, pagamenti) arriva con la milestone M1.

Per esplorare il programma con dati di esempio puoi caricare lo stabilimento dimostrativo: `docker compose exec arenile /arenile seed demo`. Fallo su un'istanza di prova, non su quella di produzione.

Tutte le variabili di configurazione sono nella [tabella del README](../../README.md#configurazione).

## Installazione su VPS

Qualsiasi VPS Linux con Docker va bene (Hetzner, OVHcloud, Aruba Cloud, Contabo, DigitalOcean, ecc.). Su Ubuntu o Debian:

```bash
# 1. Docker (script ufficiale)
curl -fsSL https://get.docker.com | sh

# 2. Firewall: solo SSH, HTTP e HTTPS
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

Poi segui [Installazione con Docker Compose](#installazione-con-docker-compose) e configura [HTTPS](#https). Consigli:

- non esporre la porta `8080` su internet se usi un reverse proxy: pubblicala solo su `127.0.0.1` (`"127.0.0.1:8080:8080"`);
- abilita gli aggiornamenti di sicurezza automatici del sistema operativo (`unattended-upgrades`);
- accedi al server solo con chiave SSH;
- copia i backup **fuori** dal VPS (vedi [Backup e ripristino](#backup-e-ripristino)).

## Raspberry Pi o mini PC (arm64)

Le immagini sono pubblicate per `amd64` e `arm64`, quindi Arenile gira su Raspberry Pi 4 o 5 (almeno 2 GB di RAM) e su mini PC.

1. Installa **Raspberry Pi OS Lite a 64 bit** (o Ubuntu Server arm64): l'immagine `arm64` non funziona su sistemi a 32 bit.
2. Usa un **SSD USB** al posto della scheda microSD per il volume dei dati: è più veloce e molto più affidabile.
3. Installa Docker con `curl -fsSL https://get.docker.com | sh` e segui la procedura Docker Compose.
4. Se il dispositivo sta nel lido, le prenotazioni online richiedono comunque che sia raggiungibile da internet: valuta un tunnel (ad esempio Cloudflare Tunnel o Tailscale Funnel) invece di aprire porte sul router.
5. Proteggi l'alimentazione con un piccolo gruppo di continuità: uno spegnimento improvviso durante una scrittura è la causa più comune di danni al disco.

Per compilare direttamente il binario `arm64` dai sorgenti:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
  -ldflags "-s -w -X main.version=dev" -o arenile ./cmd/arenile
```

## Coolify e CapRover

Il repository prevede template pronti in `deploy/coolify/` e `deploy/caprover/` (in preparazione nella milestone M0).

**Coolify**
1. Crea una nuova risorsa *Docker Compose* e incolla il `docker-compose.yml` di questa guida (o il template di `deploy/coolify/`).
2. Imposta nelle variabili d'ambiente `ARENILE_BASE_URL`, `ARENILE_SECRET_KEY`, le variabili del primo owner (`ARENILE_BOOTSTRAP_OWNER_EMAIL`, `_NAME`, `_ORG` e `ARENILE_ADMIN_PASSWORD`: il terminale di Coolify non funziona perché l'immagine non ha una shell) e, se serve, `ARENILE_SMTP_URL` e `ARENILE_MAIL_FROM`.
3. Associa il dominio al servizio sulla porta `8080`: Coolify gestisce il certificato HTTPS.
4. Configura l'healthcheck su `GET /healthz`.

**CapRover**
1. Crea una nuova app con *persistent data* e mappa il percorso `/data` su un volume.
2. Usa come immagine `ghcr.io/coste360/arenile:latest` (o il template di `deploy/caprover/`), porta del container `8080`.
3. Imposta le variabili d'ambiente come sopra, abilita HTTPS e "Force HTTPS".
4. Crea il primo owner con le variabili `ARENILE_BOOTSTRAP_OWNER_EMAIL`, `_NAME`, `_ORG` e `ARENILE_ADMIN_PASSWORD` (la console del container non funziona: l'immagine non ha una shell), oppure via SSH sul server con `docker exec -e ARENILE_ADMIN_PASSWORD='...' <container> /arenile admin create --email ... --name ... --org ...`.

In entrambi i casi imposta `ARENILE_TRUSTED_PROXIES` con la rete del proxy della piattaforma, così Arenile vede l'IP reale dei client (necessario per la limitazione delle richieste).

## HTTPS

HTTPS è obbligatorio in produzione: protegge le password e i dati dei clienti ed è richiesto dai browser per i pagamenti dentro un iframe. Hai due opzioni.

**A. Reverse proxy (consigliato).** Esempio con Caddy, che ottiene il certificato da solo:

```caddyfile
prenota.mio-lido.it {
    reverse_proxy 127.0.0.1:8080
}
```

Imposta poi `ARENILE_TRUSTED_PROXIES=127.0.0.1/32` (o la rete Docker del proxy).

**B. Certificati automatici nel binario.** Imposta `ARENILE_TLS_AUTOCERT_DOMAINS=prenota.mio-lido.it`: Arenile ottiene e rinnova i certificati in autonomia. Le porte 80 e 443 del server devono raggiungere il container (nel compose pubblica `"80:80"` e `"443:443"`: le righe sono già presenti, commentate, in `deploy/docker-compose.yml`) e i certificati vengono salvati nel data dir.

## PostgreSQL

SQLite va benissimo per uno o pochi stabilimenti. PostgreSQL è consigliato con molti stabilimenti nella stessa installazione o con più istanze in parallelo. Per usarlo imposta:

```bash
DATABASE_URL=postgres://arenile:password@db:5432/arenile?sslmode=disable
```

Il repository include la variante `deploy/docker-compose.postgres.yml` con il servizio PostgreSQL già configurato. Il passaggio da SQLite a PostgreSQL di un'installazione esistente non è automatico: va fatto esportando e reimportando i dati.

## Email

Le email (conferme, promemoria, link di gestione della prenotazione) partono dal tuo server SMTP, senza provider imposti:

```bash
ARENILE_SMTP_URL=smtp://utente:password@smtp.mio-provider.it:587
ARENILE_MAIL_FROM="Lido Esempio <prenotazioni@mio-lido.it>"
```

Per una buona consegna configura SPF, DKIM e DMARC sul dominio del mittente. Se `ARENILE_SMTP_URL` è vuota Arenile funziona comunque, ma non invia email.

## Backup e ripristino

Il backup è responsabilità di chi gestisce l'installazione. Contiene i dati personali dei clienti: conservalo cifrato e con accesso limitato.

**Backup manuale (SQLite)**

```bash
docker compose exec arenile /arenile backup --out /data/backup/arenile-$(date +%F).db
docker compose cp arenile:/data/backup/arenile-$(date +%F).db ./
```

Il comando produce una copia consistente anche con il server in funzione (SQLite `VACUUM INTO`). È previsto anche un backup automatico giornaliero opzionale nel data dir, con retention configurabile.

**Backup con PostgreSQL**: usa gli strumenti standard di PostgreSQL, ad esempio `pg_dump -Fc`.

**Ripristino**

```bash
docker compose cp ./arenile-2027-08-15.db arenile:/data/backup/
docker compose exec arenile /arenile restore --in /data/backup/arenile-2027-08-15.db
docker compose restart arenile
```

Regole d'oro:
- applica la regola **3-2-1**: tre copie, su due supporti diversi, una fuori sede (ad esempio uno storage remoto con `rclone` o `restic`);
- conserva `ARENILE_SECRET_KEY` insieme alla procedura di ripristino;
- **prova il ripristino** prima dell'inizio della stagione su un'istanza di prova;
- fai sempre un backup prima di un aggiornamento.

## Aggiornamento

```bash
cd /opt/arenile
docker compose exec arenile /arenile backup --out /data/backup/pre-update.db
docker compose pull
docker compose up -d
docker compose exec arenile /arenile version
```

- Le migrazioni sono sempre compatibili in avanti e vengono applicate all'avvio.
- Prima di ogni aggiornamento di versione minore leggi le **note di aggiornamento** nel [CHANGELOG](../../CHANGELOG.md).
- In alta stagione, aggiorna in un momento di basso traffico (sera o mattina presto) e preferisci le versioni di patch.
- Per bloccare una versione specifica usa un tag preciso al posto di `latest` (es. `ghcr.io/coste360/arenile:0.4.2`).

## Risoluzione dei problemi

| Sintomo | Cosa controllare |
|---|---|
| Il container si riavvia in continuazione | `docker compose logs arenile`; verifica che `ARENILE_SECRET_KEY` sia valorizzata e sia una chiave base64 di 32 byte |
| `/readyz` risponde con errore | Database non raggiungibile o migrazioni non allineate: controlla `DATABASE_URL` e `arenile migrate status` |
| Il widget non compare nel sito | Aggiungi il dominio del sito tra le origini autorizzate all'incorporamento nella configurazione della pagina pubblica |
| Le email non arrivano | Verifica `ARENILE_SMTP_URL`, `ARENILE_MAIL_FROM` e i record SPF/DKIM; controlla la cartella spam |
| Gli IP nei log sono tutti uguali | Imposta `ARENILE_TRUSTED_PROXIES` con la rete del reverse proxy |

Per log più dettagliati imposta temporaneamente `ARENILE_LOG_LEVEL=debug`. I log non contengono email, telefoni o token in chiaro, ma prima di condividerli in una issue verifica comunque che non ci siano dati personali.
