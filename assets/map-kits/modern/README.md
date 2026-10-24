# Arenile — Beach Map kit v3.0

Stile pieno monocolore. Sostituisce la v2.1 senza rimappare le mappe salvate.

## Compatibilità con la v2
- Stessi 107 file in `svg/`, stessi nomi (`ombrellone_*.svg`, `passerella_*.svg`, ecc.).
- `viewBox="0 0 120 120"`, `role="img"`, `<title>` come prima.
- Passerelle e pontile: striscia sempre a x 41–79 (o y 41–79), si raccordano ai bordi come in v2.
- `bandiera_mare.svg` usa ancora `--beach-flag`.

## Regole
- Elementi: solo `svg`, `title`, `g`, `path`, `rect`, `circle`, `ellipse`. Niente testo, gradienti, filtri, maschere, stili inline.
- Colori: solo `var(--beach-*, fallback)`.
- Angolo in alto a destra (x 92–120, y 0–28) sempre vuoto per gli indicatori. Esenti solo `mare` e `sabbia` (sfondi a tutto campo).
- Tessera unica (34 × 34, raggio 8 nella griglia 48) per strutture, servizi, sicurezza e mobilità. Colore per categoria: strutture e mobilità `--beach-ink`, servizi `--beach-navy`, sicurezza (primo soccorso, DAE) `--beach-clay`.
- Badge e indicatori (abbonato, note) li disegna l'app nell'angolo libero, mai l'icona.
- Un colore pieno per icona + grigio `--beach-muted` per gli arredi + dettagli in `--beach-canvas` trasparente.

## Stato delle postazioni
La parte di stato di postazioni e cabine usa `fill="var(--beach-state, var(--beach-state-free, #7C8794))"`.
Nei file c'è solo il grigio neutro "libero": il colore vero lo decide l'app impostando `--beach-state`,
così segue lo stato reale e il tema chiaro/scuro. Le varianti `ombrellone_navy|sabbia|salvia|corallo`
esistono per compatibilità dei nomi e a riposo sono tutte neutre.
```html
<span data-beach-state="occupied"><Ombrellone2Lettini /></span>
```
Stati in `beach-tokens.css`: `free`, `booked`, `occupied`, `due`, `blocked`.
Bandiera: `data-beach-flag="green|yellow|red"`.

Le variabili funzionano solo con SVG inline (SVGR). Con `<img>` valgono i fallback.

## Rigenerare
`node build.mjs` → `svg/`, `manifest.json`, `beach-tokens.css`, `catalogo.html`.
