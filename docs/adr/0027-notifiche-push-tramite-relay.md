# 0027 — Notifiche push tramite il relay di Coste360

- **Stato:** accettato
- **Data:** 2026-10-25
- **Sezioni della specifica:** `docs/spec/29-edizione-desktop-e-app-mobile.md` (29.2, notifiche push),
  `docs/spec/19-notifiche.md` (notifiche al gestore), `docs/spec/24-deploy-configurazione-e-operativita.md`
  (24.4), `docs/spec/23-privacy-e-gdpr.md`, `docs/spec/28-roadmap-e-criteri-di-accettazione.md` (M7)

## Contesto

L'app mobile (M7, repository `coste360/arenile-mobile`) deve poter avvisare il gestore di una nuova prenotazione
online, di una prenotazione da approvare o di un pagamento non riuscito anche ad app chiusa. Su iOS e Android
l'unico canale è il servizio push della piattaforma (APNs, FCM), e solo chi pubblica l'app, cioè Coste360, ha le
credenziali per usarlo. Ogni istanza Arenile è invece di un gestore diverso e non può avere quelle credenziali.

Vincoli: l'app parla solo con l'istanza (ADR 0005 dell'app); nessun dato dei clienti o del lido (nomi, email, telefoni,
codici di prenotazione, importi, nome dello stabilimento) deve arrivare a Coste360; le push sono facoltative e
spente di default; un relay irraggiungibile non deve mai rallentare o far fallire un'operazione del lido;
l'edizione desktop senza internet resta senza push.

Alternative scartate:

- **Expo Push Service o Firebase nell'app**: un terzo vedrebbe token e testi, l'app contatterebbe host diversi
  dall'istanza ed esce dalle regole di privacy dell'app.
- **Credenziali APNs/FCM nell'istanza**: impossibile, sono legate all'account di chi pubblica l'app.
- **Relay con registro dei dispositivi** (l'app registra il token presso Coste360, l'istanza invia un
  identificativo interno): l'app parlerebbe con Coste360 e il relay conserverebbe dati per ogni telefono.
- **Testo libero nel messaggio**: basterebbe un errore di programmazione per far uscire un nome o un importo.

## Decisione

