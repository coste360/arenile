## 8. Motore dei prezzi

### 8.1 Pipeline di calcolo

Il calcolo è una **funzione pura**: `Quote(input, catalogo, regole) → PriceBreakdown`. Non accede al database durante l'esecuzione: le regole vengono caricate prima. Questo la rende testabile in modo esaustivo.

```
input: stabilimento, canale, date, fasce, formula, righe [pack, zona/postazione],
       extra, cliente (convenzione), codice promozionale

per ogni riga:
  1. FORMULA A DURATA FISSA
     se formula ∈ {week, fortnight, month, season}
     e esiste fixed_duration_price(pack, formula, price_category, periodo)
       → base_riga = quel prezzo
     altrimenti passa al punto 2 sommando i giorni

  2. BASE PER GIORNO
     per ogni giorno dell'intervallo:
       applica le regole stage=base valide quel giorno
       (vince quella con priorità più alta tra quelle applicabili)
       → prezzo_giorno
     se slots ∈ {AM, PM}: usa la regola con condizione slot corrispondente,
       oppure il prezzo giornaliero × half_day_factor (impostazione stabilimento)

  3. MODIFICATORI (stage=modifier)
     per ogni giorno: applica in ordine di priorità
     le regole non stackable sostituiscono le precedenti dello stesso gruppo

  4. EXTRA
     per ogni extra: stesso procedimento (base + modificatori) con target extra,
     moltiplicato per quantità e per giorni o fasce secondo pricing_unit

somma righe + extra a livello prenotazione

  5. SCONTI (stage=discount): convenzione, poi codice promozionale, poi
     sconti automatici (es. durata lunga); stackable=false ⇒ si applica il migliore

  6. ARROTONDAMENTO: secondo l'impostazione dello stabilimento
     (nessuno, all'euro, ai 50 centesimi), applicato solo al totale finale

  7. ACCONTO: calcolato dalla policy (sezione 9)
```

### 8.2 Condizioni di una regola

Tutte opzionali; una regola si applica se **tutte** le condizioni presenti sono vere.

```json
{
  "rate_periods": ["high", "peak"],
  "date_ranges": [{"from": "2027-08-10", "to": "2027-08-20"}],
  "weekdays": [6, 7],
  "holidays": true,
  "slots": ["AM"],
  "formulas": ["day", "multi_day"],
  "price_categories": ["row1", "row2"],
  "zone_ids": ["..."],
  "spot_attributes": ["accessible"],
  "channels": ["online"],
  "min_days": 3,
  "max_days": 6,
  "lead_time_days": {"min": 0, "max": 1},
  "occupancy_percent": {"min": 80},
  "customer_tags": ["storico"],
  "agreement_ids": ["..."]
}
```

- `holidays` usa un calendario delle festività nazionali italiane incluso nel binario, con festività locali aggiungibili per stabilimento (es. santo patrono).
- `occupancy_percent` è valutata sulla zona per il singolo giorno. È opzionale e disattivata di default.

### 8.3 Azioni

```json
{"type": "set",         "amount": 2500}
{"type": "add",         "amount": 500}
{"type": "multiply",    "factor": 1.2}
{"type": "percent_off", "percent": 10}
{"type": "amount_off",  "amount": 1000, "per": "booking"}
{"type": "free_days",   "every": 7, "free": 1}
```

- `set` è valida solo nello stage `base`.
- `multiply` e `percent_off` usano aritmetica intera con arrotondamento al centesimo (half-even), mai float nel calcolo finale.
- `free_days` implementa offerte del tipo "7 giorni al prezzo di 6".

### 8.4 Risoluzione dei conflitti

- Nello stage `base` vince una sola regola per giorno: la priorità più alta; a parità, la più specifica (più condizioni); a parità, la più recente.
- Negli stage `modifier` e `discount`, le regole `stackable = true` si sommano in ordine di priorità; tra le regole `stackable = false` si applica solo quella con il risultato più favorevole al cliente.
- Se per un giorno non esiste nessuna regola `base`, il pack **non è vendibile** quel giorno: la riga viene rifiutata con `ErrNoPrice`. Il pannello segnala in configurazione i buchi di listino (vedi 8.7).

### 8.5 Prezzi IVA

- I prezzi configurati sono **IVA inclusa**.
- Ogni pack ed extra ha un `vat_rate` (configurabile, con valore predefinito a livello di stabilimento). Lo scorporo è calcolato solo per i documenti e i report.
- Il codice non contiene aliquote fisse: le imposta il gestore.

### 8.6 Snapshot del prezzo

Ogni prenotazione salva in `price_snapshot` il dettaglio completo:

```json
{
  "engine_version": 1,
  "currency": "EUR",
  "lines": [{
    "pack": "base", "zone": "row1", "spot": "A12",
    "days": [
      {"date": "2027-08-14", "slots": "FULL", "base": 3500,
       "applied": [{"rule": "r_peak", "stage": "base", "effect": "set 3500"},
                   {"rule": "r_weekend", "stage": "modifier", "effect": "+500"}],
       "total": 4000}
    ],
    "extras": [{"code": "sunbed", "qty": 1, "total": 800}],
    "total": 4800
  }],
  "discounts": [{"source": "promo:ESTATE10", "amount": -480}],
  "rounding": 0,
  "total": 4320,
  "vat": [{"rate": 22, "taxable": 3541, "tax": 779}]
}
```

Lo snapshot non cambia mai dopo la conferma. Le modifiche alla prenotazione generano un nuovo calcolo e una differenza da incassare o rimborsare.

### 8.7 Strumenti del pannello per i prezzi

- **Simulatore:** stessa API di quotazione con canale selezionabile e dettaglio passo per passo.
- **Matrice di listino:** vista tabellare pack × fila × periodo con il prezzo risultante, generata dal motore.
- **Verifica di copertura:** segnala i giorni della stagione in cui un pack attivo non ha prezzo per una categoria.
- **Copia di stagione:** duplica regole, periodi e prezzi fissi sull'anno successivo, spostando le date con lo stesso giorno della settimana dove ha senso.

---

