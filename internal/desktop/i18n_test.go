package desktop

import (
	"strings"
	"testing"
)

func TestLocale_TagsAndDetection(t *testing.T) {
	t.Cleanup(func() { SetLocale(LocaleIT) })
	for tag, want := range map[string]Locale{
		"it_IT.UTF-8": LocaleIT, "it-IT": LocaleIT, "it": LocaleIT, "IT": LocaleIT,
		"en_US.UTF-8": LocaleEN, "en-GB": LocaleEN, "en": LocaleEN, "en_US@euro": LocaleEN,
	} {
		if got, ok := LocaleFromTag(tag); !ok || got != want {
			t.Errorf("LocaleFromTag(%q) = %q %v", tag, got, ok)
		}
	}
	for _, tag := range []string{"", "C", "POSIX", "de_DE.UTF-8", "fr"} {
		if _, ok := LocaleFromTag(tag); ok {
			t.Errorf("LocaleFromTag(%q) accepted", tag)
		}
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := DetectLocale(env(map[string]string{"LANG": "en_US.UTF-8"})); got != LocaleEN {
		t.Fatalf("LANG en: %q", got)
	}
	// LC_ALL wins over LANG; LANGUAGE lists fall back in order.
	if got := DetectLocale(env(map[string]string{"LC_ALL": "it_IT.UTF-8", "LANG": "en_US.UTF-8"})); got != LocaleIT {
		t.Fatalf("LC_ALL it: %q", got)
	}
	if got := DetectLocale(env(map[string]string{"LANGUAGE": "de:en"})); got != LocaleEN {
		t.Fatalf("LANGUAGE de:en: %q", got)
	}
	// Unknown languages keep the Italian source strings (unless the OS says otherwise).
	if got := DetectLocale(env(map[string]string{"LANG": "de_DE.UTF-8"})); got != LocaleIT && got != LocaleEN {
		t.Fatalf("unknown: %q", got)
	}
}

func TestT_Messages(t *testing.T) {
	t.Cleanup(func() { SetLocale(LocaleIT) })
	SetLocale(LocaleIT)
	if T("menu.open") != "Apri pannello" || T("notify.backup_ok", "x.db") != "Backup completato: x.db" {
		t.Fatal(T("menu.open"), T("notify.backup_ok", "x.db"))
	}
	SetLocale(LocaleEN)
	if T("menu.open") != "Open panel" || T("notify.update", "1.2.0") != "Version 1.2.0 is available: download it from the Coste360 portal." {
		t.Fatal(T("menu.open"))
	}
	SetLocale("de") // unsupported: Italian
	if CurrentLocale() != LocaleIT || T("menu.quit") != "Esci" {
		t.Fatal(CurrentLocale())
	}
	if T("missing.key") != "missing.key" {
		t.Fatal("unknown keys must stay visible")
	}
	// Every message has both languages and matching format verbs.
	for k, m := range messages {
		if m[0] == "" || m[1] == "" {
			t.Errorf("%s: missing translation", k)
		}
		if strings.Count(m[0], "%") != strings.Count(m[1], "%") {
			t.Errorf("%s: format verbs differ", k)
		}
	}
}
