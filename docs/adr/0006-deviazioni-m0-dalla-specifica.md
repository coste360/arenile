# 0006 — Deviazioni di M0 dalla specifica: webhook di pagamento, modalità sviluppo, TOTP, primo owner

- **Stato:** accettato
- **Data:** 2026-10-22
- **Sezioni della specifica:** `docs/spec/13-pagamenti.md` (13.1), `docs/spec/16-utenti-ruoli-e-autenticazione.md`,
  `docs/spec/17-api.md`, `docs/spec/24-deploy-configurazione-e-operativita.md` (24.3, 24.4),
  `docs/spec/28-roadmap-e-criteri-di-accettazione.md` (M0)

## Contesto

Implementando M0 sono emerse quattro scelte che si discostano dal testo della specifica o lo
completano. `pkg/ext` è una superficie pubblica (cambiarla è un breaking change per i plugin) e
`AGENTS.md` richiede un ADR per ogni decisione che cambia la specifica.

1. **`ParseWebhook` senza credenziali.** La spec 13.1 prevede `ParseWebhook(ctx, r)`. Le credenziali
   del provider (chiave del webhook, segreto di firma) sono però per stabilimento, salvate cifrate nel
   database: un driver registrato una sola volta all'avvio non ha modo di conoscerle senza dipendere da
   `internal/store` o da uno stato globale, entrambi vietati.
2. **Modalità sviluppo.** La spec 24.4 rende obbligatori `ARENILE_BASE_URL` e `ARENILE_SECRET_KEY`.
   Per `make dev`, per i test end-to-end e per una prova locale serve poter avviare il server senza
   configurarli.
3. **Attivazione del TOTP.** La spec 16 prevede il TOTP (obbligatorio per gli owner se
   `ARENILE_REQUIRE_TOTP_FOR_OWNERS=true`) ma la spec 17 non elenca gli endpoint per attivarlo.
4. **Primo utente owner nei container.** La spec 24.3 crea il primo owner con `arenile admin create`
   dal terminale del container. L'immagine di produzione è distroless (nessuna shell): il terminale di
   Coolify e di CapRover non può eseguirlo.
5. **Scenario di accettazione M0 per gli agenti.** La spec 28 chiede a un agente di aggiungere
   `GET /api/admin/v1/establishments`, ma l'endpoint è già parte di M0 (serve al pannello dopo il login).

## Decisione

1. La firma pubblica è `ParseWebhook(ctx context.Context, r *http.Request, creds Credentials)`.
   Il core individua lo stabilimento dalla URL del webhook (`POST /webhooks/payments/{provider}/{establishment_id}`,
   spec 13, in arrivo con i pagamenti online), decifra le credenziali e le passa al driver. I driver
   restano senza stato e senza accesso al database.
2. `ARENILE_DEV=true` (o `arenile serve --dev`) rende facoltativo `ARENILE_BASE_URL` (si usa
   `http://localhost:<porta>`), disattiva l'attributo `Secure` dei cookie solo con URL `http://` e, se
   `ARENILE_SECRET_KEY` manca, genera una chiave, la salva in `<ARENILE_DATA_DIR>/dev-secret.key`
   (permessi 0600) e la mostra una sola volta su stderr. Fuori da questa modalità i due valori restano
   obbligatori e il server rifiuta di partire.
3. Si aggiungono `POST /api/admin/v1/auth/totp/enroll` (genera il segreto, salvato cifrato con AAD =
   id utente, e restituisce l'URI `otpauth://`) e `POST /api/admin/v1/auth/totp/confirm` (attiva il
   TOTP con un codice valido). Entrambi richiedono la sessione del pannello e il token CSRF. Con
   `ARENILE_REQUIRE_TOTP_FOR_OWNERS=true` un owner senza TOTP riceve `totp_enrolment_required` al login
   e, finché non lo attiva, può chiamare solo gli endpoint sotto `/api/admin/v1/auth/`
   (gli altri rispondono `403 totp_enrolment_required`).
4. `arenile serve` crea il primo owner da `ARENILE_BOOTSTRAP_OWNER_EMAIL`, `ARENILE_BOOTSTRAP_OWNER_NAME`,
   `ARENILE_BOOTSTRAP_OWNER_ORG` e `ARENILE_ADMIN_PASSWORD` **solo se il database non ha ancora utenti**
   (controllo nella stessa transazione della creazione). Ai riavvii successivi le variabili sono ignorate.
   `arenile admin create --email … --name … --org …` resta disponibile, eseguito dall'host con
   `docker exec` / `docker compose exec` (che lanciano il binario senza shell).
5. Lo scenario di accettazione M0 per gli agenti usa `GET /api/admin/v1/establishments/{id}/audit`
   (elenco paginato del registro attività di uno stabilimento, già disponibile nello store come
   `store.ListAuditEntries`), non ancora esposto via API.

## Conseguenze

- Positive: i driver di pagamento sono testabili come funzioni pure di richiesta e credenziali; si può
  provare Arenile con `docker compose up` senza configurazione; il TOTP è attivabile dal pannello;
  Coolify e CapRover creano l'owner senza terminale.
- Negative: chi scrive un driver deve usare la firma con `creds`; la password del primo owner passa da
  una variabile d'ambiente (va rimossa dopo il primo accesso, come indicato nelle guide).
  `ARENILE_DEV` non deve mai essere usata in produzione: il log di avvio riporta `"dev": true`.
- Verifiche: `TestRegistry_BuiltinsAndExtensions` (driver `manual` con la firma attuale),
  `TestParse_DevGeneratesKeyOnce`, `TestParse_ProdDoesNotGenerateKey`, `TestNew_RequiresServeConfig`,
  `TestService_TOTPEnrolmentAndLogin`, `TestNew_BootstrapOwner`, `TestParse_Invalid` (bootstrap incompleto).
