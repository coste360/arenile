# 0008 — Scelte di M1 e M2 non fissate dalla specifica

- **Stato:** accettato
- **Data:** 2026-10-22
- **Sezioni della specifica:** `docs/spec/07-motore-di-disponibilita.md`, `docs/spec/11-mappa-editor-e-vista-operativa.md`,
  `docs/spec/15-pannello-del-gestore.md`, `docs/spec/16-utenti-ruoli-e-autenticazione.md`,
  `docs/spec/18-eventi-webhook-e-punti-di-estensione.md`, `docs/spec/28-roadmap-e-criteri-di-accettazione.md` (M1, M2)

## Contesto

M1 (inventario, mappa, catalogo, configurazione guidata) e il completamento di M2 (persistenza di
listini e policy) sono stati costruiti prima delle prenotazioni (M3). Alcuni punti dipendono da M3, altri
non sono descritti dalla specifica e richiedevano una scelta esplicita.

## Decisione

1. **Metadati della richiesta fuori da `internal/http`.** Il package con request ID, IP del client e
   metodo di autenticazione si chiama `internal/reqctx` (prima `internal/http/httpctx`): i servizi di
   dominio lo leggono per l'IP dell'audit senza importare `internal/http/*`.
2. **Allocazioni future.** `mapping.AllocationChecker` riceve la transazione corrente e, fino a M3, è
   implementato da `mapping.NoAllocations` (risponde sempre "nessuna"). M3 lo implementa su
   `spot_allocations`; l'eliminazione di postazioni allocate risponde già 409
   `spot_has_future_allocations` con gli id delle postazioni.
3. **Pool implicito delle zone `zone`.** È un pool di sistema con `component_code = "spot"`, capacità
   uguale alle postazioni attive, aggiornato nella stessa transazione di ogni modifica di mappa o zona e
   non modificabile dall'API (409).
4. **Utenti.** Il titolare crea l'utente con una password iniziale o con una password temporanea
   generata, restituita una sola volta con `Cache-Control: no-store`. Nessuna email di invito finché non
   esistono le notifiche.
5. **Eventi di configurazione.** La spec 18 non definisce il payload di `config.pricing_changed` e
   `config.map_changed`: i servizi usano la versione 1 con entità, id e tipo di modifica (più
   `season_id` per i listini); la configurazione guidata scrive un solo evento per tipo con
   `reason = "setup_template"` o `"quick_pricelist"` e gli id creati. I consumatori devono trattare
   questi eventi come "ricarica la configurazione", senza dipendere dai campi.
6. **Configurazione guidata.** `force` (riapplicare un modello su uno stabilimento con mappe) e il
   fattore mezza giornata del listino rapido esistono nel servizio ma non nel contratto API: il pannello
   non li usa.
7. **Codici promozionali.** Il contatore di utilizzi si incrementa con le prenotazioni (M3); il
   simulatore rifiuta solo i codici già esauriti.

## Conseguenze

- Positive: M1/M2 sono utilizzabili senza M3; le dipendenze da M3 sono interfacce esplicite.
- Negative: la configurazione guidata e la copia del listino scrivono alcune righe (periodi, stagione
  copiata) con il proprio codice invece di chiamare `season.Service`: le regole di validazione sono
  duplicate e vanno riallineate.
- Verifiche: `TestMappingIntegration_FutureAllocations`, `TestMappingIntegration_ImplicitZonePool`,
  `TestTeamService_CreateAndLogin`, `TestApply_EveryTemplate`, `TestPromoCode_CaseInsensitive`.
