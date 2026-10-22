## 12. Pagina pubblica e widget incorporabile

### 12.1 URL e modalità di accesso

- Pagina: `https://{host}/b/{slug}` con parametri opzionali `?lang=en&from=2027-07-10&to=2027-07-12&pack=base&embed=1`.
- **Iframe:**
  ```html
  <iframe src="https://{host}/b/lido-x?embed=1"
          style="width:100%;border:0" allow="payment" loading="lazy"
          title="Prenota il tuo posto"></iframe>
  ```
- **Script** (consigliato, gestisce altezza e parametri):
  ```html
  <div data-arenile="lido-x" data-lang="it"></div>
  <script src="https://{host}/embed.js" async></script>
  ```
  `embed.js` (meno di 3 KB) crea l'iframe, ascolta i messaggi `postMessage` di tipo `arenile:resize`, `arenile:booked`, `arenile:navigate`, e rilancia un `CustomEvent` sulla pagina ospitante per chi vuole tracciare le conversioni.

### 12.2 Sicurezza dell'incorporamento

- Header `Content-Security-Policy: frame-ancestors` costruito da `allowed_embed_origins` dello stabilimento (vuoto = solo stesso dominio e link diretto).
- I messaggi `postMessage` in uscita usano come destinazione l'origine verificata del contenitore; quelli in ingresso vengono accettati solo da origini autorizzate.
- Nessuna dipendenza da cookie di terze parti: lo stato del checkout è legato a un token di sessione salvato in `sessionStorage` (con fallback in memoria) e inviato come header.

### 12.3 Flusso utente

**Passo 1 — Quando**
- Calendario con disponibilità sintetica (giorni pieni disabilitati, periodi tariffari evidenziati se il gestore lo abilita).
- Scelta della formula consentita e della fascia.

**Passo 2 — Dove**
- Se almeno una zona online è `seat`: mappa con postazioni disponibili evidenziate, legenda dei prezzi per fila.
- Se solo `zone`: schede per zona con prezzo "da" e disponibilità indicativa ("ultimi 3 posti").
- Se solo `quantity`: selettore di quantità.
- Le modalità miste mostrano mappa e schede insieme.

**Passo 3 — Cosa**
- Pack compatibili con la scelta, composizione chiara (icone), extra con quantità.
- Riepilogo del prezzo sempre visibile e aggiornato a ogni modifica (chiamata `quote` con debounce).
- Campo codice promozionale.

**Passo 4 — Chi e pagamento**
- Nome, cognome, email, telefono; nessun account.
- Accettazione delle condizioni (testo dal gestore, versionato) e consenso marketing separato e facoltativo.
- Countdown visibile della validità del blocco.
- Pagamento o conferma.

**Conferma**
- Codice prenotazione, QR, riepilogo, pulsante "aggiungi al calendario" (file `.ics`), link di gestione inviato via email.

### 12.4 Area gestione prenotazione del cliente

Raggiungibile dal link magico (`/b/{slug}/m/{token}`):
- visualizza dettagli e QR;
- paga il saldo;
- modifica o cancella entro i limiti della policy, con anteprima del rimborso o credito;
- per gli abbonati: rilascia singoli giorni.

Il token è casuale (256 bit), salvato come hash, con validità fino alla fine della prenotazione più 30 giorni. È possibile richiedere un nuovo link inserendo email e codice prenotazione.

### 12.5 Tema

```json
{
  "logo_url": "...",
  "primary_color": "#0E6BA8",
  "accent_color": "#F2B134",
  "font_family": "Inter | system | serif",
  "radius": "none | sm | md | lg",
  "show_prices_on_map": true,
  "hide_header_in_embed": true,
  "custom_texts": {"it": {"intro": "...", "terms": "..."}}
}
```

Il tema viene applicato con variabili CSS; l'anteprima nel pannello è la pagina reale in modalità prova.

### 12.6 Requisiti non funzionali della pagina pubblica

- Primo caricamento: meno di 150 KB di JS gzip; LCP sotto 2,5 secondi su 4G.
- Funzionamento completo da 320 px di larghezza.
- Contrasto WCAG AA con qualsiasi colore primario scelto: il sistema calcola automaticamente il colore del testo sopra il colore primario.
- Pagina indicizzabile quando non in modalità `embed` (titolo, descrizione, dati strutturati `LocalBusiness`).

---

