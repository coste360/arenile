# 0007 — Licenza del core: Business Source License 1.1 con conversione in AGPL-3.0 dopo tre anni

- **Stato:** accettato
- **Data:** 2026-10-22
- **Sezioni della specifica:** `docs/spec/26-licenza-e-governance-del-progetto.md`,
  `docs/spec/29-edizione-desktop-e-app-mobile.md`, `docs/spec/04-struttura-del-repository.md`

## Contesto

Arenile nasce per essere **gratuito per chi gestisce uno stabilimento balneare**: niente canoni, niente
versioni "Pro", niente commissioni sulle prenotazioni. La prima scelta della specifica era la AGPL-3.0.

La AGPL-3.0 però consente a chiunque di prendere il software e **venderlo come servizio** agli stessi
stabilimenti a cui Coste360 lo dà gratis: agenzie web, sistemisti o rivenditori potrebbero offrire
hosting, installazione, personalizzazione e assistenza a pagamento, o un "Arenile Cloud" in concorrenza,
rispettando la licenza semplicemente pubblicando il codice modificato. L'obiettivo del titolare è l'opposto:
dare il gestionale gratis alle persone, senza che intermediari si inseriscano tra Coste360 e i gestori.

Vincoli:

- il codice deve restare leggibile, modificabile e aperto ai contributi (pull request sotto CLA);
- il gestore deve poter installare Arenile da solo (o tramite i propri dipendenti) in produzione, gratis,
  anche su un server noleggiato, e pubblicare la pagina di prenotazione per i propri clienti;
- chi offre Arenile a terzi a pagamento (hosting, SaaS, installazione, configurazione, personalizzazione,
  manutenzione, assistenza, rivendita) o in un prodotto concorrente deve avere una licenza commerciale;
- il progetto deve restare credibile per la community: il codice deve diventare open source nel tempo.

Alternative considerate:

1. **AGPL-3.0 (licenza precedente).** Open source approvata OSI, massima fiducia della community. Non impedisce
   la rivendita come servizio né l'installazione a pagamento da parte di agenzie: obbliga solo a pubblicare
   le modifiche. Non soddisfa l'obiettivo del titolare.
2. **Functional Source License (FSL-1.1).** Semplice e con conversione automatica dopo due anni (in Apache-2.0
   o MIT). Vieta solo l'uso "concorrente" (un prodotto o servizio che sostituisce quello del licenziante):
   installazione e assistenza a pagamento da parte di agenzie resterebbero ambigue o consentite, e i
   parametri non sono personalizzabili. La conversione in una licenza permissiva consentirebbe inoltre fork
   chiusi.
3. **Elastic License 2.0 (ELv2).** Vieta l'offerta come servizio gestito, ma non prevede alcuna conversione
   in open source, non copre in modo chiaro installazione e assistenza a pagamento e non permette di
   definire un uso consentito su misura per gli stabilimenti.
4. **Business Source License 1.1 (BSL 1.1, SPDX `BUSL-1.1`).** Testo standard (MariaDB, usato da Sentry,
   CockroachDB, HashiCorp) con parametri personalizzabili: *Additional Use Grant*, *Change Date* e
   *Change License*. Permette di descrivere con precisione l'uso gratuito consentito e di garantire la
   conversione in AGPL-3.0.

## Decisione

Distribuiamo il core di Arenile con la **Business Source License 1.1**, testo ufficiale invariato, con questi
parametri (file `LICENSE`):

- **Licensor:** Coste360 SRL. **Licensed Work:** Arenile, (c) 2026 Coste360 SRL.
- **Additional Use Grant:** uso in produzione consentito per gestire gli stabilimenti balneari (lidi, beach
  club, spiagge attrezzate, piscine) che l'utente possiede o gestisce direttamente, inclusa la pagina di
  prenotazione e il widget per i propri clienti, oltre a valutazione, sviluppo, test e preparazione di
  contributi. Non è consentito, senza licenza commerciale di Coste360 SRL, usare Arenile o suoi derivati per
  fornire servizi a terzi a pagamento o con altro vantaggio commerciale (hosting, servizi gestiti o SaaS,
  installazione, configurazione, personalizzazione, manutenzione, assistenza, rivendita) né offrirlo in un
  prodotto o servizio concorrente con quelli di Coste360.
- **Change Date:** tre anni dalla data del primo rilascio pubblico di ciascuna versione.
- **Change License:** GNU AGPL v3.0 o successiva.

I contributi sono accettati con il [CLA](../../CLA.md), che consente a Coste360 di distribuirli con la BSL 1.1,
con licenze commerciali e con la Change License, e di modificare la licenza in futuro. Nome e logo restano
riservati ([TRADEMARK.md](../../TRADEMARK.md)). L'app mobile (sezione 29) usa la stessa licenza salvo diversa
decisione. Nella comunicazione non definiamo Arenile "open source": usiamo "codice sorgente disponibile",
"source available", "gratuito per gli stabilimenti" e "diventa AGPL-3.0 dopo 3 anni".

Il testo della licenza e della clausola d'uso va validato con un legale prima dell'apertura del repository.

## Conseguenze

- Positive: il gestore usa Arenile gratis e senza limiti; agenzie e rivenditori non possono offrirlo a
  pagamento senza accordo con Coste360, che può concedere licenze commerciali (fonte di sostenibilità del
  progetto); ogni versione diventa comunque AGPL-3.0 dopo tre anni, quindi il codice non resta chiuso per
  sempre; il testo standard della BSL è noto e riconosciuto (SPDX `BUSL-1.1`).
- Negative: Arenile non è open source secondo la definizione OSI finché una versione non si converte; alcune
  distribuzioni, cataloghi e contributori evitano software BSL; i concetti di "gestito direttamente" e di
  "servizio a terzi" possono generare casi limite da chiarire caso per caso (FAQ nel README); le dipendenze
  devono restare compatibili sia con la BSL 1.1 sia con la AGPL-3.0; serve un CLA firmato per ogni
  contributore.
- Verifiche: `.github/CODEOWNERS` richiede la revisione dei maintainer per `LICENSE`, `CLA.md` e le aree
  critiche; i metadati di licenza (etichetta OCI `org.opencontainers.image.licenses` in `deploy/Dockerfile`
  e nel workflow di release, `info.license` in `api/openapi.yaml`) devono indicare `BUSL-1.1`; la revisione
  delle pull request controlla che nuovi testi non descrivano Arenile come "open source".
