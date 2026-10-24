# 0012 — Kit di illustrazioni della mappa: `<symbol>`/`<use>`, stato sovrapposto, campo `asset`

- **Stato:** accettato; in parte sostituito da [ADR 0024](0024-kit-mappa-v3-e-scelta-del-kit.md) (provenienza
  e cartella dei kit, generatore multi-kit, stato sul disegno nel kit moderno)
- **Data:** 2026-10-23
- **Sezioni della specifica:** `docs/spec/11-mappa-editor-e-vista-operativa.md`,
  `docs/spec/12-pagina-pubblica-e-widget-incorporabile.md`, `docs/spec/06-schema-dati.md`

## Contesto

La mappa disegnava postazioni e decorazioni come cerchi e rettangoli colorati per stato. Coste360 ha
fornito un kit originale di 107 illustrazioni SVG (vista zenitale, viewBox `0 0 120 120`, colori come
`var(--beach-*, fallback)`, `manifest.json` con categoria e funzione `bookable|structure|decoration`),
copiato senza modifiche in `assets/beach-map-kit/`. Vincoli: 1.000 postazioni fluide (spec 11.1),
stato leggibile in chiaro e scuro e senza affidarsi al solo colore, nessuna dipendenza nuova, nessun
CSS custom (ADR 0009), budget JS della pagina pubblica (150 KB gzip), API first.

Alternative scartate:

- **SVG inline per elemento** (SVGR o stringhe ripetute): 1.000 copie dello stesso disegno nel DOM,
  peso e tempo di layout proporzionali al numero di postazioni.
- **`<img src=…svg>`**: niente variabili CSS (niente tema scuro, niente colore della bandiera), una
  richiesta per asset.
