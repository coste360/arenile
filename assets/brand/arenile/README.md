# Arenile — Coste360.it | Brand Kit v1.0

La dicitura ufficiale è **Arenile — Coste360.it**. Il font dei logotipi è **Titillium Web** (titoli: 700, byline: 600).

## Cosa contiene
- `arenile-symbol-color.svg`: **A** a colori, trasparente, come tracciati vettoriali.
- `arenile-symbol-navy.svg`, `arenile-symbol-white.svg`: varianti monocromatiche.
- `arenile-app-icon.svg`, `arenile-app-icon-flat.svg`: icone quadrate con tile e angoli arrotondati.
- `arenile-logo-horizontal.svg`, `arenile-logo-horizontal-dark.svg`: logo completo per light/dark.
- `arenile-logo-stacked.svg`: logo verticale.
- `arenile-logo-header.svg`: compatto per navigazione dell'app.
- `arenile-wordmark.svg`: solo scritte.
- `arenile-favicon.svg`: simbolo leggibile a dimensioni molto piccole.
- `anteprima.html`: catalogo locale dei vari logo.
- `manifest.json` e `arenile-tokens.css`: metadati e colori.

## Font Titillium Web
I file SVG usano l'attributo `font-family="Titillium Web"` per il testo (non incorporano né distribuiscono file font). Per la resa precisa, nel progetto React/Next.js carica la famiglia con `next/font/google`:

```tsx
import { Titillium_Web } from 'next/font/google';
const titillium = Titillium_Web({ subsets: ['latin'], weight: ['400', '600', '700'], display: 'swap' });
// applica titillium.className alla root o al contenitore del logo
```

**Importante:** se l'SVG è usato come `<img src="...">`, il font esterno della pagina potrebbe non arrivare all'SVG. Per avere Titillium Web nel logo, usa l'SVG inline (SVGR) oppure converti il testo in contorni dopo il caricamento del font. L'anteprima HTML usa gli SVG inline.

## Origine visiva
Questa è una **reinterpretazione vettoriale** del concept generato con la A, la sabbia e un'onda. Non è una vettorializzazione pixel-per-pixel della precedente immagine raster; gradienti e forme sono modificabili in SVG.

## PNG export pronti
- `arenile-app-icon-{1024,512,192,180}.png` per store, PWA o anteprime.
- `arenile-symbol-color-1024.png` con trasparenza.
