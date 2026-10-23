# 0019 — Funzioni rinviate: modalità prova, rimborsi garantiti, azioni dalla mappa, casse, importazioni

- **Stato:** accettato (contratto e schema; la realizzazione segue per area)
- **Data:** 2026-10-24
- **Sezioni della specifica:** `docs/spec/11-mappa-editor-e-vista-operativa.md` (11.3), `docs/spec/12-pagina-pubblica-e-widget-incorporabile.md`,
  `docs/spec/13-pagamenti.md`, `docs/spec/14-cassa-e-modulo-fiscale.md` (14.1, 14.2), `docs/spec/15-pannello-del-gestore.md` (15.4),
  `docs/spec/18-eventi-webhook-e-punti-di-estensione.md`, `docs/spec/23-privacy-e-gdpr.md`

## Contesto

L'elenco di funzioni rinviate concordato con il proprietario (A–F) tocca prenotazioni, pagamenti, cassa e integrazioni.
Per far lavorare sei costruttori in parallelo servono prima un contratto API unico, una sola migrazione (00009, entrambi
i dialetti) e le scelte trasversali scritte.

## Decisione

- **A. Modalità prova** (`test_mode_settings`, `bookings.test`, `payments.test`, `refund_requests.test`): il flag è letto
  nella transazione che crea la prenotazione o il pagamento e copiato sulla riga; un pagamento eredita il flag della sua
  prenotazione. Le righe di prova sono escluse da report, KPI, corrispettivi, emissione fiscale ed esportazioni (filtro
  `test = false`, indici parziali sulle righe di prova). Con Stripe si usa la configurazione `mode = test` quando c'è.
  La pagina pubblica accetta prenotazioni in prova solo con il link di anteprima (token firmato, 2 ore, header
  `X-Arenile-Preview`), salvo `public_enabled`. Eliminazione in blocco in un'unica transazione; l'audit resta.
- **B. Rimborso garantito**: la cancellazione (link del cliente o pannello) inserisce `refund_requests` nella stessa
  transazione; un job invia al provider con chiave di idempotenza fissa e backoff fino alla conferma del webhook. Indice
  unico `(booking_id, payment_intent_id)` sulle righe non annullate: mai perso, mai doppio. Tentativi esauriti → `failed`
  con notifica al gestore e riprova manuale.
- **C. Azioni dalla mappa**: il blocco usa `spot_blocks` (00003) e `POST /spot-blocks`; la prenotazione di gruppo è
  `POST /bookings` con più righe (già atomica, prezzo dal motore, `quote` per l'anteprima): nessun nuovo endpoint.
  `If-Match` diventa obbligatorio (428 `precondition_required`) su check-in, no-show, cancel e move e sull'eliminazione
  della mappa; i client (anche la coda offline) inviano sempre la versione letta.
- **D. Chiusura Z stampabile: HTML pronto per la stampa, nessuna libreria PDF.** `GET /cash-sessions/{id}/z-report`
  restituisce un documento HTML autonomo con stili di stampa (`@page` A4 e scontrino 80 mm), logo Arenile mono (ADR 0018)
  e intestazione dello stabilimento; il PDF è la stampa del browser. Motivo: nessuna nuova dipendenza runtime da
  mantenere (le librerie PDF pure Go mature sono archiviate o poco attive), resa tipografica uguale a quella delle
  ricevute, testo selezionabile e accessibile; il contenuto resta generato lato server (unica fonte dei totali).
- **D. Più casse** (`cash_registers`, `cash_sessions.register_id`): la migrazione crea "Cassa principale" (`main`,
  predefinita) per ogni stabilimento e una cassa per ogni altro codice già usato; una sola sessione aperta per cassa
  (indice unico parziale). Gli stabilimenti nuovi ricevono la cassa predefinita in modo pigro dal servizio cassa.
- **E. Prezzo manuale degli abbonamenti**: colonne `bookings.price_override_*`, permesso `establishment.configure`,
  audit con prima/dopo e motivo; `price_snapshot` resta immutabile (INV-21). La regola di approvazione con forzatura
  dal banco è documentata dal costruttore E in questo ADR (sezione da completare).
- **F. Integrazioni**: notifiche nel pannello (`admin_notifications`, chiave di deduplica, parametri senza dati
  personali); storico completo dei tentativi webhook (`webhook_delivery_attempts`, conservazione configurabile);
  esportazione XLSX con `excelize` (BSD-3, scrittura in streaming; dipendenza da giustificare nella PR del costruttore);
  importazione CSV (`import_jobs`: carica, anteprima, applica tutto o niente, stesso file applicato una sola volta);
  meta Open Graph della pagina pubblica generate dal server (nessuna API); le richieste annullate dal client
  (`context.Canceled`) non sono errori 500 nei log.

## Conseguenze

- La guardia `TestReportQueries_TestBookingsFlagNotYetFiltered` (ADR 0016) è sostituita da
  `TestReportQueries_TestBookingsExcluded`: `store/report.go` filtra già le righe di prova.
- Nuovi tipi di problema: `test_mode_preview_required`, `import_invalid`, `import_already_applied`, `refund_in_progress`.
- `CashSessionOpen.register` resta per compatibilità; i client usano `register_id`.
