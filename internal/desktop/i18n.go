package desktop

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Locale of the launcher's own strings (tray menu, notifications, remote
// pairing form). The panel has its own i18n; these strings live outside
// the browser. Italian is the source language (AGENTS.md); English is the
// second locale, chosen when the operating system UI language is English.
type Locale string

// Supported locales.
const (
	LocaleIT Locale = "it"
	LocaleEN Locale = "en"
)

var currentLocale atomic.Value

// SetLocale selects the locale of T (default LocaleIT).
func SetLocale(l Locale) {
	if l != LocaleEN {
		l = LocaleIT
	}
	currentLocale.Store(l)
}

// CurrentLocale returns the locale selected by SetLocale.
func CurrentLocale() Locale {
	if l, ok := currentLocale.Load().(Locale); ok {
		return l
	}
	return LocaleIT
}

// T returns the message for key in the current locale, formatted with args
// when given (pure apart from the locale). An unknown key is returned as
// is, so a missing translation is visible instead of silent.
func T(key string, args ...any) string {
	m, ok := messages[key]
	if !ok {
		return key
	}
	s := m[0]
	if CurrentLocale() == LocaleEN && m[1] != "" {
		s = m[1]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// LocaleFromTag maps an OS language tag ("it_IT.UTF-8", "en-US", "en") to
// a supported locale (pure). Unknown languages and "C"/"POSIX" report
// ok=false so the caller keeps the default.
func LocaleFromTag(tag string) (Locale, bool) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" || tag == "c" || tag == "posix" {
		return "", false
	}
	lang := tag
	if i := strings.IndexAny(lang, "_-.@"); i >= 0 {
		lang = lang[:i]
	}
	switch lang {
	case "it":
		return LocaleIT, true
	case "en":
		return LocaleEN, true
	}
	return "", false
}

// DetectLocale picks the launcher locale from the environment (LC_ALL,
// LC_MESSAGES, LANG, LANGUAGE: the POSIX precedence) and, when they say
// nothing, from the operating system (platformLocaleTags: Windows UI
// languages, macOS AppleLocale). Italian when nothing matches.
func DetectLocale(getenv func(string) string) Locale {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		for _, tag := range strings.Split(getenv(k), ":") {
			if l, ok := LocaleFromTag(tag); ok {
				return l
			}
		}
	}
	for _, tag := range platformLocaleTags() {
		if l, ok := LocaleFromTag(tag); ok {
			return l
		}
	}
	return LocaleIT
}

