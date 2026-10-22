# Politica di sicurezza

Arenile gestisce dati personali dei clienti, incassi e credenziali dei provider di pagamento dei gestori di stabilimenti balneari. Prendiamo sul serio ogni segnalazione di sicurezza e ringraziamo chi ci aiuta a proteggere gli utenti con una divulgazione responsabile.

## Versioni supportate

Il progetto è in sviluppo attivo (milestone M0) e non ha ancora versioni stabili. Le correzioni di sicurezza vengono applicate al branch `main` e, dopo il primo rilascio, secondo questa tabella:

| Versione | Supportata |
|---|---|
| `main` (sviluppo) | Sì |
| Ultima versione minore rilasciata (`x.y`) | Sì, con rilascio di patch |
| Versione minore precedente (`x.y-1`) | Solo vulnerabilità critiche, per 3 mesi dal rilascio della successiva |
| Versioni precedenti | No: aggiorna all'ultima versione |

Prima della versione `1.0.0` è supportata solo l'ultima versione rilasciata.

## Come segnalare una vulnerabilità

**Non aprire issue, discussioni o pull request pubbliche per le vulnerabilità.**

Scrivi a **security@coste360.it** oppure usa la funzione [Report a vulnerability](https://github.com/coste360/arenile/security/advisories/new) di GitHub (segnalazione privata). Includi, per quanto possibile:

- descrizione della vulnerabilità e del suo impatto;
- versione o commit interessato, database (SQLite o PostgreSQL) e modalità di installazione;
- passi per riprodurre o una prova di concetto minima;
- eventuali proposte di correzione;
- come desideri essere citato nei ringraziamenti (o se preferisci restare anonimo).

Non includere dati personali reali di clienti nella segnalazione.

## Cosa aspettarti

| Fase | Tempi indicativi |
|---|---|
| Conferma di ricezione | entro 3 giorni lavorativi |
| Prima valutazione e classificazione della gravità | entro 10 giorni lavorativi |
| Correzione e rilascio | in base alla gravità; le vulnerabilità critiche hanno priorità assoluta |
| Pubblicazione dell'avviso (GitHub Security Advisory, CVE se applicabile) | dopo il rilascio della correzione, coordinata con chi ha segnalato |

Ti terremo aggiornato sull'avanzamento. Chiediamo di non divulgare pubblicamente la vulnerabilità prima della pubblicazione della correzione o, in assenza di risposta, prima di 90 giorni dalla segnalazione.

## Ambito

Rientrano nell'ambito, ad esempio:

- accesso ai dati di uno stabilimento da parte di utenti di un altro stabilimento (isolamento multi-stabilimento);
- aggiramento di autenticazione, ruoli e permessi, TOTP o limitazione dei tentativi;
- doppia prenotazione o manipolazione di prezzi, pagamenti, rimborsi o webhook (incluse firme non verificate);
- esposizione di segreti (chiavi Stripe, segreti dei webhook, `ARENILE_SECRET_KEY`) o di dati personali nei log;
- XSS, CSRF, injection, SSRF, problemi di `frame-ancestors` e `postMessage` nel widget incorporabile;
- vulnerabilità nel caricamento di file o nelle dipendenze sfruttabili in Arenile.

Fuori ambito:

- installazioni configurate in modo non sicuro dal gestore (ad esempio senza HTTPS o con chiavi deboli), salvo che la documentazione induca in errore;
- attacchi di negazione del servizio volumetrici, ingegneria sociale, accesso fisico ai server;
- la demo pubblica, salvo vulnerabilità del software stesso: non eseguire test che degradino il servizio per altri;
- segnalazioni generate automaticamente senza un impatto dimostrato.

## Safe harbor

Non intraprenderemo azioni legali contro chi, in buona fede, ricerca e segnala vulnerabilità nel rispetto di questa politica: senza accedere, modificare o conservare dati altrui oltre il minimo necessario a dimostrare il problema, senza interrompere servizi e dando il tempo di correggere prima della divulgazione.

## Pratiche di sicurezza del progetto

- Dipendenze controllate in CI con `govulncheck` e `npm audit`; immagine Docker scansionata con Trivy.
- Segreti cifrati a riposo con AES-GCM e chiave da `ARENILE_SECRET_KEY`, con rotazione supportata.
- Password con Argon2id, TOTP opzionale, cookie `HttpOnly`/`Secure`/`SameSite=Lax`, token CSRF double-submit.
- Log applicativi senza email, telefoni o token in chiaro.

Grazie per contribuire alla sicurezza di Arenile e dei lidi che lo usano.
