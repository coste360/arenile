package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coste360/arenile/internal/desktop/control"
)

// Default settings values.
const (
	DefaultBackupRetention = 14
	DefaultLANPort         = 8443
	settingsFile           = "desktop.json"
	// settingsVersion marks files written after ADR 0025. Older builds
	// always wrote "update_check": false (the old default) whether or not
	// the operator chose it, so an unversioned file gets the new default.
	settingsVersion = 2
)

// settingsOnDisk is the file format: the settings plus a schema marker.
type settingsOnDisk struct {
	control.Settings
	Version int `json:"settings_version,omitempty"`
}

// DefaultSettings returns the settings of a fresh installation: daily and
// on-exit backups in <dataDir>/backups, update check on (ADR 0025), no
// autostart, LAN mode off. A settings file saved before ADR 0025 (no
// settings_version) gets the update check on: those builds wrote
// "update_check": false as the default, not as a choice. A false saved by
// this version stays off.
func DefaultSettings(dataDir string) control.Settings {
	return control.Settings{
		BackupDir: filepath.Join(dataDir, "backups"), BackupRetention: DefaultBackupRetention,
		BackupDaily: true, BackupOnExit: true, LANPort: DefaultLANPort, UpdateCheck: true,
	}
}

// ValidateSettings checks the values (pure; the directory check is done by
// the runtime before saving).
func ValidateSettings(s control.Settings) error {
	switch {
	case s.BackupDir == "" || !filepath.IsAbs(s.BackupDir):
		return fmt.Errorf("%w: backup_dir must be an absolute path", control.ErrInvalidSettings)
	case s.BackupRetention < 1 || s.BackupRetention > 365:
		return fmt.Errorf("%w: backup_retention must be between 1 and 365", control.ErrInvalidSettings)
	case s.LANPort != 0 && (s.LANPort < 1024 || s.LANPort > 65535):
		return fmt.Errorf("%w: lan_port must be between 1024 and 65535", control.ErrInvalidSettings)
	}
	return nil
}

// LoadSettings reads <dataDir>/desktop.json, applying defaults for a
// missing file or missing fields.
func LoadSettings(dataDir string) (control.Settings, error) {
	s := DefaultSettings(dataDir)
	b, err := os.ReadFile(filepath.Join(dataDir, settingsFile))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("desktop: read settings: %w", err)
	}
	f := settingsOnDisk{Settings: s}
	if err := json.Unmarshal(b, &f); err != nil {
		return DefaultSettings(dataDir), fmt.Errorf("desktop: settings file is corrupted: %w", err)
	}
	s = f.Settings
	if f.Version < settingsVersion {
		s.UpdateCheck = true
	}
	if s.LANPort == 0 {
		s.LANPort = DefaultLANPort
	}
	return s, nil
}

// SaveSettings writes <dataDir>/desktop.json atomically (0600).
func SaveSettings(dataDir string, s control.Settings) error {
	if err := ValidateSettings(s); err != nil {
		return err
	}
	b, err := json.MarshalIndent(settingsOnDisk{Settings: s, Version: settingsVersion}, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dataDir, settingsFile)
	if err := os.WriteFile(path+".tmp", b, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
