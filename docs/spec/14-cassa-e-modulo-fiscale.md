## 14. Cassa e modulo fiscale

### 14.1 Cassa

- Apertura sessione con fondo cassa, registrazione degli incassi per metodo, chiusura con conteggio e differenza.
- Incasso rapido dalla mappa: importo proposto = saldo dovuto, metodo, eventuale nota.
- Vendita libera senza prenotazione (lettino extra al volo, giornaliero senza anagrafica con cliente generico "Cliente cassa").
- Report di chiusura stampabile in PDF.

### 14.2 Modulo fiscale

L'emissione del documento commerciale dipende dal dispositivo e dalla configurazione del gestore. Il core non implementa protocolli specifici dei registratori nell'MVP, ma predispone l'architettura.

```go
type FiscalDriver interface {
    Code() string
    Issue(ctx context.Context, doc FiscalDocumentRequest) (*FiscalDocumentResult, error)
    Void(ctx context.Context, ref string) error
    Status(ctx context.Context) (DeviceStatus, error)
}
```

Driver previsti:
- **`none`:** nessuna emissione, il gestore usa il proprio registratore separatamente (default MVP).
- **`export`:** esportazione giornaliera dei corrispettivi per aliquota e metodo in CSV e PDF, per il commercialista.
- **Driver per registratori telematici:** contributi successivi, uno per famiglia di protocollo.

Le righe del documento derivano dallo snapshot del prezzo (scorporo IVA per aliquota). Ogni emissione o errore è tracciato in `fiscal_documents`. La conformità normativa della configurazione resta responsabilità del gestore e del suo consulente: la documentazione lo indica chiaramente.

---

