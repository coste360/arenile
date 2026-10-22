## 7. Motore di disponibilità

### 7.1 Principio

La disponibilità è garantita **dai vincoli del database**, non dal solo codice applicativo. Questo funziona identico in SQLite e PostgreSQL.

- Per le postazioni: una riga in `spot_allocations` per ogni `(spot, data, fascia)`. Il vincolo `unique(spot_id, date, slot)` rende impossibile una doppia occupazione.
- Per i pool: un contatore per `(pool, data, fascia)` aggiornato con un `UPDATE` condizionale.

Una prenotazione a giornata intera occupa entrambe le fasce. Un abbonamento stagionale da 120 giorni genera 240 righe di allocazione: volume trascurabile anche con centinaia di postazioni.

### 7.2 Occupazione di una postazione

```
BEGIN
  DELETE FROM spot_allocations
    WHERE kind = 'hold' AND expires_at < now()
      AND spot_id = :spot AND date BETWEEN :from AND :to;   -- pulizia locale
  INSERT INTO spot_allocations (spot_id, date, slot, booking_line_id, kind, expires_at)
    VALUES (...per ogni data e fascia...);
  -- violazione del vincolo unique => postazione non disponibile => rollback
COMMIT
```

L'errore di vincolo viene tradotto in `ErrSpotUnavailable` con l'elenco delle date in conflitto, in modo che l'interfaccia possa proporre alternative.

### 7.3 Consumo di un pool

```
UPDATE pool_usage
   SET held = held + :qty
 WHERE pool_id = :pool AND date = :d AND slot = :s
   AND used + held + :qty <= :capacity_effettiva
```

Se il numero di righe aggiornate è minore delle righe attese, la transazione viene annullata. Le righe di `pool_usage` mancanti vengono create con `INSERT ... ON CONFLICT DO NOTHING` prima dell'aggiornamento. La capacità effettiva tiene conto di `capacity_overrides`.

### 7.4 Blocchi temporanei (hold)

- Durante il checkout pubblico le risorse vengono occupate con `kind = 'hold'` e `expires_at = now + HOLD_TTL` (default 10 minuti, configurabile).
- Alla conferma del pagamento, gli hold diventano `booking` (aggiornamento di `kind` ed `expires_at = null`; per i pool, `held` → `used`).
- Un job ogni 30 secondi rilascia gli hold scaduti e porta la prenotazione in stato `expired`.
- Anche senza job, gli hold scaduti vengono ignorati e ripuliti al momento del conflitto (7.2), quindi un job in ritardo non blocca mai le vendite.

### 7.5 Assegnazione automatica (modalità `zone`)

Quando una zona è in modalità `zone`, la prenotazione occupa **capacità di zona** e la postazione viene assegnata in seguito.

- Ogni zona `zone` ha un pool implicito di capacità uguale al numero di postazioni attive (creato e mantenuto dal sistema).
- La prenotazione consuma il pool di zona; la riga ha `assignment_status = pending`.
- L'assegnazione avviene in tre modi: manuale dal pannello (trascinamento), automatica alla conferma se `auto_assign = true`, oppure in blocco con il comando "assegna tutte le prenotazioni di domani".
- **Algoritmo automatico (greedy con preferenze):**
  1. ordina le righe da assegnare per durata decrescente (le più lunghe sono le più difficili da collocare);
  2. per ciascuna, scegli tra le postazioni libere per tutte le date la migliore secondo un punteggio: continuità con la prenotazione precedente dello stesso cliente, vicinanza ad altre righe della stessa prenotazione, preferenze indicate dal cliente (attributi), riempimento compatto delle file;
  3. se nessuna postazione è libera per l'intero periodo, consenti lo spezzamento solo se lo permette la policy (`allow_spot_change_during_stay`), altrimenti segnala la riga al gestore.
- Invariante: la somma delle righe assegnate e non assegnate di una zona non supera mai le postazioni attive per ciascuna data e fascia.

### 7.6 Quote online

Per le zone con `online_quota`, la disponibilità mostrata al canale `online` è:

```
disponibili_online = min(liberi_totali, quota - venduti_online_nello_slot)
```

Le prenotazioni dal canale `desk` ignorano la quota. Le quote sono calcolate per data e fascia.

### 7.7 Abbonati e rilasci

- Le postazioni degli abbonati stagionali sono occupate con `kind = 'season'` per tutte le date della stagione.
- Un rilascio trasforma le allocazioni dei giorni indicati in disponibilità rivendibile: le righe `season` vengono rimosse per quelle date e viene creata una `season_release`.
- Se il giorno viene rivenduto, `resold_booking_line_id` viene valorizzato e, secondo la policy, viene maturato un credito per l'abbonato.
- Se l'abbonato revoca il rilascio prima della rivendita, le allocazioni vengono ricreate.

### 7.8 Query di disponibilità

`GET /public/v1/{slug}/availability?from=&to=&slots=&pack=` restituisce, per ogni zona visibile:
- la modalità di assegnazione;
- per le zone `seat`: l'elenco delle postazioni con stato `available | unavailable` per l'intero intervallo (una postazione è disponibile solo se libera in tutte le date e fasce richieste);
- per le zone `zone` e `quantity`: la quantità disponibile minima nell'intervallo.

La risposta non espone mai dati sui clienti. È cacheabile per 10 secondi in memoria per stabilimento e intervallo, invalidata da ogni evento di allocazione.

---

