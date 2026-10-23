# Report: come leggere i numeri

La pagina **Report** del pannello (specifica 15.3) mostra il riepilogo del periodo, il confronto con un anno
precedente e i report di dettaglio, ognuno esportabile in CSV (formato italiano: `;`, virgola decimale, date
`gg/mm/aaaa`). Questa pagina spiega **a quale data** è attribuito ogni numero, perché è la domanda che fa la differenza
quando si confrontano due schede o un report con la contabilità.

## Periodo

Si sceglie una **stagione** (il default: quella in corso, altrimenti l'ultima aperta), un **mese** o un intervallo
libero di al massimo 400 giorni. Tutte le date sono nel fuso orario dello stabilimento.

## Riepilogo e confronto anni: due sole basi di data

Le schede del riepilogo e gli indicatori del confronto usano due sole basi, stampate sotto ogni scheda, così numeri
affiancati non si contraddicono mai:

| Base | Schede | Regola |
|---|---|---|
| **Per primo giorno di soggiorno** | Prenotazioni, Valore medio, Prenotato, Abbonamenti venduti, Cancellazioni, No-show | Conta la prenotazione nel periodo in cui **inizia il soggiorno** (`starts_on`), qualunque sia il giorno in cui è stata venduta: un lido vende la stagione mesi prima, e il report della stagione descrive la stagione venduta. Le cancellazioni e i no-show stanno quindi nello stesso periodo delle prenotazioni che li contengono. |
| **Per giorno di soggiorno** | Occupazione | Unità (postazione, giorno, fascia) occupate sui mezzi giorni aperti diviso le unità disponibili. |
| **Per data di incasso** | Incassato | Pagamenti riusciti per data contabile (`business_date`), rimborsi negativi: è la base di cassa, la stessa dei corrispettivi. |

Dettagli:

- **Prenotazioni** conta le prenotazioni vendute (tutti gli stati tranne bozza, in attesa, scaduta, rifiutata),
  comprese quelle poi cancellate o no-show: è lo stesso denominatore del report *Cancellazioni e no-show*.
- **Prenotato** somma il totale delle prenotazioni in stato prenotato (confermata, presente, conclusa, no-show).
- **Saldi aperti** sono le prenotazioni in stato prenotato con primo giorno di soggiorno nel periodo e saldo da
  incassare: non hanno una base temporale propria.

## Report di dettaglio: basi dichiarate

Alcuni report rispondono a domande diverse e usano **una base diversa**, scritta nel loro titolo o nel selettore:

- **Prenotazioni per canale, anticipo e formula**: per **data di vendita** (creazione), perché misura come e quando si
  vende; l'anticipo è la distanza tra vendita e primo giorno di soggiorno.
- **Ricavi**, base *venduto* (`booked`): per **data di conferma**; base *incassato*: per data di incasso come il
  riepilogo.
- **Cancellazioni e no-show**: per primo giorno di soggiorno, come il riepilogo.
- **Abbonamenti e rilasci**: abbonamenti per data di conferma, rilasci per data del rilascio.

Per questo il numero di prenotazioni del report di dettaglio può differire da quello del riepilogo sullo stesso
periodo: il primo dice *quante ne hai vendute in quei giorni*, il secondo *quante iniziano in quei giorni*.

## Confronto con un anno precedente

Il confronto mette a fianco lo stesso intervallo di calendario dell'anno scelto (default: l'anno precedente; l'anno
di confronto deve precedere quello del periodo). Con *Stesse date del calendario* ogni giorno dell'anno precedente è
sovrapposto allo stesso giorno e mese di quello corrente (un 29 febbraio precedente si somma al 28 febbraio); con
*Stesso giorno della settimana* il periodo precedente ha la stessa durata e inizia nel giorno della settimana uguale
più vicino, così i sabati si confrontano con i sabati. La variazione percentuale manca quando il valore precedente è
zero.

## Riconciliazione con i corrispettivi

L'**incassato** del report coincide al centesimo con il totale dell'export dei corrispettivi dello stesso periodo. Per
imponibile e imposta per aliquota il riferimento è l'**export giornaliero** dei corrispettivi: il report scorpora
sempre per giorno, aliquota e metodo; l'export raggruppato per mese scorpora per mese e può differire di qualche
centesimo (il lordo coincide sempre). Vedi anche la [nota fiscale](fiscale.md).

## Cosa manca ancora

Le esportazioni di **elenco** (prenotazioni, clienti, pagamenti, corrispettivi) e il formato **XLSX** previsti dalla
specifica 15.3 arrivano con M5 parte B; le prenotazioni di prova (15.4) saranno escluse quando esisterà la modalità
prova.
