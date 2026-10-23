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
3. **Confronto anni** sullo stesso intervallo di calendario (stesso giorno e mese; 29 febbraio → 28), anno
   configurabile con `compare_year`, delta assoluto e relativo (assente se il precedente è 0).
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
   contratto di `checkInBooking` e `addBookingNote`); i conflitti si mostrano all'utente.
7. **Lato mare in alto di default.** Nuove mappe, generatore di file, modello classico e demo hanno il mare in alto
   (fila A più vicina al mare). Il cambio del lato mare di una mappa esistente può riorientare tutta la disposizione
   con `POST .../maps/{mapId}/reorient` (endpoint dedicato invece di un parametro del salvataggio: anteprima con
   `dry_run`, conteggio delle allocazioni future, conferma esplicita con `acknowledge_future_allocations`, altrimenti
   `409 spot_has_future_allocations`). Lati opposti: specchio parallelo alla riva (l'ordine lungo la riva non
   cambia); lati adiacenti: rotazione di 90° con scambio di larghezza e altezza. Solo geometria, un salvataggio
   versionato, evento `config.map_changed` con `change: "reoriented"`. Il default di colonna `maps.sea_side` resta
   `bottom` (SQLite non lo cambia senza ricostruire la tabella); l'applicazione scrive sempre il valore.
8. **Migrazione 00007** (00006 è riservata a M6): solo le colonne mancanti (`api_keys.expires_at`,
   `webhook_endpoints.description`, `previous_secret`, `previous_secret_expires_at`, `last_delivery_at`) e due indici.

## Conseguenze

- Positive: nessuna duplicazione del macchinario dei webhook e dell'autenticazione; report sempre coerenti con i dati
  (nessun aggregato da invalidare); offline senza nuove dipendenze e senza dati personali nella cache del service
  worker; il riorientamento non tocca id né allocazioni, quindi non può rompere la disponibilità.
- Negative: report calcolati a ogni richiesta (accettabile per i volumi di uno stabilimento, limite di 400 giorni);
  la modalità prova (15.4) non esiste ancora, quindi le prenotazioni di prova non sono escluse; tra 00005 e 00007 c'è
  un buco finché M6 non è integrata (goose senza `allow missing` rifiuta un database che ha 00007 prima di 00006:
  integrare M6 prima di distribuire, o abilitare l'opzione).
- Verifiche: test di tabella su periodi, confronto anni, CSV e trasformazioni della mappa; test di integrazione
  delle colonne di 00007; test di autorizzazione (chiave API rifiutata sulla gestione, isolamento per stabilimento);
  test che falliscono senza il controllo di scadenza e senza la doppia firma.
