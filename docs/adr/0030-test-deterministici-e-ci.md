# 0030 — Test deterministici, CI a due livelli, budget di tempo fuori dai gate

- **Stato:** accettato
- **Data:** 2026-11-14
- **Sezioni della specifica:** `docs/spec/25-test-e-qualita.md` (prestazioni e CI); cambia la natura
  "bloccante" dei budget di `docs/spec/12-pagina-pubblica-e-widget-incorporabile.md` (12.6, LCP sotto 2,5 s su 4G)
  e di `docs/spec/28-roadmap-e-criteri-di-accettazione.md` (tempi dell'accettazione M4 e M8)

Decisione gemella dell'ADR 0019 di Porto, con lo stesso titolo e lo stesso schema.

## Contesto

Sulle pull request girava tutta la suite (circa 15 minuti, la parte lenta sono i test di concorrenza con il race
detector, circa 7 minuti, e Playwright, circa 5) e il run falliva senza che il codice fosse sbagliato:

- **soglie di tempo misurate sul runner:** Lighthouse misurava LCP mediano 2523 ms contro 2500 ms sulla pagina
  pubblica (in locale 2,4 s); il risultato dipende dalla macchina condivisa e dal rallentamento simulato, non solo
  dal codice;
- **scanner con un database che cambia ogni giorno:** Trivy è fallito su una pull request per una vulnerabilità
  pubblicata quel giorno, indipendente dalla modifica.

I test di prestazioni Go avevano già i limiti di tempo dietro `ARENILE_PERF_BUDGETS=1` (`storetest.Perf`); restavano
nei gate il budget dell'import a righe massime, il minuto della ricerca posto e prenotazione di M8 e i due minuti
della prenotazione incorporata di M4.

## Decisione

1. **Nessun test dei gate fallisce per una durata.** I budget di tempo della specifica restano e vengono misurati:
   la durata è sempre stampata e fa fallire il test solo con `ARENILE_PERF_BUDGETS=1` (`storetest.Perf`,
   `storetest.Budget`, la spec Playwright M4, `e2e/lighthouse/run.mjs`, dove un obiettivo mancato è altrimenti un
   avviso). Li esegue il workflow **Performance** (`perf.yml`: notturno e manuale, `make test-perf`,
   `make e2e-perf`), che non blocca pull request, merge né rilascio: un run rosso lì è un segnale da misurare, non
   un `main` rotto.
2. **Attese su condizioni, non sul tempo.** Un test aspetta un evento (risposta, messaggio, stato) e usa un limite
   di tempo solo come protezione contro un blocco, largo rispetto al caso normale.
3. **CI a due livelli** (`ci.yml`):
   - **pull request:** lint, test unitari, integrazione su SQLite, web (lint, tipi, Vitest, build, budget del
     bundle), codice generato, spec Playwright essenziali (`make e2e-smoke`, elenco `E2E_SMOKE` nel Makefile, job
     "End-to-end (Playwright, essential)");
   - **main, notturno, manuale:** la suite completa: integrazione su SQLite e PostgreSQL, test di concorrenza con il
     race detector (`-timeout` esplicito, `TEST_RACE_TIMEOUT`, 30 minuti), govulncheck, Windows, immagine Docker con
     Trivy, tutte le spec Playwright e l'accettazione M4 sul binario reale (job "End-to-end (Playwright)"). I due
     scanner non girano sulle pull request perché il loro database cambia ogni giorno indipendentemente dal codice.
4. **Nuovo tentativo del singolo test, mai del job:** Playwright in CI riprova una volta il test fallito
   (`retries: 1`); su `main` i test Go falliti sono rilanciati una volta da soli (`scripts/gotest.sh` passa a
   gotestsum con `GOTEST_RERUN_FAILS=1`), mai sulle pull request; Vitest non riprova. È una rete di sicurezza per i
   guasti dell'infrastruttura: un test segnato "flaky" nel report va corretto, non tollerato.

## Conseguenze

- Positive: le pull request sono più corte e rosse solo per un difetto della modifica; il run completo su `main`
  torna una prova del codice.
- Negative: una regressione di prestazioni non blocca più il merge (la si vede nel workflow Performance e nei log,
  dove ogni durata è stampata: prima di un rilascio si guarda l'ultimo run di Performance). Una regressione su una
  spec fuori da `E2E_SMOKE`, su PostgreSQL, sotto race o su Windows si vede sul run di `main` dopo il merge; una
  modifica che tocca quelle aree lancia `make check-full` in locale o `workflow_dispatch` sul branch.
- Verifiche: `ci.yml` e `perf.yml`; nuovi test senza `waitForTimeout`/`time.Sleep` come attesa di un risultato,
  senza una soglia di latenza fuori da `storetest.Perf`/`storetest.Budget` o da `ARENILE_PERF_BUDGETS`.
