## 16. Utenti, ruoli e autenticazione

### 16.1 Ruoli

| Permesso | owner | manager | cashier | beach_staff | viewer |
|---|:-:|:-:|:-:|:-:|:-:|
| Configurazione stabilimento, prezzi, policy | ✔ | ✔ | | | |
| Utenti e ruoli | ✔ | | | | |
| Pagamenti e credenziali provider | ✔ | | | | |
| Creare e modificare prenotazioni | ✔ | ✔ | ✔ | | |
| Incassi e cassa | ✔ | ✔ | ✔ | | |
| Rimborsi | ✔ | ✔ | | | |
| Check-in, rilasci, blocchi postazione | ✔ | ✔ | ✔ | ✔ | |
| Vista mappa e prenotazioni | ✔ | ✔ | ✔ | ✔ | ✔ |
| Report ed esportazioni | ✔ | ✔ | | | ✔ |

I permessi sono definiti come costanti nel codice e verificati nel servizio di dominio, non solo nell'handler HTTP.

### 16.2 Autenticazione

- Login email e password con Argon2id; sessione con cookie `HttpOnly`, `Secure`, `SameSite=Lax`; rotazione del token al login.
- TOTP opzionale, obbligatorio per `owner` se attivato a livello di installazione.
- Reset password via email con token monouso a 30 minuti.
- Limitazione dei tentativi per IP e per account.
- Interfaccia `ext.AuthProvider` per aggiungere SSO tramite estensioni (es. OIDC).
- Chiavi API per integrazioni server-to-server, con scope e prefisso visibile (`ar_live_xxxx`).
- CLI: `arenile admin create --email ... --org ...` per il primo utente.

---

