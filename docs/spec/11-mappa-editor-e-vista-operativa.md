## 11. Mappa: editor e vista operativa

### 11.1 Rendering

- SVG con coordinate logiche (`maps.width × maps.height`) e `viewBox`, scalato al contenitore.
- Pan e zoom con gesti touch e rotella; pulsante "adatta alla vista".
- Fino a 1.000 elementi devono restare fluidi su un tablet di fascia media. Oltre questa soglia si usa la virtualizzazione per settori, non prevista nell'MVP.
- Ogni postazione è un `<g>` accessibile: `role="button"`, `aria-label` con etichetta e stato, focus da tastiera.
- La pagina pubblica offre sempre un'alternativa a elenco (per fila) per lettori di schermo e schermi piccoli.

### 11.2 Editor (solo pannello)

- Griglia con snapping (`grid_size`).
- **Generatore di file:** "crea N file da M posti, distanza X, numerazione A–Z / 1–N, a partire da…", come punto di partenza.
- Trascinamento singolo e multiplo, rotazione, allineamento e distribuzione, duplicazione.
- Palette di elementi: postazioni (tipi) ed elementi decorativi.
- Pannello proprietà: etichetta, zona, tipo, capacità, attributi, categoria di prezzo, stato attivo.
- Rinumerazione automatica di una fila selezionata.
- Annulla e ripeti (stack di comandi lato client), con salvataggio esplicito.
- **Salvataggio transazionale con controllo di versione:** `PUT /maps/{id}` con `If-Match` sulla versione; in caso di conflitto viene mostrata la differenza.
- Non è possibile eliminare una postazione con allocazioni future: si può solo disattivarla (`active = false`), e il sistema elenca le prenotazioni da spostare.
- Sfondo opzionale: immagine caricata dal gestore oppure perimetro della concessione fornito da `ConcessionDataProvider`, con scala e rotazione regolabili.

### 11.3 Vista operativa

- Selettore della data sempre visibile, con frecce giorno precedente e successivo e "oggi".
- Selettore della fascia: giorno intero, mattina, pomeriggio.
- **Colori per stato** (personalizzabili nel tema del pannello, con legenda sempre visibile):

| Stato | Significato |
|---|---|
| Libero | Nessuna allocazione |
| Prenotato online | Confermato, canale online |
| Prenotato cassa | Confermato, canale desk |
| Abbonato | Allocazione `season` |
| Abbonato assente oggi | Rilascio del giorno |
| In attesa / hold | Checkout o approvazione in corso |
| Da incassare | `paid_amount < total_amount` |
| Presente | Check-in effettuato |
| Bloccato | Blocco manuale |
| Prelazione rinnovo | Hold di rinnovo |

- Indicatori sovrapposti: icona extra, nota presente, cliente VIP.
- **Tocco su una postazione:** scheda rapida con cliente, periodo, pack, extra, saldo; azioni: nuova prenotazione, check-in, incassa, sposta, rilascia giorno, blocca, apri dettaglio.
- **Trascinamento** di una prenotazione su un'altra postazione: anteprima del conflitto, conferma, ricalcolo prezzo se cambia la categoria.
- **Selezione multipla** per prenotazioni di gruppo.
- **Ricerca** per nome cliente, telefono o codice: evidenzia le postazioni corrispondenti.
- Coda delle righe da assegnare (modalità `zone`) in un pannello laterale, da trascinare sulla mappa.
- Aggiornamento in tempo reale via SSE (sezione 21).

---

