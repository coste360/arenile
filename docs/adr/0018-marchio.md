# 0018 — Marchio ufficiale: Arenile — Coste360.it

- **Stato:** accettato
- **Data:** 2026-10-23
- **Sezioni della specifica:** `docs/spec/12-pagina-pubblica-e-widget-incorporabile.md` (12.5 tema), `docs/spec/README.md`

## Contesto

Il nome **Arenile** e il dominio **arenile.app** sono confermati; Coste360 SRL ha fornito il brand kit
v1.0 (simbolo "A" con sabbia e onda, icone, logotipi in Titillium Web, colori navy `#092D62`, oceano
`#0D79AC`, ciano `#16CDE1`, sabbia `#FFF0D1`, testo `#0B305F`) e il logo aziendale Coste360. I logotipi
del kit contengono `<text>`: usati come `<img>`, in un'email, in un'icona desktop o in un'immagine social
il font non c'è e la resa cambia. La pagina pubblica ha un budget di peso (spec 3.3) e il pannello non
deve caricare font solo per un logo.

## Decisione

- I file ufficiali stanno in `assets/brand/arenile/` e `assets/brand/coste360/`, ciascuno con un
  `NOTICE.md`: tutti i diritti riservati a Coste360 SRL, **esclusi dalla licenza del codice** (BSL 1.1 /
  AGPL-3.0), uso del marchio secondo `TRADEMARK.md`.
- Il testo dei logotipi è **convertito in tracciati** con Titillium Web (OFL-1.1) da
  `scripts/brand/build-brand.mjs`, che produce anche i PNG (favicon, PWA, apple-touch, maschera, email,
  tray, icona macOS, immagine social) con il Chromium di Playwright. Gli output sono versionati: build e
  CI non dipendono dallo script. **Nessuna nuova dipendenza di runtime**: il font non viene caricato da
  nessuna app (pannello, pagina pubblica, documentazione).
- Il logo Coste360 è il PNG originale fornito dall'azienda (`logo-{primary,white}.png`, 781x312), usato così
  com'è: non viene ridisegnato né vettorializzato. Il link è sempre https://www.coste360.it/.
- Il tema shadcn predefinito (`web/shared/src/styles/globals.css`) ha i **colori allineati al design
  system Coste360** (decisione del titolare, 2026-10-23; fonte: `DESIGN.md` dell'app Coste360): primario
  navy `#0b2542`, testo ink `#0f172a` / `#475569`, superfici `#ffffff` / `#f8fafc`, bordi slate, stati
  `#15803d` / `#f59e0b` e, per l'errore, `#b91c1c` (un passo più scuro di `#dc2626`, che come testo sulla
  propria tinta non arriva a 4,5:1); focus e link in primary-lighter `#185482` (l'azzurro `#38bdf8` non
  raggiunge 3:1 sul bianco ed è solo decorativo); in scuro, derivato perché Coste360 non ne ha uno, primario
  azzurro `#38bdf8` con testo navy su superfici navy. Componenti, layout, raggi e tipografia shadcn restano
  invariati, così come i colori della mappa (`--status-*`, `--map-*`, `--beach-*`) e i file del logo. Le
  coppie testo/sfondo dei token, i bordi dei campi (3:1) e l'anello di focus sono verificati da
  `web/shared/src/theme/brand.test.ts`; i colori usati solo come grafica (avviso `#f59e0b`, alcune serie dei
  grafici) restano sotto 3:1 sul bianco, come prima, e vanno sempre accompagnati da un testo. La pagina
  pubblica mantiene il tema dello stabilimento (spec 12.5): cambia solo il default (`DEFAULT_PRIMARY` =
  primary-lighter `#185482`, AA con il testo bianco); link e anello di focus hanno una variante chiara e una
  scura (`--ring-light` / `--ring-dark`, come `--primary-on-surface`), perché un primario abbastanza scuro
  per il testo bianco non arriva a 3:1 come anello sulle superfici scure. Il primo tema (navy `#092D62`,
  oceano `#0D79AC`, ciano `#16CDE1` del brand kit) è sostituito; il brand kit resta la fonte del logo.
  Questa ADR è emendata invece di essere sostituita perché la decisione è dello stesso giorno e non è
  ancora rilasciata. `index.html` porta favicon e immagine social Arenile come default; nel browser `seo.ts` li
  sostituisce con il logo dello stabilimento quando c'è. I crawler dei social non eseguono JavaScript e
  vedono quindi sempre la scheda Arenile: una scheda per stabilimento richiederebbe di iniettare i meta lato
  server su `/b/{slug}` (non fatto). Sulla pagina pubblica c'è solo "powered by Arenile" (dicitura lasciata
  in inglese in tutte le lingue, come richiesto), senza logo Coste360.
- Nell'interfaccia il marchio passa solo dai componenti condivisi `BrandLogo`, `MadeByCoste360`,
  `Coste360Signature` (`web/shared/src/ui/patterns/Brand.tsx`), mai da icone lucide; le copie SVG in
  `web/shared/src/brand/` sono verificate identiche a `assets/brand` da un test.
- Le email portano la firma "Arenile — Coste360.it" con il marchio come immagine inline (`cid:`), senza
  immagini remote.

## Conseguenze

- Positive: logo identico ovunque, zero byte di font sulla pagina pubblica, sorgente unica dei file.
- Negative: i tracciati vanno rigenerati con lo script a ogni cambio del kit; ogni email pesa ~3,5 KB in
  più; l'icona `.icns` è più pesante (gradienti).
- Verifiche: `brand.test.ts` (contrasto AA), `Brand.test.tsx` (copie identiche, niente `<text>`),
  `TestBrandMarkMatchesBrandKit` (marchio delle email), budget della pagina pubblica.
- Da rivedere con un legale: testi di `TRADEMARK.md` e dei `NOTICE.md` (asset creati con l'aiuto
  dell'intelligenza artificiale).
