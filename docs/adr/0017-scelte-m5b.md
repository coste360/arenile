# 0017 — Scelte di M5 (parte B): passaggio di stagione, rinnovi, lista d'attesa, crediti

- **Stato:** accettato
- **Data:** 2026-10-23
- **Sezioni della specifica:** `docs/spec/08-motore-dei-prezzi.md` (8.7), `docs/spec/09-policy-di-prenotazione.md`,
  `docs/spec/10-flusso-di-prenotazione-e-ciclo-di-vita.md` (10.4, 10.5), `docs/spec/13-pagamenti.md` (13.4),
  `docs/spec/19-notifiche.md`, `docs/spec/23-privacy-e-gdpr.md`, `docs/spec/28-roadmap-e-criteri-di-accettazione.md`

## Contesto

Il criterio di accettazione di M5 è il ciclo fine stagione → nuova stagione (copia della configurazione, proposta di
rinnovi, conferme) senza interventi manuali sul database. Servono anche lista d'attesa e crediti. Esistono già la
copia di stagione di M2 (`pricingconfig.CopySeason`), il riferimento `bookings.renewal_of_booking_id`, lo stato
`renewal_offer` della mappa operativa, il registro `customer_credits` di M3 con il metodo di pagamento `credit`, gli
hold, i link magici e il gateway di M4.

## Decisione

- **Passaggio di stagione** (`POST /seasons/{id}/rollover`, pacchetto `rollover`): anteprima (`dry_run`, default) e
  applicazione in una sola transazione (periodi, chiusure spostate sull'anno, regole, prezzi fissi, policy; catalogo e
  mappe non dipendono dalla stagione). Idempotente con la chiave unica `season_rollovers (establishment_id,
  source_season_id, target_year)`: una seconda applicazione restituisce il risultato registrato.
- **Rinnovi** (pacchetto `renewal`): una proposta è una prenotazione `held` della nuova stagione sulla stessa
  postazione e con lo stesso pack, prezzo del nuovo listino, hold fino alla scadenza della prelazione. La postazione
  resta riservata dalle allocazioni (vincoli del database, ADR 0003), non da controlli applicativi. Le tabelle
  `renewal_campaigns` / `renewal_proposals` tengono solo il ciclo di vita (una proposta aperta per abbonamento). La
  conferma arriva solo dal webhook del provider o dal driver manuale (accettazione dal pannello). Il link al cliente
  è il link magico della prenotazione proposta.
- **Rinnovi, dettagli di realizzazione**: il servizio prenotazioni limita a 24 ore l'hold di una nuova prenotazione;
  la proposta nasce con quel limite e il suo hold (prenotazione e allocazioni) è prorogato alla scadenza della
  prelazione nella stessa transazione (`booking.ExtendHold`), senza toccare `inventory.MaxHoldTTL`. Il periodo del
  nuovo abbonamento è l'intera stagione di destinazione quando quello di origine arrivava a fine stagione, altrimenti
  le date spostate sull'anno (`renewal.PassInterval`). La proposta è creata con `force` (le finestre di anticipo della
  policy non valgono per l'offerta del gestore; le violazioni restano nell'audit). L'uso del credito nell'accettazione
  richiede la porta opzionale `renewal.CreditApplier` (servizio crediti).
- **Lista d'attesa** (pacchetto `waitlist`): ordine `priority`, poi data di iscrizione; l'offerta è un hold riservato
  (`waitlist_offers.hold_id`, un'offerta attiva per iscrizione) creato da un sink degli eventi di rilascio; l'accettazione
  converte l'hold in prenotazione con sessione di checkout di M4. Link personale con token salvato come hash.
- **Crediti** (pacchetto `credit`): si estende `customer_credits` (fonti `goodwill`, `expiry`, `adjustment`, segno per
  fonte, `reference_credit_id`) e si aggiunge `customer_credit_balances` con `CHECK (balance >= 0)` aggiornata con
  UPDATE condizionale nella stessa transazione di ogni movimento: il saldo non può diventare negativo nemmeno in
  concorrenza. La scadenza è materializzata con righe `expiry` (una per credito), consumo FIFO per data di scadenza;
  validità dei crediti da policy (`credit_validity_days`).
- **Crediti nel checkout pubblico**: solo con identità verificata (link magico della prenotazione, accettazione di un
  rinnovo o di un'offerta). Il checkout anonimo non mostra né usa il saldo (l'email non è verificata: nessuna
  enumerazione dei crediti).
- **Lista d'attesa e crediti, dettagli di realizzazione**: l'hold di un'offerta usa il canale dell'iscrizione (quota
  online per `online`/`api`, ADR 0011) e un limite dedicato `inventory.MaxOfferHoldTTL` (7 giorni); le risorse sono
  scelte dalla disponibilità (prima le postazioni liberate dall'evento), ma decide sempre il database (un tentativo per
  transazione). Il link della lista d'attesa è ruotato da ogni email che lo porta (vale l'ultimo) e nella coda email è
  salvato solo cifrato con il portachiavi. Il servizio clienti, porta `Credits` di pagamenti e abbonamenti, delega al
  registro crediti (`customer.UseCreditLedger`), quindi ogni movimento aggiorna il saldo senza ricablare quei servizi.
  Gli storni di crediti già spesi sono limitati al saldo residuo. Il pagamento con credito di una prenotazione `held`
  (rinnovo) può confermarla tramite il driver manuale, come un pagamento in cassa.

- **Integrazione** (integratore di M5 parte B): la proposta accettata online diventa `converted` appena la
  prenotazione è confermata, tramite il sink `renewal` su `booking.confirmed` (stessa transizione idempotente del job
  `renewals.sync`, che resta come recupero): senza, il cliente tornato dal pagamento vedeva "accettato, da pagare" fino
  a un minuto. Le iscrizioni alla lista d'attesa create dal pannello hanno canale `desk` e quindi la policy della cassa
  (di solito pagamento in stabilimento); quelle dalla pagina pubblica seguono la policy e il pagamento online. Il link
  della lista d'attesa è `/b/{slug}/m/w/{token}`, sotto `/m/` per avere gli stessi header `no-referrer`/`no-store`
  dell'area di gestione. L'anonimizzazione svuota `renewal_proposals.decline_reason` insieme agli altri testi liberi.

## Conseguenze

- Positive: nessuna nuova dipendenza; disponibilità sempre garantita dal database; il ciclo di accettazione riusa
  prenotazioni, hold, link magici, gateway e outbox esistenti.
- Negative: gli hold lunghi dei rinnovi e delle offerte superano `inventory.MaxHoldTTL` (24 h) e richiedono un limite
  dedicato; la tabella dei saldi duplica un aggregato (invariante saldo = somma dei movimenti da verificare nei test).
- Verifiche: `TestMigration00008_ModelsRoundtrip`, `TestMigration00008_CreditBackfillAndDown` (SQLite e PostgreSQL);
  i builder aggiungono gli invarianti in `docs/invariants.md` con i test di concorrenza (`TestConcurrency_*`).
