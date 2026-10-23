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
)

// DefaultSettings returns the settings of a fresh installation: daily and
// on-exit backups in <dataDir>/backups, no update check, no autostart, LAN
// mode off.
func DefaultSettings(dataDir string) control.Settings {
	return control.Settings{
		BackupDir: filepath.Join(dataDir, "backups"), BackupRetention: DefaultBackupRetention,
		BackupDaily: true, BackupOnExit: true, LANPort: DefaultLANPort,
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
	if err := json.Unmarshal(b, &s); err != nil {
		return DefaultSettings(dataDir), fmt.Errorf("desktop: settings file is corrupted: %w", err)
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
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dataDir, settingsFile)
	if err := os.WriteFile(path+".tmp", b, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
