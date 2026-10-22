# 0001 — Un unico binario Go con le SPA incorporate

- **Stato:** accettato
- **Data:** 2026-10-22
- **Sezioni della specifica:** `docs/spec/01-obiettivi-e-principi.md`, `docs/spec/03-architettura-e-stack.md`, `docs/spec/24-deploy-configurazione-e-operativita.md`

## Contesto

Arenile deve poter essere installato in pochi minuti da un gestore di stabilimento balneare o da chi lo
assiste, anche su un VPS economico o un Raspberry Pi, senza servizi esterni obbligatori (niente Redis,
code esterne, object storage). Servono API REST, aggiornamenti in tempo reale, job periodici (scadenza
degli hold, riconciliazione dei pagamenti, outbox) e due interfacce web (pannello e pagina pubblica).
Alternative valutate: microservizi con coda esterna; backend Node/TypeScript con frontend servito da un
web server separato; framework full-stack con rendering lato server.

## Decisione

- Il backend è scritto in Go ed è distribuito come **un solo binario statico** (`cmd/arenile`,
  `CGO_ENABLED=0`) che contiene API, SSE, job runner in-process, outbox e migrazioni.
- Le SPA `web/admin` e `web/booking` sono compilate con Vite e incorporate con `embed.FS`
  (`internal/http/static`); se la build frontend manca il server mostra una pagina segnaposto.
- Un'immagine Docker distroless con un solo volume (`/data`); la configurazione è solo da variabili
  d'ambiente.
- Il binario espone sottocomandi per le operazioni (`serve`, `migrate`, `admin create`, `seed demo`,
  `backup`, `restore`, `version`).

## Conseguenze

- Positive: installazione e aggiornamento banali; nessun problema di versioni disallineate tra frontend
  e backend; consumo di memoria contenuto; build multi-architettura semplice.
- Negative: la scalabilità orizzontale richiede accorgimenti (broker SSE e job runner sono in memoria;
  per più istanze servono PostgreSQL e `ext.EventBus` con `LISTEN/NOTIFY`); la build del binario
  completo richiede prima la build del frontend.
- Verifiche: la CI compila il binario senza CGO; `go build ./...` deve funzionare anche senza build
  frontend (`.gitkeep` nelle cartelle statiche).
