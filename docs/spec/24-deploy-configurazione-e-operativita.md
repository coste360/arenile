## 24. Deploy, configurazione e operatività

### 24.1 Comandi del binario

```
arenile serve                 # avvia il server (esegue le migrazioni se ARENILE_AUTO_MIGRATE=true)
arenile migrate up|status
arenile admin create          # crea il primo utente owner
arenile seed demo             # carica uno stabilimento dimostrativo completo
arenile backup --out FILE     # backup consistente (SQLite: VACUUM INTO; Postgres: indica pg_dump)
arenile restore --in FILE
arenile version
```

### 24.2 Dockerfile (multi-stage)

```dockerfile
# 1. Frontend
FROM node:22-alpine AS web
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci
COPY web ./web
COPY api ./api
RUN cd web && npm run build          # produce web/admin/dist e web/booking/dist

# 2. Backend
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/admin/dist   ./internal/http/static/admin
COPY --from=web /src/web/booking/dist ./internal/http/static/booking
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" -o /out/arenile ./cmd/arenile

# 3. Runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/arenile /arenile
VOLUME ["/data"]
ENV ARENILE_DATA_DIR=/data ARENILE_ADDR=:8080
EXPOSE 8080
HEALTHCHECK NONE
USER nonroot
ENTRYPOINT ["/arenile", "serve"]
```

> Le versioni delle immagini di base vanno allineate all'ultima stabile al momento del rilascio. L'healthcheck è demandato all'orchestratore tramite `/healthz` perché l'immagine distroless non contiene `curl`.

### 24.3 docker-compose.yml (installazione standard)

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

Variante `docker-compose.postgres.yml` con servizio PostgreSQL e `DATABASE_URL`.

### 24.4 Variabili d'ambiente

| Variabile | Default | Descrizione |
|---|---|---|
| `ARENILE_ADDR` | `:8080` | Indirizzo di ascolto |
| `ARENILE_BASE_URL` | — | URL pubblico, obbligatorio in produzione |
| `ARENILE_DATA_DIR` | `/data` | Database SQLite, file caricati, backup |
| `DATABASE_URL` | vuoto | Se valorizzato con `postgres://`, usa PostgreSQL |
| `ARENILE_SECRET_KEY` | — | Chiave di cifratura (32 byte base64); generata e mostrata al primo avvio se assente in sviluppo |
| `ARENILE_AUTO_MIGRATE` | `true` | |
| `ARENILE_TLS_AUTOCERT_DOMAINS` | vuoto | Abilita certificati automatici |
| `ARENILE_SMTP_URL` | vuoto | `smtp://user:pass@host:587` |
| `ARENILE_MAIL_FROM` | — | |
| `ARENILE_HOLD_TTL` | `10m` | |
| `ARENILE_LOG_LEVEL` | `info` | |
| `ARENILE_TRUSTED_PROXIES` | vuoto | CIDR dei proxy per l'IP reale |
| `ARENILE_REQUIRE_TOTP_FOR_OWNERS` | `false` | |
| `ARENILE_TELEMETRY` | `false` | Nessuna telemetria di default |

### 24.5 Operatività

- `/healthz` (processo vivo) e `/readyz` (database raggiungibile, migrazioni allineate).
- `/metrics` in formato Prometheus, disattivato di default, protetto da token.
- Backup automatico giornaliero opzionale nel data dir con retention configurabile (SQLite).
- Aggiornamento: `docker compose pull && docker compose up -d`; migrazioni sempre compatibili in avanti; note di aggiornamento nel changelog per ogni versione minore.
- Template one-click per Coolify e CapRover; documentazione per VPS e per Raspberry Pi (build `arm64`).
- Immagini multi-architettura `amd64` e `arm64` pubblicate su GitHub Container Registry.

---

