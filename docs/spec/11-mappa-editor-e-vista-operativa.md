## 11. Mappa: editor e vista operativa

### 11.1 Rendering

- SVG con coordinate logiche (`maps.width × maps.height`) e `viewBox`, scalato al contenitore.
- Pan e zoom con gesti touch e rotella; pulsante "adatta alla vista".
- Fino a 1.000 elementi devono restare fluidi su un tablet di fascia media. Oltre questa soglia si usa la virtualizzazione per settori, non prevista nell'MVP.
- Ogni postazione è un `<g>` accessibile: `role="button"`, `aria-label` con etichetta e stato, focus da tastiera.
- La pagina pubblica offre sempre un'alternativa a elenco (per fila) per lettori di schermo e schermi piccoli.
- **Illustrazioni** (ADR 0012): postazioni ed elementi sono disegnati con il kit Coste360 (`assets/beach-map-kit`),
  un solo sprite `<symbol>` per pagina e un `<use>` per elemento; il disegno di una postazione è 1,3 volte la
  sua impronta, che resta l'area cliccabile. Il tema chiaro/scuro passa dalle variabili `--beach-*`.
- **Stato sovrapposto, mai nel disegno:** un anello colorato intorno al disegno (tinta forte per gli stati da
  gestire, appena accennata per i liberi) e un badge d'angolo con un glifo per ogni stato prenotato (globo
  online, banco cassa, spunta presente, stella abbonato, ecc.), così la mappa si legge anche senza colori.
- La legenda e i controlli di zoom stanno sopra il disegno; il disegno mantiene le proporzioni della mappa fino a
  un'altezza massima, così sul telefono non lascia spazio vuoto, e un avviso rimanda allo zoom o all'elenco
  quando le etichette scendono sotto i 9 px.
- Gli elementi decorativi con un nome sono esposti come immagini (`role="img"`); la bandiera dello stato del
  mare è nominata e didascalizzata con il colore (verde, gialla, rossa).

### 11.2 Editor (solo pannello)

- Griglia con snapping (`grid_size`).
- **Generatore di file:** "crea N file da M posti, distanza X, numerazione A–Z / 1–N, a partire da…", come punto di partenza.
- Trascinamento singolo e multiplo, rotazione, allineamento e distribuzione, duplicazione.
- Palette di elementi: illustrazioni del kit, con ricerca e categorie, per postazioni ed elementi decorativi
  (più il testo libero).
- Pannello proprietà: etichetta, zona, tipo, illustrazione, capacità, attributi, categoria di prezzo, stato
  attivo; per gli elementi: tipo, illustrazione, colore della bandiera, testo, dimensioni.
- **Tipo e illustrazione:** il tipo è semantico (pack `requires_spot_kind`, assegnazione), l'illustrazione lo
  disegna. Scegliere un'illustrazione di un'altra famiglia cambia il tipo e il pannello lo dice; cambiare il tipo
  riporta un'illustrazione di un altro tipo a quella predefinita. Un elemento portato a "Testo" perde
  l'illustrazione (anche il backend la rifiuta).
- **Passerelle modulari:** tessere di 60 unità (il viewBox del kit a scala 0,5; le assi disegnate sono più
  strette del lato), tratti dritti lunghi un numero intero di tessere; si agganciano alla griglia delle tessere
  solo quando il passo della griglia divide 60, altrimenti tengono la posizione.
- **Lato del mare** (`maps.sea_side`: `top`, `bottom`, `left`, `right`): sui lidi italiani il mare è di solito in
  alto, quindi `top` è il predefinito delle nuove mappe, del generatore di file, del modello classico e della demo,
  con la fila A più vicina al mare. Se cambia il lato del mare di una mappa con elementi, l'editor propone
  "Ribalta la disposizione" oppure "Solo il mare". Ribaltare (`POST /maps/{id}/reorient`, `If-Match`) sposta
  postazioni, elementi e passerelle così che ogni fila resti alla stessa distanza dal mare: lati opposti a
  specchio (l'ordine lungo la riva non cambia), lati adiacenti con una rotazione di 90° e scambio di larghezza e
  altezza; le tessere delle passerelle restano sulla loro griglia. Solo geometria (id, etichette, file, zone e
  allocazioni invariati), in un unico salvataggio versionato; un'anteprima conta le allocazioni future, che
  vanno confermate esplicitamente. Con modifiche non salvate si può spostare solo il mare.
- Rinumerazione automatica di una fila selezionata.
- Annulla e ripeti (stack di comandi lato client), con salvataggio esplicito.
- **Salvataggio transazionale con controllo di versione:** `PUT /maps/{id}` con `If-Match` sulla versione; in caso di conflitto viene mostrata la differenza.
- Non è possibile eliminare una postazione con allocazioni future: si può solo disattivarla (`active = false`), e il sistema elenca le prenotazioni da spostare.
- Sfondo opzionale: immagine caricata dal gestore oppure perimetro della concessione fornito da `ConcessionDataProvider`, con scala e rotazione regolabili.

### 11.3 Vista operativa

- Selettore della data sempre visibile, con frecce giorno precedente e successivo e "oggi".
- Selettore della fascia: giorno intero, mattina, pomeriggio.
- **Colori per stato** (personalizzabili nel tema del pannello, con legenda sempre visibile sopra la mappa) e
  un glifo per ogni stato prenotato, così lo stato non dipende dal solo colore:

| Stato | Significato | Glifo |
|---|---|---|
| Libero | Nessuna allocazione | nessuno (alone appena accennato) |
| Prenotato online | Confermato, canale online | globo |
| Prenotato cassa | Confermato, canale desk | banco |
| Abbonato | Allocazione `season` | stella |
| Abbonato assente oggi | Rilascio del giorno | freccia in uscita |
| In attesa / hold | Checkout o approvazione in corso | orologio |
| Da incassare | `paid_amount < total_amount` | euro |
| Presente | Check-in effettuato | spunta |
| Bloccato | Blocco manuale | croce |
| Prelazione rinnovo | Hold di rinnovo | rinnovo |

- Indicatori sovrapposti: icona extra, nota presente, cliente VIP.
- **Tocco su una postazione:** scheda rapida con cliente, periodo, pack, extra, saldo; azioni: nuova prenotazione, check-in, incassa, sposta, rilascia giorno, blocca, apri dettaglio.
- **Trascinamento** di una prenotazione su un'altra postazione: anteprima del conflitto, conferma, ricalcolo prezzo se cambia la categoria.
- **Selezione multipla** per prenotazioni di gruppo.
- **Ricerca** per nome cliente, telefono o codice: evidenzia le postazioni corrispondenti.
- Coda delle righe da assegnare (modalità `zone`) in un pannello laterale, da trascinare sulla mappa.
- Aggiornamento in tempo reale via SSE (sezione 21).

---

