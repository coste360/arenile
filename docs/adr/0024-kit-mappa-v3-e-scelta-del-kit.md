# 0024 — Kit mappa v3 "Moderno" come predefinito e scelta del kit per stabilimento

- **Stato:** accettato
- **Data:** 2026-10-24
- **Sezioni della specifica:** `docs/spec/11-mappa-editor-e-vista-operativa.md`,
  `docs/spec/12-pagina-pubblica-e-widget-incorporabile.md`; sostituisce in parte
  [ADR 0012](0012-kit-mappa.md) (punti 1, 2 e 4)

## Contesto

Coste360 ha consegnato il kit mappa **v3.0** ("stile pieno monocolore"): gli stessi 107 file SVG della v2.1, con
gli stessi nomi, ma ridisegnati come icone piene a un colore. La parte di stato di postazioni e cabine usa
`fill="var(--beach-state, var(--beach-state-free, #7C8794))"`: il colore lo decide l'app, non c'è alone nel
disegno, e l'angolo in alto a destra (x 92–120, y 0–28 del viewBox) resta vuoto per badge e indicatori. Strutture,
servizi e sicurezza sono una tessera unica colorata per categoria (`--beach-ink`, `--beach-navy`, `--beach-clay`).

Vincoli: le mappe salvate non devono cambiare (gli `asset` restano gli stessi id), il gestore deve poter tenere il
disegno v2.1 a cui è abituato, nessuna dipendenza nuova, nessun CSS custom (ADR 0009), API first, la mappa deve
restare leggibile senza colore (ADR 0012), budget della pagina pubblica.

Alternative scartate:

- **Sostituire la v2.1 con la v3**: semplice, ma toglie al gestore un disegno che conosce, e la v3 cambia il modo
  di leggere gli stati (corpo colorato invece dell'alone).
- **Kit per mappa** invece che per stabilimento: due mappe dello stesso lido con stili diversi confondono il
  personale e la pagina pubblica; nessuno l'ha chiesto.
- **Colonna nuova `establishments.map_kit`** con migrazione: il tema (`theme`, JSON) è già il posto delle scelte
  di aspetto ed è già esposto alla pagina pubblica; una colonna richiederebbe una migrazione per entrambi i
  dialetti senza alcun vincolo da far rispettare al database.
- **Usare i token `--beach-state-*` del kit** (5 stati): l'app ne ha 11 (libero, prenotato in cassa, prenotato
  online, da incassare, presente, abbonato, abbonato assente oggi, in attesa, prelazione rinnovo, bloccato, non
  disponibile) e una sola palette per legenda, planning e mappa.

## Decisione

1. **Due kit nel binario.** I kit stanno in `assets/map-kits/<chiave>/` (`svg/`, `manifest.json`,
   `beach-tokens.css`, `README.md`, `catalogo.html` del designer e `NOTICE.md` con la provenienza © Coste360),
   copiati senza modifiche: `modern` (v3.0, **predefinito**) e `classic` (v2.1, la cartella `assets/beach-map-kit`
   spostata). Lo script `build.mjs` e i sorgenti del designer non sono importati: la fonte sono gli SVG consegnati.
2. **Generatore multi-kit.** `web/scripts/gen-map-kit.mjs` legge tutti i kit (`KITS`, il primo è il predefinito),
   pretende **lo stesso catalogo** (id, categoria e tipo uguali in ogni kit, altrimenti fallisce) e scrive un solo
   `generated/manifest.ts` (catalogo, `MAP_KIT_KEYS`, `DEFAULT_MAP_KIT`, `MAP_KITS` con versione, stile e asset con
   stato), un `generated/symbols-<chiave>.ts` per kit (chunk pigro separato, ~10 KB gzip ciascuno) e
   `kitassets_gen.go` con `MapKits` e `DefaultMapKit`. Le regole dipendono dal **profilo** del kit:
   - `halo` (v2.1): l'allowlist di ADR 0012, e la normalizzazione di `ombrellone_quadrato` (solo per `classic`:
     la v3 lo disegna già dritto).
   - `fill` (v3): elementi solo `g/path/rect/circle/ellipse` (più `svg` e `title`, tolti); `fill` e `stroke` solo
     `none` o `var(--beach-*, #hex)` (anche annidata); **angolo degli indicatori vuoto** tranne `mare` e `sabbia`,
     verificato sulla geometria vera (contorni con trasformazioni, curve e archi campionati, rettangoli
     arrotondati, mezzo spessore del tratto anche sotto scale non uniformi, regola di riempimento `nonzero`/`evenodd`,
     riempimento che coprisse l'angolo), non sul riquadro, con lunghezze solo numeriche (niente unità o
     percentuali) e trasformazioni solo numeriche perché la verifica misuri ciò che il browser disegna; ogni asset
     prenotabile deve dipingere il suo stato con `--beach-state`; `stateful`, `cornerFree`, `indicatorSlot` e
     `stateVariable` del manifest devono corrispondere a ciò che il disegno fa. Un errore nomina file e regola.
3. **Scelta per stabilimento.** Il tema ha `map_kit` (`modern` | `classic`, OpenAPI `Theme.map_kit`, validato da
   `ValidateTheme` contro `mapping.MapKits`); assente vale `modern`. Si sceglie in **Configurazione →
   Stabilimento → Mappa** con l'anteprima di ciascun kit (`MapKitPreview`: la stessa fila di postazioni nei
   principali stati, con badge, passerella e bar). Il tema è sostituito per intero in scrittura, quindi la sezione
   Mappa reinvia il resto del tema, la sezione del tema e la pagina Online reinviano `map_kit`, e il backend delle
   impostazioni online conserva il kit salvato quando il tema ricevuto non lo indica. **Nessuna migrazione**: il campo vive
   nel JSON del tema già esistente. **Stabilimenti esistenti**: nessuno ha un valore, quindi passano tutti al kit
   moderno al primo avvio della versione; gli id sono gli stessi, le mappe non cambiano, e chi preferisce il
   disegno di prima sceglie Classico.
