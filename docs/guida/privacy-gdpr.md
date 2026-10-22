# Privacy e GDPR

Questa guida aiuta il gestore dello stabilimento balneare a usare Arenile nel rispetto del Regolamento (UE) 2016/679 (GDPR) e del Codice in materia di protezione dei dati personali (D.Lgs. 196/2003 come modificato dal D.Lgs. 101/2018).

> [!CAUTION]
> Questa guida ha valore **informativo** e non costituisce consulenza legale. Il modello di informativa va adattato alla tua attività e, se possibile, verificato con un consulente privacy o con il tuo DPO. La conformità del trattamento resta responsabilità del titolare.

## Chi fa cosa: i ruoli

| Soggetto | Ruolo GDPR | Note |
|---|---|---|
| **Gestore dello stabilimento** (la società o ditta che gestisce il lido) | **Titolare del trattamento** | Decide finalità e mezzi: raccoglie le prenotazioni, incassa, invia le comunicazioni. Risponde agli interessati. |
| **Chi ospita il servizio** (provider del VPS, fornitore di hosting gestito, informatico che amministra il server per conto del gestore) | **Responsabile del trattamento** (art. 28) | Serve un contratto o atto di nomina (DPA). Se installi Arenile su un server tuo e lo gestisci tu, il provider dell'infrastruttura è di norma responsabile per l'hosting. |
| **Stripe** (se attivi i pagamenti online) | Titolare autonomo o responsabile, secondo i termini di Stripe | Verifica l'accordo sul trattamento dei dati nel tuo account Stripe. |
| **Provider SMTP** (se invii email) | Responsabile del trattamento | Verifica il DPA del fornitore. |
| **Coste360 SRL** | Nessun ruolo, nell'installazione self-hosted | Coste360 sviluppa il software ma **non riceve né tratta** i dati delle tue installazioni. La telemetria è disattivata di default (`ARENILE_TELEMETRY=false`). |

Se un fornitore offre Arenile come servizio ospitato, quel fornitore è responsabile del trattamento per tuo conto e deve sottoscrivere con te un accordo ex art. 28.

## Quali dati tratta Arenile

| Categoria | Dati | Finalità | Base giuridica (indicativa) |
|---|---|---|---|
| Prenotazione online | Nome, cognome, email o telefono | Gestire la prenotazione, inviare conferma e link di gestione | Esecuzione del contratto (art. 6.1.b) |
| Clienti al desk e abbonati | Come sopra, più eventuali note e storico | Gestione del rapporto, abbonamenti stagionali e rinnovi | Esecuzione del contratto |
| Pagamenti | Importi, metodo, riferimenti della transazione (i dati della carta restano a Stripe) | Incasso, rimborsi, contabilità | Contratto e obbligo legale (art. 6.1.c) |
| Documenti contabili | Prenotazioni e pagamenti | Obblighi fiscali e di conservazione | Obbligo legale |
| Marketing | Consenso separato e facoltativo, con data, fonte e versione del testo | Newsletter e offerte | Consenso (art. 6.1.a) |
| Utenti del pannello | Email, ruolo, accessi, audit log | Sicurezza e tracciabilità delle operazioni | Legittimo interesse (art. 6.1.f) |
| Dati tecnici | IP per la limitazione delle richieste, log senza email e telefoni in chiaro | Sicurezza del servizio | Legittimo interesse |

Arenile **non richiede** dati particolari (art. 9). Evita di inserire nelle note informazioni sulla salute o altri dati sensibili; se servono per l'accessibilità del posto, limita il dato al minimo necessario (ad esempio "postazione accessibile").

## Come Arenile ti aiuta

