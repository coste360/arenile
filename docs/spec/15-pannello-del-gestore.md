## 15. Pannello del gestore

### 15.1 Navigazione

1. **Oggi** — mappa operativa del giorno, arrivi, saldi da incassare, righe da assegnare, meteo opzionale.
2. **Mappa** — vista operativa per qualsiasi data.
3. **Prenotazioni** — elenco filtrabile (date, stato, canale, zona, saldo), dettaglio, creazione.
4. **Calendario** — vista a griglia postazioni × giorni (planning), utile per collocare soggiorni lunghi.
5. **Clienti** — anagrafica, storico, crediti, abbonamenti.
6. **Abbonamenti** — elenco stagionali, rilasci, rinnovi.
7. **Cassa** — sessione corrente, incassi, chiusure.
8. **Report** — occupazione, ricavi, canali, confronto con l'anno precedente, esportazioni.
9. **Configurazione** — stabilimento, stagioni e periodi, mappe, zone, catalogo, prezzi, policy, pagina pubblica, pagamenti, notifiche, utenti, integrazioni, webhook.

### 15.2 Configurazione guidata

Al primo avvio:
1. dati dello stabilimento e lingue;
2. scelta di un modello: "Lido classico a file", "Spiaggia attrezzata a quantità", "Lido con piscina", "Vuoto";
3. generatore delle file sulla mappa;
4. listino rapido: prezzo giornaliero per fila e periodo, con generazione automatica delle regole;
5. pagina pubblica: attivazione, tema, codice di incorporamento;
6. pagamenti: manuale o Stripe.

Ogni passo è saltabile e modificabile in seguito.

**Ricomincia da zero** (solo il titolare, con il nome dello stabilimento scritto per conferma, irreversibile):
- *Ricomincia la configurazione* cancella stagioni, periodi, chiusure, mappe, postazioni, zone, pool, catalogo,
  prezzi, codici sconto, convenzioni, policy e biglietti d'ingresso; è possibile solo finché lo stabilimento non
  ha dati operativi (prenotazioni, abbonamenti, incassi, sessioni di cassa, biglietti, lista d'attesa, rinnovi,
  crediti);
- *Azzera lo stabilimento* cancella anche i dati operativi e i clienti, con una seconda conferma.

Restano sempre profilo dello stabilimento, utenti, pagamenti online, email, dispositivi, casse, stampanti e
registro attività (`POST /api/admin/v1/establishments/{id}/setup/reset`).

### 15.3 Report (MVP)

- Occupazione per giorno e per zona (percentuale di fasce occupate).
- Ricavi per periodo, per canale, per pack, per metodo di pagamento.
- Saldi aperti.
- Confronto stagione su stagione.
- Esportazioni CSV e XLSX di prenotazioni, clienti, pagamenti, corrispettivi.

### 15.4 Modalità prova

Interruttore nel pannello: le prenotazioni create sono marcate `test = true`, escluse da report ed esportazioni, eliminabili in blocco. Con Stripe, usa automaticamente le chiavi di test se configurate.

---

