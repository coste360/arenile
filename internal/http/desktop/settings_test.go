package desktop

import (
	"testing"

	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/http/api"
)

func TestSettingsFromAPI_KeepRunningInTray(t *testing.T) {
	current := control.Settings{KeepRunningInTray: true}
	req := api.DesktopSettings{BackupDir: "/b", BackupRetention: 3}
	if s := settingsFromAPI(req, current); !s.KeepRunningInTray {
		t.Fatal("omitted keep_running_in_tray must keep the current value")
	}
	off := false
	req.KeepRunningInTray = &off
	if s := settingsFromAPI(req, current); s.KeepRunningInTray {
		t.Fatal("explicit false ignored")
	}
	out := settingsToAPI(control.Settings{KeepRunningInTray: true, LANPort: 8443})
	if out.KeepRunningInTray == nil || !*out.KeepRunningInTray || *out.LanPort != 8443 {
		t.Fatalf("to API: %+v", out)
	}
}
