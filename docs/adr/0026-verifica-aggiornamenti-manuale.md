# 0026 — Verifica aggiornamenti su richiesta

- **Stato:** accettato
- **Data:** 2026-10-25
- **Sezioni della specifica:** `docs/spec/29-edizione-desktop-e-app-mobile.md` (29.1), ADR 0025 (punto 4 e
  alternativa scartata "Chiamata di rete durante la richiesta del pannello")

## Contesto

Con l'ADR 0025 il controllo della nuova versione gira solo in background (30 secondi dopo l'avvio, poi ogni 24 ore)
e nessuna richiesta del pannello aspetta la rete. I gestori però non trovavano dove vedere versione e stato, e
chi sa che è uscita una versione non aveva modo di controllare subito: doveva aspettare il giorno dopo o
riavviare. L'app desktop ha anche un menu (icona e menu **Arenile** su macOS) dove "Verifica aggiornamenti" è
un'azione attesa.

Vincoli dell'ADR 0025 che restano: nessuna chiamata con il controllo spento o su versioni non di release, nessun
identificativo, un esito fallito non è mai un errore. In più il server di Coste360 non deve ricevere raffiche di
richieste da clic ripetuti.

## Decisione

1. `POST /api/admin/v1/system/update/check` (stessi permessi di `GET /api/admin/v1/system/update`) esegue il
   controllo subito e risponde con lo stato aggiornato. È l'unica richiesta del pannello che aspetta la rete, ed
   è esplicita (pulsante **Verifica aggiornamenti**); `GET` continua a rispondere dalla copia in memoria.
2. Limite lato server: al massimo un controllo di rete al minuto (`update.ManualMinInterval`, conta anche quello
   automatico) e mai due insieme: chi chiede mentre un controllo è in corso ne attende l'esito. Entro il minuto
   la risposta viene dalla memoria, senza errori.
3. Con il controllo spento (`ARENILE_UPDATE_CHECK=false`, impostazione `update_check`) o con una versione non di
   release la risposta è `status: disabled` senza chiamate di rete (INV-98). Un controllo fallito è
   `status: unavailable` con `200`.
4. Il controllo manuale sposta anche la pianificazione automatica (il successivo automatico parte 24 ore dopo
   l'ultimo tentativo).
5. L'app desktop chiama lo stesso servizio in processo (`update.Service.Check`) dal menu e mostra l'esito come
   notifica; la notifica automatica di nuova versione resta una sola per versione e installazione
   (`notify-state.json` nella cartella dati).
6. `SystemUpdate` espone `pending` (primo controllo non ancora avvenuto): il pannello rilegge lo stato ogni 15
   secondi finché è vero, poi ogni ora.

## Conseguenze

- Positive: versione e aggiornamenti hanno un posto chiaro (pagina **Informazioni su Arenile**); il gestore può
  controllare subito; l'avviso compare pochi secondi dopo l'avvio senza ricaricare.
- Negative: una richiesta del pannello può durare fino al timeout del client di aggiornamento quando il server di
  Coste360 è lento; è solo quella esplicita del pulsante, che mostra "Verifica in corso…".
- Verifiche: `TestCheckNow_Scenarios`, `TestCheckNow_DevBuildNoNetwork`, `TestConcurrency_CheckNow`
  (`internal/domain/update`), `TestCheckSystemUpdate_Scenarios` (`internal/http/admin`),
  `TestLauncher_NotifyUpdate_PersistsAcrossRestarts`, `TestUpdateCheckMessage_Scenarios` (`internal/desktop`),
  `AboutPage.test.tsx`, `UpdateBanner.test.tsx`; invariante INV-98.
