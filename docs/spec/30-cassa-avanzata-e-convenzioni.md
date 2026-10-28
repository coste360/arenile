## 30. Cassa avanzata e convenzioni

Questa sezione descrive la milestone M8 (sezione 28). L'obiettivo è dare alla cassa tutto quello che serve in
agosto, quando la spiaggia è piena e ogni minuto conta, e fare delle convenzioni uno strumento completo: dalla
scelta in cassa fino alla rendicontazione richiesta dai bandi di concessione.

### 30.1 Principi

- **Le invarianti non si toccano.** Nessuna funzione di questa sezione introduce la doppia occupazione di una
  postazione (INV-01): niente overbooking. Scambi, soggiorni spezzati e spostamenti programmati passano dalle
  allocazioni e dai vincoli del database, in una sola transazione.
- Ogni cambio di stato passa da `internal/domain/booking`, con audit log ed evento outbox nella stessa
  transazione (anche per entrambe le prenotazioni di uno scambio).
- Ogni nuova operazione nasce in `api/openapi.yaml`; il pannello usa solo componenti shadcn.
- Dati personali al minimo: si raccoglie solo quello che serve alla convenzione o al gruppo (sezione 23).
- Tutto funziona anche nell'edizione desktop senza internet, tranne ciò che per natura richiede la rete
  (prenotazione online, invio WhatsApp).

### 30.2 Convenzioni

Le convenzioni sono uno standard nei bandi di concessione: tariffe agevolate per i residenti, accordi con alberghi,
B&B e strutture ricettive del territorio, condizioni per categorie tutelate. Il concessionario le promette
nell'offerta e deve poterle applicare e dimostrare per tutta la durata della concessione.

#### 30.2.1 Partner

Anagrafica dei soggetti con cui il lido ha un accordo, separata dai clienti:

| Campo | Note |
|---|---|
| nome | "Hotel Miramare", "Comune di …", "Associazione …" |
| tipo | `struttura_ricettiva` (hotel, B&B, affittacamere, campeggio, casa vacanze), `ente`, `azienda`, `associazione`, `altro` |
| comune, indirizzo | facoltativi |
| referente, email, telefono | facoltativi, mai nei log |
| partita IVA / codice fiscale | facoltativi, per l'estratto conto |
| note, attivo | |

#### 30.2.2 Convenzione

Estende l'attuale `agreements` (spec 6, motore prezzi spec 8, stage `discount`):

- **Destinatari:** un partner (e i suoi ospiti), una **categoria di clienti** (residenti, over 65, persone con
  disabilità e accompagnatore, famiglie numerose, forze dell'ordine, dipendenti di un'azienda…) o singoli clienti.
