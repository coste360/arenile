# 0018 — Marchio ufficiale: Arenile (brand kit v2.0) e Coste360.it

- **Stato:** accettato (emendato il 2026-10-23: brand kit v2.0)
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

## Emendamento — brand kit v2.0 (decisione del titolare, 2026-10-23)

- Il **brand kit Arenile v2.0** fornito da Coste360 SRL sostituisce il v1.0 in `assets/brand/arenile/`: "A"
  sabbia `#FFF0D1` con la traversa a onda azzurra `#38BDF8`, duna sabbia calda `#F2C893`, riquadro navy
  `#0B2542` (palette Coste360, blu secondario `#0EA5E9`), Titillium Web 700, **nessuna byline** nei logotipi
  (il testo alternativo è "Arenile"; la dicitura "Arenile — Coste360.it" resta solo nella firma delle email e
  come titolo della firma Coste360). Tutto il resto di questa ADR resta valido (file fuori dalla licenza,
  tracciati e PNG generati da `scripts/brand/build-brand.mjs`, nessun font caricato dalle app, copie
  verificate da test).
- **Rifinitura approvata** (stesso disegno, prudente): distanza uniforme di 22 unità su 1024 tra l'onda e le
  gambe della "A" (i tagli sono offset della curva dell'onda), tolta la tacca storta del piede destro, simbolo
  centrato otticamente nel riquadro, varianti per sfondi chiari con la "A" navy (sul bianco la "A" sabbia
  sparisce), testo dei logotipi in tracciati (`outlined/`). Dettagli in `assets/brand/arenile/NOTICE.md`.
- **Regole d'uso** (in UI tramite `BrandLogo`, `variant` + `tone`, `web/shared/src/ui/README.md`):
  - **icona app a colori** (riquadro navy, "A" sabbia): è l'icona dell'app. App desktop (Dock, barra delle
    applicazioni e installer di Windows, Linux), icone PWA, apple-touch, favicon, marchio delle email,
    intestazione della pagina pubblica senza logo dello stabilimento, icona nell'intestazione della
    documentazione;
  - **logo/simbolo a colori con la "A" navy** sui fondi chiari, nei punti principali (accesso, primo avvio,
    intestazione della sidebar in tema chiaro, logo della documentazione in chiaro, immagine social);
  - **bianco** sui fondi navy/blu Coste360: intestazione scura e piè di pagina navy dei siti, sidebar e accesso
    in **tema scuro**, fasce e hero navy; la template image della barra dei menu di macOS è la stessa
    silhouette monocromatica (nera, colorata dal sistema), con le distanze attorno all'onda allargate per i
    18 px;
  - **navy monocromatico** sui fondi chiari dove serve un segno a un colore: marchi piccoli, "powered by
    Arenile" della pagina pubblica e, quando avranno un marchio, stampe (riepilogo Z, ricevute) ed
    esportazioni PDF (oggi non ne portano);
  - i logotipi a colori con la "A" sabbia (`-dark`) restano nel kit per fondi navy dove serve il colore pieno,
    ma l'interfaccia usa il bianco.

## Conseguenze

- Positive: logo identico ovunque, zero byte di font sulla pagina pubblica, sorgente unica dei file.
- Negative: i tracciati vanno rigenerati con lo script a ogni cambio del kit (poi
  `scripts/desktop/gen-icons.sh` per le icone desktop); ogni email pesa qualche KB in più per il marchio inline.
- Verifiche: `brand.test.ts` (contrasto AA), `Brand.test.tsx` (copie identiche, niente `<text>`),
  `TestBrandMarkMatchesBrandKit` (marchio delle email), budget della pagina pubblica.
- Da rivedere con un legale: testi di `TRADEMARK.md` e dei `NOTICE.md` (asset creati con l'aiuto
  dell'intelligenza artificiale).

## Emendamento — logo Coste360 vettoriale (decisione del titolare, 2026-10-27)

- Il titolare ha fornito il logo Coste360 completo (simbolo e "Coste360.it") in SVG con il testo in
  tracciati: `assets/brand/coste360/logo.svg`, nero. Il file è stato vettorializzato dal logo aziendale con
  uno strumento di intelligenza artificiale (QuiverAI Arrow, come indica il commento nel file) ed è stato
  approvato dal titolare così com'è. Rientra nella verifica legale già prevista per il materiale del marchio.
- Il pannello (firma sotto l'account nella sidebar, accesso, primo avvio) usa questo SVG al posto del PNG:
  `logo-primary.svg` (riempimento `#0c2442`, il primario Coste360 di `icon-primary.svg`) sui fondi chiari e
  `logo-white.svg` (`#fff`) in tema scuro. Le due varianti cambiano solo il riempimento del gruppo
  `content_group`; un test lo verifica. Il disegno non viene modificato.
- La documentazione usa ancora i PNG originali (`logo-{primary,white}.png`). Il resto della regola resta
  valido: il logo non viene ridisegnato e il link è sempre https://www.coste360.it/.
