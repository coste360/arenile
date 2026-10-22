## 13. Pagamenti

### 13.1 Interfaccia

```go
package ext

type PaymentProvider interface {
    Code() string
    Capabilities() PaymentCapabilities // online, refunds, partial refunds, wallets

    // Crea un'intenzione di pagamento e restituisce i dati per il client.
    CreatePayment(ctx context.Context, req CreatePaymentRequest) (*CreatePaymentResult, error)

    // Verifica e interpreta un webhook del provider.
    ParseWebhook(ctx context.Context, r *http.Request) (*PaymentEvent, error)

    Refund(ctx context.Context, req RefundRequest) (*RefundResult, error)
}

type CreatePaymentRequest struct {
    EstablishmentID string
    BookingID       string
    Amount          int64
    Currency        string
    Description     string
    CustomerEmail   string
    ReturnURL       string
    IdempotencyKey  string
    Metadata        map[string]string
}

type CreatePaymentResult struct {
    ProviderRef  string
    Mode         string // "redirect" | "embedded"
    RedirectURL  string
    ClientSecret string
}
```

I provider si registrano nel registry dell'applicazione; ogni stabilimento sceglie il provider attivo e salva le credenziali cifrate (sezione 22).

### 13.2 Driver inclusi nel core

- **`manual`:** nessun pagamento online; la prenotazione va in `confirmed` (o `awaiting_approval`) e il saldo viene incassato alla cassa.
- **`stripe`:** chiavi dell'account del gestore (`publishable`, `secret`, `webhook secret`).
  - Modalità predefinita: **Stripe Checkout** in redirect a livello di finestra principale (`target=_top` dal widget, oppure nuova scheda se il contenitore lo impedisce). È la soluzione più robusta dentro gli iframe.
  - Modalità opzionale: **Payment Element** incorporato, abilitabile dal gestore, testata su Safari con `allow="payment"`.
  - Webhook gestiti: `checkout.session.completed`, `payment_intent.succeeded`, `payment_intent.payment_failed`, `charge.refunded`.
  - Idempotenza: `IdempotencyKey = booking_id + tentativo`.
  - Metadata: `establishment_id`, `booking_id`, `booking_code`.

### 13.3 Gestione dei webhook

- Endpoint: `POST /webhooks/payments/{provider}/{establishment_id}`.
- Verifica della firma obbligatoria.
- Ogni evento viene registrato con l'ID del provider in una tabella di deduplica: lo stesso evento non viene mai processato due volte.
- L'aggiornamento di `payments`, `bookings` e allocazioni avviene in un'unica transazione.
- Riconciliazione: un job ogni 15 minuti interroga il provider per i pagamenti `pending` più vecchi di 10 minuti, per coprire webhook persi.

### 13.4 Rimborsi e crediti

- I rimborsi calcolati dalla policy vengono eseguiti tramite `Refund` se il provider lo supporta, altrimenti registrati come rimborso manuale da completare.
- I crediti sono un saldo per cliente e stabilimento (`customer_credits`: amount, source, expires_on), utilizzabile come metodo di pagamento in prenotazioni successive.

---

