package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// notifyStateFile keeps what the launcher has already told the operator
// through native notifications, across restarts. It is internal state of
// the app (like window.json), not a setting: desktop.json, its
// settings_version and the panel's settings page are not involved.
const notifyStateFile = "notify-state.json"

// NotifyState is the content of notify-state.json.
type NotifyState struct {
	// UpdateNotified is the last version announced by the native "new
	// version" notification: each version is announced at most once per
	// installation; a newer one is announced again.
	UpdateNotified string `json:"update_notified,omitempty"`
}

// LoadNotifyState reads <dataDir>/notify-state.json. A missing or
// unreadable file is an empty state: at worst a notification is repeated.
func LoadNotifyState(dataDir string) NotifyState {
	var s NotifyState
	if dataDir == "" {
		return s
	}
	b, err := os.ReadFile(filepath.Join(dataDir, notifyStateFile))
	if err != nil {
		return s
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return NotifyState{}
	}
	return s
}

// SaveNotifyState writes <dataDir>/notify-state.json atomically (0600).
func SaveNotifyState(dataDir string, s NotifyState) error {
	if dataDir == "" {
		return nil
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dataDir, notifyStateFile)
	if err := os.WriteFile(path+".tmp", b, 0o600); err != nil {
		return fmt.Errorf("desktop: save notification state: %w", err)
	}
	return os.Rename(path+".tmp", path)
}
