## 17. API

### 17.1 Convenzioni

- REST JSON, versione nel percorso: `/api/admin/v1`, `/api/public/v1`.
- Contratto in `api/openapi.yaml`, unica fonte di verità; codice server e client TypeScript generati.
- Errori in formato `application/problem+json` (RFC 9457) con `type` stabile, ad esempio `spot_unavailable`, `no_price`, `policy_violation`, `hold_expired`.
- Paginazione a cursore (`?cursor=&limit=`).
- `Idempotency-Key` accettato su tutti i `POST` che creano risorse o pagamenti.
- Concorrenza ottimistica con `ETag` e `If-Match` su configurazione e mappe.
- Date di calendario come `YYYY-MM-DD` nel fuso dello stabilimento; istanti in RFC 3339 UTC.

### 17.2 API pubblica (selezione)

| Metodo | Percorso | Descrizione |
|---|---|---|
| GET | `/api/public/v1/{slug}` | Dati dello stabilimento, tema, lingue, configurazione del flusso |
| GET | `/api/public/v1/{slug}/calendar?month=` | Disponibilità sintetica per giorno |
| GET | `/api/public/v1/{slug}/maps/{id}` | Geometria della mappa (senza dati sensibili) |
| GET | `/api/public/v1/{slug}/availability` | Disponibilità dettagliata (7.8) |
| GET | `/api/public/v1/{slug}/catalog` | Pack ed extra vendibili online |
| POST | `/api/public/v1/{slug}/quote` | Calcolo prezzo e policy |
| POST | `/api/public/v1/{slug}/bookings` | Crea prenotazione con hold |
| PATCH | `/api/public/v1/{slug}/bookings/{id}` | Dati cliente, extra, codice promozionale |
| POST | `/api/public/v1/{slug}/bookings/{id}/pay` | Avvia pagamento |
| GET | `/api/public/v1/{slug}/bookings/{id}` | Stato (con token di sessione) |
| DELETE | `/api/public/v1/{slug}/bookings/{id}` | Abbandono, rilascia l'hold |
| GET | `/api/public/v1/{slug}/manage/{token}` | Prenotazione dal link magico |
| POST | `/api/public/v1/{slug}/manage/{token}/cancel` | Cancellazione secondo policy |
| POST | `/api/public/v1/{slug}/manage/{token}/releases` | Rilascio giorni (abbonati) |
| POST | `/api/public/v1/{slug}/waitlist` | Iscrizione lista d'attesa |

Limiti: 60 richieste al minuto per IP sulle letture, 10 al minuto sulle creazioni; protezione aggiuntiva opzionale con challenge invisibile (interfaccia `ext.BotProtection`, nessun servizio imposto).

### 17.3 API di amministrazione (gruppi)

- `establishments`, `seasons`, `rate-periods`, `closure-days`
- `maps`, `zones`, `spots`, `decorations`, `pools`
- `components`, `packs`, `extras`
- `price-rules`, `fixed-prices`, `promo-codes`, `agreements`, `pricing/simulate`, `pricing/matrix`, `pricing/coverage`
- `policies`
- `bookings` (+ `assign`, `move`, `check-in`, `cancel`, `approve`, `reject`), `assignments/auto`
- `season-passes`, `releases`, `renewals`
- `customers`, `credits`, `waitlist`
- `payments`, `refunds`, `cash-sessions`, `fiscal-documents`
- `reports/*`, `exports/*`
- `users`, `memberships`, `api-keys`, `audit-log`
- `webhooks`, `notification-templates`
- `stream` (SSE)

---

