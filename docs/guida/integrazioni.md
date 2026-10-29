# Integrazioni: chiavi API e webhook

Arenile può parlare con altri programmi: un gestionale, un CRM, un foglio di calcolo automatico, il sito del lido.
Ci sono due strumenti, entrambi nel pannello sotto **Configurazione** e riservati al **titolare**:

- le **chiavi API**, con cui un programma esterno *chiede* dati ad Arenile o crea prenotazioni;
- i **webhook**, con cui Arenile *avvisa* un programma esterno quando succede qualcosa (una prenotazione confermata, un
  pagamento ricevuto...).

Riferimento tecnico completo: [`api/openapi.yaml`](../../api/openapi.yaml) (gruppi *API keys* e *Webhook endpoints*),
specifica [17](../spec/17-api.md), [18](../spec/18-eventi-webhook-e-punti-di-estensione.md) e [22](../spec/22-sicurezza.md).

## Chiavi API

### Creare una chiave

1. **Configurazione → Chiavi API → Nuova chiave**.
2. Dai un nome riconoscibile (es. "Gestionale contabile") e scegli i **permessi** (scope): solo quelli che servono.
3. Facoltativo: una **scadenza**. Dopo quell'istante la chiave non funziona più (puoi prorogarla).
4. La chiave completa (`ar_live_<prefisso>_<segreto>`) compare **una sola volta**: copiala subito nel programma che la
   userà. Arenile conserva solo l'impronta SHA-256, quindi non può mostrarla di nuovo. Se la perdi, revocala e creane
   un'altra.

Il programma la invia in ogni richiesta:

```http
GET /api/admin/v1/establishments/<id>/bookings HTTP/1.1
Authorization: Bearer ar_live_abcd2345_...
```

### Permessi

Gli scope sono gli stessi permessi dei ruoli del personale (spec 16.1):

| Scope | Cosa permette |
|---|---|
| `bookings.read` | Leggere mappa, disponibilità e prenotazioni (sola lettura) |
| `bookings.write` | Creare e modificare prenotazioni |
| `operations.run` | Check-in, rilasci, blocchi delle postazioni |
| `cash.operate` | Incassi e sessioni di cassa |
| `refunds.issue` | Rimborsi |
| `reports.read` | Report ed esportazioni |
| `establishment.configure` | Configurazione dello stabilimento, prezzi, policy |
| `payments.configure` | Provider di pagamento |

Regole fisse:

- una chiave vale **solo per lo stabilimento** in cui è stata creata;
- una chiave **non può mai** gestire utenti, chiavi API o webhook (`users.manage` non è assegnabile e queste pagine
  rispondono sempre `403` a una richiesta fatta con una chiave);
- le prenotazioni e i blocchi temporanei creati con una chiave sono sempre del **canale `api`**, anche se la richiesta
  indica `desk`: consumano e rispettano la **quota online** come la pagina pubblica (ADR 0011);
- ogni chiave ha un limite di **300 richieste al minuto**; oltre, la risposta è `429` con l'header `Retry-After`;
- una scrittura che perde una gara con un'altra modifica degli stessi dati risponde `409` con tipo
  `concurrent_update` e l'header `Retry-After`: ripeti la stessa richiesta (con la stessa `Idempotency-Key`, se la
  usi) dopo `retry_after` secondi;
- la **revoca** ha effetto dalla richiesta successiva; l'ultimo utilizzo è aggiornato al massimo una volta al minuto;
- creazione, modifica e revoca finiscono nel registro attività (mai la chiave né la sua impronta).

**Perché SHA-256 e non argon2id?** Il segreto è casuale a 256 bit: nessun attacco a dizionario o forza bruta è
possibile, quindi un hash lento non aggiunge sicurezza e costerebbe CPU a ogni richiesta (un modo facile per
sovraccaricare un piccolo server). Le password, scelte da persone, restano invece con argon2id.

## Webhook in uscita

### Configurare un endpoint

1. **Configurazione → Webhook → Nuovo endpoint**.
2. Indica l'**URL** del ricevitore: `https://` obbligatorio (`http://` è ammesso solo verso `localhost` per le prove).
3. Scegli gli **eventi**: singoli tipi (`booking.confirmed`), un gruppo (`booking.*`, `payment.*`) o `*` per tutti.
   L'elenco completo è nella specifica 18.1.
4. Copia il **segreto di firma** (`whsec_...`): compare una sola volta.
5. Premi **Invia evento di prova**: Arenile manda subito un evento `webhook.test` solo a questo endpoint e ti mostra la
   risposta. Funziona anche con l'endpoint in pausa, così puoi provarlo prima di attivarlo.

### Cosa riceve il tuo server

