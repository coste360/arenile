# 0012 — Kit di illustrazioni della mappa: `<symbol>`/`<use>`, stato sovrapposto, campo `asset`

- **Stato:** accettato
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
   duplicati) più i glifi dei badge; ogni elemento è un `<use href="#ark-<id>">` scalato sulla sua
   geometria. I corpi dei simboli sono un chunk caricato in modo pigro (~10 KB gzip), mai nel primo
   schermo della pagina pubblica. È l'unica eccezione `dangerouslySetInnerHTML` del CSS guard, giustificata
   dalla sanificazione al build.
4. **Stato sovrapposto, non nel disegno.** Sotto il disegno un alone (disco o quadrato arrotondato)
   con i token `--status-*` (tinta forte per gli stati da gestire, appena accennata per i liberi);
   sopra, un badge d'angolo con glifo per presente, abbonato, abbonato assente oggi, in attesa,
   bloccato, da incassare e prelazione rinnovo; l'etichetta (A12) è una pillola sotto il disegno con
   dimensione minima di 9 px come prima. Il disegno di una postazione è 1,3 volte la sua impronta
   (`SPOT_ART_SCALE`, il kit lascia un margine trasparente nel viewBox); l'impronta resta l'area
   cliccabile e la geometria salvata.
5. **Campo `asset` (e `variant`).** `Spot` e `Decoration` hanno `asset` opzionale (id del kit,
   validato dal backend contro la lista generata: le postazioni solo asset prenotabili, le decorazioni
   qualunque asset); le decorazioni hanno `variant` opzionale, valida solo se l'asset la prevede
   (`bandiera_mare`: `green|yellow|red`, applicata con le utility `beach-flag-*` di `globals.css` che
   impostano `--beach-flag` sull'elemento). Senza `asset` vale l'illustrazione predefinita del tipo
   (`umbrella → ombrellone_2_lettini`, `gazebo → gazebo`, `tent → tenda_righe`, `palm → ombrellone_paglia`,
   `walkway → passerella_orizzontale` o `_verticale` secondo il lato lungo, `shower → doccia`,
   `lifeguard → torretta_bagnino`, `bar → bar`, `entrance → ingresso`, `cabin_block → cabine_multiple`;
   `custom` e `text` restano forme e testo).
6. **Tipi.** `SpotKind` aggiunge le famiglie prenotabili del kit `sunbed`, `daybed`, `cabana`, `hammock`
   (i valori esistenti restano): il tipo è semantico (pacchetti `requires_spot_kind`, assegnazione).
   `DecorationKind` **non** cambia: le strutture del kit senza un tipo dedicato sono `custom` con
   `asset`. La migrazione `00004` (entrambi i dialetti) aggiunge le colonne e allarga il CHECK di
   `spots.kind`; su SQLite ricostruisce `spots` con la procedura documentata (foreign key disattivate
   fuori dalla transazione, migrazione `NO TRANSACTION` su un'unica connessione).
7. **Passerelle modulari.** Le tessere `passerella_*` hanno sempre lato `KIT_TILE` = 60 unità
   (le 120 unità del viewBox a scala 0,5): i tratti dritti sono spessi una tessera e lunghi un numero
   intero di tessere, ridisegnate ripetute alla stessa scala; l'editor allinea i bordi alla griglia di
   60 (le frecce e il duplica avanzano di una tessera), così i pezzi si raccordano.
8. **Modelli e demo.** Il modello classico e la demo usano il kit (ombrelloni con due lettini, gazebo
   con lettini, passerella d'ingresso fino al mare, bar, docce, WC, DAE, torretta, bandiera, sedia JOB,
   rampa, palme e fioriere). `seed demo` su una demo già presente assegna gli asset alle postazioni che
   non ne hanno e sostituisce le decorazioni senza asset, una sola volta.

## Conseguenze

- Positive: la mappa somiglia a un lido vero con lo stesso costo di rendering di prima (un `<use>` per
  elemento, un simbolo per tipo di disegno); tema chiaro/scuro gratuito tramite variabili; stato
  leggibile anche in scala di grigi; aggiungere un'illustrazione = SVG + voce nel manifest + etichette +
  `make gen`.
- Negative: un'eccezione in più nel CSS guard; il disegno delle postazioni sborda dall'impronta
  (scelta visiva, l'impronta resta la geometria); `seed demo` riscrive le decorazioni della demo
  pre-kit; il downgrade di `00004` riporta a `custom` i tipi nuovi.
- Verifiche: test del generatore (`web/scripts/gen-map-kit.test.mjs`: sanificazione, insieme degli id
  uguale a manifest e file, viewBox, file generati allineati), `gen-check`, test di validazione e
  integrazione in `internal/domain/mapping` e `internal/domain/setup`, test del renderer
  (`web/shared/src/map/kit/kit.test.tsx`, incluso il caso da 1.000 postazioni), test delle etichette
  in quattro lingue, controllo e2e del `<use>` e del badge sulla mappa operativa, guard dei file generati
  (`scripts/agents/guard-generated.sh`).
