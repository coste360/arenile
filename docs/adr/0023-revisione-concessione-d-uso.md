# 0023 — Revisione legale della concessione d'uso (Additional Use Grant)

- **Stato:** accettato
- **Data:** 2026-10-24
- **Sezioni della specifica:** `docs/spec/26-licenza-e-governance-del-progetto.md`; integra
  [ADR 0007](0007-licenza-bsl.md)

## Contesto

L'ADR 0007 ha scelto la BSL 1.1 con conversione in AGPL-3.0 dopo tre anni e una concessione d'uso gratuita per gli
stabilimenti gestiti direttamente. Il testo della concessione era da validare con un legale. Restavano casi pratici
ambigui: consorzi e associazioni di lidi, agenzie web che inseriscono il widget, modelli di installazione "in un
clic", aiuto gratuito legato a offerte a pagamento, e la clausola sui prodotti concorrenti, che rinviava ai prodotti
di Coste360 alla data di rilascio, un riferimento che un terzo non può verificare.

## Decisione

Adottiamo il testo della concessione d'uso rivisto dal legale (file `LICENSE`). Rispetto alla versione precedente:

- Il divieto riguarda due casi distinti: (i) servizi a terzi relativi ad Arenile (hosting, SaaS, installazione,
  configurazione, personalizzazione, manutenzione, assistenza, rivendita) e (ii) prodotti o servizi per terzi che
  sostituiscono Arenile o offrono funzioni sostanzialmente simili, come gestionali o sistemi di prenotazione per
  stabilimenti balneari. Il criterio è oggettivo e non dipende più dal catalogo di Coste360.
- Offrire ai propri clienti i servizi dello stabilimento non è un servizio relativo ad Arenile.
- Inserire o collegare la pagina di prenotazione o il widget non è un servizio relativo ad Arenile, se chi lo fa non
  installa, ospita, configura, personalizza, mantiene o assiste Arenile.
- "Stabilimento" comprende le piscine all'aperto e i servizi accessori dello stesso sito, non le camere d'albergo;
  un hotel può usarlo per il lido che gestisce direttamente.
- Il controllo è definito (oltre il 50% dei diritti di voto o il potere di dirigere la gestione). Un consorzio o
  un'associazione che non controlla i soci non gestisce direttamente i loro stabilimenti.
- L'aiuto gratuito è permesso, salvo che faccia parte di un'offerta a pagamento, o serva a promuoverla, legata ad
  Arenile o alla gestione, alle prenotazioni o al sito dello stabilimento.
- Un modello di installazione generico o "in un clic" è permesso se il fornitore non gestisce Arenile per il gestore e
  non vende un'offerta dedicata ad Arenile.

Non cambiano i parametri della BSL: licenziante, Change Date di tre anni per versione, Change License AGPL-3.0 o
successiva.

## Conseguenze

- Positive: casi frequenti (agenzie, consorzi, hosting in un clic, hotel) hanno una risposta scritta; la clausola sui
  concorrenti è verificabile da chiunque.
- Negative: le agenzie che aiutano gratis a installare Arenile come parte di un'offerta per il sito del lido hanno
  bisogno di una licenza commerciale; le piscine al chiuso non rientrano nella concessione gratuita.
- Verifiche: README (sezione Licenza e FAQ), la pagina Licenza della documentazione (it/en) e `CONTRIBUTING.md`
  riassumono il testo e rimandano al file `LICENSE` come testo vincolante. Il CLA resta da validare.
