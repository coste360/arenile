# Changelog

Tutte le modifiche rilevanti di Arenile sono documentate in questo file.

Il formato segue [Keep a Changelog](https://keepachangelog.com/it-IT/1.1.0/) e il progetto adotta il [Versionamento Semantico](https://semver.org/lang/it/). I rilasci sono generati a partire dai messaggi [Conventional Commits](https://www.conventionalcommits.org/it/v1.0.0/). Ogni versione minore include, quando necessario, una sezione **Note di aggiornamento**.

## [Unreleased]

Milestone **M0 — Fondamenta**, **M1 — Inventario e mappa** e **M2 — Prezzi e policy** (completate, non rilasciate).

### Added

- Pannello e pagina di prenotazione con UI kit shadcn/ui (ADR 0009): barra laterale con selettore dello stabilimento e menu utente, breadcrumb, palette comandi (Ctrl/⌘+K), tema chiaro/scuro/sistema, toast; tabelle con ordinamento e filtro, pannelli laterali di modifica, calendari e selettori di date gg/MM/aaaa in italiano al posto di `<input type="date">`, importi con `MoneyInput`; editor della mappa con barra degli strumenti a icone, zoom fino al 400%, legenda e pannello proprietà; stagioni con linea temporale e calendario, chiusure su calendario, simulatore con grafico e dettaglio per giorno; pagina pubblica mobile-first con passo "Quando" e selettore del periodo.
- Controllo `web/scripts/css-guard.mjs` in `npm run lint`: rifiuta fogli di stile diversi dal tema (anche in `public/` e `.html`), CSS-in-JS, `style={…}` e stili programmatici fuori dalle esenzioni elencate (geometria della mappa, variabili del tema, iframe dell'embed), valori arbitrari di Tailwind che aggirano il tema (colori, `text-[10px]`, proprietà) e i vecchi token `--ar-*`.
- Test e2e del guscio del pannello (tema scuro persistente, palette comandi) e del selettore di date della pagina pubblica.
- M1 — stabilimenti: creazione di nuovi stabilimenti dal pannello (solo owner dell'organizzazione) con selettore, impostazioni con `If-Match` (fuso, lingue, origini embed, tema, impostazioni prezzi, festività locali, policy predefinita), archiviazione e ripristino.
- M1 — stagioni, periodi tariffari (senza sovrapposizioni, serializzati sulla stagione) e giorni di chiusura per fascia AM/PM; copia delle date di stagione.
- M1 — utenti e ruoli (solo owner): aggiunta con password iniziale o temporanea, cambio ruolo e ambito, disattivazione con chiusura delle sessioni; l'ultimo owner attivo non può essere rimosso.
- M1 — mappe: documento versionato salvato in una transazione con `ETag`/`If-Match` (412 con `current_version`, 428 senza header), zone (posto, zona, quantità), pool con capacità per periodo, pool implicito per le zone a zona sincronizzato con le postazioni attive, generatore di file con anteprima, eliminazione rifiutata per postazioni con allocazioni future (verifica predisposta, sempre "nessuna" fino a M3).
- M1 — catalogo: componenti, pack ed extra con archiviazione quando referenziati.
- M1 — configurazione guidata: modelli `classic_rows` (8 × 15), `quantity_beach`, `lido_with_pool`, `empty`, listino rapido per categoria × periodo, stato della configurazione; `seed demo` completo e idempotente (owner dimostrativo, listino, prezzi fissi, policy, codice promo e convenzione).
- M2 — persistenza e API di regole di prezzo, prezzi a durata fissa, codici promozionali, convenzioni e policy di prenotazione collegati ai motori puri; simulatore, matrice di listino, verifica di copertura e copia di stagione del listino.
- Pannello: impostazioni dello stabilimento con anteprima del tema, stagioni con timeline dei periodi, chiusure, zone, pool, catalogo, utenti, procedura guidata in 6 passi, editor della mappa SVG (selezione multipla, trascinamento, tastiera, annulla/ripeti, allinea/distribuisci, rinumerazione, dialogo di conflitto), regole di prezzo con riepilogo in linguaggio naturale, griglia dei prezzi fissi, matrice con esportazione CSV, calendario della copertura, simulatore con dettaglio per regola, policy con esempi della spec 9.
- Eventi `config.map_changed` e `config.pricing_changed` scritti nell'outbox nella stessa transazione delle modifiche, con un unico payload versione 1 (`entity`, `entity_id`, `change`, più `season_id`, `map_id`, `code`, `version` e, per la configurazione guidata, `reason`, `template`, `map_ids`, `zone_ids`, `counts`; spec 18.1).
- `GET /setup/status` restituisce `price_categories`: le categorie di prezzo che il listino rapido deve coprire (zone e override delle postazioni); la griglia del passo 4 della procedura guidata le usa per le righe.
- L'eliminazione di una zona, da sola o con la sua mappa, è rifiutata (409) quando regole di prezzo, codici promozionali o convenzioni (`conditions.zone_ids`), pack (`allowed_zone_ids`) o policy (`scope.zones`) la referenziano; l'errore elenca le entità da modificare prima.
- Test end-to-end Playwright del percorso di accettazione M1/M2: login, nuovo stabilimento dal selettore, modello classico, listino rapido, spostamento e salvataggio di una postazione, simulatore.
- M1/M2: contratto API di amministrazione per stabilimenti, utenti, stagioni, periodi tariffari, chiusure, mappe (documento versionato con `ETag`/`If-Match`, generatore di file), zone, pool, componenti, pack, extra, regole di prezzo, prezzi a durata fissa, codici promozionali, convenzioni, policy, simulatore, matrice di listino, verifica di copertura, copia di stagione e configurazione guidata; migrazione `00002_inventory_catalog_pricing` per SQLite e PostgreSQL; nuovi tipi di errore `version_conflict`, `precondition_required`, `spot_has_future_allocations`, `not_implemented`; pannello con rotte per stabilimento (`/admin/e/{id}/...`), selettore dello stabilimento e menu Configurazione.

- Specifica tecnica e funzionale completa in `docs/spec/` (un file per sezione).
- Struttura del repository: monobinario Go (`cmd/arenile`), SPA pannello e booking in `web/` (npm workspaces), contratto API in `api/openapi.yaml`.
- Librerie di base: `internal/clock` (orologio iniettabile), `internal/money` (importi in centesimi, arrotondamento half-even, scorporo IVA), `internal/calendar` (date di calendario, intervalli, fasce AM/PM).
- CLI con i comandi `serve`, `migrate up|status`, `admin create`, `seed demo`, `backup --out`, `restore --in`, `version`.
- Configurazione da variabili d'ambiente (`ARENILE_*`, `DATABASE_URL`).
- Migrazioni per SQLite e PostgreSQL, autenticazione e ruoli, audit log, outbox eventi, job runner in-process.
- Motore prezzi e motore delle policy come librerie pure con test table-driven e di proprietà.
- Dockerfile multi-stage, `docker-compose.yml` e variante PostgreSQL.
- Makefile come unica interfaccia (`setup`, `dev`, `gen`, `lint`, `test`, `test-db`, `test-race`, `e2e`, `check`, `check-full`, `seed`, `fmt`) e CI su GitHub Actions.
- Supporto agli agenti AI: `AGENTS.md`, `CLAUDE.md`, `.claude/` con regole, skill e subagent.
- README tecnico in italiano, guide per gestori in `docs/guida/` (installazione, privacy e GDPR, nota fiscale).
- File di governance: licenza AGPL-3.0, CLA (bozza da validare), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1), `SECURITY.md`, `TRADEMARK.md`, modelli di issue e pull request.

### Changed

- Interfaccia senza CSS custom: un solo foglio di stile (`web/shared/src/styles/globals.css`, variabili del tema shadcn); rimossi `index.css` delle due app, `tokens.css`, le primitive scritte a mano e i token `--ar-*`. Il tema dello stabilimento sovrascrive le variabili shadcn (`--primary`, `--highlight`, `--radius`, `--app-font`) con lo stesso controllo del contrasto.
- Matrice di listino e linea temporale delle stagioni mostrano il nome del periodo tariffario invece del codice; il simulatore mostra l'effetto delle regole come importo o percentuale ("= 25,00 €", "−10%") invece del formato interno del motore.
- JS iniziale della pagina di prenotazione: 119,7 KB gzip su 150 (prima 118,3).
- Licenza cambiata da AGPL-3.0 a Business Source License 1.1 (BUSL-1.1), con conversione in AGPL-3.0-or-later tre anni dopo il rilascio di ogni versione (ADR 0007); aggiunto `.github/CODEOWNERS`.
- Roadmap estesa con M6 (edizione desktop, locale o client remoto) e M7 (app mobile), sezione 29 della specifica.
- `pkg/ext`: `PaymentProvider.ParseWebhook` riceve anche le credenziali decifrate dello stabilimento (`ParseWebhook(ctx, r, creds)`), diversamente dalla spec 13.1 ([ADR 0006](docs/adr/0006-deviazioni-m0-dalla-specifica.md)).
- `make test-race` compila con il tag `integration` e fallisce se non trova test `TestConcurrency_*`.
- L'editor della mappa usa un *roving tabindex*: la mappa è un solo punto di tabulazione (il primo elemento selezionato, altrimenti la prima postazione); Inizio/Fine e PagSu/PagGiù spostano fuoco e selezione tra gli elementi, le frecce lo fanno in sola lettura.

### Added (deviazioni e integrazioni alla specifica, ADR 0006)

- Modalità sviluppo `ARENILE_DEV=true` / `arenile serve --dev`: URL di base facoltativo e chiave segreta generata e salvata in `dev-secret.key`.
- Endpoint `POST /api/admin/v1/auth/totp/enroll` e `POST /api/admin/v1/auth/totp/confirm` per attivare la verifica in due passaggi.
- Creazione del primo owner all'avvio da `ARENILE_BOOTSTRAP_OWNER_EMAIL`, `_NAME`, `_ORG` e `ARENILE_ADMIN_PASSWORD` (solo se non esistono utenti), necessaria con l'immagine distroless senza shell.
- `compose.yaml` nella radice e `make up`: istanza locale vuota compilata dai sorgenti; i compose in `deploy/` hanno una sezione `build`.

### Fixed

- Selettore di date: l'elenco degli anni non andava oltre l'anno corrente, impedendo di scegliere date della stagione successiva.
- Campo importo: un valore non valido veniva cancellato all'uscita dal campo invece di restare visibile con l'errore.
- Un `PUT` della mappa senza header `If-Match` risponde 428 `precondition_required` invece di 400.
- `If-Match: *` è una precondizione soddisfatta da qualsiasi versione (RFC 9110): il salvataggio della mappa e l'applicazione del generatore la accettano invece di rispondere 428. La forma debole `W/"3"` resta accettata (proxy che comprimono le risposte, ADR 0008).
- Salvataggio della mappa e modifica o eliminazione di una zona si serializzano sulle righe delle zone coinvolte: su PostgreSQL non è più possibile che un salvataggio che aggiunge postazioni a una zona e la modifica della stessa zona in `quantity` vadano entrambi a buon fine (`TestConcurrency_ZoneUpdateVsSaveMap`); la capacità del pool implicito è calcolata dopo il lock.
- L'eliminazione di una mappa è rifiutata, come quella della zona, quando una zona a quantità della mappa ha pool creati a mano: prima i pool venivano cancellati in cascata senza audit, azzerando il `pool_id` dei componenti.
- Il simulatore con `season_id` esplicito rifiuta date fuori dalla stagione (errore sul campo `starts_on`) invece di calcolare un prezzo fuorviante o un 422 `no_price`.
- La verifica delle allocazioni future fallisce se il fuso orario dello stabilimento non è caricabile, invece di ripiegare su UTC (stessa regola delle policy).
- Il simulatore prezzi mostra il nome delle regole applicate invece del loro identificativo.
- La consegna dei webhook rivendica ogni tentativo con un lease: più istanze sullo stesso database non inviano due volte lo stesso webhook.
- Il recupero di una `Idempotency-Key` scaduta non può più cancellare la chiave appena rivendicata da una richiesta concorrente.
- Il segreto di firma degli endpoint webhook è cifrato con l'id dell'endpoint come dato associato (AAD).

[Unreleased]: https://github.com/coste360/arenile/commits/main
