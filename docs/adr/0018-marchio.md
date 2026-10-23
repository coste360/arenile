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
- Il lockup Coste360 (`coste360-logo-{primary,white,black}.svg`) è ricostruito sull'icona ufficiale e
  sul PNG fornito: la scritta "Coste360.it" combacia con Titillium Web **Black (900)**, non 700.
- Il tema shadcn predefinito (`web/shared/src/styles/globals.css`) usa il navy come primario chiaro,
  l'oceano per focus e link, il ciano come primario scuro su superfici blu notte; ogni coppia testo/sfondo
  è verificata AA da `web/shared/src/theme/brand.test.ts`. La pagina pubblica mantiene il tema dello
  stabilimento (spec 12.5): cambia solo il default (`DEFAULT_PRIMARY` = oceano, leggibile in entrambi gli
  schemi). `index.html` porta favicon e immagine social Arenile come default; nel browser `seo.ts` li
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
