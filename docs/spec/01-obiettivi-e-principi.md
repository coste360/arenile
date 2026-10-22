## 1. Obiettivi e principi

**Obiettivi**
- Sostituire quaderno, Excel e gestionali chiusi con uno strumento gratuito, moderno e installabile in pochi minuti.
- Offrire una pagina di prenotazione online incorporabile in qualsiasi sito con un iframe.
- Rendere il motore di prenotazione e dei prezzi interamente configurabile, senza modificare codice.

**Principi di progetto**
- **Un binario, un'immagine, un volume.** Nessun servizio esterno obbligatorio (niente Redis, niente code esterne, niente object storage).
- **Configurazione come dati.** Ogni differenza tra stabilimenti si esprime con risorse, pack, regole di prezzo e policy, mai con condizioni nel codice.
- **Valori predefiniti sensati.** Ogni stabilimento parte da un modello funzionante; le opzioni avanzate restano nascoste finché non servono.
- **API-first.** Pannello e pagina pubblica usano esclusivamente le API documentate.
- **Correttezza prima di tutto sulla disponibilità.** Una doppia prenotazione è un difetto bloccante.
- **Dati del gestore.** Esportazione completa sempre disponibile.

---

