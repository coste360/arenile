## 28. Roadmap e criteri di accettazione

### M0 — Fondamenta
- Repository, struttura, CI, Dockerfile, compose, OpenAPI iniziale.
- Configurazione, migrazioni su entrambi i dialetti, CLI, autenticazione e ruoli, audit log, outbox, job runner.
- Supporto agli agenti (sezione 27): `AGENTS.md`, `CLAUDE.md`, `.claude/` con regole, skill, subagent e impostazioni, script degli hook, devcontainer, template dei task, `docs/architecture.md`, `docs/invariants.md`, primi cinque ADR, Makefile completo, `make seed`.
- **Accettazione:** `docker compose up` avvia un'istanza vuota, `arenile admin create` permette il login, la CI è verde su SQLite e PostgreSQL. Un agente avviato in un devcontainer nuovo, con il solo task "aggiungi l'endpoint `GET /api/admin/v1/establishments`", completa il lavoro usando `/new-endpoint` e supera `make check` senza istruzioni aggiuntive.

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
- **Accettazione:** prenotazione completa da smartphone dentro un iframe su un dominio esterno in meno di 2 minuti; nessuna doppia vendita tra cassa e online in un test con carico parallelo; budget di peso e Lighthouse rispettati.

### M5 — Completamento della stagione
- Rinnovi degli abbonamenti, lista d'attesa, crediti.
- Report completi e confronto anni.
- PWA del pannello con offline limitato (21.2).
- Webhook in uscita configurabili, chiavi API.
- Traduzioni inglese, tedesco, francese.
- **Accettazione:** ciclo completo di fine stagione → nuova stagione (copia configurazione, proposta rinnovi, conferme) eseguito senza interventi manuali sul database.

### Dopo l'MVP
- Driver Satispay e altri provider; driver per registratori telematici.
- Notifiche WhatsApp e SMS tramite estensioni.
- App Expo per lo staff in spiaggia.
- Bar e ristorante con addebito sulla postazione.
- Prezzi per occupazione (regole `occupancy_percent`) abilitati nell'interfaccia.
- Multi-mappa avanzata e virtualizzazione per stabilimenti molto grandi.
