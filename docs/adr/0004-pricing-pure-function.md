# 0004 — Motore prezzi come funzione pura

- **Stato:** accettato
- **Data:** 2026-10-22
- **Sezioni della specifica:** `docs/spec/08-motore-dei-prezzi.md`, `docs/spec/09-policy-di-prenotazione.md`

## Contesto

I listini degli stabilimenti balneari sono molto vari: periodi tariffari, file, weekend e festivi, mezze
giornate, settimane e abbonamenti a prezzo fisso, convenzioni, codici promozionali, offerte "7 giorni al
prezzo di 6". Il calcolo deve essere configurabile senza codice, spiegabile al gestore (simulatore,
matrice di listino) e riproducibile per anni (contestazioni, rimborsi).

## Decisione

- Il motore (`internal/domain/pricing`) è una **funzione pura**:
  `Quote(input, catalogo, regole) → PriceBreakdown`. Non accede al database, non legge l'orologio, non usa
  casualità: regole, catalogo e date arrivano come input, caricati prima dal chiamante.
- Aritmetica intera in centesimi (`money.Cents`), arrotondamento half-even al centesimo; nessun float
  nel calcolo.
- L'output completo è salvato come `price_snapshot` con `engine_version`; si incrementa `engine_version`
  quando l'output per un input esistente può cambiare.
- Simulatore, matrice di listino, verifica di copertura e quotazione pubblica usano tutti la stessa
  funzione.

## Conseguenze

- Positive: test esaustivi e veloci (table-driven e property test con `rapid`); la matrice coincide per
  costruzione con il simulatore; snapshot verificabili nel tempo.
- Negative: il chiamante deve caricare in anticipo tutte le regole e i dati necessari (es. occupazione
  per le regole `occupancy_percent`); serve disciplina per non introdurre dipendenze nascoste.
- Verifiche: INV-11...INV-16 in `docs/invariants.md`; regola `.claude/rules/pricing.md`; skill
  `/new-price-rule-test`; criterio M2 di almeno 150 casi di test.
