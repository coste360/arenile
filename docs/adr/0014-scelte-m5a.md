# 0014 — Scelte di M5 (parte A): report, chiavi API, webhook configurabili, PWA offline e lato mare

- **Stato:** accettato
- **Data:** 2026-10-23
- **Sezioni della specifica:** `docs/spec/11-mappa-editor-e-vista-operativa.md`, `docs/spec/15-pannello-del-gestore.md` (15.3),
  `docs/spec/16-utenti-ruoli-e-autenticazione.md`, `docs/spec/17-api.md`, `docs/spec/18-eventi-webhook-e-punti-di-estensione.md` (18.2),
  `docs/spec/21-tempo-reale-e-funzionamento-offline.md` (21.2), `docs/spec/22-sicurezza.md`, `docs/spec/23-privacy-e-gdpr.md`,
  `docs/spec/28-roadmap-e-criteri-di-accettazione.md` (M5)

## Contesto

La parte A di M5 porta report completi con confronto anni, webhook in uscita e chiavi API gestibili dal pannello,
la PWA del pannello con offline limitato e una correzione della mappa chiesta dal product owner (sui lidi italiani
il mare è di solito in alto). Cinque builder lavorano in parallelo sullo stesso contratto; tabelle e macchinario di
consegna dei webhook e l'autenticazione con chiave API esistono da M0.

## Decisione

1. **Report come query, non materializzati.** Nove endpoint `GET .../reports/{summary, occupancy, revenue, bookings,
   cancellations, season-passes, top-customers, open-balances, comparison}` con `from`/`to` (al massimo 400 giorni) e
   `format=json|csv` (o `Accept: text/csv`). Nessuna tabella di aggregati: query sul sottoinsieme SQL comune e
   raggruppamenti di calendario in Go (`internal/domain/report`). Unico indice aggiunto:
   `bookings (establishment_id, created_at)`.
2. **Definizioni fissate nel contratto.** Unità di occupazione = (postazione, giorno, fascia); capacità esclusi
   chiusure e blocchi; rilasci non rivenduti liberi. Ricavi su due basi esplicite: `collected` (pagamenti per
   `business_date`, rimborsi negativi, IVA da `vat_breakdown`) e `booked` (prenotazioni confermate per giorno di
   conferma). Rapporti in punti base interi (`RatioBP`), mai float. CSV con BOM, `;`, CRLF e virgola decimale come i
   corrispettivi. Classifica clienti solo con il nome: nessun contatto (23).
   **Riepilogo e confronto anni** usano due sole basi di data, così le schede di uno stesso periodo non si
   contraddicono: base *soggiorno* (occupazione per giorno di soggiorno; prenotazioni, valore medio, prenotato,
   abbonamenti, cancellazioni e no-show per primo giorno di soggiorno `starts_on`, qualunque sia la data di vendita:
   un lido vende la stagione mesi prima) e base *incasso* (`business_date` del pagamento). Il pannello stampa la
   base sotto ogni scheda; i report di dettaglio prenotazioni (data di vendita, per anticipo) e ricavi `booked`
   (data di conferma) mantengono le loro basi dichiarate.
3. **Confronto anni** sullo stesso intervallo di calendario (stesso giorno e mese; 29 febbraio → 28), anno
   configurabile con `compare_year` (deve precedere l'anno di `from`), delta assoluto e relativo (assente se il
   precedente è 0). La serie sovrappone l'anno precedente a quello corrente **per giorno di calendario** (un 29
   febbraio precedente si somma al 28 febbraio, uno corrente non ha riscontro), mai per indice, così un valore non
   finisce nel mese sbagliato; con `align=weekday` l'allineamento è giorno per giorno.
4. **Chiavi API**: `internal/domain/apikey` gestisce, `internal/domain/auth` continua ad autenticare (formato
   `ar_live_<prefix>_<secret>`, solo hash). Aggiunta la scadenza (`api_keys.expires_at`, rifiutata in
   autenticazione). Gestione riservata a `users.manage` (proprietario); una chiave non gestisce mai chiavi né webhook;
   scope = permessi tranne `users.manage`. Il segreto compare solo nella risposta di creazione, senza
   `Idempotency-Key` (la risposta memorizzata conterrebbe il segreto).