- **Stato dentro il disegno** (ricolorare l'ombrellone): il disegno cambia a ogni stato, i colori del
  kit si perdono e lo stato resta affidato al solo colore.
- **Nuovi valori di `DecorationKind`** per ogni famiglia del kit: duplicherebbe la tassonomia del kit
  nel contratto e, su SQLite, ricostruirebbe anche `map_decorations` per allargare il CHECK.

## Decisione

1. **Provenienza.** I file del kit sono la fonte di verità e restano intatti
   (`assets/beach-map-kit/NOTICE.md`: © Coste360 SRL, opera originale realizzata con l'aiuto di un
   modello di generazione di immagini, distribuita con la licenza del repository, BSL 1.1).
2. **Codice generato.** `web/scripts/gen-map-kit.mjs` (solo moduli built-in di Node, eseguito da
   `make gen` e quindi verificato da `gen-check` in `make check`) legge manifest e SVG, li **sanifica**
   con una allowlist (elementi `g/path/rect/circle/ellipse/line/polyline/polygon/text/tspan`, attributi
   geometrici e di presentazione; la generazione fallisce su script, `style`, `use`, `image`,
   `foreignObject`, link, animazioni, attributi `on*`, `href`, `url(...)`, `javascript:`, viewBox
   diverso o manifest non allineato ai file) e scrive `web/shared/src/map/kit/generated/{symbols,manifest}.ts`
   e `internal/domain/mapping/kitassets_gen.go`. Adattamenti necessari all'app vivono nel generatore:
   `ombrellone_quadrato` (disegnato ruotato di 45°) viene raddrizzato e ricentrato
   (`translate(9 9) rotate(-45 51 51)`), il tipo `asset` del manifest diventa `bookable` per le
   Postazioni e `structure` altrove, a ogni asset prenotabile corrisponde uno `SpotKind`.
3. **`<symbol>` + `<use>`.** Un solo `<svg><defs>` nascosto per pagina (`MapKitDefs`) contiene solo i
   simboli usati (unione registrata dalle istanze montate; la prima ne è responsabile, quindi mai id
   duplicati, e al suo smontaggio la successiva subentra) più i glifi dei badge; lo sprite è reso con un
   portal su `<body>`, fuori dal sottoalbero dell'istanza responsabile, così un pannello nascosto o un
   popover che si chiude non spegne i disegni delle altre mappe; ogni elemento è un
   `<use href="#ark-<id>">` scalato sulla sua geometria. I corpi dei simboli sono un chunk caricato in modo pigro (~10 KB gzip), mai nel primo
   schermo della pagina pubblica. È l'unica eccezione `dangerouslySetInnerHTML` del CSS guard, giustificata
   dalla sanificazione al build.
4. **Stato sovrapposto, non nel disegno.** Sotto il disegno un alone (disco o quadrato arrotondato)
   con i token `--status-*`, più grande del riquadro del disegno (`HALO_SCALE` = 1,08 volte il riquadro,
   che è già `SPOT_ART_SCALE` volte l'impronta): l'anello e la tinta restano visibili intorno
   all'illustrazione, non solo sotto (tinta forte per gli stati da gestire, appena accennata e senza
   anello per i liberi). Sopra, un badge d'angolo con un glifo per **ogni** stato prenotato: globo
   (online), banco (cassa), spunta (presente), stella (abbonato), freccia (assente oggi), orologio
   (in attesa), croce (bloccato), euro (da incassare), rinnovo (prelazione); solo il libero non ha badge.
   L'etichetta (A12) è una pillola sotto il disegno con dimensione minima di 9 px come prima; sotto quella
   soglia la mappa mostra un avviso che rimanda allo zoom o all'elenco. Il disegno di una postazione è 1,3
   volte la sua impronta (`SPOT_ART_SCALE`, il kit lascia un margine trasparente nel viewBox); l'impronta
   resta l'area cliccabile e la geometria salvata. Legenda e controlli di zoom stanno sopra il disegno,
   che mantiene le proporzioni della mappa fino all'altezza massima indicata dal chiamante.
5. **Campo `asset` (e `variant`).** `Spot` e `Decoration` hanno `asset` opzionale (id del kit,
   validato dal backend contro la lista generata: le postazioni solo asset prenotabili, le decorazioni
   qualunque asset); le decorazioni hanno `variant` opzionale, valida solo se l'asset la prevede
   (`bandiera_mare`: `green|yellow|red`, applicata sull'elemento con la proprietà arbitraria di Tailwind
   `[--beach-flag:var(--beach-flag-<colore>)]`, un token del tema e nessuna utility custom; la bandiera
   senza etichetta è didascalizzata con il colore e nominata "Bandiera stato del mare: rossa"). Senza `asset` vale l'illustrazione predefinita del tipo
   (`umbrella → ombrellone_2_lettini`, `gazebo → gazebo`, `tent → tenda_righe`, `palm → ombrellone_paglia`,
   `walkway → passerella_orizzontale` o `_verticale` secondo il lato lungo, `shower → doccia`,
   `lifeguard → torretta_bagnino`, `bar → bar`, `entrance → ingresso`, `cabin_block → cabine_multiple`;
   `custom` e `text` restano forme e testo).
6. **Tipi.** `SpotKind` aggiunge le famiglie prenotabili del kit `sunbed`, `daybed`, `cabana`, `hammock`
   (i valori esistenti restano): il tipo è semantico (pacchetti `requires_spot_kind`, assegnazione) e la
   lista è una sola, `mapping.SpotKinds`, usata anche dal catalogo. L'illustrazione lo disegna: nell'editor
   scegliere un'illustrazione di un'altra famiglia cambia il tipo e il pannello lo dice (i pacchetti che
   richiedono il vecchio tipo non verranno più assegnati lì); cambiare il tipo riporta un'illustrazione di un
   altro tipo a quella predefinita; un elemento portato a `text` perde `asset` e `variant`, e il backend
   rifiuta un `asset` su `text`.
   `DecorationKind` **non** cambia: le strutture del kit senza un tipo dedicato sono `custom` con
   `asset`. La migrazione `00004` (entrambi i dialetti) aggiunge le colonne e allarga il CHECK di
   `spots.kind`; su SQLite ricostruisce `spots` con la procedura documentata (foreign key disattivate
   fuori dalla transazione, migrazione `NO TRANSACTION` su un'unica connessione).
7. **Passerelle modulari.** Le tessere `passerella_*` hanno sempre lato `KIT_TILE` = 60 unità
   (le 120 unità del viewBox a scala 0,5; le assi disegnate dal kit sono più strette del lato, quindi
   un tratto appare come una striscia dentro la tessera): i tratti dritti sono spessi una tessera e
   lunghi un numero intero di tessere, ridisegnate ripetute alla stessa scala. L'editor allinea i bordi
   alla griglia di 60 (le frecce e il duplica avanzano di una tessera) solo quando il passo della griglia
   della mappa divide 60, così le tessere non litigano con la griglia delle postazioni; con un passo
   diverso o senza griglia tengono la posizione e si normalizza solo la misura.
8. **Modelli e demo.** Il modello classico e la demo usano il kit (ombrelloni con due lettini, gazebo
   con lettini, passerella d'ingresso fino al mare, bar, docce, WC, DAE, torretta, bandiera, sedia JOB,
   rampa, palme e fioriere), con i servizi dimensionati dalla scala dell'ombrellone e il cartello
   d'ingresso accanto alla testa della passerella. `seed demo` su una demo già presente assegna gli
   asset alle postazioni che non ne hanno e sostituisce **solo** le quattro decorazioni piatte del modello
   pre-kit (ingresso, bar, docce, bagnino, riconosciute per tipo, posizione e misura); le decorazioni
   aggiunte a mano restano e non vengono coperte; ogni mappa cambiata ha una nuova versione e una voce di
   audit `setup.demo_map_kit_upgraded`, e il comando stampa cosa ha tolto e aggiunto.

## Conseguenze

- Positive: la mappa somiglia a un lido vero; un solo simbolo per tipo di disegno e un `<use>` per
  elemento, quindi il DOM proprio resta piccolo (circa 4 nodi per postazione: alone, gruppo, `<use>`,
  etichetta, più il badge quando c'è); tema chiaro/scuro gratuito tramite variabili; ogni stato prenotato
  ha un glifo, quindi la mappa si legge anche in scala di grigi (il libero è l'assenza di badge);
  aggiungere un'illustrazione = SVG + voce nel manifest + etichette + `make gen`.
- Negative: il costo di disegno **non** è quello di prima: il browser espande ogni `<use>` in un albero
  ombra (`ombrellone_2_lettini` ha 28 elementi), quindi 1.000 postazioni sono circa 28.000 nodi
  rasterizzati contro i 2.000 dei cerchi; le misure sono sotto, in Verifiche. Per limitare i re-render,
  le unità per pixel passate alle postazioni memoizzate sono quantizzate a quarti di potenza di due
  (`quantiseUnitsPerPixel`), così un passo di rotella non ridisegna 1.000 `Spot` in React. Un'eccezione
  in più nel CSS guard; il disegno delle postazioni sborda dall'impronta (scelta visiva, l'impronta resta
  la geometria); `seed demo` sostituisce le quattro decorazioni del modello pre-kit; il downgrade di
  `00004` riporta a `custom` i tipi nuovi.
- Verifiche: test del generatore (`web/scripts/gen-map-kit.test.mjs`: sanificazione, insieme degli id
  uguale a manifest e file, viewBox, file generati allineati), `gen-check`, test di validazione e
  integrazione in `internal/domain/mapping` e `internal/domain/setup`, test del renderer
  (`web/shared/src/map/kit/kit.test.tsx`, incluso il caso da 1.000 postazioni), test delle etichette
  in quattro lingue, controllo e2e del `<use>` e del badge sulla mappa operativa, guard dei file generati
  (`scripts/agents/guard-generated.sh`).
- Misure in Chromium headless (Playwright, mappa operativa con una mappa di 1.000 ombrelloni generata con
  il generatore di file, 20 passi di rotella in avvicinamento, 20 in allontanamento e 20 di trascinamento,
  binario `f2b3ed6`, MacBook con Apple Silicon): CPU non rallentata, primo disegno 0,5 s dalla
  navigazione alle 1.000 postazioni, fotogrammi p50 16,7 ms, p95 16,8 ms, massimo 33 ms, nessun long task;
  con CPU rallentata 4× (ordine di grandezza di un tablet di fascia media) primo disegno 1,0 s, p50 16,7 ms,
  p95 67 ms, massimo 117 ms, 33 long task per 2,3 s complessivi su 60 interazioni. Il requisito "fluido su
  un tablet di fascia media" (spec 11.1) è quindi rispettato con margine ridotto: oltre questa soglia resta
  la virtualizzazione per settori prevista dalla spec. Script: `scratchpad/perf.mjs` della sessione, da
  ripetere a ogni cambiamento del renderer.
- Budget della pagina pubblica (`npm run budget`, gzip): prima del kit (`0912ba4`) JS 141,2 KB e CSS
  13,3 KB; dopo, JS 141,3 KB e CSS 13,6 KB (i token `--beach-*` e le chiavi i18n della mappa condivise);
  la pagina di prenotazione non importa il renderer della mappa.
- La specifica è aggiornata con questo ADR: 05 (glossario), 06 (`spots.kind`, `asset`, `variant`), 11
  (illustrazioni, stato sovrapposto, legenda, tipo/illustrazione, passerelle).
