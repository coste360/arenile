# Beach Map — Refined (shadcn-friendly)

**107 veri SVG vettoriali, ridisegnati** per editor di stabilimenti balneari, con vista dall'alto e senza prospettiva.

## File
- `svg/`: 107 file SVG individuali (120 × 120 viewBox), con `<title>` accessibile, stroke uniforme.
- `catalogo.html`: catalogo con ricerca, filtri, tema light/dark, ingrandimento e salvataggio di ogni SVG.
- `manifest.json`: nomi, categorie e funzione (`bookable`, `structure`, `decoration`).
- `beach-tokens.css`: variabili compatibili con shadcn/ui.

## Inserimento in Next.js / React

### Uso rapido (SVG standalone)
Copia `svg` in `public/beach/`, poi:
```tsx
<img src="/beach/ombrellone_2_lettini.svg" width={76} height={76} alt="Ombrellone con due lettini" />
```

### Tema dinamico (inline SVG)
Per fare ereditare le variabili CSS di shadcn, gli SVG devono essere **inline nel DOM** (ad esempio con SVGR):
```tsx
import Ombrellone from '@/assets/ombrellone_2_lettini.svg';
import '@/styles/beach-tokens.css';

<Ombrellone className="h-20 w-20 select-none" aria-label="Ombrellone con due lettini" />
```
Configura SVGR come loader; non usare `next/image` se vuoi la propagazione delle CSS custom properties. Gli SVG standalone usano i colori di fallback inclusi nei `var()`.

## Consigli editor
- Ogni risorsa visuale è distinta da stato prenotazione: stato nel layer HTML/React, non all'interno dell'illustrazione.
- `bookable`: postazioni prenotabili; `structure`: strutture non prenotabili; `decoration`: layer ambientali.
- Per trascinamento/rotazione usa un wrapper con trasformazioni CSS, mantenendo il viewBox SVG.
- Passerelle modulari da 120 unità: la geometria è disegnata per raccordarsi ai bordi delle tile; usa lo stesso zoom per tutti i pezzi.
- `role=img` e `<title>` incorporati.

## Scelte stilistiche
- Stessa prospettiva zenitale, outline consistente (circa 2px), spigoli arrotondati, riempimenti desaturati.
- I componenti *di interfaccia* restano di shadcn/ui e Lucide; i disegni sono **asset di mappa**, non sostituti delle icone di toolbar.
- Nessuna sfumatura complessa, bitmap, filtri raster o effetti 3D.


## Novità v2.1.0
- Aggiunti 13 nuovi elementi per la gestione reale degli stabilimenti: palapa, tenda a righe, ombrellone quadrato/a vela, DAE, corridoio di lancio, bandiera mare colorabile, sedia JOB, area cani, fontanella, rosa dei venti, amaca, pouf e ombrellone con tavolino.
- `bandiera_mare.svg` usa `fill: var(--beach-flag, ...)` per poter cambiare colore da codice.
- `manifest.json` aggiornato con i nuovi asset.
