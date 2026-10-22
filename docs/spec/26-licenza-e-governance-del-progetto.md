## 26. Licenza e governance del progetto

- Licenza del core: **Business Source License 1.1** (SPDX `BUSL-1.1`, modello *source available*), decisione in [ADR 0007](../adr/0007-licenza-bsl.md). Parametri: licenziante Coste360 SRL; *Additional Use Grant* che consente l'uso in produzione gratuito per gestire gli stabilimenti balneari (lidi, beach club, piscine) posseduti o gestiti direttamente, inclusa la pagina di prenotazione per i propri clienti, oltre a valutazione, sviluppo, test e contributi; *Change Date* tre anni dopo il primo rilascio pubblico di ciascuna versione; *Change License* **AGPL-3.0 o successiva**.
- Non consentito senza licenza commerciale di Coste360 SRL: offrire Arenile o suoi derivati a terzi a pagamento o con altro vantaggio commerciale (hosting, servizio gestito o SaaS, installazione, configurazione, personalizzazione, manutenzione, assistenza, rivendita) o in un prodotto concorrente.
- **CLA** obbligatorio per i contributi, firmato tramite bot (CLA Assistant) alla prima pull request: consente a Coste360 SRL, titolare del progetto, di distribuire i contributi con la BSL 1.1, con licenze commerciali e con la AGPL-3.0, e di modificare la licenza nel tempo. Testo della licenza e della clausola d'uso da validare con un legale prima dell'apertura del repository.
- Marchio: il nome e il logo del progetto vanno registrati da Coste360 SRL e restano riservati; `TRADEMARK.md` chiarisce che i fork, anche delle versioni convertite in AGPL-3.0, devono cambiare nome.
- `.github/CODEOWNERS` assegna ai maintainer la revisione obbligatoria di licenza, CLA e aree critiche del dominio.
- File di progetto: `README.md` (installazione in 5 minuti), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `docs/` con guide per gestori e per sviluppatori.
- Issue template per bug, richieste di funzionalità e nuovi driver (pagamento, fiscale, notifiche).
- Demo pubblica con dati di esempio, azzerata ogni notte.

---

