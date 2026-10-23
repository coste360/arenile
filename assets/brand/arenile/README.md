# Arenile — Brand Kit v2.0 (rifinito)

Marchio **Arenile**: una "A" color sabbia la cui traversa è un'onda azzurra, sopra una duna di sabbia calda, su
un riquadro navy. Nessun sottotitolo o byline nei logotipi. Font dei logotipi: **Titillium Web** 700.

## Palette
- Navy `#0B2542` · Sabbia `#FFF0D1` · Sabbia calda `#F2C893` · Onda `#38BDF8` · Blu secondario `#0EA5E9`
  (`arenile-tokens.css`).

## Contenuto
- **Icone app** (riquadro navy): `arenile-app-icon.svg` (principale), `-flat`, `-compact` (piccole dimensioni),
  `-monochrome` (bianca su navy), `-light` (riquadro sabbia, A navy); `arenile-favicon.svg`.
- **Simbolo** (trasparente): `arenile-symbol-color.svg` (sfondi chiari: A navy, onda azzurra, duna sabbia
  calda), `arenile-symbol-color-dark.svg` (sfondi scuri: A sabbia, colori originali del kit),
  monocromatici `-navy`, `-white`, `-sand`, `-outline` (tecnico).
- **Logotipi** (simbolo + "Arenile"): `arenile-logo-horizontal`, `arenile-logo-header` (navigazione),
  `arenile-logo-stacked` (verticale). Senza suffisso = sfondi chiari (A navy, testo navy); `-dark` = sfondi
  scuri (A sabbia, testo bianco). `arenile-wordmark.svg` / `-white.svg`: solo scritta.
- **PNG**: `arenile-app-icon-{16,32,64,128,180,192,256,512,1024}.png`, `arenile-app-icon-light-1024.png`,
  `arenile-symbol-color{,-dark}-1024.png`.
- `outlined/` e `png/`: file derivati (vedi `NOTICE.md`), generati da `scripts/brand/build-brand.mjs`.

## Dove usare cosa
- **Icona app a colori** (riquadro navy, A sabbia): è l'icona dell'app. App desktop (Dock, barra di Windows,
  installer, Linux), icone PWA, apple-touch, favicon, schede social, icona nell'intestazione della
  documentazione.
- **Logo/simbolo a colori, variante chiara** (A navy): punti principali del marchio su fondo chiaro (login,
  intestazione della barra laterale in tema chiaro, logo della documentazione in tema chiaro).
- **Simbolo/logo bianco**: su fondi navy o blu Coste360 (intestazione scura del sito, piè di pagina navy,
  barra laterale e login in tema scuro, fasce e hero navy). La template image della barra dei menu di macOS
  è la stessa silhouette monocromatica (nera, colorata dal sistema).
- **Simbolo/logo navy monocromatico**: su fondo chiaro dove serve un segno a un colore (marchi piccoli,
  "powered by Arenile" della pagina pubblica, stampe e PDF).
- Le varianti `-dark` a colori (A sabbia) restano nel kit per i fondi navy in cui serve il colore pieno.

## Titillium Web
Gli SVG dei logotipi in questa cartella hanno il testo vivo (`<text font-family="Titillium Web">`) per poterli
modificare; non incorporano il font. Le app usano le copie in `outlined/`, con il testo convertito in tracciati.

## Rifinitura (2026-10-23)
Rispetto al kit v2.0 fornito: distanza uniforme (22 unità su 1024) tra onda e gambe della "A" (taglio della
gamba sinistra che segue la curva dell'onda, taglio della gamba destra e piede uniformi), rimossa la tacca
storta sul bordo interno del piede destro, simbolo centrato otticamente nel riquadro, varianti per sfondi chiari
con la A navy, testo dei logotipi convertito in tracciati (`outlined/`). Il disegno resta lo stesso.
