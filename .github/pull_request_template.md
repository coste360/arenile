## Cosa cambia

<!-- Descrivi la modifica e il motivo. Collega la issue: "Closes #123". -->

Closes #

## Come è stato testato

<!-- Test aggiunti o aggiornati, comandi eseguiti e risultati, prove manuali (con quali dati). -->

- [ ] Test unitari
- [ ] Test di integrazione (SQLite / PostgreSQL)
- [ ] Test di concorrenza
- [ ] End-to-end
- [ ] Prova manuale

## Domande aperte

<!-- Dubbi, compromessi, punti su cui vuoi un parere. Nuove dipendenze: spiega perché sono necessarie. -->

## Checklist

- [ ] `make check` passa
- [ ] `make check-full` passa (obbligatorio se la modifica tocca disponibilità, prezzi, pagamenti, migrazioni o il flusso pubblico di prenotazione)
- [ ] Le modifiche al contratto API partono da `api/openapi.yaml` e il codice generato è aggiornato (`make gen`)
- [ ] Le modifiche di schema hanno migrazioni per SQLite **e** PostgreSQL; nessuna migrazione esistente è stata modificata
- [ ] Documentazione aggiornata (`docs/`, guida per gestori, README) se cambiano comportamento, configurazione o API
- [ ] Voce aggiunta in `CHANGELOG.md` sotto `[Unreleased]`
- [ ] Titolo della PR in formato Conventional Commits (`feat:`, `fix:`, `docs:` ...)
- [ ] Ho firmato il [CLA](https://github.com/coste360/arenile/blob/main/CLA.md) (il bot lo chiede alla prima pull request)