1. **Relay senza stato.** Coste360 gestisce un relay HTTPS che riceve dall'istanza una richiesta per ogni
   dispositivo da avvisare e la inoltra ad APNs o FCM. Il relay non conserva token né messaggi oltre la
   deduplica (24 ore sull'`id`) e registra solo esiti aggregati. Questo repository implementa l'istanza, non il
   relay.
2. **Protocollo, versione 1.** `POST <ARENILE_PUSH_RELAY_URL>` (predefinito
   `https://push.coste360.it/v1/notifications`) con corpo JSON:

   ```json
   {
     "v": 1,
     "id": "0193…",
     "device_push_id": "<token nativo APNs o FCM>",
     "platform": "ios",
     "environment": "production",
     "message": { "key": "booking_new_online", "locale": "it" },
     "ttl_seconds": 3600
   }
   ```

   `id` è l'identificativo della consegna (UUIDv7, deduplica sul relay); `device_push_id` è il token che il
   sistema operativo ha dato all'app: un identificativo opaco generato da Apple o Google, privo di significato
   per Coste360 e non collegabile a una persona senza i dati dell'istanza, ma persistente: è quindi un dato personale
   pseudonimo, non anonimo; `platform` è `ios` o `android`;
   `environment` è `production` o `sandbox` (build di sviluppo iOS); `message` contiene solo una **chiave di un
   catalogo chiuso** e la lingua (`it`, `en`, `de`, `fr`). Il relay compone il testo dal proprio catalogo, con
   titolo fisso "Arenile":

   | Chiave | it | en | de | fr |
   |---|---|---|---|---|
   | `booking_new_online` | Nuova prenotazione online | New online booking | Neue Online-Buchung | Nouvelle réservation en ligne |
   | `booking_approval_requested` | Prenotazione da approvare | Booking awaiting approval | Buchung wartet auf Freigabe | Réservation à approuver |
   | `payment_failed` | Pagamento non riuscito | Payment failed | Zahlung fehlgeschlagen | Paiement échoué |

   Una chiave sconosciuta diventa "Novità in Arenile". Non esistono campi liberi: il formato stesso impedisce di
   inviare dati personali. Toccando la notifica l'app si apre e rilegge i dati dall'istanza.
3. **Autenticazione dell'istanza.** Coste360 rilascia a chi lo chiede una chiave del relay
   `ARENILE_PUSH_RELAY_KEY=<id>.<segreto>` (id di 4–64 caratteri `[a-z0-9_-]`, segreto di almeno 32 caratteri).
   Ogni richiesta porta `Arenile-Relay-Key: <id>` e `Arenile-Signature: t=<unix>,v1=<hex HMAC-SHA256(segreto,
   "<t>.<corpo>")>`, lo stesso schema delle firme dei webhook (`event.Sign`): il segreto non viaggia mai, il
   relay rifiuta firme fuori da ±5 minuti e può revocare o limitare una chiave. L'User-Agent è
   `arenile/<versione>`. Senza chiave le push non sono disponibili e l'istanza non fa alcuna chiamata.
4. **Risposte del relay.** `2xx`: consegnata. `410`: token non più valido, l'istanza cancella la registrazione.
   Altri `4xx`: errore definitivo, nessun nuovo tentativo. `429`, `5xx` o errore di rete: nuovi tentativi dopo
   30 s, 2, 5 e 15 minuti (o dopo `Retry-After`, al massimo un'ora), poi fallita. Una consegna non partita entro
   un'ora scade: un avviso in ritardo è solo rumore.
5. **Consenso a tre livelli.**
   - *Installazione*: chi gestisce il server imposta `ARENILE_PUSH_RELAY_KEY`; `/meta` espone
     `capabilities.push_notifications` vero solo con la chiave e sull'edizione server (mai sul desktop).
   - *Gestore*: `PUT /api/admin/v1/establishments/{id}/push-settings` (permesso `establishment.configure`,
     `If-Match`) accende le push dello stabilimento (`enabled`, **spento di default**) e sceglie quali eventi
     inviare (tutti accesi quando le push sono accese).
   - *Utente*: l'app registra il proprio token solo dopo che l'utente ha attivato le notifiche nell'app e
     concesso il permesso del sistema, e sceglie gli argomenti tra quelli ammessi dal suo ruolo.
6. **Registrazione del token nativo** (`/api/device/v1/push`, solo token del dispositivo):
   `PUT {token, environment, locale, topics}` crea la registrazione del dispositivo o la aggiorna mantenendo il suo
   id, così la deduplica delle consegne per (registrazione, evento) resta valida (una per dispositivo, piattaforma presa dal dispositivo abbinato, solo `ios` e `android`); risponde
   `409 push_unavailable` se l'istanza non ha la chiave o il gestore non ha acceso le push. `DELETE` la
   cancella (idempotente), `GET` dice se le push sono disponibili, se il dispositivo è registrato e quali
   argomenti il ruolo ammette. Il token è cifrato a riposo con il key ring di `ARENILE_SECRET_KEY` (AES-256-GCM,
   AAD = id del dispositivo) e conservato anche come SHA-256 per evitare due registrazioni dello stesso token
   nello stesso stabilimento; non compare mai in log, audit o risposte. La revoca del dispositivo cancella la
   registrazione nella stessa transazione. Audit `device.push_registered`, `device.push_unregistered`,
   `push_settings.updated` (mai il token).
7. **Argomenti e ruoli.** `booking_new_online` (evento `booking.confirmed` sul canale online, permesso
   `bookings.read`), `booking_approval_requested` (`booking.approval_requested`, `bookings.write`),
   `payment_failed` (`payment.failed`, `refunds.issue`). Il ruolo si ricontrolla all'invio: un utente
   disattivato, senza ruolo o con un ruolo che non ha più il permesso non riceve nulla, come un dispositivo
   revocato o scaduto per inattività.
8. **Dall'outbox al relay.** Un sink dell'outbox (`internal/domain/push`) decide i destinatari e accoda una riga
   in `push_deliveries` per (registrazione, evento), unica, quindi un evento riconsegnato non accoda due volte.
   Un job in processo (`push.deliver`, ogni 15 secondi e subito dopo un accodamento) invia le righe dovute con un
   lease condizionale, così due istanze non inviano la stessa riga; le righe chiuse si cancellano dopo 7 giorni. Lo
   stesso job orario di conservazione (`push.retention`) cancella le registrazioni che non possono più ricevere nulla
   (dispositivo revocato o scaduto per inattività, utente disattivato), con il loro token cifrato.
   Nessuna chiamata di rete avviene durante una richiesta del pannello o dentro una transazione.

## Conseguenze

- Positive: push senza dati dei clienti né contenuti per costruzione (l'unico dato personale è il token
  pseudonimo); il relay è sostituibile (`ARENILE_PUSH_RELAY_URL`, per esempio un relay proprio di chi ricompila
  l'app con le proprie credenziali); l'istanza resta l'unico posto dove sta il collegamento tra telefono e utente;
  spegnere le push dal pannello le ferma subito.
- Negative: Coste360 vede l'IP dell'istanza, l'id della chiave e i token nativi in transito (necessari ad APNs e
  FCM): con le push attive Coste360 è responsabile del trattamento (art. 28 GDPR) per conto del gestore, con Apple e
  Google come sub-responsabili, e serve un accordo sul trattamento dei dati; senza chiave del relay niente push
  (l'app aggiorna i dati all'apertura); il testo è generico per scelta e non dice di quale prenotazione si tratta.
- Verifiche: `TestBuildPayload_NoPersonalData`, `TestRelayClient_SignatureAndOutcomes` (relay finto con
  `httptest`), `TestSign_MatchesWebhookScheme`, `TestKey_NeverPrintsSecret` (`internal/pushrelay`),
  `TestPushService_*` e `TestConcurrency_PushDeliveriesSingleSend` (`internal/domain/push`, tag `integration`,
  SQLite e PostgreSQL), `TestPush_EndToEndThroughFakeRelay` e `TestPush_NoRelayKeyNoNetwork` (`internal/app`),
  `TestMigration00013_PushNotifications` (`internal/store`), `TestBuild_PushNotifications` (`internal/domain/meta`),
  `TestParse_PushRelay` (`internal/config`), `PushSettingsCard.test.tsx`; invariante INV-100.
