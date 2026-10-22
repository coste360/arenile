## 19. Notifiche

- Canale email nel core (SMTP); interfaccia `Notifier` per WhatsApp o SMS come estensioni.
- Template predefiniti in tutte le lingue abilitate, modificabili dal gestore in Markdown con variabili (`{{booking.code}}`, `{{customer.first_name}}`, `{{manage_url}}`).
- Notifiche al cliente: conferma, promemoria del saldo, promemoria il giorno prima, cancellazione, rimborso, proposta di rinnovo, posto disponibile dalla lista d'attesa.
- Notifiche al gestore: nuova prenotazione online, richiesta di approvazione, pagamento fallito, chiusura cassa con differenza, errore di webhook.
- Invio asincrono tramite outbox, con tentativi e registro in `message_log`.
- Allegati: QR in linea, file `.ics`.

---

