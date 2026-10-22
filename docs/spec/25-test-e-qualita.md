## 25. Test e qualità

| Livello | Strumenti | Copertura richiesta |
|---|---|---|
| Unit motore prezzi | Test table-driven Go | Ogni tipo di condizione e azione, conflitti, arrotondamenti, IVA, durate fisse, mezze giornate |
| Proprietà motore prezzi | `pgregory.net/rapid` | Invarianti: totale ≥ 0, monotonia degli sconti, snapshot ricalcolabile |
| Unit disponibilità | Go | Hold, scadenze, pool, quote, rilasci, assegnazione automatica |
| Concorrenza | Go con N goroutine | Nessuna doppia allocazione con 100 richieste parallele sulla stessa postazione, su SQLite e PostgreSQL |
| Integrazione | Go + database reale (SQLite in memoria, PostgreSQL in container) | Repository e migrazioni su entrambi i dialetti |
| Contratto API | Validazione delle risposte contro `openapi.yaml` | Tutti gli endpoint |
| Frontend | Vitest + Testing Library | Componenti della mappa, form, calcolo locale del riepilogo |
| End-to-end | Playwright | Flusso pubblico completo con Stripe in modalità test e webhook simulati; flusso cassa; editor mappa; iframe su pagina ospite di prova |
| Accessibilità | `axe-core` in Playwright | Pagina pubblica senza violazioni gravi |
| Prestazioni | Lighthouse CI | Budget della pagina pubblica (12.6) |

**CI (GitHub Actions):** lint (`golangci-lint`, ESLint, Prettier), test, build dell'immagine, scansione di sicurezza, verifica che il codice generato da OpenAPI sia aggiornato.

**Rilasci:** versionamento semantico, changelog generato da Conventional Commits, tag che pubblica binari e immagini.

---

