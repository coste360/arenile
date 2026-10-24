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
  nell'edizione desktop**. Sull'edizione server gli endpoint che toccano una stampante, compresi creazione e
  modifica, rispondono `409 printing_unavailable`, `PrinterList.escpos_available` è false, la voce Stampanti non
  compare nel menu (la pagina resta raggiungibile e lo spiega) e il pannello usa la stampa del browser per la
  chiusura Z; la ricevuta non fiscale esiste solo sulla stampante termica.
- `host` deve essere un indirizzo privato o di rete locale (anche dopo la risoluzione DNS, controllata a ogni
  connessione): `422 printer_address_not_allowed`. Loopback solo con `ARENILE_PRINTERS_ALLOW_LOOPBACK` (sviluppo
  e test, predefinito con `ARENILE_DEV`); gli indirizzi dei metadati cloud (169.254.169.254, fd00:ec2::254) mai.
  Il server non diventa un proxy TCP verso Internet né verso i servizi locali.
- Il documento si compone **sul server**: `internal/escpos` (codificatore puro, fixture di byte), `internal/printing/templates`
  (ricevuta, riepilogo Z, pagina di prova, biglietto `TicketDoc`), testo UTF-8 traslitterato nella codepage
  (CP858 predefinita per € e accenti). Il logo dello stabilimento diventa un raster con dithering in cache per stampante.
- Il biglietto della vendita giornaliera (ADR 0020) arriva da `printing.TicketSource`, implementata dal servizio
  `ticketing` (`PrintableTicket`): stessi dati della stampa dal browser (numero #NNN, giorno, zona e postazione o "da
  assegnare", formula, pack ed extra o tipi d'ingresso, totale con IVA, metodo, QR del codice del varco, "Non valido
  ai fini fiscali", "RISTAMPA" dopo la prima stampa), senza il nome del cliente; cassetto solo per i contanti. Il
  pannello manda il biglietto alla stampante termica quando l'instradamento di `daily_ticket` è pronto (vendita
  rapida e ristampa dal Ticket del giorno), altrimenti usa la stampa del browser come prima.
- Instradamento in `printer_routes`: regola della cassa, altrimenti regola predefinita del documento; `printer_id`
  esplicito vince. Un lavoro (`print_jobs`) si accoda già composto. La chiave di idempotenza è l'header
  `Idempotency-Key`, legata alla richiesta (tipo, documento, stampante): stessa chiave → stesso lavoro, stessa
  chiave per un'altra richiesta → `422 idempotency_key_reused`, nessuna chiave → nuovo lavoro (ristampa voluta); il
  pannello manda una chiave nuova per ogni clic, quindi deduplica il doppio clic, non la ristampa. Un job in-process
  reclama i lavori con aggiornamento condizionato (lease), raggruppa per stampante e serve le stampanti in parallelo
  (una stampante che non risponde non blocca le altre), invia con timeout e riprova con backoff fino a
  `max_attempts`, poi `failed` con riprova manuale. La consegna è **almeno una volta**: un lease scaduto a metà
  invio viene reclamato (tentativo contato) e rispedito senza il comando di apertura cassetto. Il contenuto dei
  lavori conclusi viene azzerato dopo 30 giorni (`printing.retention`). Nessun dato personale nei log né in
  `last_error`.
- Punto di estensione: l'interfaccia `printing.Driver` (implementazione `network`). Un ponte futuro implementa Driver
  passando il payload a un dispositivo abbinato tramite l'API dei dispositivi (come il ponte fiscale), aggiunge un
  valore a `PrinterDriver` e al CHECK `printers.driver` con una nuova migrazione; servizio, instradamento, modelli e
  job non cambiano.

## Conseguenze

- Positive: stampa immediata e affidabile in stabilimento, senza driver di sistema; documenti identici su ogni
  stampante; il doppio clic produce una sola stampa.
- Negative: chi usa il VPS continua con la stampa del browser fino al ponte; le tabelle codepage dei cloni variano
  (campo `codepage_table` per forzarle).
- Verifiche: test golden del codificatore e dei modelli (58/80 mm), test con una stampante TCP finta in-process,
  `TestConcurrency_PrintJobOnce`, test di integrazione della migrazione 00011, `TestTicketing_PrintsOnThermalPrinter`
  (biglietti reali della vendita giornaliera fino alla stampante finta), Playwright con stampante di rete finta.
