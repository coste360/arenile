# Pannello installabile e uso senza connessione

Il pannello del gestore è una **PWA**: si installa sul tablet o sul telefono della cassa come un'app e continua a
funzionare, in modo limitato, quando la connessione della spiaggia cade (specifica 21.2).

## Installare il pannello

- **Chrome, Edge, Android**: apri il pannello, poi il menu del browser e scegli *Installa Arenile* (o *Aggiungi a
  schermata Home*).
- **Safari su iPhone e iPad**: apri il pannello, tocca *Condividi* e poi *Aggiungi alla schermata Home*.

L'app si apre a schermo intero sulla pagina *Oggi*. Quando esce una nuova versione compare il messaggio *È disponibile
una nuova versione del pannello* con il pulsante **Aggiorna**: l'aggiornamento non avviene mai da solo, così nessuno
viene interrotto durante un check-in. Le operazioni in coda (vedi sotto) non si perdono aggiornando.

## Cosa si può fare offline

L'indicatore in alto a destra mostra sempre lo stato: **Online**, **Offline**, **Sincronizzazione**, con il numero di
operazioni in coda e di quelle da rivedere. Quando sei offline un avviso indica l'ora dei dati che stai consultando.

| Offline puoi | Offline non puoi |
|---|---|
| Consultare la mappa e le prenotazioni di oggi e di domani | Creare nuove prenotazioni |
| Vedere i saldi aperti del giorno | Vedere gli incassi del giorno (la pagina dice *Non disponibile offline*) |
| Fare il check-in di una prenotazione confermata | Spostare una prenotazione su un'altra postazione |
| Aggiungere una nota a una prenotazione | Rimborsi, pagamenti con carta o link di pagamento |
| Registrare un incasso in contanti su una prenotazione esistente | Modificare configurazione, prezzi, utenti |

Creare e spostare prenotazioni richiede il server, perché solo il server garantisce che la postazione sia libera: il
pannello lo spiega nel modulo e non invia nulla.

Le operazioni fatte offline compaiono subito sulla mappa e vengono **inviate in ordine alla riconnessione**, ognuna con
una chiave di idempotenza: anche se la rete cade a metà invio, il server le applica una volta sola. L'arrivo tipico
(incasso del saldo in contanti e poi check-in sulla stessa prenotazione) funziona anche offline: le due operazioni
partono nell'ordine in cui le hai fatte. Se il server ne
rifiuta una (per esempio un incasso in contanti senza una sessione di cassa aperta, o un check-in su una prenotazione
annullata nel frattempo) compare l'avviso *Operazioni offline da rivedere*: tocca **Rivedi** per vedere il motivo e
scegliere **Riprova** (dopo aver risolto, per esempio aprendo la cassa) o **Scarta**.

## Quali dati restano sul dispositivo

Per funzionare offline il pannello salva nel browser (IndexedDB), solo per lo stabilimento che stai guardando:

- il tuo profilo utente e l'elenco degli stabilimenti;
- le mappe e la configurazione necessaria a disegnarle (zone, pack, extra, stagioni, chiusure);
- la situazione della spiaggia di oggi e di domani (nome visualizzato del cliente, codice e saldo della prenotazione);
- il dettaglio delle prenotazioni di oggi e di domani che hai aperto;
- le operazioni in coda non ancora inviate.

Non salva mai elenchi o schede clienti, contatti, pagamenti, report, utenti dello staff o configurazione dei pagamenti;
il service worker conserva solo i file dell'applicazione, mai le risposte del server. I dati salvati scadono dopo 48
ore, vengono sostituiti quando cambi stabilimento e **cancellati all'uscita** (*Esci*, dopo aver provato a inviare le
operazioni in coda) o quando sul dispositivo accede un altro utente. Le **operazioni in coda** invece non vengono mai
scartate da sole: se la sessione di un collega è scaduta con incassi in coda e accedi tu sullo stesso dispositivo, le
vedi nell'indicatore e partono con la tua sessione (contengono solo codice della prenotazione, postazione e importo).
Su un dispositivo condiviso esci sempre a fine turno. Vedi anche [Privacy e GDPR](privacy-gdpr.md).
