# Arenile su Coolify

Installazione del gestionale per stabilimenti balneari Arenile su [Coolify](https://coolify.io),
la piattaforma self-hosted per il deploy su VPS. Due template Docker Compose pronti:

| File | Database | Quando usarlo |
|---|---|---|
| `arenile.yml` | SQLite (nel volume `/data`) | Un singolo stabilimento: la scelta consigliata |
| `arenile-postgres.yml` | PostgreSQL 17 | Più stabilimenti o infrastruttura già basata su PostgreSQL |

## Installazione

1. In Coolify: **Projects → + New → Docker Compose Empty**.
2. Incolla il contenuto di `arenile.yml` (oppure `arenile-postgres.yml`).
3. In **Environment Variables** imposta `ARENILE_SECRET_KEY` con il valore generato da:

   ```sh
   openssl rand -base64 32
   ```

   Salvala in un luogo sicuro: senza questa chiave i dati cifrati non sono recuperabili.
4. Facoltativo: `ARENILE_SMTP_URL` (`smtp://utente:password@host:587`) e `ARENILE_MAIL_FROM`
   per le email di conferma prenotazione.
5. Assegna il dominio al servizio `arenile` (Coolify valorizza `SERVICE_FQDN_ARENILE_8080`
   e quindi `ARENILE_BASE_URL`) e premi **Deploy**.
6. Crea il primo utente owner dal terminale del container (scheda **Terminal**):

   ```sh
   /arenile admin create
   ```

## Note

- Il certificato TLS è gestito dal proxy di Coolify: lascia vuota `ARENILE_TLS_AUTOCERT_DOMAINS`.
- `ARENILE_TRUSTED_PROXIES` è preimpostata sulle reti private Docker, così Arenile legge l'IP
  reale dei clienti (necessario per il rate limiting delle prenotazioni).
- Health check: configuralo in Coolify su `GET /healthz`, porta `8080` (l'immagine è distroless
  e non contiene `curl`). `/readyz` verifica database e migrazioni.
- Backup: con SQLite usa `/arenile backup --out /data/backup.db` o i backup automatici nel volume;
  con PostgreSQL usa i backup pianificati di Coolify sul servizio `postgres`.
- Aggiornamento: imposta `ARENILE_VERSION` (es. `v1.2.0`) e rifai il deploy; le migrazioni
  sono sempre compatibili in avanti.
