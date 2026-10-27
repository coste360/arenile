# 0028 — Cassa avanzata e convenzioni (M8)

- **Stato:** accettato (le quattro decisioni della sezione 30.13 sono valori predefiniti che il titolare può
  rivedere)
- **Data:** 2026-10-27
- **Sezioni della specifica:** `docs/spec/30-cassa-avanzata-e-convenzioni.md` (tutta), `docs/spec/28-roadmap-e-criteri-di-accettazione.md`
  (M8), `07` (7.5 segmenti, 7.6 quota online), `08` (convenzioni nello stage `discount`), `09` (rilasci), `10`
  (10.3 modifiche), `11.3`, `12`, `14`, `15`, `23`

## Contesto

La milestone M8 porta in cassa le funzioni che servono con la spiaggia piena (Trova posto con cambi di posto,
scambio tra prenotazioni, annulla, duplicazione, calendario della postazione, stampa della mappa) e fa delle
convenzioni uno strumento completo, fino al registro richiesto dai bandi di concessione. La sezione 30.13 lascia
aperte quattro decisioni; le altre scelte riguardano dove mettere il codice senza toccare le invarianti esistenti
(INV-01 nessuna doppia occupazione, INV-21 snapshot del prezzo, audit ed evento nella stessa transazione) e come
lavorare in parallelo su cinque filoni senza conflitti.

## Decisione

### Decisioni della sezione 30.13 (valori predefiniti)

1. **Soggiorno spezzato su categorie di prezzo diverse:** ogni giorno è prezzato con la categoria della postazione
   occupata quel giorno. Se i segmenti hanno la stessa categoria il prezzo non cambia rispetto alla postazione
   unica. Il motore prezzi resta puro: riceve i segmenti (postazione, date, categoria) come input della riga.
2. **Conto del partner:** estratto conto **e** pagamenti registrati per partner (`partner_payments`). Saldo =
   addebitato - pagato; addebitato = somma di `bookings.partner_amount`, mantenuto dal servizio delle prenotazioni
   nella stessa transazione del totale (INV-104). Un pagamento sbagliato si annulla (resta con motivo e
   operatore), non si cancella. La fattura resta fuori da Arenile. Una prenotazione cancellata, scaduta o rifiutata
   non addebita nulla al partner (l'eventuale penale resta con l'ospite); il saldo iniziale dell'estratto conto
   conta le prenotazioni finite prima del periodo e i pagamenti ricevuti prima.
3. **Registro delle convenzioni:** Excel (`internal/xlsx`) e documento HTML stampabile con impaginazione neutra
   (PDF con la stampa del browser, come la chiusura Z, ADR 0019): intestazione dello stabilimento (nome,
   ragione sociale, partita IVA), stagione o periodo, data di generazione; per convenzione condizioni in italiano
   leggibile, prenotazioni, giorni-postazione, persone, valore dello sconto concesso; totali per categoria del
   registro (`residents`, `accommodation`, `protected`, `companies`, `associations`, `other`) e totale generale;
   spazio per data e firma. Nessun formato specifico di un Comune finché l'associazione di categoria non ne indica
   uno.
4. **Contingente riservato:** la parte riservata e non usata torna alla vendita generale `release_days_before`
   giorni prima della data (per convenzione, predefinito **3**). Il rilascio è implicito nella data
   (`agreement_quota_usage.release_on`): nessun job. Due convenzioni con riserva sulle stesse zone sono garantite
   ciascuna da sola (le riserve non si sommano): per riserve indipendenti si usano zone diverse. La categoria del
   registro, se non indicata, deriva dalla categoria di clienti o dal tipo di partner (struttura ricettiva, azienda,
   associazione), altrimenti `other`.

### Scelte di progetto

- **Contratto:** tutte le operazioni in `api/openapi.yaml` (tag `partners`, più `bookings`, `availability`,
  `operations`, `reports`, `public`); i campi nuovi sugli schemi esistenti sono tutti facoltativi, così il codice
  generato dei client e dei server esistenti non cambia. `PrintableFormat` è una stringa con pattern e non un enum,
  per non rinominare le costanti generate di altri formati.
