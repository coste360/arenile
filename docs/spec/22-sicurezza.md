## 22. Sicurezza

- HTTPS obbligatorio in produzione; il binario può fare da terminatore TLS con certificati automatici (`autocert`) oppure stare dietro un reverse proxy.
- Header di sicurezza: CSP restrittiva sul pannello, `frame-ancestors` per stabilimento sulla pagina pubblica, `X-Content-Type-Options`, `Referrer-Policy`, HSTS.
- CSRF: cookie `SameSite=Lax` più token double-submit sulle richieste che modificano dati del pannello.
- **Segreti cifrati a riposo** (chiavi Stripe, segreti dei webhook, TOTP) con AES-GCM e chiave da `ARENILE_SECRET_KEY`; rotazione supportata con più chiavi attive.
- Isolamento per stabilimento verificato a livello di repository: ogni query operativa richiede `establishment_id` dal contesto autenticato, con test automatici che tentano accessi incrociati.
- Limitazione delle richieste su login, API pubblica e creazione prenotazioni.
- Caricamento file (loghi, sfondi): solo immagini, verifica del tipo reale, ridimensionamento lato server, rimozione dei metadati EXIF.
- Dipendenze controllate in CI (`govulncheck`, `npm audit`), immagine Docker scansionata (Trivy).
- Policy di segnalazione delle vulnerabilità in `SECURITY.md`.

---

