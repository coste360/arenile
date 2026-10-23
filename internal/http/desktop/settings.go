package desktop

import (
	"net/http"

	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// GetDesktopSettings implements GET /api/desktop/v1/settings (owner).
func (h *Handlers) GetDesktopSettings(w http.ResponseWriter, r *http.Request) {
	if !h.owner(w, r) {
		return
	}
	s, err := h.Control.Settings(r.Context())
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsToAPI(s))
}

// UpdateDesktopSettings implements PUT /api/desktop/v1/settings (owner):
// the backup folder is chosen in the panel (no native dialog, no new
// dependency) and checked for writability by the runtime.
func (h *Handlers) UpdateDesktopSettings(w http.ResponseWriter, r *http.Request, _ api.UpdateDesktopSettingsParams) {
	if !h.owner(w, r) {
		return
	}
	var req api.DesktopSettings
	if !decode(w, r, &req) {
		return
	}
	current, err := h.Control.Settings(r.Context())
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	s, err := h.Control.UpdateSettings(r.Context(), settingsFromAPI(req, current))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsToAPI(s))
}

// ListDesktopBackups implements GET /api/desktop/v1/backups (owner).
func (h *Handlers) ListDesktopBackups(w http.ResponseWriter, r *http.Request) {
	if !h.owner(w, r) {
		return
	}
	dir, items, err := h.Control.Backups(r.Context())
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.DesktopBackupList{Directory: dir, Items: make([]api.DesktopBackup, 0, len(items))}
	for _, b := range items {
		out.Items = append(out.Items, backupToAPI(b))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateDesktopBackup implements POST /api/desktop/v1/backups (owner).
func (h *Handlers) CreateDesktopBackup(w http.ResponseWriter, r *http.Request, _ api.CreateDesktopBackupParams) {
	if !h.owner(w, r) {
		return
	}
	b, err := h.Control.BackupNow(r.Context(), control.BackupManual)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, backupToAPI(b))
}

// RestoreDesktopBackup implements POST
// /api/desktop/v1/backups/{backupName}/restore (owner): 202, the launcher
// restarts the internal server on the restored database.
func (h *Handlers) RestoreDesktopBackup(w http.ResponseWriter, r *http.Request, backupName api.BackupName, _ api.RestoreDesktopBackupParams) {
	if !h.owner(w, r) {
		return
	}
	b, err := h.Control.ScheduleRestore(r.Context(), backupName)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusAccepted, api.DesktopRestoreScheduled{Backup: backupToAPI(b), RestartRequired: true})
}

func settingsToAPI(s control.Settings) api.DesktopSettings {
	port, keep := s.LANPort, s.KeepRunningInTray
	return api.DesktopSettings{
		BackupDir: s.BackupDir, BackupRetention: s.BackupRetention, BackupDaily: s.BackupDaily,
		BackupOnExit: s.BackupOnExit, UpdateCheck: s.UpdateCheck, Autostart: s.Autostart,
		LanEnabled: s.LANEnabled, LanPort: &port, KeepRunningInTray: &keep,
	}
}

// settingsFromAPI maps the request; an omitted keep_running_in_tray keeps
// the current value (clients older than the window shell).
func settingsFromAPI(a api.DesktopSettings, current control.Settings) control.Settings {
	s := control.Settings{
		BackupDir: a.BackupDir, BackupRetention: a.BackupRetention, BackupDaily: a.BackupDaily,
		BackupOnExit: a.BackupOnExit, UpdateCheck: a.UpdateCheck, Autostart: a.Autostart,
		LANEnabled: a.LanEnabled, KeepRunningInTray: current.KeepRunningInTray,
	}
	if a.KeepRunningInTray != nil {
		s.KeepRunningInTray = *a.KeepRunningInTray
	}
	if a.LanPort != nil {
		s.LANPort = *a.LanPort
	}
	return s
}

func backupToAPI(b control.Backup) api.DesktopBackup {
	return api.DesktopBackup{Name: b.Name, SizeBytes: b.SizeBytes, CreatedAt: b.CreatedAt, Reason: api.DesktopBackupReason(b.Reason)}
}
