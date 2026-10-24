# 0022 — Testi delle email ai clienti modificabili e promemoria automatici

- **Stato:** accettato (realizzato)
- **Data:** 2026-10-24
- **Sezioni della specifica:** `docs/spec/19-notifiche.md`, `docs/spec/09-policy-di-prenotazione.md`, `docs/spec/20-internazionalizzazione.md`

## Contesto

La spec 19 prevede template modificabili dal gestore e i promemoria del saldo e del giorno prima. Fino alla 1.0
le email erano fisse (Markdown incorporato, `internal/domain/notify`), la tabella `notification_templates`
(migrazione 00005, Markdown intero per modello) non aveva API né pagina, e i due promemoria avevano solo gli
interruttori nelle impostazioni email, disattivati: nessun job li inviava.

Dare al gestore il Markdown intero era sconsigliabile: un modello modificato può rompere QR, riepilogo, importi,
link di gestione o piè di pagina legale, e le variabili `{{booking.code}}` sono poco leggibili per chi gestisce
uno stabilimento. Per i promemoria servivano orari nel fuso dello stabilimento (con l'ora legale) e la garanzia
di non inviare due volte lo stesso promemoria anche con più istanze o esecuzioni sovrapposte.

## Decisione

- **Testi modificabili, blocchi fissi.** Per ogni email al cliente (conferma, modifica, cancellazione, richiesta
  non accettata, richiesta ricevuta, rimborso, link di gestione, promemoria del saldo, promemoria del giorno prima)
  e per ogni lingua (it, en, de, fr) si modificano **oggetto, testo iniziale, testo finale e firma**. QR,
  riepilogo, importi, avviso del saldo, pulsante e link di gestione, condizioni, nota sul calendario e piè di
  pagina restano fissi nel modello incorporato, che li dispone attorno ai partial `{{> opening}}`,
  `{{> closing}}`, `{{> signature}}`. Logo e colori vengono dal tema dello stabilimento.
- **Testo semplice e segnaposto sicuri.** Niente Markdown né HTML: una riga vuota separa i paragrafi, un a capo
  diventa `<br>`, ogni carattere è sottoposto a escape. I segnaposto sono un insieme chiuso in italiano
  (`{nome}`, `{cognome}`, `{codice}`, `{data}`, `{postazione}`, `{stabilimento}`, `{importo}`, `{saldo}`,
  `{scadenza}`), validati al salvataggio, in anteprima e nella prova: uno sconosciuto risponde
  `422 email_template_invalid` con il campo. I valori si sostituiscono prima dell'escape e una sola volta.
  Limiti: oggetto 1–200 caratteri su una riga, testi iniziale e finale 2000, firma 500.
- **Solo le personalizzazioni nel database.** I testi predefiniti sono il front matter dei modelli incorporati
  (`subject`, `opening`, `closing`, `signature`), quindi seguono gli aggiornamenti di Arenile. La migrazione
  **00012** aggiunge `email_template_overrides` (una riga per stabilimento, tipo e lingua, `version` come ETag,
  `If-Match` con `"0"` per la prima personalizzazione); "ripristina predefinito" elimina la riga. Permesso
  `establishment.configure` per salvare, ripristinare e inviare la prova (all'indirizzo dell'utente collegato,
  oggetto con `[PROVA]`), `bookings.read` per leggere e per l'anteprima; salvataggio e ripristino scrivono il
  registro di audit con i soli nomi dei campi cambiati. La vecchia `notification_templates` resta: se una riga
  esiste, vince come prima.
- **Anteprima sul server.** L'anteprima rende i testi non salvati con una prenotazione di esempio (nessun dato
  reale), con le immagini in linea come data URI; il pannello la mostra in un `iframe` con `sandbox` vuoto.
- **Promemoria come job in processo.** `notify.reminders` gira ogni 5 minuti: per ogni stabilimento calcola il
  giorno e l'istante di invio nel suo fuso (`time.Date` nella `Location`, ora legale gestita dalla libreria
  standard, orari inesistenti spostati al primo istante valido) e, dall'istante fino alla fine di quel giorno
  locale, accoda il promemoria alle prenotazioni idonee. Giorno prima: inizio domani, alle 18:00 predefinite.
  Saldo: scadenza dalla policy della prenotazione (`balance_due`: `days_before:N`, oppure il giorno di inizio se
  all'arrivo), inviato `balance_lead_days` (predefinito 2, da 0 a 30) giorni prima alle 10:00 predefinite. Orari
  e anticipo stanno nelle impostazioni email (`reminders`), gli interruttori sono quelli già esistenti; un client
  che non manda `reminders` lascia invariato l'orario salvato. Un giorno perso a server spento non viene
  recuperato in ritardo.
- **Una sola volta per prenotazione e tipo.** La chiave `reminder:<tipo>:<id prenotazione>` nell'indice unico
  `message_log (establishment_id, dedupe_key)` (00005, `INSERT ... ON CONFLICT DO NOTHING`) rende l'invio
  idempotente senza una tabella nuova. Idonee: confermate, non di prova, create prima dell'istante di invio,
  cliente con email non anonimizzato, saldo positivo per il promemoria del saldo; le stesse regole si ricontrollano
  all'invio (un pagamento arrivato nel frattempo annulla il messaggio).
- **Pagina "Email ai clienti"** in Configurazione: promemoria (interruttori, orari, anticipo) e editor con scelta
  del segnaposto, anteprima dal vivo, prova e ripristino. I promemoria escono dalla pagina "Email e notifiche".

## Conseguenze

- Positive: il gestore cambia tono e contenuto senza poter rompere le parti che contano (QR, importi, link,
  piè di pagina) né iniettare markup; gli aggiornamenti dei testi predefiniti arrivano a chi non li ha toccati;
  i promemoria non richiedono servizi esterni né cron di sistema e non partono mai due volte.
- Negative: grassetti e link nei testi modificabili non sono possibili (scelta voluta); il titolo dell'email e i
  paragrafi tecnici fissi (es. nota sui link precedenti) non sono modificabili; il promemoria è unico per
  prenotazione anche se le date cambiano dopo l'invio; se il server resta spento per tutto il giorno del
  promemoria, quel promemoria non parte.
- Verifiche: INV-95, INV-96, INV-97 in `docs/invariants.md`; `TestValidateTexts_Scenarios`,
  `TestRender_EditableTextsEscaped`, `TestReminderWindow_TimezonesAndDST` (`internal/domain/notify`);
  `TestReminders_SchedulingAndEligibility`, `TestConcurrency_ReminderOnce`, `TestEmailTemplates_EditorUseCases`
  (tag `integration`); `TestEmailTemplatesHTTP_ValidationAndETag` (`internal/http/admin`);
  `TestMigration00012_EmailTemplateOverrides` (`internal/store`); Vitest `customerEmails.test.tsx`; e2e
  `customer-emails.spec.ts`.
