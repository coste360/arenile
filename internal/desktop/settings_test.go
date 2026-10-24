package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/coste360/arenile/internal/desktop/control"
)

func TestSettings_RoundTripAndValidation(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadSettings(dir)
	if err != nil || s != DefaultSettings(dir) || !s.UpdateCheck || s.LANEnabled {
		t.Fatalf("defaults %+v %v", s, err)
	}
	s.BackupRetention = 30
	s.LANEnabled = true
	if err := SaveSettings(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(dir)
	if err != nil || got != s {
		t.Fatalf("roundtrip %+v %v", got, err)
	}
	for _, bad := range []control.Settings{
		{BackupDir: "relative", BackupRetention: 1},
		{BackupDir: dir, BackupRetention: 0},
		{BackupDir: dir, BackupRetention: 1, LANPort: 80},
	} {
		if err := ValidateSettings(bad); !errors.Is(err, control.ErrInvalidSettings) {
			t.Errorf("ValidateSettings(%+v) = %v", bad, err)
		}
	}
}

// ADR 0025: a settings file saved before the update check became the
// default (no settings_version) gets it on, because older builds always
// wrote the old default "update_check": false; a false saved by this
// version (settings_version 2) stays off.
func TestLoadSettings_UpdateCheckDefault(t *testing.T) {
	tests := []struct {
		name string
		file string
		want bool
	}{
		{"missing field", `{"backup_dir":"/b","backup_retention":14,"backup_daily":true}`, true},
		{"old build wrote the default false", `{"backup_dir":"/b","backup_retention":14,"update_check":false}`, true},
		{"old build with true", `{"backup_dir":"/b","backup_retention":14,"update_check":true}`, true},
		{"switched off after ADR 0025", `{"backup_dir":"/b","backup_retention":14,"update_check":false,"settings_version":2}`, false},
		{"on after ADR 0025", `{"backup_dir":"/b","backup_retention":14,"update_check":true,"settings_version":2}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, settingsFile), []byte(tt.file), 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := LoadSettings(dir)
			if err != nil || s.UpdateCheck != tt.want {
				t.Fatalf("update_check = %v (%v), want %v", s.UpdateCheck, err, tt.want)
			}
		})
	}
	// The operator's choice survives a save and a load.
	dir := t.TempDir()
	s := DefaultSettings(dir)
	s.UpdateCheck = false
	if err := SaveSettings(dir, s); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadSettings(dir); err != nil || got.UpdateCheck {
		t.Fatalf("saved false came back as %v (%v)", got.UpdateCheck, err)
	}
}
