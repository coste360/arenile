## 19. Notifiche

- Canale email nel core (SMTP); interfaccia `Notifier` per WhatsApp o SMS come estensioni.
- Template predefiniti in tutte le lingue abilitate. Il gestore modifica, per ogni email al cliente e lingua, oggetto, testo iniziale, testo finale e firma in testo semplice con segnaposto da un elenco chiuso (`{nome}`, `{codice}`, `{data}`, ...); QR, riepilogo, importi, link di gestione e piè di pagina restano fissi (ADR 0022, che sostituisce i template Markdown modificabili con variabili `{{booking.code}}`).
- Promemoria del saldo e del giorno prima inviati automaticamente da un job, agli orari configurati nel fuso dello stabilimento, una sola volta per prenotazione (ADR 0022).
- Notifiche al cliente: conferma, promemoria del saldo, promemoria il giorno prima, cancellazione, rimborso, proposta di rinnovo, posto disponibile dalla lista d'attesa.
- Notifiche al gestore: nuova prenotazione online, richiesta di approvazione, pagamento fallito, chiusura cassa con differenza, errore di webhook.
- Invio asincrono tramite outbox, con tentativi e registro in `message_log`.
- Allegati: QR in linea, file `.ics`.

---