4. **Il kit arriva alle mappe da un contesto.** `MapKitProvider` (pannello: il layout di `/e/$id`; pagina
   pubblica: `BeachMap kit={establishment.theme.map_kit}`) e `useMapKit`; `BeachMap` accetta anche `kit` per le
   anteprime. I simboli si chiamano `ark-<kit>-<id>` e il registro dello sprite li tiene per kit, quindi due mappe
   con kit diversi sulla stessa pagina (l'anteprima) non si pestano; lo sprite resta uno.
5. **Stato nel kit moderno.** L'app imposta `--beach-state` sul gruppo del disegno con la classe
   `[--beach-state:var(--status-<stato>)]` (proprietà arbitraria di Tailwind su un token del tema, come per la
   bandiera): tutti gli 11 stati restano, con i colori della palette dell'app. Niente alone. Il primo figlio della
   postazione è un **anello di contorno senza riempimento** (0,94 volte il riquadro del disegno, che nella v3 sta in
   12..108 del viewBox), trasparente a riposo, che compare al passaggio del mouse, al focus da tastiera e per
   selezione e risultati di ricerca come fascia del colore di fondo sotto un anello scuro (`--map-spot-selected`),
   visibile su qualunque colore pieno. Il badge d'angolo si sposta al centro dell'angolo libero (0,383 volte il
   riquadro, contro 0,34 del classico). Un colore letterale (colori delle zone nell'editor, override del pannello)
   non può arrivare a una variabile CSS senza stili inline: in quel caso la postazione si disegna come nel classico.
   Il classico non cambia.
6. **Colori degli stati per il corpo pieno.** I token `--status-*` diventano per tema.
   - **Libero è un grigio neutro**, come vuole il kit (il suo "libero" predefinito è grigio): il verde si legge
     come "occupato/presente" e una spiaggia libera non deve essere un campo di ombrelloni colorati. Chiaro
     `#71717a` (prima il verde `#22c55e`, 2,1:1), scuro `#d4d4d8`; ≥ 3:1 su tutte le sabbie (`--beach-sand-soft`,
     `--beach-sand`, `--map-sand`) nei due temi, ed è lontano almeno 2:1 in luminosità da Bloccato (chiaro
     `#374151`, scuro `#8e8e96`), da Non disponibile (scuro `#52525b`) e da Presente.
   - **Prenotato online** `#0284c7` nel chiaro e `#bae6fd` nello scuro, **Presente** `#1e3a8a` e `#818cf8`: entrambi
     blu, differiscono anche in luminosità (≥ 2:1; prima 2,0:1 nel chiaro e invisibile nello scuro).
   - **In attesa** `#b45309` nel chiaro (era `#f59e0b`, 2,0:1), `#f59e0b` nello scuro: resta arancione e arriva a 3:1.
   - Gli altri stati da gestire hanno nello scuro valori propri dove non arrivavano a 3:1. **Abbonato assente oggi**
     e **Non disponibile** restano tinte leggere **per scelta**: sono stati di assenza o indisponibilità, e hanno
     il badge (freccia) o il disegno spento e tratteggiato.
7. **Bloccato senza colore.** Una barra diagonale disegnata dall'app sopra il disegno (fascia del colore di fondo
   sotto una linea del colore del testo), in entrambi i kit, oltre alla croce del badge: il grigio del bloccato si
   confonde con il libero o il non disponibile per molti daltonici e sui tablet in scala di grigi.

## Conseguenze

- Positive: la mappa si legge a colpo d'occhio (lo stato è il corpo dell'ombrellone, non un anello sottile);
  passare da un kit all'altro non tocca dati né mappe; aggiungere un kit = cartella + voce in `KITS` + nomi nelle
  lingue; il kit del designer è verificato a ogni `make gen`, con messaggi che il designer può usare.
- Negative: due kit da mantenere (ogni nuovo asset va disegnato in entrambi, il generatore lo impone); due chunk
  di simboli invece di uno (si scarica solo quello dello stabilimento, tranne nella pagina di scelta); i token
  `--status-*` hanno ora valori per tema, quindi chi li legge come esadecimali usa `DEFAULT_STATUS_COLORS` (chiaro)
  o `DARK_STATUS_COLORS` (scuro); Libero grigio cambia anche il colore di legenda, planning e grafici.
- Verifiche: `web/scripts/gen-map-kit.test.mjs` (profili, colori, angolo con i casi limite, catalogo uguale tra i
  kit, file generati allineati), `web/shared/src/map/kit/kit.test.tsx` (kit predefinito, `--beach-state` per
  stato, anello, posizione del badge, barra del bloccato, due kit nello stesso sprite, provider e valori
  sconosciuti), `web/shared/src/theme/brand.test.ts` (stati sulla sabbia a 3:1 nei due temi, Libero
  grigio a 3:1 su tutte le sabbie e lontano da Bloccato, Non disponibile e Presente, Prenotato online/Presente a
  2:1, contorno di selezione), `web/shared/src/map/colors.test.ts` (token chiari e scuri
  allineati alle palette), `internal/domain/establishment/validate_test.go` e il test di integrazione della PATCH
  dello stabilimento (`theme.map_kit`), test della sezione Mappa nel pannello, e2e della mappa operativa.
