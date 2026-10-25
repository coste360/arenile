## 23. Privacy e GDPR

- Dati minimi per una prenotazione online: nome, cognome ed email; il telefono è sempre facoltativo, anche dal
  pannello (ADR 0020). La vendita giornaliera al banco non richiede alcun dato del cliente.
- Testi delle condizioni e dell'informativa versionati; ogni prenotazione salva la versione accettata.
- Consenso marketing separato, facoltativo, con data, fonte e versione del testo.
- **Esportazione** dei dati di un cliente in JSON dal pannello.
- **Anonimizzazione** su richiesta: i dati personali vengono sostituiti, mentre prenotazioni e pagamenti restano per gli obblighi contabili.
- **Conservazione configurabile:** anonimizzazione automatica dei clienti senza prenotazioni da N anni (default 5, modificabile).
- Log applicativi senza dati personali in chiaro (email e telefoni mascherati).
- Notifiche push dell'app mobile (facoltative, ADR 0027): il relay di Coste360 riceve solo il token nativo del
  telefono, la piattaforma e una chiave di un catalogo chiuso con la lingua; mai nomi, codici, importi o il nome dello
  stabilimento. Il token è cifrato a riposo nell'istanza e cancellato alla revoca del dispositivo.
- Documentazione per il gestore con un modello di informativa da adattare e l'indicazione dei ruoli (titolare: gestore; responsabile: chi ospita il servizio).

---

