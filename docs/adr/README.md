# Architecture Decision Records

Ogni scelta non ovvia del progetto ha un ADR breve: contesto, decisione, conseguenze, stato.
Gli agenti e i contributori li leggono **prima** di proporre alternative. Cambiare una decisione esistente
richiede un nuovo ADR che sostituisce il precedente (il vecchio passa a "sostituito da NNNN", non si cancella).

## Indice

| N. | Titolo | Stato |
|---|---|---|
| [0001](0001-go-monobinary.md) | Un unico binario Go con le SPA incorporate | accettato |
| [0002](0002-sqlite-default-postgres-optional.md) | SQLite di default, PostgreSQL opzionale | accettato |
| [0003](0003-allocations-unique-constraint.md) | Disponibilità garantita dal vincolo unique sulle allocazioni | accettato |
| [0004](0004-pricing-pure-function.md) | Motore prezzi come funzione pura | accettato |
| [0005](0005-openapi-spec-first.md) | Contratto OpenAPI spec-first con codice generato | accettato |
| [0006](0006-deviazioni-m0-dalla-specifica.md) | Deviazioni di M0: `ParseWebhook` con credenziali, `ARENILE_DEV`, endpoint TOTP, primo owner da variabili | accettato |

## Come aggiungere un ADR

1. Copia [`template.md`](template.md) in `NNNN-titolo-in-kebab-case.md` con il numero successivo.
2. Compila contesto, decisione e conseguenze in italiano, in modo conciso.
3. Aggiungi la riga all'indice qui sopra.
4. Se sostituisci un ADR, aggiorna lo stato del precedente e collega i due documenti.
