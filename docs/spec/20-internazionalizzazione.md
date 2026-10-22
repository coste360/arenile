## 20. Internazionalizzazione

- Lingue incluse: italiano (fonte), inglese, tedesco, francese. Struttura pronta per contributi della community.
- Testi dell'interfaccia in file JSON per i18next; testi del backend (email, PDF) in file `go-i18n`.
- Contenuti del gestore (nomi di pack, zone, condizioni) come campi `json i18n` con fallback alla lingua predefinita dello stabilimento.
- Formati di data, numero e valuta con `Intl` lato client e `golang.org/x/text` lato server.
- La lingua della pagina pubblica è scelta da parametro, poi dalla lingua del browser, poi dal default dello stabilimento.

---

