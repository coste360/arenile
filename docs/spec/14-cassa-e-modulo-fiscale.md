## 14. Cassa e modulo fiscale

### 14.1 Cassa

- Apertura sessione con fondo cassa, registrazione degli incassi per metodo, chiusura con conteggio e differenza.
- Incasso rapido dalla mappa: importo proposto = saldo dovuto, metodo, eventuale nota.
- Vendita libera senza prenotazione (lettino extra al volo, giornaliero senza anagrafica con cliente generico "Cliente cassa").
- Report di chiusura stampabile in PDF.

### 14.2 Modulo fiscale

L'emissione del documento commerciale dipende dal dispositivo e dalla configurazione del gestore. Il core non implementa protocolli specifici dei registratori nell'MVP, ma predispone l'architettura.

```go
type FiscalDriver interface {
    Code() string
    Issue(ctx context.Context, doc FiscalDocumentRequest) (*FiscalDocumentResult, error)
    Void(ctx context.Context, ref string) error
    Status(ctx context.Context) (DeviceStatus, error)
}
```

Driver previsti:
- **`none`:** nessuna emissione, il gestore usa il proprio registratore separatamente (default MVP).
- **`export`:** esportazione giornaliera dei corrispettivi per aliquota e metodo in CSV e PDF, per il commercialista.
- **Driver per registratori telematici:** contributi successivi, uno per famiglia di protocollo.

Le righe del documento derivano dallo snapshot del prezzo (scorporo IVA per aliquota). Ogni emissione o errore è tracciato in `fiscal_documents`. La conformità normativa della configurazione resta responsabilità del gestore e del suo consulente: la documentazione lo indica chiaramente.

### 14.3 Vendita giornaliera e biglietti

- **Vendita rapida** (cassa "Vendita rapida" o mappa del giorno, "Vendi giornata" su una postazione libera): una sola
  schermata con *dove* (postazione precisa oppure solo zona/fila), fascia (giornata, mattina, pomeriggio), pack,
  extra, prezzo dal motore per quel giorno, pagamento contanti o carta (driver manuale; i contanti richiedono una
  sessione di cassa aperta) e nome facoltativo per l'etichetta in mappa. Nasce una prenotazione confermata (canale
  `desk`) tramite il servizio delle prenotazioni, sul cliente condiviso "Cliente al banco" (spec 10.6); nessun
  telefono né email.
- **Numero progressivo per giorno di validità:** la sequenza è quella del giorno di utilizzo del biglietto
  (`ticket_date`, fuso dello stabilimento), condivisa tra vendite di postazioni e ingressi e unica per stabilimento su
  tutte le casse e i dispositivi; una vendita per un altro giorno usa la numerazione di quel giorno. Garantito dal
  database (contatore per stabilimento e giorno aggiornato con un upsert atomico nella transazione di vendita, più
  `UNIQUE (establishment_id, ticket_date, ticket_number)`). Buchi solo per biglietti annullati, che mantengono il numero.
- **Vendita per zona:** la capacità della zona è consumata subito (pool implicito per le zone `zone`, allocazione
  provvisoria sull'ultima postazione libera per le zone `seat`, mostrata in mappa come "da assegnare"), quindi non si
  vendono mai più biglietti dei posti; il biglietto resta "da assegnare" finché lo staff lo colloca (pannello "Da
  assegnare" della mappa, assegnazione automatica in ordine, spostamenti successivi), sempre con allocazione atomica.
  La prenotazione di un biglietto si cambia solo con le azioni del biglietto (assegna, sposta, annulla): la
  cancellazione, la modifica e lo spostamento generici la rifiutano (409 `ticket_owned`), così biglietto e
  prenotazione non divergono mai. Assegnazione e spostamento **non cambiano il totale** già incassato: il prezzo
  della nuova postazione resta solo nell'audit.
- **Etichette in mappa:** `#037` senza nome, il cognome se indicato.
- **Stampa:** scontrino 80 o 58 mm, A4 o A5 dal browser, con stabilimento, numero e data, postazione o zona (in
  grande sotto il numero), formula, pack, extra, totale con scorporo IVA, metodo, dicitura "Non valido ai fini
  fiscali" (il documento fiscale è quello del pagamento) e QR. Il QR porta un **gate token** casuale (128 bit) e mai
  il codice della prenotazione, che è progressivo e resta stampato in chiaro sotto il QR. Ristampa, annullamento con
  rimborso tramite i pagamenti (il numero resta, audit) e spostamento.
- **Biglietti d'ingresso** (spiaggia libera attrezzata, piscina): tipi (Adulto, Ridotto, Bambino, ...) prezzati per
  periodo come pack su una zona `quantity`, il cui pool è la capienza giornaliera condivisa con l'online; stessa
  numerazione; una vendita è un gruppo che entra insieme da una sola zona (tipi di zone diverse si vendono in biglietti
  separati): un biglietto, un QR, **una scansione fa entrare il gruppo**. Al varco il QR (token) fa entrare da solo;
  il numero del giorno o il codice della prenotazione scritti a mano passano solo come inserimento manuale confermato
  dallo staff (`manual`), registrato nell'audit. Un biglietto la cui prenotazione non è più confermata non entra.
- **Ticket del giorno:** elenco con numero, ora, postazione o zona, stato, importo e metodo, esportabile in CSV. Il
  riepilogo è per giorno di validità (una vendita anticipata compare nel giorno di utilizzo; gli annullati sono
  contati ma non sommati) e non è una riconciliazione di cassa: incassi, rimborsi e corrispettivi seguono la data
  contabile dei pagamenti, nei totali della sessione di cassa e nei corrispettivi per aliquota.

---
