# Documentazione di Arenile

Sito della documentazione (spec 28, M4) costruito con [Astro Starlight](https://starlight.astro.build):
italiano come lingua sorgente (locale radice) e inglese, ricerca integrata (Pagefind), riferimento API
generato da `api/openapi.yaml`, screenshot generati da Playwright sui dati dimostrativi e componenti
interattivi reali importati da `web/shared` (alias `@arenile/shared`, nessuna copia).

È un pacchetto npm a sé (come `e2e/`), fuori dai workspace di `web/`, ma usa le dipendenze di
`web/node_modules` per i componenti condivisi: `make setup` installa entrambi. Si usa solo tramite `make`:

| Comando | Cosa fa |
|---|---|
| `make docs-dev` | server di sviluppo con ricarica (http://localhost:4321) |
| `make docs-build` | build statica in `docs-site/dist`: ricerca, riferimento API, verifica dei link interni |
| `make docs-check` | verifica dei tipi (componenti, configurazione, stringhe) |
| `make docs-preview` | serve la build statica |
| `make docs-screenshots` | compila SPA e binario, avvia `arenile` con `seed demo` su una cartella temporanea e rigenera screenshot chiaro/scuro e dati demo |

`make docs-build` non richiede il server: screenshot (`src/assets/screenshots/{light,dark}/*.webp`) e dati
demo (`src/data/*.json`) sono generati e committati. Con `ARENILE_DOCS_STRICT=1` (CI delle release) uno
screenshot mancante fa fallire la build; altrimenti al suo posto compare un segnaposto.

## Struttura

- `src/content/docs/` contenuti (`gestore/`, `installazione/`, `api/`, `sviluppo/`, stessi percorsi in `en/`);
- `src/components/` componenti per le pagine `.mdx` (registro in `src/components/index.ts`,
  vetrina dal vivo su `/componenti/`):
  - `<Screenshot name="admin-map" alt="…" caption="…" />` variante chiara o scura secondo il tema, ingrandimento al clic,
    cornice browser o telefono; i nomi validi sono in `screenshots.manifest.json` (un nome sconosciuto fa fallire la build);
  - `<LiveMap />` la mappa vera (`BeachMap` + kit Coste360) sulla spiaggia dimostrativa;
  - `<LivePriceSimulator pack="comfort" period="high" duration="week" />` risultati reali del motore prezzi
    (`POST /pricing/simulate`) sul listino dimostrativo, nessun calcolo duplicato nel browser;
  - `<StatusBadges />` gli stati delle postazioni con i badge condivisi (HTML statico, zero JavaScript);
  - `<ApiOperation id="createPublicBooking" />` metodo, percorso e sintesi letti dal contratto, con link al riferimento;
- `src/styles/global.css` integrazione Tailwind v4 + Starlight; i token (colori, raggio, font, palette della mappa)
  vengono da `web/shared/src/styles/globals.css` tramite `scripts/sync-theme.mjs` (file generato e ignorato da git);
- `src/routeData.ts` tiene i 200+ link del riferimento API fuori dalla barra laterale delle altre pagine;
- `screenshots.manifest.json` elenco degli screenshot (nome, percorso, viewport, descrizione): fonte unica per
  la pipeline di acquisizione (`e2e/docs/`) e per i contenuti.

## Screenshot

La pipeline vive in `e2e/docs/` e usa l'installazione di Playwright di `e2e/`. Per aggiungere uno screenshot:
aggiungi la voce al manifest, poi `make docs-screenshots` (oppure `DOCS_ONLY=nome make docs-screenshots`).
Orologio del browser fermo sul giorno dimostrativo, tema chiaro e scuro, animazioni disattivate, istanti
mascherati, uscita WebP ottimizzata: due esecuzioni producono gli stessi file.

## Pubblicazione

`.github/workflows/docs.yml` costruisce il sito su main, sulle pull request e a ogni release (chiamato da
`release.yml`, con screenshot rigenerati). Il sito è un artifact; il deploy su GitHub Pages si attiva con la
variabile di repository `DOCS_PAGES=true` (opzionali `DOCS_SITE_URL`, `DOCS_BASE_PATH`, `DOCS_STRICT`).

## Indirizzo e marchio

L'indirizzo canonico è `https://docs.arenile.app`; si cambia con la variabile d'ambiente `ARENILE_DOCS_SITE`
(origine, ad esempio `https://arenile.app`) e `ARENILE_DOCS_BASE` (percorso, ad esempio `/docs/`): canonical,
sitemap e immagine social (`public/og.png`, URL assoluto) li seguono. Logo (chiaro/scuro, testo convertito in
tracciati), favicon (`public/favicon.svg` con fallback PNG) e logo Coste360 del piè di pagina (`public/brand/`)
sono copie dei file del kit in `assets/brand/` (© Coste360 SRL, fuori dalla licenza del codice, vedi
`assets/brand/*/NOTICE.md` e `TRADEMARK.md`); i colori vengono dai token condivisi di `globals.css`.

`astro` è fissato a 7.4.1 perché le versioni successive richiedono Node.js >= 22.23.4.