- **Condizione economica:** percentuale, importo fisso, oppure prezzo fisso per pack (es. "giornaliero residenti
  12 €"). Ambito: zone, pack, extra, periodi tariffari, giorni della settimana, fasce.
- **Validità:** dal–al, stagioni; più versioni nel tempo con storico (la condizione applicata resta nello
  snapshot del prezzo, INV-21).
- **Contingente:** facoltativo, per data: "massimo 10 postazioni al giorno in convenzione" oppure "5 postazioni
  riservate ai residenti fino a 7 giorni prima". Il contingente riservato è una quota di capacità come
  `online_quota` (spec 7.6), garantita dagli stessi vincoli, non un controllo applicativo.
- **Requisito da verificare:** facoltativo (es. "documento di residenza", "tessera associativa", "conferma
  dell'hotel"). In cassa l'operatore spunta "verificato": si registrano data e operatore, **mai una copia del
  documento**.
- **Canali:** cassa; online con codice; online con **link della convenzione** (`/b/{slug}?c={codice}`), che il
  partner pubblica sul proprio sito o invia ai propri ospiti.

#### 30.2.3 Applicazione in cassa

- **Sulla prenotazione:** menu "Convenzione" nella nuova prenotazione e nella scheda; il prezzo si ricalcola
  subito e mostra lo sconto applicato. Cambiare convenzione su una prenotazione esistente segue le regole di
  modifica (spec 10.3), con differenza da incassare o rimborsare.
- **Sul cliente:** convenzione predefinita sulla scheda cliente (es. residente verificato), proposta in
  automatico a ogni prenotazione.
- **Ospite di una struttura:** "ospite di Hotel Miramare", con periodo del soggiorno in hotel facoltativo; se la
  convenzione lo richiede, l'operatore verifica.
- **Visibile sulla mappa:** badge della convenzione o del partner nella scheda rapida e, a scelta, sulla
  postazione (30.4).

#### 30.2.4 Conto del partner

Facoltativo, per gli accordi in cui paga la struttura e non il cliente:

- la prenotazione può essere **a carico del partner** (tutta o in parte);
- il partner ha un **estratto conto**: prenotazioni, giorni-postazione, importi, pagamenti ricevuti, saldo;
- esportazione in Excel e PDF per la fattura, che resta fuori da Arenile (spec 14).

#### 30.2.5 Rendiconti

- **Per partner:** prenotazioni, presenze, giorni-postazione, incasso, sconto concesso, per periodo.
- **Registro delle convenzioni per il bando:** per una stagione o un periodo, l'elenco delle convenzioni attive
  con le loro condizioni e, per ciascuna, prenotazioni, giorni-postazione, persone e valore dello sconto
  concesso, suddiviso per categoria (residenti, strutture ricettive, categorie tutelate). Esportazione in Excel e
  in PDF firmabile, pensata per dimostrare al Comune il rispetto degli impegni presi nell'offerta.

### 30.3 Gruppi e ospiti

- Sulla prenotazione: **nome del gruppo**, **numero di persone** (adulti, bambini), struttura di provenienza
  (partner o testo libero), nomi degli ospiti facoltativi.
- Il numero di persone alimenta le presenze nei report.
- Nessun dato degli ospiti oltre al nome: niente contatti, niente documenti.

### 30.4 Postazione sulla mappa

La casella della vista operativa (spec 11.3) mostra più informazioni, senza perdere leggibilità:

- **Arrivo e partenza:** stato "primo giorno" e "ultimo giorno" con il proprio glifo, oltre al colore.
- **Data di fine** della prenotazione (`16/8`), utile in ogni vista a intervallo.
- **Allestimento:** lettini, sdraio, sedie, ombrellone maxi, presi dagli extra con l'opzione "mostra sulla mappa"
  (es. `2L`, `2L 1S`).
- **Badge:** da saldare (già presente), convenzione o partner, nota, gruppo.
- **Selettore delle etichette** esteso: Codice / Cliente / Fine / Allestimento, combinabili, memorizzato per
  utente.
- **Filtri rapidi** che evidenziano le postazioni: arrivi di oggi, partenze di oggi, da saldare, presenti, una
  convenzione, un gruppo.
- **Vista a intervallo** (dal–al): oltre a libero e occupato, lo stato **libero in parte** con "N giorni su M".
  Un clic mostra quali giorni sono liberi.
- I colori restano personalizzabili; ogni stato mantiene un glifo, così lo stato non dipende dal solo colore.

### 30.5 Calendario della postazione

Dalla scheda rapida, e al passaggio del mouse sul desktop, il **calendario della stagione** della postazione:

- griglia mesi × giorni della stagione; ogni giorno mostra lo stato (libero, prenotato, abbonato, rilasciato,
  bloccato, da saldare); le mezze giornate sono mezze celle;
- clic su un giorno libero: nuova prenotazione da quel giorno, con le date successive libere già proposte;
- clic su un giorno occupato: apre la prenotazione;
- `GET /api/admin/v1/establishments/{id}/spots/{spotId}/calendar?season_id=`; i nomi dei clienti solo con
  `bookings.read`, mai negli eventi SSE.

### 30.6 Trova posto

Ricerca della disponibilità per la cassa, dal pulsante **Trova posto** e dalla nuova prenotazione.

- **Dati:** date, fascia, numero di postazioni (per i gruppi, preferibilmente vicine), zone, pack o allestimento,
  attributi (prima fila, vicino alla passerella, accessibile), **numero massimo di cambi di posto** (0–3),
  convenzione.
- **Risultato:** proposte ordinate: prima quelle senza cambi, poi con meno cambi, poi per continuità e vicinanza.
  Ogni proposta mostra i segmenti ("A5 dal 10 al 14, A7 dal 15 al 16"), il prezzo stimato con la convenzione e
  le postazioni evidenziate sulla mappa.
- **Prenota:** crea la prenotazione con le allocazioni per segmento in una sola transazione; se nel frattempo una
  data è stata presa, 409 e nuova ricerca.
- Riusa l'algoritmo a segmenti dell'assegnazione automatica (spec 7.5, `allow_spot_change_during_stay`), esteso
  alle zone `seat` per la cassa.
- **Prezzo con il cambio di posto:** se i segmenti sono della stessa categoria di prezzo il prezzo non cambia; se
  sono di categorie diverse vale, per ogni giorno, la categoria della postazione occupata (decisione da confermare,
  sezione 30.13).
- **Online:** la pagina pubblica può proporre il soggiorno spezzato solo se la policy lo consente e il cliente lo
  accetta esplicitamente, con i segmenti indicati prima del pagamento.

### 30.7 Spostamenti

- **Da quando:** lo spostamento chiede da quale giorno (oggi, domani, data scelta); il server supporta già `from`.
- **Programmati:** "dal 15 agosto passa ad A7" è uno spostamento con `from` futuro, applicato subito alle
  allocazioni future. La scheda mostra i segmenti della prenotazione. Non serve un job.
- **Scambio tra due prenotazioni:** trascinando una prenotazione su una postazione occupata, il pannello propone lo
  **scambio**. Anteprima con la differenza di prezzo per entrambe, conferma, scambio delle allocazioni
  sull'intervallo comune a partire dalla data scelta, in una sola transazione con audit ed evento per entrambe.
  Fuori dall'intervallo comune ciascuna resta dov'era. Nessun istante di doppia occupazione.
- **Gruppi:** spostamento di più prenotazioni o righe insieme, mantenendo la disposizione relativa.
- **Annulla l'ultimo spostamento** entro 5 minuti, se le postazioni di partenza sono ancora libere.

### 30.8 Assenze degli abbonati

- Il rilascio (spec 9, `customer_can_release_season_days`) si estende da un giorno a un **intervallo** e a
  **giorni ricorrenti** ("ogni lunedì di luglio").
- I giorni rilasciati sono subito vendibili; il credito segue `season_release_credit`.
- Lo stesso intervallo si può inserire dalla cassa, dall'app e dalla pagina di gestione dell'abbonamento.
- L'abbonato riceve la conferma; se un giorno rilasciato viene venduto, il credito si conferma.

### 30.9 Azioni sulla prenotazione

- **Duplica:** stessa postazione, cliente, extra e convenzione, nuove date; prezzo ricalcolato; disponibilità
  verificata.
- **Cambia cliente:** l'API esiste già (`customer_id` nella modifica); manca il comando nel pannello, con ricerca
  del cliente e anteprima della differenza di prezzo.
- **Riepilogo via WhatsApp:** link `wa.me` al numero del cliente con un testo pronto (date, postazione, importo,
  link di gestione della prenotazione). Nessuna API a pagamento, nessun dato inviato a terzi da Arenile:
  l'operatore conferma l'invio dalla propria app. Disponibile solo se il cliente ha un telefono.
- **Stampa del riepilogo** per il cliente, con QR della prenotazione.

### 30.10 Stampa della mappa

- Mappa del giorno in A4 o A3 orizzontale, con data, legenda, etichette scelte (30.4) e totali (occupate, libere,
  arrivi, partenze).
- **Lista per i bagnini:** per fila, postazione, cognome, allestimento, arrivo/partenza, da saldare.
- Funziona offline (CSS di stampa dell'SVG); nessun servizio esterno.

### 30.11 Vendita online per postazione

- Nell'editor, ogni postazione ha l'opzione **vendibile online**, oltre alla quota online di zona (spec 7.6).
- La pagina pubblica mostra e vende solo le postazioni abilitate; la cassa le vede tutte.

### 30.12 Fuori perimetro

- **Overbooking:** escluso. Contraddice INV-01, che è una garanzia per il gestore e per il cliente; lo scambio
  (30.7) e la lista d'attesa coprono i casi reali.
- **Sharing** (stessa postazione divisa tra più clienti): da chiarire con i gestori prima di qualsiasi lavoro.
- **Revenue manager** e prezzi per occupazione: restano nella sezione "Dopo l'MVP" della roadmap.

### 30.13 Decisioni aperte

- Prezzo di un soggiorno spezzato su categorie diverse (30.6): per giorno, oppure sempre quello della prima
  postazione.
- Conto del partner (30.2.4): solo estratto conto, oppure anche pagamenti registrati per partner.
- Registro delle convenzioni (30.2.5): formato PDF richiesto dai Comuni (da raccogliere con l'associazione di
  categoria).
- Contingenti riservati (30.2.2): rilascio automatico della quota non usata (quanti giorni prima).
