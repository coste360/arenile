## 21. Tempo reale e funzionamento offline

### 21.1 Tempo reale

- Endpoint SSE `GET /api/admin/v1/stream?establishment=` autenticato.
- Gli eventi di allocazione e prenotazione vengono inviati come messaggi leggeri (`{type, spot_ids, dates}`); il client invalida le query interessate di TanStack Query.
- Broker in memoria nel processo. Per installazioni con più istanze in parallelo l'interfaccia `ext.EventBus` permette di usare `LISTEN/NOTIFY` di PostgreSQL.

### 21.2 Offline (pannello)

Obiettivo realistico per l'MVP: **continuità della consultazione e delle operazioni a basso rischio**.

- Il pannello è una PWA: shell e risorse in cache.
- Cache locale (IndexedDB) della mappa e delle prenotazioni del giorno corrente e del successivo.
- Operazioni consentite offline e messe in coda: check-in, nota, registrazione di un incasso in contanti su prenotazione esistente.
- Operazioni **non** consentite offline: creazione o spostamento di prenotazioni, perché richiedono la garanzia di disponibilità del server.
- Alla riconnessione la coda viene inviata in ordine con chiavi di idempotenza; eventuali conflitti vengono mostrati all'utente. Il server conserva la risposta di ogni chiave per 24 ore e per utente (sessione, chiave API o token del dispositivo): una ripetizione restituisce la risposta originale con `Idempotent-Replayed: true`, una richiesta ancora in corso con la stessa chiave risponde `409 idempotency_in_progress` (da ritentare). Le risposte 5xx e quelle con `Retry-After` non vengono conservate: `409 concurrent_update` (la modifica ha perso una gara con un'altra sugli stessi dati) e `429 rate_limited` si ritentano con la stessa chiave dopo `retry_after` secondi (ADR 0029). L'app mobile usa la stessa coda (sezione 29.2).
- Indicatore di stato della connessione sempre visibile.

---

