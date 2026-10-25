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
| [0012](0012-kit-mappa.md) | Kit di illustrazioni della mappa: `<symbol>`/`<use>`, stato sovrapposto, campi `asset`/`variant`, provenienza | accettato, in parte sostituito da [0024](0024-kit-mappa-v3-e-scelta-del-kit.md) |
| [0013](0013-scelte-m4.md) | Scelte di M4: percorsi pubblici e sessione di checkout, webhook, cliente provvisorio, link magici, `payment_intents`, capacità facoltative dei driver, Stripe senza SDK, email | accettato |
| [0014](0014-pagamenti-online-m4.md) | Pagamenti online di M4: SDK Stripe ufficiale, webhook unico percorso di conferma, pagamenti tardivi e doppi rimborsati, rimborsi prenotati e registrati alla conferma, riconciliazione per intent | accettato |
| [0015](0015-scelte-m6.md) | Scelte di M6: `/meta` pubblico, token per dispositivo ristretti allo stabilimento, abbinamento con QR, API desktop solo da loopback, ponte fiscale con long polling, launcher headless senza tag e tray con il tag `desktop` (CGO solo su macOS e Windows) | accettato |
| [0016](0016-scelte-m5a.md) | Scelte di M5 (parte A): report come query e confronto anni, chiavi API con scadenza, webhook configurabili con rotazione del segreto, PWA offline senza nuove dipendenze, mare in alto e riorientamento della mappa | accettato |
| [0017](0017-scelte-m5b.md) | Scelte di M5 (parte B): passaggio di stagione idempotente, rinnovi come prenotazioni held in prelazione, lista d'attesa con hold riservati, crediti con saldo vincolato dal database | accettato |
| [0018](0018-marchio.md) | Marchio ufficiale Arenile (brand kit v2.0 rifinito, regole d'uso colore/bianco/navy) e Coste360.it: asset in `assets/brand` esclusi dalla licenza del codice, testo in tracciati senza font a runtime, tema shadcn sui colori del kit, componenti condivisi del marchio | accettato |
| [0019](0019-funzioni-rinviate.md) | Funzioni rinviate: modalità prova, rimborso garantito, azioni dalla mappa con If-Match obbligatorio, più casse e chiusura Z in HTML stampabile (nessuna libreria PDF), prezzo manuale degli abbonamenti, notifiche, tentativi webhook, XLSX, importazione CSV | accettato |
| [0020](0020-vendita-giornaliera-e-telefono-facoltativo.md) | Vendita giornaliera con biglietti numerati per giorno (contatore atomico e vincolo unico), vendita per zona sui vincoli esistenti, biglietti d'ingresso su zone quantity, telefono facoltativo ovunque | accettato |
| [0021](0021-stampa-escpos-di-rete.md) | Stampa ESC/POS su stampanti di rete (Wi-Fi/Ethernet, porta 9100) solo nell'edizione desktop in v1: codificatore e modelli lato server, instradamento per cassa, lavori di stampa idempotenti con riprova, interfaccia Driver per i ponti futuri | accettato |
| [0022](0022-modelli-email-e-promemoria.md) | Testi delle email ai clienti modificabili (oggetto, apertura, chiusura, firma; segnaposto sicuri, blocchi fissi, solo le personalizzazioni nel database) e promemoria del giorno prima e del saldo come job in processo, una volta per prenotazione | accettato |
| [0023](0023-revisione-concessione-d-uso.md) | Revisione legale della concessione d'uso: servizi a terzi e prodotti sostitutivi distinti, widget e installazione in un clic consentiti, consorzi, hotel, piscine all'aperto, aiuto gratuito | accettato |
| [0024](0024-kit-mappa-v3-e-scelta-del-kit.md) | Kit mappa v3 "Moderno" predefinito e "Classico" (v2.1) come alternativa, scelti per stabilimento nel tema (`theme.map_kit`, nessuna migrazione); generatore multi-kit con regole per profilo; stato sul disegno con `--beach-state`, palette degli stati per tema, barra sul bloccato | accettato |
| [0025](0025-avviso-nuova-versione-acceso-di-default.md) | Avviso di nuova versione acceso di default su tutte le edizioni (`ARENILE_UPDATE_CHECK`, impostazione desktop): un GET al giorno di un file pubblico di Coste360 con solo versione, edizione e sistema nell'User-Agent, nessun identificativo, non è telemetria; esito fallito mai mostrato come errore | accettato |
| [0026](0026-verifica-aggiornamenti-manuale.md) | Verifica aggiornamenti su richiesta (`POST /api/admin/v1/system/update/check`, menu dell'app desktop): al massimo un controllo di rete al minuto e mai due insieme, nessuna chiamata con il controllo spento; `pending` nello stato perché il pannello rilegga presto lo stato dopo l'avvio | accettato |
| [0027](0027-abbinamento-demo-app-mobile.md) | Abbinamento allo stabilimento dimostrativo (`POST /api/device/v1/demo-pairing`): codice per l'utente demo senza login, solo con `ARENILE_DEMO_PAIRING_*`, 5 all'ora per IP, `404` altrove; unica eccezione dell'app al "nessun server predefinito" | accettato |

## Come aggiungere un ADR

1. Copia [`template.md`](template.md) in `NNNN-titolo-in-kebab-case.md` con il numero successivo.
2. Compila contesto, decisione e conseguenze in italiano, in modo conciso.
3. Aggiungi la riga all'indice qui sopra.
4. Se sostituisci un ADR, aggiorna lo stato del precedente e collega i due documenti.
