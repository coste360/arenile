package desktop

import (
	"testing"

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
