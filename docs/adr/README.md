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
| [0007](0007-licenza-bsl.md) | Licenza del core: Business Source License 1.1, conversione in AGPL-3.0 dopo tre anni | accettato |
| [0008](0008-scelte-m1-m2.md) | Scelte di M1/M2: `internal/reqctx`, allocazioni future, pool implicito, password temporanea, payload degli eventi di configurazione | accettato |
| [0009](0009-ui-kit-shadcn.md) | UI kit: shadcn/ui, niente CSS custom | accettato |
| [0010](0010-scelte-m3.md) | Scelte di M3: allocazioni cancellate al rilascio, hold del pannello, abbonamenti come prenotazioni, registro unico dei pagamenti, topic SSE | accettato |
| [0011](0011-revisione-m3.md) | Revisione di M3: quota online anche sul canale `api`, `price_current` per le prenotazioni modificate dopo la conferma, hold confermati sotto lock e puliti per intero, check-in solo nel soggiorno, deviazioni dichiarate dalla spec 11.3 | accettato |
| [0012](0012-kit-mappa.md) | Kit di illustrazioni della mappa: `<symbol>`/`<use>`, stato sovrapposto, campi `asset`/`variant`, provenienza | accettato |
| [0013](0013-scelte-m4.md) | Scelte di M4: percorsi pubblici e sessione di checkout, webhook, cliente provvisorio, link magici, `payment_intents`, capacità facoltative dei driver, Stripe senza SDK, email | accettato |
| [0014](0014-pagamenti-online-m4.md) | Pagamenti online di M4: SDK Stripe ufficiale, webhook unico percorso di conferma, pagamenti tardivi e doppi rimborsati, rimborsi prenotati e registrati alla conferma, riconciliazione per intent | accettato |

## Come aggiungere un ADR

1. Copia [`template.md`](template.md) in `NNNN-titolo-in-kebab-case.md` con il numero successivo.
2. Compila contesto, decisione e conseguenze in italiano, in modo conciso.
3. Aggiungi la riga all'indice qui sopra.
4. Se sostituisci un ADR, aggiorna lo stato del precedente e collega i due documenti.
