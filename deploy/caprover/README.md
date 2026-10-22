# Arenile su CapRover

Installazione del gestionale per stabilimenti balneari Arenile su [CapRover](https://caprover.com).

## Opzione 1 — One-Click App (consigliata)

1. In CapRover: **Apps → One-Click Apps/Databases → Template personalizzato** (in fondo alla lista).
2. Incolla il contenuto di `one-click-app.yml` e premi **Next**.
3. Compila le variabili:
   - **Chiave segreta**: genera il valore con `openssl rand -base64 32` e conservalo con cura;
   - SMTP e mittente email (facoltativi) per le conferme di prenotazione.
4. A installazione completata, dalla scheda dell'app:
   - abilita **HTTPS** e **Force HTTPS**;
   - imposta un dominio personalizzato (es. `prenota.mio-lido.it`) e aggiorna `ARENILE_BASE_URL`;
   - crea il primo utente owner dal terminale del container: `/arenile admin create`.

## Opzione 2 — App con `captain-definition`

`captain-definition` usa l'immagine già pubblicata `ghcr.io/coste360/arenile:latest`, senza build
sul server (utile su VPS piccole o Raspberry Pi: l'immagine è multi-architettura `amd64`/`arm64`).

```sh
cd deploy/caprover
caprover deploy -a arenile   # oppure carica un tar con captain-definition dalla dashboard
```

Poi configura l'app dalla dashboard:

- **Container HTTP Port**: `8080`;
- **Persistent Directories**: percorso `/data` (database SQLite, file caricati, backup);
- **Environment Variables**: almeno `ARENILE_BASE_URL` e `ARENILE_SECRET_KEY`
  (elenco completo in `deploy/env.example`).

## Note

- Il TLS è gestito da CapRover (nginx + Let's Encrypt): lascia vuota `ARENILE_TLS_AUTOCERT_DOMAINS`.
- `ARENILE_TRUSTED_PROXIES` va impostata sulle reti private Docker per leggere l'IP reale dei clienti.
- Health check: l'immagine distroless non ha `curl`; usa `GET /healthz` da un monitor esterno.
- Aggiornamento: cambia il tag dell'immagine (o rifai il deploy di `latest`); le migrazioni del database
  vengono eseguite all'avvio e sono sempre compatibili in avanti.
