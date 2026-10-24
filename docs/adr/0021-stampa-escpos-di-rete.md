# 0021 — Stampa ESC/POS su stampanti di rete (solo edizione desktop in v1)

- **Stato:** accettato (realizzato)
- **Data:** 2026-10-24
- **Sezioni della specifica:** `docs/spec/14-cassa-e-modulo-fiscale.md`, `docs/spec/29-app-desktop-e-mobile.md`

## Contesto

Biglietti della vendita giornaliera, ricevute non fiscali e chiusure di cassa si stampano oggi con la stampa del
browser, lenta e poco affidabile sulle stampanti termiche. Le stampanti ESC/POS più diffuse negli stabilimenti
(Epson TM-T20/T88, Xprinter, Rongta, Bixolon) accettano byte grezzi via TCP sulla porta 9100. Solo un server che sta
nella rete dello stabilimento può raggiungerle: l'edizione desktop (locale o modalità rete locale), non il VPS.
USB, ponte dall'app desktop per il VPS e Bluetooth dall'app mobile sono stati rinviati dal proprietario.

## Decisione

- v1 stampa ESC/POS **solo su stampanti di rete** (Wi-Fi/Ethernet, TCP raw, porta predefinita 9100) e **solo
  nell'edizione desktop**. Sull'edizione server gli endpoint che toccano una stampante rispondono
  `409 printing_unavailable`, `PrinterList.escpos_available` è false e il pannello usa la stampa del browser con un
  avviso. La configurazione si può preparare su entrambe le edizioni.
- `host` deve essere un indirizzo privato o di rete locale (anche dopo la risoluzione DNS, controllata a ogni
  connessione): `422 printer_address_not_allowed`. Il server non diventa un proxy TCP verso Internet.
- Il documento si compone **sul server**: `internal/escpos` (codificatore puro, fixture di byte), `internal/printing/templates`
  (ricevuta, riepilogo Z, pagina di prova, biglietto generico `TicketDoc`), testo UTF-8 traslitterato nella codepage
  (CP858 predefinita per € e accenti). Il logo dello stabilimento diventa un raster con dithering in cache per stampante.
- Instradamento in `printer_routes`: regola della cassa, altrimenti regola predefinita del documento; `printer_id`
  esplicito vince. Un lavoro (`print_jobs`) si accoda già composto, con chiave di idempotenza unica per stabilimento;
  un job in-process lo reclama con aggiornamento condizionato (lease), lo invia con timeout e riprova con backoff fino
  a `max_attempts`, poi `failed` con riprova manuale. Nessun dato personale nei log né in `last_error`.
- Punto di estensione: l'interfaccia `printing.Driver` (implementazione `network`). Un ponte futuro implementa Driver
  passando il payload a un dispositivo abbinato tramite l'API dei dispositivi (come il ponte fiscale), aggiunge un
  valore a `PrinterDriver` e al CHECK `printers.driver` con una nuova migrazione; servizio, instradamento, modelli e
  job non cambiano.

## Conseguenze

- Positive: stampa immediata e affidabile in stabilimento, senza driver di sistema; documenti identici su ogni
  stampante; ristampe accidentali deduplicate.
- Negative: chi usa il VPS continua con la stampa del browser fino al ponte; le tabelle codepage dei cloni variano
  (campo `codepage_table` per forzarle).
- Verifiche: test golden del codificatore e dei modelli (58/80 mm), test con una stampante TCP finta in-process,
  `TestConcurrency_PrintJobOnce`, test di integrazione della migrazione 00011, Playwright con stampante di rete finta.
