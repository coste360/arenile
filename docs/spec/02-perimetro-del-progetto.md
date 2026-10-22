## 2. Perimetro del progetto

Il progetto include tutto ciò che serve per gestire uno stabilimento, dalla configurazione della mappa all'incasso, senza componenti a pagamento né servizi obbligatori di terze parti.

| Area | Incluso |
|---|---|
| Mappa, risorse, pack, extra | Editor, vista operativa, inventario a posti, zone e quantità |
| Motore prezzi e policy | Regole configurabili, simulatore, copia di stagione |
| Pagina pubblica e widget | Link diretto, iframe, script di incorporamento, temi |
| Pagamenti | Interfaccia `PaymentProvider`, driver manuale, driver Stripe con l'account del gestore |
| Multi-stabilimento | Più stabilimenti nella stessa installazione |
| Backup | Comando CLI, backup automatico su file |
| Dati della concessione | Interfaccia `ConcessionDataProvider` per importare perimetro e sfondo da fonti esterne (vuota di default) |
| Integrazioni | Webhook in uscita, chiavi API, punti di estensione Go |

Chi vuole estendere il progetto (nuovi provider di pagamento, driver fiscali, canali di notifica, integrazioni) lo fa tramite i punti di estensione descritti nella sezione 18, senza modificare il codice del core.

---