Una richiesta `POST` con corpo JSON (l'*envelope* della specifica 18.1) e questi header:

| Header | Contenuto |
|---|---|
| `Content-Type` | `application/json` |
| `Arenile-Signature` | `t=<timestamp unix>,v1=<firma>` (due `v1` durante la rotazione del segreto) |
| `Arenile-Event-Id` | Id dell'evento, `evt_<uuid>`: uguale a ogni nuovo tentativo |
| `Arenile-Event-Type` | Tipo dell'evento, es. `booking.confirmed` |
| `User-Agent` | `Arenile-Webhooks/1` |

```json
{
  "id": "evt_01J...",
  "type": "booking.confirmed",
  "establishment_id": "01J...",
  "occurred_at": "2027-07-01T09:12:44Z",
  "data": { "...": "..." },
  "version": 1
}
```

Il contenuto di `data` è esattamente quello definito per ogni evento: Arenile non aggiunge dati personali oltre al
contratto dell'evento.

Rispondi con un codice **2xx entro 10 secondi**. Elabora il lavoro pesante dopo aver risposto. Le consegne sono
*almeno una volta*: lo stesso evento può arrivare più di una volta (dopo un errore di rete o un reinvio), quindi
**scarta i duplicati usando `Arenile-Event-Id`**.

### Verificare la firma

La firma è un HMAC-SHA256, in esadecimale, calcolato con il segreto dell'endpoint (tutta la stringa, `whsec_` incluso)
sul testo `<t>.<corpo grezzo>`. Verifica sempre che una delle firme `v1` coincida (confronto a tempo costante), che
`t` sia entro 5 minuti dall'ora attuale e di usare il **corpo grezzo**, prima di qualsiasi parsing JSON.

La guida completa, con esempi in Node.js, Python, Go e PHP, un vettore di prova e i problemi frequenti, è in
[Verificare la firma dei webhook](webhook-verifica-firma.md).

### Tentativi, errori e registro

- Una consegna fallita (codice diverso da 2xx, timeout, errore di rete) viene ritentata con **attesa esponenziale**:
  30 secondi, 1, 2, 4, 8 minuti... fino a un massimo di 6 ore tra un tentativo e l'altro, per **24 ore**. Poi la
  consegna resta **fallita** nel registro (non viene più ritentata da sola).
- I **redirect non vengono seguiti**: un `3xx` conta come errore. Correggi l'URL nel pannello.
- Dopo **20 errori consecutivi** l'endpoint viene **disattivato** automaticamente (stato *disattivato*, con il motivo);
  il fatto finisce nel registro attività e nel log del server, e il titolare riceve una **notifica nel pannello** e
  un'**email** (se la notifica al gestore per gli errori dei webhook è attiva, come da impostazione predefinita). Ogni tentativo (codice HTTP, durata, errore) resta visibile nel dettaglio della consegna. Quando il ricevitore è di nuovo a posto, riattivalo: il contatore riparte da zero.
- La **query string** dell'URL (dove spesso si mette un token del ricevitore) non compare nel registro attività né
  nell'elenco della pagina *Webhook*: lì vedi `…/arenile?…`. L'URL completo resta nel modulo di modifica. Meglio
  comunque affidarsi alla firma `Arenile-Signature` che a un token nell'URL.
- Con `ARENILE_WEBHOOKS_ALLOW_PRIVATE` spento (default in produzione) un URL verso `localhost`, `127.0.0.1` o `::1` è
  rifiutato già al salvataggio, con lo stesso messaggio degli indirizzi privati: altrimenti ogni consegna fallirebbe
  e l'endpoint verrebbe disattivato dopo 20 tentativi.
- Il **registro delle consegne** mostra per ogni evento stato, tentativi, codice HTTP, errore, il corpo inviato e
  l'inizio della risposta. Da lì puoi **reinviare** una consegna riuscita o fallita (l'endpoint deve essere attivo):
  riparte con altre 24 ore di tentativi.
- Le consegne concluse restano nel registro **30 giorni**.
- Il corpo di un evento non supera 256 KiB.

### Rotazione del segreto

**Ruota segreto** genera un nuovo segreto (mostrato una volta). Per un periodo di grazia (24 ore di default, da 0 a
168) ogni richiesta porta **due firme** `v1`, una per segreto: il ricevitore la accetta sia con il vecchio sia con il
nuovo segreto, così puoi aggiornarlo senza perdere consegne. Finito il periodo di grazia resta solo il nuovo.

### Sicurezza della rete (SSRF)

L'URL di un webhook lo sceglie chi configura lo stabilimento: per impedire che il server venga usato per raggiungere
la propria rete interna, Arenile **non consegna** a indirizzi:

- sempre bloccati: link-local (compreso `169.254.169.254`, i metadati dei cloud), multicast, broadcast, `0.0.0.0/8`,
  intervalli riservati e di documentazione;
- bloccati salvo configurazione: loopback (`127.0.0.0/8`, `::1`), reti private (`10/8`, `172.16/12`, `192.168/16`,
  `fc00::/7`), CGNAT (`100.64/10`).

Il nome dell'host viene risolto una volta e la connessione va proprio all'indirizzo controllato (nessuna seconda
risoluzione DNS che possa cambiare destinazione); un nome che risolve anche verso un solo indirizzo bloccato viene
rifiutato. I proxy delle variabili d'ambiente non sono usati.

Se i tuoi ricevitori sono sulla **stessa rete locale** del server (installazione in casa o nel lido), imposta
`ARENILE_WEBHOOKS_ALLOW_PRIVATE=true`: loopback e reti private diventano raggiungibili, link-local e metadati restano
bloccati. In modalità sviluppo (`ARENILE_DEV=true`) è attivo di default.
