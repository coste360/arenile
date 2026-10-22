# Contribuire ad Arenile

Grazie per voler contribuire ad Arenile, il gestionale gratuito per stabilimenti balneari con codice sorgente disponibile. Sono benvenuti codice, traduzioni, documentazione, driver di pagamento, fiscali e di notifica, e anche semplici segnalazioni dal campo da parte di chi gestisce un lido.

Prima di iniziare leggi il [Codice di condotta](CODE_OF_CONDUCT.md). Partecipando al progetto accetti di rispettarlo.

## Indice

- [Come posso aiutare](#come-posso-aiutare)
- [Prima di scrivere codice](#prima-di-scrivere-codice)
- [Ambiente di sviluppo](#ambiente-di-sviluppo)
- [Flusso di lavoro](#flusso-di-lavoro)
- [Regole tecniche](#regole-tecniche)
- [Commit e pull request](#commit-e-pull-request)
- [Licenza e CLA](#licenza-e-cla)
- [Agenti AI](#agenti-ai)

## Come posso aiutare

- **Segnalare un bug**: apri una issue con il modello "Segnalazione bug". Indica versione, database (SQLite o PostgreSQL), passi per riprodurre e, se possibile, log senza dati personali.
- **Proporre una funzionalità**: usa il modello "Richiesta di funzionalità" e descrivi il problema concreto del lido prima della soluzione.
- **Nuovo driver**: pagamento (`PaymentProvider`), fiscale (`FiscalDriver`) o notifiche (`Notifier`): apri prima una issue con il modello "Nuovo driver" per concordare l'interfaccia.
- **Task per agente**: issue piccole e verificabili, con perimetro e comando di verifica; adatte anche a chi è alle prime armi.
- **Traduzioni**: l'italiano è la lingua sorgente; inglese, tedesco e francese sono le prime traduzioni previste.
- **Vulnerabilità**: **non** aprire issue pubbliche, segui [SECURITY.md](SECURITY.md).

## Prima di scrivere codice

1. Cerca se esiste già una issue; in caso contrario aprine una. Per modifiche non banali attendi un riscontro dei maintainer: evita lavoro che potrebbe non essere accettato.
2. Leggi le sezioni pertinenti della [specifica](docs/spec/README.md), [`docs/architecture.md`](docs/architecture.md), [`docs/invariants.md`](docs/invariants.md) e gli ADR in [`docs/adr/`](docs/adr/README.md). Per cambiare una decisione esistente serve un nuovo ADR.
3. Un contributo tocca **un'area di dominio** e il relativo endpoint. Le modifiche grandi vanno divise in più pull request.

## Ambiente di sviluppo

Prerequisiti: Go 1.27+, Node.js 22+, npm 10+, Docker, `make`. In alternativa usa il devcontainer incluso.

```bash
make setup   # una tantum
make seed    # dati dimostrativi
make dev     # backend + Vite per pannello e booking
```

Il `Makefile` è l'unica interfaccia supportata. Le verifiche principali:

| Comando | Quando |
|---|---|
| `make fmt` | Prima di ogni commit (Go con `gofumpt`/`goimports`, frontend con Prettier) |
| `make gen` | Dopo ogni modifica a `api/openapi.yaml` |
| `make check` | Dopo ogni modifica: `gen` + verifica del codice generato + `lint` + `test` |
| `make check-full` | Prima di aprire la pull request: `check` + `test-db` + `test-race` + `e2e` |

`make check-full` è **obbligatorio** per le modifiche che toccano disponibilità, prezzi, pagamenti, migrazioni o il flusso pubblico di prenotazione.

Note sui test:
- `make test` esegue i test unitari Go e Vitest: non devono richiedere Docker né rete.
- I test di integrazione su database usano il build tag `integration`; quelli su PostgreSQL vengono eseguiti quando è impostata `ARENILE_TEST_POSTGRES_URL` (`make test-db` avvia il container).
- I test di concorrenza si chiamano `TestConcurrency_*` ed eseguono con `-race` (`make test-race`).

## Flusso di lavoro

1. Fai un fork e crea un branch da `main` con un nome descrittivo (`feat/pricing-free-days`, `fix/hold-expiry`).
2. Scrivi il codice **e i test**. Una correzione di bug include un test che fallisce senza la correzione.
3. Aggiorna la documentazione (`docs/`, guida per gestori, README) se cambiano comportamento, configurazione o API.
4. Aggiungi una voce a [CHANGELOG.md](CHANGELOG.md) nella sezione `[Unreleased]`.
5. Esegui `make check` (e `make check-full` quando richiesto) e apri la pull request compilando il modello.

## Regole tecniche

Le regole complete sono in [AGENTS.md](AGENTS.md) e valgono per persone e agenti. In sintesi:

**API-first (OpenAPI)**
- Il contratto è `api/openapi.yaml` (OpenAPI 3.1), unica fonte di verità.
- Prima modifica il contratto, poi `make gen`, poi implementa l'handler.
- Non modificare mai i file generati (`*.gen.go`, `web/shared/api/generated/**`).
- Errori in formato `application/problem+json` con `type` stabile.

**Migrazioni**
- Ogni modifica di schema richiede una nuova migrazione **per entrambi i dialetti**: `internal/migrations/sqlite/` e `internal/migrations/postgres/`, con lo stesso numero.
- Non modificare mai una migrazione già presente su `main`: aggiungine una nuova.
- Le migrazioni devono essere compatibili in avanti; verifica con `make test-db`.

**Backend Go**
- Importi sempre in centesimi `int64` (`internal/money`), mai `float`.
- Date di calendario `YYYY-MM-DD` nel fuso dello stabilimento (`internal/calendar`); istanti in UTC.
- Tempo iniettato con `clock.Clock`, mai `time.Now()` diretto nella logica.
- Il motore prezzi è una funzione pura: niente database, orologio o casualità.
- La disponibilità è garantita da vincoli del database: non sostituirli con controlli applicativi.
- Ogni cambio di stato di una prenotazione passa da `internal/domain/booking` e scrive audit log ed evento outbox nella stessa transazione.
- Ogni query operativa è filtrata per `establishment_id` dal contesto autenticato.
- Errori con `fmt.Errorf("contesto: %w", err)` ed errori di dominio sentinella tipizzati; `context.Context` come primo parametro; log solo con `log/slog`, **mai** email, telefoni o token in chiaro.
- Test table-driven chiamati `TestXxx_Scenario`; nessuno stato globale, dipendenze iniettate nei costruttori.
- Commenti e identificatori in inglese; testi per l'utente tramite chiavi i18n con l'italiano come sorgente.

**Frontend**
- TypeScript strict, client API generato (`openapi-fetch`), nessuna chiamata manuale non tipizzata.
- La pagina pubblica ha un budget di meno di 150 KB di JS gzip: niente librerie pesanti.
- Accessibilità: contrasto WCAG AA, navigazione da tastiera, alternativa a elenco della mappa.

**Dipendenze**
- Nessuna nuova dipendenza di runtime (modulo Go o pacchetto npm) senza spiegarne il motivo nella pull request.

## Commit e pull request

Usiamo **[Conventional Commits](https://www.conventionalcommits.org/it/v1.0.0/)**: il changelog dei rilasci è generato dai messaggi.

```text
feat(pricing): supporta l'azione free_days
fix(availability): rilascia l'hold scaduto prima del rinnovo
docs(guida): aggiunge installazione su Raspberry Pi
test(booking): copre la cancellazione con credito
refactor(store): estrae il filtro per establishment_id
chore(ci): aggiorna l'immagine di base
```

Tipi ammessi: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`, `ci`, `chore`. Un cambiamento incompatibile si indica con `!` (`feat(api)!: ...`) e con una nota `BREAKING CHANGE:` nel corpo.

Pull request:
- piccole e mirate, un argomento per pull request;
- descrizione compilata secondo il modello: cosa cambia, come è stato testato, domande aperte;
- la CI deve essere verde; i maintainer possono chiedere modifiche prima del merge;
- il merge avviene con squash: il titolo della pull request deve essere un Conventional Commit valido.

## Licenza e CLA

Arenile è distribuito con la [Business Source License 1.1](LICENSE) (BSL 1.1, SPDX `BUSL-1.1`): è gratuito per chi lo usa per gestire i propri stabilimenti balneari, non può essere offerto a terzi come servizio a pagamento (hosting, SaaS, installazione, personalizzazione, assistenza, rivendita) senza una licenza commerciale di Coste360 SRL, e ogni versione diventa AGPL-3.0 tre anni dopo il suo rilascio. I tuoi contributi vengono distribuiti con la stessa licenza. Dettagli nella sezione "Licenza e marchio" del [README](README.md#licenza-e-marchio) e nell'[ADR 0007](docs/adr/0007-licenza-bsl.md).

**Contributi benvenuti**: correzioni di bug, nuove funzionalità concordate in una issue, driver di pagamento, fiscali e di notifica, traduzioni, documentazione, test e segnalazioni dal campo. **Contribuire non concede diritti di rivendita**: avere una pull request accettata non autorizza a offrire Arenile come servizio a terzi né a rivenderlo; per questi usi serve comunque una licenza commerciale (scrivi a **info@coste360.it**).

Per accettare i contributi chiediamo la firma del [Contributor License Agreement](CLA.md). **Alla prima pull request il bot CLA Assistant pubblica un commento** con le istruzioni: la firma richiede un solo commento e vale per tutti i contributi successivi. Il CLA consente a Coste360 SRL, titolare del progetto, di distribuire i contributi con la BSL 1.1, con licenze commerciali e con la AGPL-3.0 alla Change Date, e di modificare la licenza nel tempo; tu mantieni la titolarità del diritto d'autore.

> Testo della licenza e della clausola d'uso da validare con un legale prima dell'apertura del repository.

## Agenti AI

Il repository è progettato anche per agenti di coding. Se usi Claude Code o uno strumento compatibile:

- le istruzioni condivise sono in [AGENTS.md](AGENTS.md) (fonte unica), con [CLAUDE.md](CLAUDE.md) per le note specifiche di Claude Code;
- regole per percorso, skill (`/new-endpoint`, `/new-migration`, `/pre-pr-check`, ...) e subagent di revisione sono in `.claude/`;
- il modello di issue "Task per agente" definisce obiettivo, perimetro, criteri di accettazione e comando di verifica.

Chi apre la pull request resta responsabile del codice prodotto con un agente: va letto, capito e verificato come codice scritto a mano.

## Domande

Per domande generali usa le [Discussions](https://github.com/coste360/arenile/discussions). Per questioni riservate scrivi a **info@coste360.it**.
