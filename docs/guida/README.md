# Guida per gestori di stabilimenti balneari

Questa guida è pensata per chi gestisce un lido, una spiaggia attrezzata o uno stabilimento balneare e vuole installare e usare **Arenile**, il gestionale gratuito e a codice sorgente disponibile per la gestione della spiaggia, delle prenotazioni di ombrelloni e lettini e della cassa.

> [!IMPORTANT]
> Arenile è in sviluppo attivo (milestone M0, fondamenta) e non è ancora pronto per la produzione. Queste guide descrivono il funzionamento previsto e verranno aggiornate a ogni rilascio. Lo stato delle funzionalità è nella [tabella del README](../../README.md#stato-delle-funzionalità).

## Indice

| Guida | Contenuto |
|---|---|
| [Installazione](installazione.md) | Docker, VPS, Raspberry Pi (arm64), Coolify e CapRover, HTTPS, backup e ripristino, aggiornamento |
| [Privacy e GDPR](privacy-gdpr.md) | Ruoli titolare/responsabile, dati raccolti, consensi, diritti degli interessati, modello di informativa |
| [Nota fiscale](fiscale.md) | Cosa fa e cosa non fa Arenile su corrispettivi, documento commerciale e registratore telematico |
| [Integrazioni](integrazioni.md) | Chiavi API per i gestionali esterni, webhook in uscita, verifica della firma, sicurezza della rete |
| [Verifica della firma dei webhook](webhook-verifica-firma.md) | Formato di `Arenile-Signature`, controlli da fare, esempi in Node.js, Python, Go e PHP, vettore di prova |
| [Pannello offline](pannello-offline.md) | Installare il pannello come app, cosa si può fare senza connessione, dati salvati sul dispositivo |

## In breve

- **Gratuito e senza commissioni**: Arenile non trattiene percentuali sulle prenotazioni. I pagamenti online passano dal tuo account Stripe; paghi solo le commissioni del circuito di pagamento.
- **I dati restano tuoi**: il programma gira su un server scelto da te (un VPS, un mini PC, un Raspberry Pi) e puoi esportare tutto in qualsiasi momento.
- **Prenotazioni dal tuo sito**: la pagina di prenotazione si incorpora nel sito del lido con un iframe o con lo script `embed.js` (vedi il [README](../../README.md#incorporare-il-widget-nel-tuo-sito)).
- **Tu resti responsabile** degli aspetti fiscali e privacy della tua attività: Arenile ti fornisce gli strumenti, ma la configurazione va verificata con il tuo commercialista e, se necessario, con un consulente privacy.

## Serve aiuto?

- Domande sull'uso: [Discussions](https://github.com/coste360/arenile/discussions).
- Problemi e malfunzionamenti: apri una [issue](https://github.com/coste360/arenile/issues/new/choose).
- Problemi di sicurezza: segui [SECURITY.md](../../SECURITY.md).
