# App desktop: rete locale e modalità remota

L'app desktop di Arenile (spec 29.1) funziona in due modi alternativi, che non si sincronizzano tra loro:

- **Locale**: il server gira sul PC della cassa, senza internet. Con la **modalità rete locale** gli altri PC, i
  tablet e i telefoni della Wi-Fi del lido usano lo stesso archivio dal browser (e, da M7, dall'app mobile).
- **Remota**: l'app non avvia un server; mostra il pannello di un'istanza esistente (VPS, server in sede, anche
  raggiungibile solo via VPN) e fa da **ponte fiscale** verso il registratore telematico collegato al PC della cassa.

Per passare da Locale a Remota: backup dall'app, `arenile restore --in FILE` sul server, poi cambio di modalità
nell'app.

## Modalità rete locale

1. Nelle impostazioni dell'app desktop (o dal menu nella barra di sistema) attiva **Modalità rete locale**. La
   porta predefinita è 8443.
2. Il menu **Mostra QR rete locale** apre la pagina *Configurazione → Dispositivi → Rete locale* con l'indirizzo
   (per esempio `https://192.168.1.20:8443/admin/`), l'impronta SHA-256 del certificato e il pulsante per abbinare
   un dispositivo.
3. Dagli altri dispositivi apri l'indirizzo nel browser. La prima volta il browser avvisa che la connessione non è
   privata: il certificato è creato dal PC della cassa, non da un'autorità pubblica. Apri i dettagli del
   certificato, confronta l'impronta con quella della pagina e, solo se coincide, prosegui.
4. L'app mobile e l'app desktop in modalità remota si abbinano con il QR: il QR contiene anche l'impronta, quindi
   le app verificano il certificato da sole e non mostrano avvisi.

Cosa sapere:

- **Sicurezza.** Sono accettate solo le connessioni dalla rete locale (indirizzi privati `10.x`, `172.16–31.x`,
  `192.168.x`, link-local e IPv6 ULA). Tutto il resto viene chiuso prima della cifratura, anche se il router
  inoltra la porta per errore o il PC ha un indirizzo IPv6 pubblico. Il controllo è ripetuto su ogni richiesta.
- **Certificato.** È un certificato ECDSA valido 825 giorni, conservato nella cartella dati (`lan/`). Non cambia ai
  riavvii né se cambia l'indirizzo IP; viene rinnovato 30 giorni prima della scadenza (bisogna allora riabbinare le
  app e riaccettarlo nei browser). Da 60 giorni prima del rinnovo la pagina *Rete locale*, il menu nella barra di
  sistema e una notifica all'avvio indicano la data, così il rinnovo si pianifica in un giorno di chiusura.
- **Indirizzo fisso.** Riserva nel router un indirizzo IP fisso per il PC della cassa: l'indirizzo del pannello
  resta lo stesso. Il nome `<nome-pc>.local` funziona sulle reti dove il sistema operativo lo annuncia (macOS,
  Windows 10 e successivi); Arenile non installa un servizio mDNS proprio.
- **Doppie prenotazioni.** Il server resta uno solo: le garanzie della sezione 7 non cambiano.

## Modalità remota

1. Nel pannello dell'istanza apri *Configurazione → Dispositivi → Abbina un dispositivo*.
2. Nell'app desktop scegli **Remota** e inserisci l'indirizzo dell'istanza e il codice (oppure incolla il testo
   del QR). L'app verifica l'istanza, riceve un **token per dispositivo** e lo conserva nel portachiavi del sistema
   operativo (o, se non disponibile, in un file con permessi ristretti nella cartella dati).
3. Il pannello si apre in una finestra dedicata. Opzioni nel file `remote.json` della cartella dati: `kiosk`
   (schermo intero su Edge/Chrome), `autostart`, `fiscal_bridge`, `fiscal_driver`.

L'indirizzo deve usare `https`. Se l'istanza usa un certificato autofirmato (per esempio un'altra app desktop in
modalità rete locale), il QR ne porta l'impronta e l'app la verifica; la finestra passa da un proxy locale su
`127.0.0.1` perché i componenti web di sistema non sanno verificare un'impronta.

Il dispositivo compare nell'elenco *Dispositivi* dell'istanza e si revoca da lì: alla revoca l'app rimuove il token
e chiede di abbinarla di nuovo.

## Ponte fiscale

Con la modalità remota il registratore telematico resta collegato al PC della cassa, che il server online non può
raggiungere. Lo stabilimento usa il driver fiscale `bridge` (`settings.fiscal.driver = "bridge"`):

- a ogni incasso il server crea il documento fiscale "in attesa" e mette un lavoro in coda;
- l'app della cassa tiene aperta una connessione in uscita verso il server (nessuna porta da aprire sul PC),
  riceve il lavoro, lo esegue sul driver locale e invia l'esito;
- il documento diventa emesso con il numero del registratore, oppure in errore con il messaggio del dispositivo.

Se la connessione cade a metà, il lavoro viene riconsegnato e l'app reinvia l'esito già ottenuto senza stampare
una seconda volta. Gli errori temporanei (carta finita, registratore spento) rimettono il lavoro in coda con attese
crescenti, fino a 5 tentativi. Se il PC della cassa è spento, i lavori aspettano sul server.

Oggi è disponibile il driver locale `noop` (registra l'esito senza dispositivo, come il driver `none` del server); i
driver per i registratori telematici sono contributi successivi (vedi la [nota fiscale](fiscale.md)). Stampante
delle ricevute e lettore QR non passano ancora dal ponte: un lettore QR USB in modalità tastiera funziona già nella
finestra del pannello.
