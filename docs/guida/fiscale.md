# Nota fiscale

Questa pagina chiarisce cosa fa e cosa non fa Arenile rispetto agli adempimenti fiscali di uno stabilimento balneare: corrispettivi, documento commerciale, registratore telematico, IVA.

> [!CAUTION]
> **La conformità normativa della configurazione resta responsabilità del gestore e del suo consulente** (commercialista o consulente fiscale). Arenile è uno strumento gestionale: non fornisce consulenza fiscale e non garantisce che una determinata configurazione sia conforme alla normativa applicabile alla tua attività. Le informazioni di questa pagina descrivono il funzionamento del software, non gli obblighi di legge.

## In sintesi

| Domanda | Risposta |
|---|---|
| Arenile è un registratore telematico? | **No.** Non memorizza né trasmette i corrispettivi all'Agenzia delle Entrate. |
| Arenile emette il documento commerciale? | **Non nell'MVP.** Il documento commerciale si emette con il tuo registratore telematico (o con la procedura che usi già). |
| Le ricevute di Arenile hanno valore fiscale? | **No.** Ricevute, voucher e report di cassa sono documenti **non fiscali**. |
| Arenile aiuta con i corrispettivi? | **Sì.** Il driver `export` produce il riepilogo giornaliero per aliquota e metodo di pagamento in CSV e PDF per il commercialista. |
| Le aliquote IVA sono predefinite? | **No.** Le imposta il gestore, per stabilimento, pack ed extra. Il codice non contiene aliquote fisse. |

## Come funziona il modulo fiscale

Il modulo fiscale è progettato con un'interfaccia a driver (`FiscalDriver`), così da poter supportare in futuro diversi dispositivi senza modificare il resto del programma.

| Driver | Comportamento | Stato |
|---|---|---|
| `none` | Nessuna emissione: il gestore usa il proprio registratore telematico separatamente. È il **default**. | Pianificato (M3) |
| `export` | Esportazione giornaliera dei corrispettivi per aliquota IVA e metodo di pagamento, in CSV e PDF, da consegnare al commercialista. | Pianificato (M3) |
| Registratori telematici | Un driver per ciascuna famiglia di protocollo, che invia al dispositivo le righe da emettere. | Dopo l'MVP, come contributi della community |

Ogni emissione o errore di un driver viene tracciato. Le righe del documento derivano dallo **snapshot del prezzo** salvato con la prenotazione, con lo scorporo dell'IVA per aliquota.

## Prezzi e IVA

- I prezzi configurati in Arenile sono **IVA inclusa**, come avviene normalmente per i servizi al consumatore finale.
- Ogni pack ed extra ha la propria aliquota, con un valore predefinito impostato a livello di stabilimento.
- Lo scorporo dell'IVA è calcolato solo per documenti e report, con aritmetica intera in centesimi e arrotondamento al centesimo.
- Se l'aliquota corretta per un servizio (ad esempio noleggio di attrezzature, servizi accessori, bar) non ti è chiara, chiedi al tuo commercialista prima di configurare il listino.

## Cassa

La cassa di Arenile registra aperture, incassi per metodo (contanti, carta, bonifico, pagamenti online, crediti), chiusure con conteggio e differenza, e produce un report di chiusura in PDF. Questi dati servono al controllo di gestione e alla riconciliazione con il registratore e con gli estratti del POS o di Stripe; **non sostituiscono** gli adempimenti fiscali.

## Pagamenti online

Con il driver Stripe gli incassi online arrivano direttamente sul conto Stripe dello stabilimento. Arenile non trattiene commissioni; Stripe applica le proprie commissioni di circuito, che compaiono nei suoi report. Il trattamento fiscale e contabile di questi incassi (inclusi acconti e rimborsi) va concordato con il commercialista.

## Contribuire con un driver fiscale

Se conosci il protocollo di una famiglia di registratori telematici e vuoi contribuire un driver, apri una issue con il modello [Nuovo driver](https://github.com/coste360/arenile/issues/new?template=driver.yml) indicando dispositivi supportati, documentazione tecnica e modalità di test senza un dispositivo reale.

## Riferimenti

- Sezione 14 della [specifica tecnica](../spec/14-cassa-e-modulo-fiscale.md) (Cassa e modulo fiscale).
- Per gli obblighi di legge rivolgiti al tuo commercialista e consulta il sito dell'Agenzia delle Entrate: https://www.agenziaentrate.gov.it