// messages: key -> {Italian, English}. Keys are grouped by owner.
var messages = map[string][2]string{
	// Tray menu (launcher.go).
	"menu.open":          {"Apri pannello", "Open panel"},
	"menu.backup":        {"Backup ora", "Back up now"},
	"menu.data_dir":      {"Apri cartella dati", "Open data folder"},
	"menu.autostart":     {"Avvio automatico all'accensione", "Start at login"}, //nolint:misspell // Italian label (spec 29.1)
	"menu.lan":           {"Modalità rete locale", "Local network mode"},
	"menu.lan_renewal":   {" — certificato in rinnovo il %s", " — certificate renewal on %s"}, //nolint:misspell // Italian
	"menu.lan_qr":        {"Mostra QR rete locale", "Show local network QR"},
	"menu.restore":       {"Ripristina un backup…", "Restore a backup…"},
	"menu.quit":          {"Esci", "Quit"},
	"menu.bridge_off":    {"Ponte fiscale: disattivato", "Fiscal bridge: off"},
	"menu.bridge_on":     {"Ponte fiscale: attivo (%s)", "Fiscal bridge: on (%s)"},
	"menu.switch_local":  {"Passa alla modalità locale (al prossimo avvio)", "Switch to local mode (at the next start)"},
	"menu.unpair":        {"Scollega questo PC dall'istanza", "Unpair this PC from the instance"},
	"menu.window_title":  {"Arenile — %s", "Arenile — %s"},
	"menu.pairing_title": {"Arenile — collega questo PC a un'istanza", "Arenile — pair this PC with an instance"},
	"menu.open_browser":  {"Apri nel browser", "Open in the browser"},

	// Application menu of the window on macOS (shell/menu.go).
	"app.about":       {"Informazioni su Arenile", "About Arenile"},
	"app.settings":    {"Impostazioni…", "Settings…"},
	"app.hide":        {"Nascondi Arenile", "Hide Arenile"},
	"app.hide_others": {"Nascondi altre", "Hide Others"},
	"app.show_all":    {"Mostra tutte", "Show All"},
	"app.quit":        {"Esci da Arenile", "Quit Arenile"},
	"app.edit":        {"Modifica", "Edit"},
	"app.undo":        {"Annulla", "Undo"},
	"app.redo":        {"Ripeti", "Redo"},
	"app.cut":         {"Taglia", "Cut"},
	"app.copy":        {"Copia", "Copy"},
	"app.paste":       {"Incolla", "Paste"},
	"app.select_all":  {"Seleziona tutto", "Select All"},
	"app.view":        {"Vista", "View"},
	"app.reload":      {"Ricarica", "Reload"},
	"app.zoom_reset":  {"Dimensioni reali", "Actual Size"},
	"app.zoom_in":     {"Ingrandisci", "Zoom In"},
	"app.zoom_out":    {"Rimpicciolisci", "Zoom Out"},
	"app.full_screen": {"Schermo intero", "Enter Full Screen"},
	"app.window":      {"Finestra", "Window"},
	"app.minimize":    {"Riduci", "Minimize"},
	"app.window_zoom": {"Zoom", "Zoom"},
	"app.close":       {"Chiudi finestra", "Close Window"},
	"app.front":       {"Porta tutto in primo piano", "Bring All to Front"},

	// Notifications (launcher.go, remote/mode.go).
	"notify.lan_not_started":   {"Modalità rete locale non avviata: controlla le impostazioni.", "Local network mode not started: check the settings."},
	"notify.daily_backup_fail": {"Backup giornaliero non riuscito: controlla la cartella dei backup.", "Daily backup failed: check the backup folder."},
	"notify.update":            {"È disponibile la versione %s: apri il pannello per scaricarla.", "Version %s is available: open the panel to download it."},
	"notify.fatal":             {"Arenile si è fermato per un errore: riaprilo. Dettagli nella cartella dati (logs).", "Arenile stopped because of an error: open it again. Details in the data folder (logs)."},
	"notify.restore_fail":      {"Ripristino non riuscito: i dati attuali non sono stati modificati.", "Restore failed: the current data was not changed."},
	"notify.restore_ok":        {"Ripristino completato: ricarica la pagina del pannello.", "Restore completed: reload the panel page."},
	"notify.backup_fail":       {"Backup non riuscito: controlla la cartella dei backup.", "Backup failed: check the backup folder."},
	"notify.backup_ok":         {"Backup completato: %s", "Backup completed: %s"},
	"notify.setting_fail":      {"Impostazione non modificata: %s", "Setting not changed: %s"},
	"notify.keyring_locked":    {"Portachiavi del sistema non disponibile o bloccato: sbloccalo (o consenti l'accesso) e riapri Arenile. I dati non sono stati toccati.", "System keychain unavailable or locked: unlock it (or allow the access) and open Arenile again. The data was not touched."},
	"notify.key_missing":       {"Chiave segreta dell'archivio non trovata: Arenile non riparte con una chiave nuova. Dettagli nella cartella dati (logs).", "Secret key of the database not found: Arenile does not start with a new key. Details in the data folder (logs)."},
	"notify.cert_renewal":      {"Il certificato della rete locale sarà rinnovato il %s: dopo quel giorno riabbina le app e riaccetta il certificato nei browser.", "The local network certificate will be renewed on %s: after that day pair the apps again and accept the certificate in the browsers."}, //nolint:misspell // Italian
	"notify.bridge_fail":       {"Ponte fiscale non avviato: %s", "Fiscal bridge not started: %s"},
	"notify.revoked":           {"Questo PC è stato scollegato dal pannello dell'istanza: abbinalo di nuovo.", "This PC was unpaired from the instance panel: pair it again."},
	"notify.switch_local":      {"Al prossimo avvio Arenile userà il server interno di questo PC.", "At the next start Arenile will use the internal server of this PC."},
	"notify.unpair_fail":       {"Scollegamento non riuscito: %s", "Unpairing failed: %s"},
	"notify.unpaired":          {"PC scollegato dall'istanza.", "PC unpaired from the instance."},
	"notify.download_ok":       {"Scaricato nella cartella Download: %s", "Saved to the Downloads folder: %s"},
	"notify.download_fail":     {"Download non riuscito: %s", "Download failed: %s"},
	"notify.pair_needed":       {"Modalità remota: abbina questo PC con il codice del pannello (Configurazione → Dispositivi).", "Remote mode: pair this PC with the code from the panel (Settings → Devices)."},

	// Errors shown to the operator (remote/mode.go).
	"err.not_paired":      {"modalità remota: questo PC non è abbinato a un'istanza (avvia con -pair oppure usa la build con finestra)", "remote mode: this PC is not paired with an instance (start with -pair or use the build with the window)"},
	"err.unreachable":     {"istanza non raggiungibile, revoca il dispositivo dal pannello: %v", "instance unreachable, revoke the device from the panel: %v"},
	"err.unexpected_code": {"risposta inattesa dell'istanza (%d)", "unexpected answer from the instance (%d)"},

	// Pairing form (remote/window_webview.go).
	"pair.title":       {"Collega questo PC", "Pair this PC"},
	"pair.intro":       {"Nel pannello dell'istanza apri Configurazione → Dispositivi, crea un codice di abbinamento e inseriscilo qui insieme all'indirizzo dell'istanza (oppure incolla il testo del QR).", "In the instance panel open Settings → Devices, create a pairing code and enter it here with the instance address (or paste the QR text)."},
	"pair.url":         {"Indirizzo dell'istanza", "Instance address"},
	"pair.code":        {"Codice di abbinamento", "Pairing code"},
	"pair.qr":          {"Oppure testo del QR", "Or the QR text"},
	"pair.name":        {"Nome di questo PC", "Name of this PC"},
	"pair.name_value":  {"PC cassa", "Cash desk PC"},
	"pair.submit":      {"Collega", "Pair"},
	"pair.in_progress": {"Collegamento in corso…", "Pairing…"},
}
