# 0027 — Abbinamento allo stabilimento dimostrativo per l'app mobile

- **Stato:** accettato
- **Data:** 2026-10-25
- **Sezioni della specifica:** `docs/spec/29-edizione-desktop-e-app-mobile.md` (29.2, "L'app non ha un server
  predefinito"), `docs/spec/24-deploy-configurazione-e-operativita.md` (24.4)

## Contesto

L'app mobile si collega solo all'istanza del gestore, con un codice di abbinamento monouso creato dal pannello
(*Configurazione → Dispositivi*). Chi la scarica dallo store senza avere Arenile non può vederla funzionare, e la
revisione di Apple rifiuta spesso le app che non si usano senza un account o un server.

Precompilare email e password della demo nell'app non è un'opzione: l'app non accede con email e password ma con
il token del dispositivo, senza cookie, e una password nel codice dell'app sarebbe estraibile da chiunque.

## Decisione

1. Nuovo `POST /api/device/v1/demo-pairing`, non autenticato: restituisce un codice di abbinamento nuovo
   (`DevicePairingCode`, 5 minuti) per un utente dimostrativo, che l'app scambia subito con
   `POST /api/device/v1/pair`. Il dispositivo ha il ruolo di quell'utente.
2. Attivo solo se l'istanza imposta `ARENILE_DEMO_PAIRING_USER` (email) e `ARENILE_DEMO_PAIRING_ESTABLISHMENT`
   (slug), entrambe o nessuna; mai sull'edizione desktop. Senza configurazione, e con una configurazione
   sbagliata (utente o stabilimento inesistenti, utente senza ruolo attivo), risponde `404` come una rotta che
   non esiste; l'errore di configurazione va nel log.
3. Limite di 5 richieste all'ora per IP. Ogni codice è registrato nel registro attività come azione di sistema
   (`device.demo_pairing_code_created`) con l'IP.
4. L'app mostra "Prova la demo" solo come scelta esplicita e solo verso l'istanza dimostrativa di Coste360
   (ADR dell'app mobile): resta l'unica eccezione a "nessun server predefinito".

## Conseguenze

- Ogni prova crea un dispositivo sull'istanza demo: chi la gestisce assegna all'utente dimostrativo un ruolo
  limitato e azzera i dati periodicamente (dispositivi compresi).
- Le istanze dei gestori non cambiano: senza le due variabili l'endpoint non fa nulla.

## Alternative scartate

- **Credenziali demo nell'app:** estraibili e incompatibili con l'accesso solo a token.
- **Un codice fisso e riutilizzabile:** i codici sono monouso per costruzione (aggiornamento condizionale);
  renderli riutilizzabili indebolirebbe l'abbinamento di tutte le istanze.
