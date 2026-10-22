# 0005 — Contratto OpenAPI spec-first con codice generato

- **Stato:** accettato
- **Data:** 2026-10-22
- **Sezioni della specifica:** `docs/spec/17-api.md`, `docs/spec/03-architettura-e-stack.md`

## Contesto

Pannello, pagina pubblica, widget e integrazioni di terze parti usano esclusivamente le API documentate
(principio API-first). Backend Go e frontend TypeScript devono restare allineati senza scrivere a mano
tipi duplicati, e gli agenti di coding hanno bisogno di un punto unico e verificabile da cui partire.

## Decisione

- `api/openapi.yaml` (OpenAPI 3.1) è l'**unica fonte di verità** del contratto.
- Il codice server Go (modelli e interfaccia chi) è generato con `oapi-codegen`
  (`api/oapi-codegen.yaml`) in `internal/http/api/api.gen.go`.
- I tipi TypeScript sono generati con `openapi-typescript` in `web/shared/api/generated/schema.d.ts` e
  usati con `openapi-fetch` e TanStack Query.
- `make gen` rigenera entrambi; `make check` fallisce se il codice generato differisce da quello
  versionato. I file generati non si modificano mai a mano.
- Convenzioni: errori `application/problem+json` (RFC 9457) con `type` stabile, paginazione a cursore,
  `Idempotency-Key` sulle creazioni, `ETag`/`If-Match` sulla configurazione, versione nel percorso.

## Conseguenze

- Positive: tipi coerenti tra server e client; documentazione API sempre aggiornata; revisione delle
  modifiche al contratto in un solo file.
- Negative: ogni nuova operazione passa dal file YAML e dalla generazione; i limiti del generatore
  possono richiedere adattamenti nello stile dello schema.
- Verifiche: controllo in CI del codice generato aggiornato; hook `scripts/agents/guard-generated.sh`;
  regola `.claude/rules/api-contract.md`; skill `/new-endpoint`.
