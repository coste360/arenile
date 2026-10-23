package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/coste360/arenile/internal/desktop"
	"github.com/coste360/arenile/internal/fiscalbridge"
)

// configFile is the remote mode configuration in the data directory. It
// never contains the device token (kept in the OS keyring, see TokenStore).
const configFile = "remote.json"

// Config is the remote mode configuration (launcher setting
// "Locale | Remota", spec 29.1).
type Config struct {
	// Mode is desktop.ModeLocal or desktop.ModeRemote: the alternative
	// modes are never synchronised; switching back to local keeps the
	// pairing so the operator can return to it.
	Mode desktop.Mode `json:"mode"`
	// InstanceURL is the base URL of the instance (from the QR or typed).
	InstanceURL string `json:"instance_url"`
	// Fingerprint pins a self-signed certificate (desktop LAN instance or
	// private server); empty means the system trust store.
	Fingerprint       string    `json:"fingerprint,omitempty"`
	DeviceID          string    `json:"device_id"`
	DeviceName        string    `json:"device_name"`
	EstablishmentID   string    `json:"establishment_id"`
	EstablishmentName string    `json:"establishment_name,omitempty"`
	PairedAt          time.Time `json:"paired_at"`
	// Kiosk opens the panel full screen without browser chrome (cash desk PC).
	Kiosk bool `json:"kiosk"`
	// Autostart starts the app at login.
	Autostart bool `json:"autostart"`
	// FiscalBridge runs the bridge agent towards the local fiscal driver.
	FiscalBridge bool `json:"fiscal_bridge"`
	// FiscalDriver is the code of the local driver (fiscalbridge.LocalDrivers)
	// and FiscalDriverConfig its configuration (serial port, IP...).
	FiscalDriver       string          `json:"fiscal_driver"`
	FiscalDriverConfig json.RawMessage `json:"fiscal_driver_config,omitempty"`
}

// Paired reports whether the configuration names a paired instance.
func (c Config) Paired() bool { return c.InstanceURL != "" && c.DeviceID != "" }

// ErrNoConfig: the remote mode was never configured.
var ErrNoConfig = errors.New("remote: not configured")

// LoadConfig reads <dataDir>/remote.json.
func LoadConfig(dataDir string) (Config, error) {
	b, err := os.ReadFile(filepath.Join(dataDir, configFile))
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, ErrNoConfig
	}
	if err != nil {
		return Config{}, fmt.Errorf("remote: read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("remote: config file is corrupted: %w", err)
	}
	if c.FiscalDriver == "" {
		c.FiscalDriver = fiscalbridge.NoopCode
	}
	return c, nil
}

// SaveConfig writes <dataDir>/remote.json atomically (0600).
func SaveConfig(dataDir string, c Config) error {
	if c.Mode != desktop.ModeLocal && c.Mode != desktop.ModeRemote {
		return fmt.Errorf("remote: invalid mode %q", c.Mode)
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dataDir, configFile)
	if err := os.WriteFile(path+".tmp", b, 0o600); err != nil {
		return fmt.Errorf("remote: write config: %w", err)
	}
	return os.Rename(path+".tmp", path)
}

// SetMode switches between local and remote mode (the launcher setting).
// Switching to remote without a pairing is allowed: the app asks to pair
// at the next start.
func SetMode(dataDir string, m desktop.Mode) error {
	c, err := LoadConfig(dataDir)
	if err != nil && !errors.Is(err, ErrNoConfig) {
		return err
	}
	c.Mode = m
	return SaveConfig(dataDir, c)
}
