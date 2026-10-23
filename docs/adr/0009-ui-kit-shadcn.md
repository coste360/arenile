# 0009 — UI kit: shadcn/ui, niente CSS custom

- **Stato:** accettato
- **Data:** 2026-10-22
- **Sezioni della specifica:** `docs/spec/12-pagina-pubblica-e-widget-incorporabile.md` (12.5, tema dello
  stabilimento), `docs/spec/15-pannello-del-gestore.md`

## Contesto

Fino a M2 il pannello e la pagina di prenotazione usavano poche primitive scritte a mano
(`Button`, `Card`, `Dialog`, `Input`, `Label`), un foglio di stile per app, i token `--ar-*` e
molti stili in linea. Ogni funzionalità ricostruiva tabelle, dialoghi, campi data e importi a modo
suo: aspetto non uniforme, formati delle date diversi, accessibilità da verificare caso per caso.
Il titolare ha chiesto un UI kit stabile, curato e moderno, senza scrivere CSS.

Alternative considerate: una libreria di componenti chiusa (MUI, Mantine, Chakra), più pesante e con
un proprio sistema di stile che si scontra con il budget di 150 KB della pagina di prenotazione e con
il tema dello stabilimento; continuare con componenti propri, che richiede di mantenere CSS e
accessibilità da soli.

## Decisione

1. **shadcn/ui è l'unico sistema di componenti** di `web/admin` e `web/booking`: CLI shadcn in
   modalità Tailwind v4, primitive Radix, icone lucide-react, sonner per i toast, react-day-picker per
   i calendari, recharts tramite i chart di shadcn, TanStack Table con il modello data-table. I
   componenti sono generati con `npx shadcn@latest add <nome> --cwd shared` in
   `web/shared/src/ui` (`web/shared/components.json`) e importati da `@arenile/shared/ui`; la pagina
   di prenotazione importa i singoli file (`@arenile/shared/ui/<nome>`) per il budget.
2. **Niente CSS custom.** Nessun file `.css` nuovo, CSS module, CSS-in-JS o `style={{…}}`; lo stile si
   fa con le varianti dei componenti e le utility Tailwind sui token del tema. Unica eccezione per lo
   stile in linea: la geometria dinamica della mappa SVG (`web/shared/src/map`, editor della mappa).
   L'unico foglio di stile è `web/shared/src/styles/globals.css` (variabili shadcn chiaro/scuro e
   mappatura `@theme`). I vecchi token `--ar-*` sono rimossi. La dipendenza `cn` (il merge di classi
   pubblicato da shadcn, generato dalla CLI con `import { cn } from "cn"`) è fissata a una versione
   esatta in `web/shared/package.json` perché è pre-1.0: si aggiorna insieme alla CLI.
3. **Aspetto:** base neutra, primario blu mare profondo, accento sabbia/ambra (`--highlight`), raggio
   0,75rem, Inter Variable (fontsource), tema chiaro e scuro con selettore (predefinito: sistema).
4. **Tema dello stabilimento (spec 12.5):** `applyTheme()` sovrascrive le stesse variabili
   (`--primary`, `--highlight`, `--radius`, `--app-font`…) sulla pagina pubblica, con il calcolo del
   contrasto WCAG esistente; i componenti shadcn seguono così il marchio dello stabilimento.
5. **Pattern condivisi** in `@arenile/shared/ui/patterns` (PageHeader, DataTable, EmptyState,
   ConfirmDialog, FormSection/FieldRow, MoneyInput/PercentInput, DatePicker/DateRangePicker,
   I18nTextInput, StatusBadge, ThemeProvider): date sempre gg/MM/aaaa con date-fns in italiano, mai
   `<input type="date">`; importi in centesimi formattati con `Intl` it-IT.

## Conseguenze

- Positive: aspetto coerente tra le schermate; accessibilità di dialoghi, menu, select e calendari
  delegata a Radix; i componenti sono codice nostro (copiati, non una dipendenza chiusa) e si
  modificano quando serve; nessun CSS da mantenere.
- Negative: il codice generato va aggiornato a mano quando shadcn cambia (le modifiche locali sono
  elencate in `web/shared/src/ui/README.md`); più dipendenze npm (Radix, cmdk, vaul, recharts,
  react-day-picker); la pagina di prenotazione passa da 118 a circa 120 KB gzip di JS iniziale.
- Verifiche: `web/scripts/css-guard.mjs` (in `npm run lint`, quindi in `make check`) rifiuta fogli di
  stile (anche in `public/` e nei file `.html`), CSS-in-JS, stili in linea e programmatici
  (`element.style`, `<style>`), valori arbitrari di Tailwind che aggirano il tema (colori, dimensioni
  come `text-[10px]`, proprietà) e i token `--ar-*`. Le uniche esenzioni sono elencate nello script
  con la loro ragione: geometria SVG della mappa (`BeachMap`, `MapLegend`, `MapCanvas`), variabili del
  tema scritte a runtime (`theme.ts`, `ThemeProvider`), iframe dell'embed sulla pagina ospite, i
  componenti shadcn generati dalla CLI, lo sprite del kit della mappa (`MapKitDefs`, corpi SVG sanificati
  al build, ADR 0012) e i file di test, che possono leggere stili calcolati o contenere i frammenti SVG
  che il generatore del kit deve rifiutare (`gen-map-kit.test.mjs`); il colore della bandiera del kit usa
  la proprietà arbitraria `[--beach-flag:…]` su un token del tema, non utility custom;
  `npm run budget` controlla il budget della pagina di prenotazione; regole in
  `.claude/rules/frontend.md` e `AGENTS.md`; test e2e su tema scuro, palette comandi e selettore di
  date.
