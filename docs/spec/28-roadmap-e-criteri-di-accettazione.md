## 28. Roadmap e criteri di accettazione

### M0 — Fondamenta
- Repository, struttura, CI, Dockerfile, compose, OpenAPI iniziale.
- Configurazione, migrazioni su entrambi i dialetti, CLI, autenticazione e ruoli, audit log, outbox, job runner.
- Supporto agli agenti (sezione 27): `AGENTS.md`, `CLAUDE.md`, `.claude/` con regole, skill, subagent e impostazioni, script degli hook, devcontainer, template dei task, `docs/architecture.md`, `docs/invariants.md`, primi cinque ADR, Makefile completo, `make seed`.
- **Accettazione:** `docker compose up` avvia un'istanza vuota, `arenile admin create` permette il login, la CI è verde su SQLite e PostgreSQL. Un agente avviato in un devcontainer nuovo, con il solo task "aggiungi l'endpoint `GET /api/admin/v1/establishments/{id}/audit`" (registro attività dello stabilimento; l'elenco degli stabilimenti è già parte di M0, vedi ADR 0006), completa il lavoro usando `/new-endpoint` e supera `make check` senza istruzioni aggiuntive.

### M1 — Inventario e mappa
- Stabilimento, stagioni, periodi, chiusure.
- Editor della mappa con generatore di file, zone, postazioni, decorazioni, pool.
- Catalogo: componenti, pack, extra.
- Configurazione guidata con i modelli.
- **Accettazione:** un gestore configura un lido di 8 file da 15 posti in meno di 15 minuti partendo dal modello.

### M2 — Prezzi e policy
- Motore prezzi completo (8.1–8.6), prezzi a durata fissa, codici promozionali, convenzioni.
- Simulatore, matrice di listino, verifica di copertura, copia di stagione.
- Policy (sezione 9).
- **Accettazione:** suite di test del motore prezzi con almeno 150 casi; la matrice di listino coincide con i calcoli del simulatore.

### M3 — Prenotazioni dal pannello e cassa
- Motore di disponibilità con allocazioni, hold, pool, quote.
- Vista operativa della mappa, calendario planning, prenotazioni, clienti, abbonamenti stagionali e rilasci.
- Pagamenti manuali, cassa con apertura e chiusura, esportazione corrispettivi.
- SSE per l'aggiornamento della mappa.
- **Accettazione:** test di concorrenza superati; una giornata simulata con 300 operazioni non produce incoerenze tra allocazioni, prenotazioni e incassi.

### M4 — Pagina pubblica e pagamenti online (MVP pubblico)
- Pagina pubblica nelle tre modalità (posto, zona, quantità), widget `embed.js`, tema.
- Driver Stripe (Checkout), webhook, riconciliazione, rimborsi.
- Link magico per la gestione del cliente, email transazionali, QR e `.ics`.
- Assegnazione automatica per le zone `zone`.
- Documentazione completa del sistema (sito `docs-site/`, Astro Starlight, italiano come lingua sorgente e inglese): guida del gestore funzione per funzione, installazione e operatività, API e webhook, sviluppo ed estensioni. Gli screenshot sono generati da Playwright sui dati demo in tema chiaro e scuro e vengono rigenerati a ogni release, quindi non invecchiano. Le pagine includono componenti reali e interattivi (mappa con il kit di elementi, simulatore prezzi su dati demo, badge di stato), presi da `web/shared` senza duplicarli. Il riferimento API è generato da `api/openapi.yaml`. Il sito ha ricerca integrata ed è pubblicato con ogni release.
- **Accettazione:** prenotazione completa da smartphone dentro un iframe su un dominio esterno in meno di 2 minuti; nessuna doppia vendita tra cassa e online in un test con carico parallelo; budget di peso e Lighthouse rispettati.

### M5 — Completamento della stagione
- Rinnovi degli abbonamenti, lista d'attesa, crediti.
- Report completi e confronto anni.
- PWA del pannello con offline limitato (21.2).
- Webhook in uscita configurabili, chiavi API.
- Traduzioni inglese, tedesco, francese.
- **Accettazione:** ciclo completo di fine stagione → nuova stagione (copia configurazione, proposta rinnovi, conferme) eseguito senza interventi manuali sul database.

### M6 — Edizione desktop (sezione 29.1)
- Launcher `cmd/arenile-desktop` con icona nella barra di sistema, server su `127.0.0.1`, apertura automatica del pannello.
- Pacchetti firmati per Windows (installer `.exe`), macOS (`.dmg`, notarizzato) e Linux (AppImage), pubblicati con lo stesso tag dell'edizione server.
- Primo avvio guidato dal browser, chiave segreta nel portachiavi del sistema, backup automatici su cartella a scelta, ripristino dal menu.
- Modalità desktop rilevata dal pannello: pagina pubblica e pagamenti online disattivati con spiegazione del passaggio a un'istanza online.
- Modalità remota: l'app si collega a un'istanza online o privata con token per dispositivo (condiviso con M7) e fa da ponte verso stampante e registratore telematico locali.
- **Accettazione:** su un PC Windows e un Mac senza connessione, un gestore scarica il pacchetto, lo installa e registra la prima prenotazione in cassa in meno di 10 minuti senza usare il terminale; il backup prodotto dal desktop, ripristinato con `arenile restore` su una VPS, porta online tutti i dati senza perdite.

### M7 — App mobile (sezione 29.2)
- Lato core: token per dispositivo con abbinamento tramite QR code, pagina *Dispositivi* nel pannello con revoca, endpoint `GET /api/admin/v1/meta`, pubblicazione di `openapi.yaml` come asset di release, modalità rete locale dell'edizione desktop.
- Repository `coste360/arenile-mobile` (Expo): collegamento all'istanza, mappa del giorno, check-in con QR, incasso in contanti, rilasci, offline con coda.
- Notifiche push facoltative tramite relay che non riceve dati personali.
- **Accettazione:** lo staff collega un telefono a un'istanza scansionando un QR in meno di un minuto; con il telefono offline esegue check-in e incassi che, alla riconnessione, arrivano al server senza duplicati; nessuna richiesta di rete verso domini diversi dall'istanza (verificato con un proxy di ispezione), salvo il relay push se attivato.

### Dopo l'MVP
- Driver Satispay e altri provider; driver per registratori telematici.
- Notifiche WhatsApp e SMS tramite estensioni.
- Bar e ristorante con addebito sulla postazione.
- Prezzi per occupazione (regole `occupancy_percent`) abilitati nell'interfaccia.
- Multi-mappa avanzata e virtualizzazione per stabilimenti molto grandi.