5. **Webhook**: `internal/domain/webhook` gestisce endpoint, segreti, registro e reinvii; `internal/domain/event`
   resta l'unico che consegna (Dispatcher, Deliverer, backoff 24 h, disattivazione automatica). Rotazione con periodo
   di grazia: due firme `v1` nello stesso header (`previous_secret` cifrato, AAD `<id>:previous`). Evento di prova
   `webhook.test` scritto già distribuito (arriva solo all'endpoint provato) e consegnato in modo sincrono. URL solo
   https (http solo verso localhost). Stati: active, paused (dal gestore), disabled (automatico).
6. **PWA**: resta `vite-plugin-pwa` (MIT, organizzazione ufficiale vite-pwa, già in uso da M0) in modalità
   `generateSW` per shell e risorse statiche; scartato un service worker scritto a mano (più codice da mantenere
   senza benefici, la coda richiede comunque l'interfaccia per i conflitti). I dati offline (mappe, prenotazioni di
   oggi e domani) sono in IndexedDB scritti dall'app con un piccolo wrapper sull'API nativa: nessuna nuova
   dipendenza, il service worker non mette mai in cache le risposte `/api/` (dati personali, per utente). Coda
   offline solo per check-in, nota e incasso in contanti, inviata in ordine con `Idempotency-Key` (aggiunta al
   contratto di `checkInBooking` e `addBookingNote`); i conflitti si mostrano all'utente. Un check-in accodato
   dietro un'altra operazione sulla stessa prenotazione non invia `If-Match` (ogni operazione alza la versione sul
   server; a proteggerlo basta la transizione di stato). Al cambio di utente sullo stesso browser si cancella la
   copia dei dati, **non la coda**: contiene incassi in contanti davvero riscossi e check-in davvero fatti (nessun
   dato personale) e li invia la nuova sessione; scartarla in silenzio lascerebbe la cassa scoperta. Il service worker passa da
   `autoUpdate` a `prompt` (nessun ricaricamento durante un'operazione: l'utente conferma l'aggiornamento); la copia
   offline è la persistenza di TanStack Query scritta a mano su `dehydrate`/`hydrate` con un'allowlist di chiavi
   (`admin/src/offline/cache.ts`), senza i pacchetti `persist-client`.
7. **Lato mare in alto di default.** Nuove mappe, generatore di file, modello classico e demo hanno il mare in alto
   (fila A più vicina al mare). Il cambio del lato mare di una mappa esistente può riorientare tutta la disposizione
   con `POST .../maps/{mapId}/reorient` (endpoint dedicato invece di un parametro del salvataggio: anteprima con
   `dry_run`, conteggio delle allocazioni future, conferma esplicita con `acknowledge_future_allocations`, altrimenti
   `409 spot_has_future_allocations`). Lati opposti: specchio parallelo alla riva (l'ordine lungo la riva non
   cambia); lati adiacenti: rotazione di 90° con scambio di larghezza e altezza. Scelta consapevole: le
   trasformazioni **non compongono** tra percorsi diversi (alto → destra → basso, due quarti di giro, inverte
   l'ordine lungo la riva; alto → basso, specchio, lo conserva), documentato in `ReorientTransform` e fissato da
   `TestReorientLayout_PathsDoNotCompose`; ogni singolo passo è reversibile. Solo geometria, un salvataggio
   versionato, evento `config.map_changed` con `change: "reoriented"`. Il default di colonna `maps.sea_side` resta
   `bottom` (SQLite non lo cambia senza ricostruire la tabella); l'applicazione scrive sempre il valore.
8. **Migrazione 00007** (00006 è riservata a M6): solo le colonne mancanti (`api_keys.expires_at`,
   `webhook_endpoints.description`, `previous_secret`, `previous_secret_expires_at`, `last_delivery_at`) e due indici.

## Conseguenze

- Positive: nessuna duplicazione del macchinario dei webhook e dell'autenticazione; report sempre coerenti con i dati
  (nessun aggregato da invalidare); offline senza nuove dipendenze e senza dati personali nella cache del service
  worker; il riorientamento non tocca id né allocazioni, quindi non può rompere la disponibilità.
- Negative: report calcolati a ogni richiesta (accettabile per i volumi di uno stabilimento, limite di 400 giorni);
  la modalità prova (15.4) non esiste ancora, quindi le prenotazioni di prova non sono escluse (il test
  `TestReportQueries_TestBookingsFlagNotYetFiltered` fallisce appena compare una colonna di prova su `bookings`
  senza che `store/report.go` la filtri); tra 00005 e 00007 c'è un buco finché M6 non è integrata (goose senza
  `allow missing` rifiuta un database che ha 00007 prima di 00006: integrare M6 prima di distribuire, o abilitare
  l'opzione; la numerazione finale la decide chi integra in base all'ordine di merge).
- **Scostamenti dalla specifica rimandati a M5 parte B** (da chiudere prima di dichiarare M5 completa):
  - 15.3 chiede esportazioni **CSV e XLSX** di prenotazioni, clienti, pagamenti e corrispettivi (`exports/*` in
    17.3): la parte A esporta solo i report aggregati, solo in CSV. Le esportazioni di elenco e l'XLSX (che richiede
    una dipendenza con licenza MIT/BSD/Apache da motivare nella PR) restano da fare.
  - 18.2 chiede la disattivazione automatica di un endpoint «con avviso al proprietario»: oggi il fatto è scritto nel
    registro attività e nel log e il pannello mostra lo stato *disattivato*; nessuna email raggiunge il proprietario
    (le notifiche di `internal/domain/notify` sono oggi legate a una prenotazione e a un cliente). Da aggiungere con
    un modello email per il gestore nella stessa transazione di `DisableWebhookEndpoint`.
- Verifiche: test di tabella su periodi, confronto anni, CSV e trasformazioni della mappa; test di integrazione
  delle colonne di 00007; test di autorizzazione (chiave API rifiutata sulla gestione, isolamento per stabilimento);
  test che falliscono senza il controllo di scadenza e senza la doppia firma.