- **Migrazione 00013 (SQLite e PostgreSQL):** tabelle `partners`, `customer_categories`,
  `customer_category_members`, `agreement_versions`, `agreement_quota_usage`, `agreement_quota_allocations`,
  `partner_payments`, `requirement_verifications`, `move_undo_log`; colonne nuove su `agreements`, `bookings`,
  `spots`, `extras`. Le colonne nuove sulle tabelle esistenti sono nulle o hanno come predefinito il valore zero
  del campo Go, così il codice che non le conosce continua a inserire righe valide: per questo la postazione ha
  `online_excluded` (predefinito falso) e l'API espone il contrario, `online_bookable`.
- **Un posto per riga e mezza giornata:** indice unico `spot_allocations (booking_line_id, date, slot)` (parziale,
  righe con prenotazione). Con il vincolo esistente `(spot_id, date, slot)` uno scambio o un soggiorno spezzato
  non possono né occupare due volte una postazione né dare due postazioni alla stessa riga (INV-101, INV-102). I
  segmenti di una riga si ricavano dalle allocazioni: nessuna tabella dei segmenti.
- **Contingente delle convenzioni come quota di capacità (INV-103):** il tetto `max_per_day` è un contatore
  `agreement_quota_usage.used` aggiornato solo con `UPDATE ... WHERE used + :n <= max_units` e protetto anche da
  un `CHECK`; la capacità riservata vale per le vendite fuori convenzione con lo stesso meccanismo della quota
  online delle zone seat (zona bloccata e riconteggio dopo l'inserimento, nella stessa transazione): libere -
  riservate non usate e non rilasciate >= 0, altrimenti rollback con `409 agreement_quota_exceeded`. Nessun
  controllo applicativo preventivo sostituisce i vincoli.
- **Package:** `internal/domain/partner` (partner, categorie, verifiche sul cliente, pagamenti del partner);
  `internal/domain/findspot` (orchestrazione di Trova posto: carica l'occupazione, prezza, prenota tramite
  `booking`) con l'algoritmo puro in `internal/domain/availability/findspot.go`, accanto all'assegnazione a
  segmenti che estende; scambio, spostamento di gruppo, annulla, duplica e verifica del requisito sulla
  prenotazione in `internal/domain/booking` (file nuovi `swap.go`, `moveundo.go`, `duplicate.go`,
  `requirement.go`), perché ogni cambio di una prenotazione passa da lì con audit ed evento nella stessa
  transazione; calendario della postazione e lista per i bagnini in `internal/domain/inventory`
  (`spotcalendar.go`, `lifeguard.go`), accanto alla vista operativa del giorno; rendiconti e registro in
  `internal/domain/report` (`partners.go`, `agreements_register.go`); le convenzioni restano in
  `internal/domain/pricingconfig` con i campi estesi e lo storico delle versioni.
- **Scambio:** una transazione, prenotazioni bloccate in ordine di ID (niente deadlock tra due scambi incrociati),
  allocazioni rilasciate e ricreate dal giorno scelto sull'intervallo comune, prezzo ricalcolato se cambia la
  categoria, audit ed evento `booking.modified` per entrambe. Le versioni attese sono nel corpo
  (`expected_version`), perché `If-Match` vale per una sola risorsa.
- **Annulla entro 5 minuti:** `move_undo_log` scritto nella transazione dello spostamento; annullabile solo
  dall'utente che l'ha fatto, solo se le prenotazioni non sono cambiate dopo e le postazioni di partenza sono
  ancora libere (lo dicono i vincoli: `409 move_undo_conflict`); dopo 5 minuti `410 move_undo_expired`.
- **Spostamento programmato:** è uno spostamento con `from` futuro (già supportato), applicato subito alle
  allocazioni future; nessun job.
- **Verifica dei requisiti:** solo data, operatore, testo del requisito e una nota breve; mai copie o numeri di
  documento (spec 23).
- **Dati del gruppo:** nome del gruppo, adulti e bambini (alimentano le presenze dei report), provenienza, nomi
  degli ospiti facoltativi; nessun contatto degli ospiti.
- **Vendita online per postazione:** la pagina pubblica non mostra né vende le postazioni con
  `online_bookable = false`; la cassa le vede tutte. La quota online di zona resta (spec 7.6).
- **Link della convenzione:** `/b/{slug}?c={codice}`; `GET /api/public/v1/{slug}/agreements/{codice}` risponde
  solo per convenzioni attive, valide e con canale online, altrimenti `404` (non rivela i codici). Il codice si
  applica nel preventivo e nel checkout con `agreement_code`; l'esito è `PublicQuote.agreement`.
- **Riepilogo via WhatsApp:** testo e link `wa.me` costruiti dal server nella lingua del cliente con la stessa
  formattazione delle email (date, postazioni, totale, saldo); Arenile non chiama servizi esterni. Ogni richiesta
  emette un link di gestione nuovo e revoca i precedenti, come la rigenerazione del link magico (audit
  `booking.magic_link_regenerated`); la risposta è `no-store` e il testo non finisce nei log. Il link `wa.me` c'è
  solo se il cliente ha un telefono.
- **Riepilogo stampabile:** documento HTML autonomo (`internal/printout`, A4 o scontrino 80 mm, nessuno script,
  immagini solo `data:`) con il QR della prenotazione e senza link di gestione; PDF con la stampa del browser.

## Conseguenze

- Positive: nessuna invariante esistente cambia; le nuove (INV-101..INV-105) sono garantite dal database o da un
  valore mantenuto nella stessa transazione; cinque filoni lavorano su file e package propri.
- Negative: l'indice unico nuovo su `spot_allocations` (tabella calda) aggiunge un costo di scrittura; il registro
  in PDF dipende dalla stampa del browser; le quattro decisioni potrebbero cambiare dopo il confronto con i gestori
  e con l'associazione di categoria (in quel caso un nuovo ADR).
- Limiti noti, da chiudere dopo M8 (INV-103 resta `partial` finché i primi tre non sono chiusi):
  - un hold del pannello convertito in prenotazione (`ConvertHold`) non conta il contingente della convenzione, quindi
    `max_per_day` può essere superato da quella via;
  - la disponibilità pubblica non sottrae la capacità riservata (la prenotazione è rifiutata con `409`, ma il
    preventivo può mostrarla libera);
  - due riserve sulle stesse zone sono garantite ciascuna da sola, non sommate (decisione 4): l'editor non avvisa
    quando le zone di due riserve si sovrappongono; i blocchi delle postazioni e le revoche dei rilasci occupano
    capacità senza passare dal controllo delle riserve, quindi possono portare le unità libere sotto la riserva (le
    vendite successive fuori convenzione ricevono `409 agreement_reserved`);
  - il requisito non verificato è registrato e mostrato, non blocca; l'appartenenza alla categoria di clienti non è
    richiesta per usarne la convenzione;
  - sostituire le righe (`lines`) di un soggiorno spezzato lo riporta sulle postazioni indicate (una modifica che non
    le sostituisce conserva i segmenti, INV-106);
  - il soggiorno spezzato è proposto solo dalla cassa (Trova posto): la pagina pubblica non propone segmenti con
    accettazione esplicita (spec 30.6, ultimo punto) e vende solo postazioni libere per l'intero periodo;
  - il rilascio di giorni dell'abbonato (singolo o per intervallo) scrive audit ed evento `season.day_released`
    (lista d'attesa e webhook) ma non invia la conferma via email all'abbonato richiesta da spec 30.8;
  - uno scambio o uno spostamento di gruppo su una prenotazione online con un pagamento in corso (`held`,
    `pending_payment`) la riprezza: il webhook del pagamento può confermarla con `paid != total` o sotto la soglia
    di conferma, e vale allora la scadenza con rimborso (INV-51); nessun test copre scambio contro webhook.
- Verifiche: `TestMigration00013_ModelsRoundtrip` (`internal/store`, SQLite e PostgreSQL), `TestError_M8Mapping`
  (`internal/http/problem`), `TestResetEstablishment_EveryTableClassified`; i test di INV-101..INV-105 in
  `docs/invariants.md`; l'accettazione di M8 `TestAcceptance_M8CashDeskAndAgreements` e
  `TestConcurrency_AcceptanceM8Swaps` (`internal/acceptance`, SQLite e PostgreSQL) e `e2e/tests/m8.spec.ts`
  (Playwright sul binario reale).
