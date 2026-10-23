# 0014 — Scelte di M6: edizione desktop, token per dispositivo, meta, ponte fiscale

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
- Le operazioni M6 rispondono `501` finché i builder non le completano; il token per dispositivo fallisce chiuso.
