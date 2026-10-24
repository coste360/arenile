## 10. Flusso di prenotazione e ciclo di vita

### 10.1 Flusso pubblico

```
1. GET  availability            → date, fasce, zone, postazioni
2. POST quote                   → prezzo e policy (nessuna scrittura)
3. POST bookings (hold)         → crea prenotazione in stato held, occupa con hold,
                                  restituisce booking_id, hold_expires_at, client_secret
4. PATCH bookings/{id}/customer → dati cliente (nome, cognome, email; telefono facoltativo), consensi
5. POST bookings/{id}/pay       → crea il pagamento sul provider (o conferma senza pagamento)
6. webhook provider             → pagamento riuscito ⇒ status confirmed, hold → booking
7. GET  bookings/{id}           → la pagina interroga lo stato fino a confirmed
```

La conferma avviene **solo** dal webhook (o dal driver manuale), mai dal ritorno del browser.

### 10.2 Stati della prenotazione

```
          ┌──────────── expired (hold scaduto)
          │
draft ─▶ held ─▶ pending_payment ─▶ confirmed ─▶ checked_in ─▶ completed
                     │                 │
                     │                 ├──▶ cancelled (con rimborso/credito da policy)
                     │                 └──▶ no_show
                     └──▶ awaiting_approval ─▶ confirmed | rejected
```

- `draft`: solo dal pannello, prenotazione in compilazione (non occupa risorse).
- `held`: risorse occupate temporaneamente.
- `pending_payment`: pagamento avviato, hold esteso fino all'esito (max 30 minuti).
- `awaiting_approval`: risorse occupate con hold lungo (configurabile), in attesa del gestore.
- `confirmed`: risorse occupate stabilmente.
- `checked_in`: presenza registrata (scansione QR o tocco sulla mappa).
- `completed`: chiusura automatica a fine periodo.

Ogni transizione è un metodo del servizio `booking`, scrive l'audit log e pubblica un evento nell'outbox nella **stessa transazione**.

### 10.3 Modifiche

- Cambio di postazione o date: nuova allocazione nella stessa transazione, rilascio della vecchia, ricalcolo del prezzo, differenza registrata come importo da incassare o rimborsare.
- Aggiunta o rimozione di extra: stessa logica.
- Il cliente può modificare dal link magico solo entro i limiti della policy; il pannello può sempre forzare, con registrazione nell'audit.

### 10.4 Rinnovo degli abbonamenti

- Alla creazione della nuova stagione il gestore lancia "proponi rinnovi": per ogni abbonamento della stagione precedente viene creata una prenotazione in `held` con hold lungo (es. fino a una data scelta) sulla stessa postazione, con prezzo del nuovo listino.
- Il cliente riceve un link per confermare e pagare; alla scadenza gli hold non confermati vengono rilasciati.
- Diritto di prelazione esplicito, visibile in mappa con un colore dedicato.

### 10.5 Lista d'attesa

- Quando una richiesta non trova disponibilità, il cliente può iscriversi alla lista d'attesa.
- Ogni rilascio di allocazioni pubblica un evento; un job verifica le iscrizioni compatibili in ordine cronologico e invia una notifica con un link che crea un hold riservato per un tempo limitato.

### 10.6 Vendita giornaliera al banco

- La vendita rapida (spec 14.3) crea la prenotazione direttamente in stato `confirmed` (canale `desk`) con il servizio
  `booking`, come le prenotazioni del pannello: audit ed evento outbox nella stessa transazione del pagamento manuale e
  del biglietto numerato. Le vendite senza cliente sono registrate su un cliente condiviso "Cliente al banco" creato al
  primo uso; il nome facoltativo resta sul biglietto.
- L'annullamento di un biglietto è la cancellazione della prenotazione (`cancelled`) con rimborso tramite i pagamenti
  o senza rimborso; il biglietto conserva il numero ed è segnato annullato.
- L'assegnazione di un biglietto venduto per zona e ogni spostamento successivo sono una `Move` della prenotazione
  (spec 7.5, INV-83).

---