Funzioni previste dalla specifica (vedi la [tabella di stato](../../README.md#stato-delle-funzionalità)):

- **Minimizzazione**: per una prenotazione online bastano nome, cognome, email o telefono. Nessun account obbligatorio per il cliente.
- **Testi versionati**: condizioni e informativa sono versionate e ogni prenotazione salva la versione accettata.
- **Consenso marketing separato**, facoltativo, mai preselezionato, con data, fonte e versione del testo.
- **Esportazione** dei dati di un cliente in formato JSON dal pannello (diritto di accesso e portabilità).
- **Anonimizzazione su richiesta**: i dati personali vengono sostituiti, mentre prenotazioni e pagamenti restano per gli obblighi contabili (diritto alla cancellazione nei limiti dell'art. 17.3).
- **Conservazione configurabile**: anonimizzazione automatica dei clienti senza prenotazioni da N anni (default 5, modificabile).
- **Sicurezza**: password Argon2id, autenticazione a due fattori, segreti cifrati, ruoli con permessi minimi, audit log, log senza dati personali in chiaro.
- **Nessun cookie di terze parti** nel widget di prenotazione e nessun tracciamento di default.

## Cosa devi fare tu

1. **Pubblica un'informativa** per i clienti (modello sotto) e inseriscila nel testo delle condizioni della pagina di prenotazione.
2. **Nomina i responsabili** (hosting, SMTP, eventuale fornitore di assistenza) con un accordo ex art. 28.
3. **Tieni il registro dei trattamenti** (art. 30) se obbligato o se lo ritieni opportuno: è comunque una buona pratica.
4. **Imposta la conservazione** in modo coerente con l'informativa e con gli obblighi contabili.
5. **Assegna i ruoli** del pannello con il principio del minimo privilegio (ad esempio `beach_staff` per il personale di spiaggia, `cashier` per la cassa).
6. **Proteggi i backup**: contengono dati personali, vanno cifrati e conservati in luogo sicuro.
7. **Gestisci le richieste** degli interessati entro un mese, usando esportazione e anonimizzazione.
8. **Data breach**: in caso di violazione valuta la notifica al Garante entro 72 ore (art. 33).
9. Se aggiungi strumenti di analisi o pixel pubblicitari al tuo sito, gestisci il relativo consenso ai cookie sul sito: sono fuori da Arenile.

## Modello di informativa (traccia da adattare)

Usa questa struttura come base per l'informativa ai clienti ai sensi degli artt. 13 e 14 GDPR. Sostituisci le parti tra parentesi quadre.

```markdown
# Informativa sul trattamento dei dati personali — [Nome dello stabilimento]

## 1. Titolare del trattamento
[Ragione sociale], [indirizzo], P.IVA [...], email [...], PEC [...].
[Eventuale DPO: nome e contatti.]

## 2. Dati trattati
Nome, cognome, email e/o telefono; dati della prenotazione (date, postazione,
servizi); dati relativi ai pagamenti (importi, esito, riferimenti della
transazione: i dati della carta sono trattati direttamente da [Stripe]);
eventuale consenso al marketing.

## 3. Finalità e basi giuridiche
a) gestione della prenotazione e del servizio — esecuzione del contratto;
b) adempimenti fiscali e contabili — obbligo legale;
c) comunicazioni di servizio (conferme, promemoria, rinnovi) — esecuzione del contratto;
d) invio di offerte e newsletter — consenso, facoltativo e revocabile;
e) sicurezza del sistema di prenotazione — legittimo interesse.

## 4. Natura del conferimento
Il conferimento dei dati di cui alle lettere a)–c) è necessario per prenotare;
il consenso di cui alla lettera d) è facoltativo e il rifiuto non ha conseguenze
sulla prenotazione.

## 5. Destinatari
Fornitore del servizio di hosting [nome], fornitore del servizio email [nome],
gestore dei pagamenti [Stripe], commercialista, autorità ove previsto dalla legge.
I responsabili del trattamento sono nominati ai sensi dell'art. 28 GDPR.

## 6. Trasferimenti extra UE
[Indicare se presenti, ad esempio per il gestore dei pagamenti, e le garanzie
adottate (decisione di adeguatezza, clausole contrattuali standard).]

## 7. Conservazione
Dati della prenotazione: per la durata del rapporto e successivamente per [N]
anni; documenti contabili: per il periodo previsto dalla legge (di norma 10 anni);
dati per marketing: fino alla revoca del consenso o per [N] mesi dall'ultimo
contatto. Trascorsi i termini i dati vengono cancellati o anonimizzati.

## 8. Diritti dell'interessato
Accesso, rettifica, cancellazione, limitazione, portabilità, opposizione, revoca
del consenso in qualsiasi momento, scrivendo a [email]. Diritto di proporre
reclamo al Garante per la protezione dei dati personali (www.garanteprivacy.it).

## 9. Versione
Versione [x], aggiornata al [data].
```

Ricorda di **aggiornare la versione** dell'informativa nel pannello ogni volta che la modifichi: Arenile registra quale versione ha accettato ciascun cliente.

## Riferimenti

- Regolamento (UE) 2016/679 (GDPR), in particolare artt. 5, 6, 13, 17, 28, 30, 32, 33.
- Garante per la protezione dei dati personali: https://www.garanteprivacy.it
- Sezione 23 della [specifica tecnica](../spec/23-privacy-e-gdpr.md).
