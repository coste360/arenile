# Documentazione di Arenile

Sito della documentazione (spec 28, M4) costruito con [Astro Starlight](https://starlight.astro.build):
italiano come lingua sorgente (locale radice) e inglese, ricerca integrata (Pagefind), screenshot
generati da Playwright sui dati dimostrativi e componenti interattivi reali importati da `web/shared`
(alias `@arenile/shared`, nessuna copia).

È un pacchetto npm a sé (come `e2e/`), fuori dai workspace di `web/`. Si usa solo tramite `make`:

| Comando | Cosa fa |
|---|---|
| `make docs-dev` | server di sviluppo con ricarica |
| `make docs-build` | build statica in `docs-site/dist` (pubblicata a ogni release) |
| `make docs-screenshots` | rigenera gli screenshot chiaro/scuro in `src/assets/screenshots/` |

Struttura dei contenuti (`src/content/docs/`, stessi percorsi in `en/`):

- `gestore/` guida del gestore, funzione per funzione;
- `installazione/` installazione e operatività (GitHub: sorgenti e istruzioni; immagine Docker pubblica su GHCR;
  pacchetti desktop dal portale Coste360 previo accesso);
- `api/` API e webhook (riferimento generato da `api/openapi.yaml`);
- `sviluppo/` architettura, estensioni, driver di pagamento, contributi.

`astro` è fissato a 7.4.1 perché le versioni successive richiedono Node.js >= 22.23.4.
