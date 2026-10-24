package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coste360/arenile/internal/domain/update"
	"github.com/coste360/arenile/internal/updates"
)

// The native notification of a new version is shown once per version,
// also when the core restarts after a restore (a new update service
// reports the same version again).
func TestLauncher_NotifyUpdate_Scenarios(t *testing.T) {
	ui := newFakeUI()
	l := &Launcher{ui: ui}
	for _, v := range []string{"1.2.0", "1.2.0", "1.2.0", "1.3.0", "1.3.0"} {
		l.notifyUpdate(v)
	}
	if len(ui.notes) != 2 || ui.notes[0] != T("notify.update", "1.2.0") || ui.notes[1] != T("notify.update", "1.3.0") {
		t.Fatalf("notes %q", ui.notes)
	}
	// Before the UI exists nothing panics.
	(&Launcher{}).notifyUpdate("1.2.0")
}

// pageFakeUI records clickable notifications (PageNotifier, macOS).
type pageFakeUI struct {
	*fakeUI
	pages []string
}

func (p *pageFakeUI) NotifyPage(_, m, u string) {
	p.mu.Lock()
	p.notes = append(p.notes, m)
	p.pages = append(p.pages, u)
	p.mu.Unlock()
}

// Each version is announced at most once per installation: the announced
// version survives a restart of the app (notify-state.json in the data
// dir); a newer version is announced again, an older one never.
func TestLauncher_NotifyUpdate_PersistsAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	start := func() (*Launcher, *pageFakeUI) {
		ui := &pageFakeUI{fakeUI: newFakeUI()}
		l := &Launcher{ui: ui, dataDir: dir}
		l.cfg.BaseURL = "http://127.0.0.1:5"
		return l, ui
	}
	steps := []struct {
		version string
		want    bool
	}{
		{"1.2.0", true},  // first announcement
		{"1.2.0", false}, // restart: already announced
		{"1.1.9", false}, // an older version is never announced
		{"1.3.0", true},  // a newer one is
		{"1.3.0", false},
	}
	for i, st := range steps {
		l, ui := start()
		l.notifyUpdate(st.version)
		if got := len(ui.notes) == 1; got != st.want {
			t.Fatalf("step %d (%s): notes %q", i, st.version, ui.notes)
		}
		if st.want && (ui.notes[0] != T("notify.update", st.version) || ui.pages[0] != "http://127.0.0.1:5"+PageAbout) {
			t.Fatalf("step %d: %q -> %q", i, ui.notes, ui.pages)
		}
	}
	if got := LoadNotifyState(dir).UpdateNotified; got != "1.3.0" {
		t.Fatalf("state %q", got)
	}
	// A corrupted file is an empty state (at worst one repeated notice).
	if err := os.WriteFile(filepath.Join(dir, notifyStateFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if LoadNotifyState(dir) != (NotifyState{}) || LoadNotifyState("") != (NotifyState{}) || SaveNotifyState("", NotifyState{}) != nil {
		t.Fatal("corrupted or missing state")
	}
	// The settings file is not involved.
	if _, err := os.Stat(filepath.Join(dir, settingsFile)); !os.IsNotExist(err) {
		t.Fatalf("desktop.json touched: %v", err)
	}
}

func TestUpdateCheckMessage_Scenarios(t *testing.T) {
	tests := []struct {
		name string
		st   update.Status
		msg  string
		page string
	}{
		{"newer", update.Status{State: update.StateOK, CheckEnabled: true, Current: "1.0.0", Latest: "1.1.0", UpdateAvailable: true}, T("notify.update", "1.1.0"), PageAbout},
		{"up to date", update.Status{State: update.StateOK, CheckEnabled: true, Current: "1.1.0", Latest: "1.1.0"}, T("notify.update_none", "1.1.0"), PageAbout},
		{"unavailable", update.Status{State: update.StateUnavailable, CheckEnabled: true, Current: "1.1.0"}, T("notify.update_unavailable"), PageAbout},
		{"off", update.Status{State: update.StateDisabled, Current: "1.1.0"}, T("notify.update_disabled"), PageDesktop},
		{"dev", update.Status{State: update.StateDisabled, CheckEnabled: true, Current: "dev"}, T("notify.update_dev", "dev"), PageAbout},
	}
	for _, tt := range tests {
		msg, page := UpdateCheckMessage(tt.st)
		if msg != tt.msg || page != tt.page {
			t.Errorf("%s: %q %q", tt.name, msg, page)
		}
	}
}

func TestUpdateURL_Scenarios(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", updates.DefaultURL},
		{"updates.example.com/latest.json", updates.DefaultURL},
		{"ftp://mirror/latest.json", updates.DefaultURL},
		{" https://mirror.lan/latest.json ", "https://mirror.lan/latest.json"},
		{"http://127.0.0.1:9000/latest.json", "http://127.0.0.1:9000/latest.json"},
	}
	for _, tt := range tests {
		if got := UpdateURL(tt.in); got != tt.want {
			t.Errorf("UpdateURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUpdateCheckOff_Scenarios(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"true", false},
		{"maybe", false},
		{"false", true},
		{"0", true},
		{" OFF ", true},
	}
	for _, tt := range tests {
		if got := UpdateCheckOff(tt.in); got != tt.want {
			t.Errorf("UpdateCheckOff(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
