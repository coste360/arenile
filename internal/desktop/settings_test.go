package desktop

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/desktop/control"
)

func TestSettings_RoundTripAndValidation(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadSettings(dir)
	if err != nil || s != DefaultSettings(dir) || s.UpdateCheck || s.LANEnabled {
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
