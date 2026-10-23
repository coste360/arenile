## 5. Modello di dominio

### 5.1 Glossario

| Termine | Significato |
|---|---|
| **Organizzazione** | Soggetto titolare (la società che gestisce). Può avere più stabilimenti. |
| **Stabilimento** | Un lido. Ha fuso orario, stagioni, mappe, catalogo, listini, policy. |
| **Stagione** | Periodo di apertura annuale (es. 2027-05-01 → 2027-09-30). |
| **Periodo tariffario** | Intervallo di date con nome (bassa, media, alta, Ferragosto) usato dalle regole di prezzo. |
| **Mappa** | Rappresentazione grafica di un'area (spiaggia, piscina). |
| **Zona** | Raggruppamento logico di postazioni (Prima fila, Zona B, Piscina). |
| **Postazione** | Elemento prenotabile con posizione: ombrellone, gazebo, tenda, palma, lettino, daybed, cabana, amaca (`kind`, semantico). Il disegno è un'illustrazione del kit della mappa (`asset`, ADR 0012), predefinita per tipo. |
| **Pool** | Inventario a quantità senza posizione: "lettini sfusi", "ombrelloni zona libera". |
| **Elemento decorativo** | Elemento non prenotabile della mappa: passerella, docce, torretta, bar, oppure una qualsiasi illustrazione del kit (bandiera, WC, DAE, palme…) come `custom` con `asset`. |
| **Componente** | Unità di inventario consumata da un pack: ombrellone, lettino, sdraio, cabina. |
| **Pack** | Prodotto vendibile composto da componenti (1 ombrellone + 2 lettini). |
| **Extra** | Aggiunta opzionale a un pack (lettino aggiuntivo, cabina, telo, parcheggio). |
| **Fascia** | Unità temporale minima: `AM` (mattina) e `PM` (pomeriggio). Il giorno intero è `AM+PM`. |
| **Formula** | Tipo di durata: mezza giornata, giorno, multi-giorno, settimana, quindicina, mese, stagione. |
| **Prenotazione** | Acquisto di uno o più pack su un intervallo di date, con extra, cliente e pagamenti. |
| **Riga di prenotazione** | Un pack prenotato con la sua assegnazione (postazione, zona o pool). |
| **Allocazione** | Occupazione di una risorsa per data e fascia. È ciò che garantisce l'assenza di doppie prenotazioni. |
| **Abbonamento stagionale** | Prenotazione con formula stagione, con diritto di rinnovo l'anno successivo. |
| **Rilascio** | Giorno in cui un abbonato libera il proprio posto, rivendibile. |
| **Canale** | Origine della prenotazione: `desk` (cassa), `online`, `api`. |

### 5.2 Modalità di assegnazione

Ogni zona ha una `assignment_mode`:

- `seat` — il cliente sceglie la postazione esatta sulla mappa.
- `zone` — il cliente sceglie la zona; la postazione viene assegnata dal gestore o automaticamente (sezione 7.5).
- `quantity` — il cliente indica solo la quantità; il consumo avviene su un pool.

Uno stabilimento può avere zone con modalità diverse. La pagina pubblica si adatta automaticamente: mostra la mappa solo se almeno una zona visibile online è `seat`.

---

