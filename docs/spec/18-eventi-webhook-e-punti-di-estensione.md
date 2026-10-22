## 18. Eventi, webhook e punti di estensione

### 18.1 Eventi di dominio

Scritti nell'outbox nella stessa transazione della modifica, poi distribuiti dal job runner.

```
booking.created          booking.held            booking.confirmed
booking.modified         booking.cancelled       booking.checked_in
booking.no_show          booking.completed       booking.approval_requested
allocation.released      season.day_released     season.renewal_offered
payment.succeeded        payment.failed          payment.refunded
customer.created         customer.anonymized
cash_session.closed      config.pricing_changed  config.map_changed
```

Formato:

```json
{
  "id": "evt_01J...",
  "type": "booking.confirmed",
  "establishment_id": "...",
  "occurred_at": "2027-07-01T09:12:44Z",
  "data": { "booking": { "...": "..." } },
  "version": 1
}
```

### 18.2 Webhook in uscita

- Configurabili per stabilimento, con filtro sugli eventi.
- Firma `Arenile-Signature: t=<timestamp>,v1=<HMAC-SHA256>` sul corpo con il segreto dell'endpoint.
- Tentativi con backoff esponenziale fino a 24 ore; disattivazione automatica dopo errori ripetuti, con avviso al proprietario.
- Pagina di log delle consegne con possibilità di reinvio.

### 18.3 Punti di estensione Go (`pkg/ext`)

```go
type Extensions struct {
    PaymentProviders  []PaymentProvider
    FiscalDrivers     []FiscalDriver
    EventSinks        []EventSink            // consumatori interni degli eventi
    ConcessionData    ConcessionDataProvider // perimetro, satellite, scadenze
    TenantResolver    TenantResolver         // risolve lo stabilimento da host/header
    AuthProviders     []AuthProvider
    Notifiers         []Notifier             // canali aggiuntivi (WhatsApp, SMS)
    BotProtection     BotProtection
    HTTPRoutes        func(r chi.Router)     // rotte aggiuntive (es. billing)
    AdminUIPlugins    []UIPluginManifest     // voci di menu e pagine extra nel pannello
}

func main() {
    app := arenile.New(config.FromEnv(), arenile.WithExtensions(ext))
    app.Run()
}
```

Il binario ufficiale usa un `Extensions` con le sole implementazioni del core. Chi vuole aggiungere funzionalità (integratori, fornitori di hosting, sviluppatori di driver) crea un proprio `main` che importa il modulo e registra le proprie implementazioni.

**Plugin UI del pannello:** un manifest dichiara voci di menu e un URL di pagina, caricata in un'area dedicata del pannello (iframe con token di sessione a breve durata). Così un'estensione può aggiungere pagine al pannello senza modificare il frontend del core.

---

