// Package desktop implements the thin HTTP handlers of the desktop edition
// API (/api/desktop/v1, spec 29.1): first owner creation from the browser,
// desktop settings, backups and restore.
//
// Decision (M6 contract): these operations are part of api/openapi.yaml
// (tag "desktop") because the admin SPA calls them through the generated
// client, but they exist only on the desktop edition: without a Control
// (server edition) every operation answers 404 not_found. The
// /api/desktop/ prefix is wrapped in middleware.LoopbackOnly (TCP peer and
// Host must be loopback), session authentication and CSRF; first-run is
// the only unauthenticated path and is protected by the launcher's
// one-time setup token instead.
package desktop

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// UsersCounter tells whether any user exists (first-run status).
type UsersCounter interface {
	AnyUser(ctx context.Context) (bool, error)
}

// Handlers implements the desktop operations of api.ServerInterface. Owner:
// desktop runtime builder.
//
//	GetFirstRunStatus     -> UsersCounter                                     -> api.FirstRunStatus
//	CompleteFirstRun      -> Control.ConsumeSetupToken, Auth.CreateOwner,
//	                         Auth.Login (cookies as admin.Login)              -> api.Me (201)
//	GetDesktopSettings    -> Control.Settings          (owner)                -> api.DesktopSettings
//	UpdateDesktopSettings -> Control.UpdateSettings    (owner)                -> api.DesktopSettings
//	ListDesktopBackups    -> Control.Backups           (owner)                -> api.DesktopBackupList
//	CreateDesktopBackup   -> Control.BackupNow(manual) (owner)                -> api.DesktopBackup (201)
//	RestoreDesktopBackup  -> Control.ScheduleRestore   (owner)                -> api.DesktopRestoreScheduled (202)
type Handlers struct {
	// Control is nil on the server edition (every operation 404).
	Control       control.Control
	Auth          *auth.Service
	Users         UsersCounter
	Log           *slog.Logger
	SecureCookies bool
}

// enabled answers 404 on the server edition.
func (h *Handlers) enabled(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.Control == nil {
		problem.Write(w, r, problem.NotFound)
		return false
	}
	return true
}

// GetFirstRunStatus implements GET /api/desktop/v1/first-run.
func (h *Handlers) GetFirstRunStatus(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w, r) {
		return
	}
	problem.Write(w, r, problem.NotImplemented)
}

// CompleteFirstRun implements POST /api/desktop/v1/first-run.
func (h *Handlers) CompleteFirstRun(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w, r) {
		return
	}
	problem.Write(w, r, problem.NotImplemented)
}

// GetDesktopSettings implements GET /api/desktop/v1/settings.
func (h *Handlers) GetDesktopSettings(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w, r) {
		return
	}
	problem.Write(w, r, problem.NotImplemented)
}

// UpdateDesktopSettings implements PUT /api/desktop/v1/settings.
func (h *Handlers) UpdateDesktopSettings(w http.ResponseWriter, r *http.Request, params api.UpdateDesktopSettingsParams) {
	if !h.enabled(w, r) {
		return
	}
	problem.Write(w, r, problem.NotImplemented)
}

// ListDesktopBackups implements GET /api/desktop/v1/backups.
func (h *Handlers) ListDesktopBackups(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w, r) {
		return
	}
	problem.Write(w, r, problem.NotImplemented)
}

// CreateDesktopBackup implements POST /api/desktop/v1/backups.
func (h *Handlers) CreateDesktopBackup(w http.ResponseWriter, r *http.Request, params api.CreateDesktopBackupParams) {
	if !h.enabled(w, r) {
		return
	}
	problem.Write(w, r, problem.NotImplemented)
}

// RestoreDesktopBackup implements POST /api/desktop/v1/backups/{backupName}/restore.
func (h *Handlers) RestoreDesktopBackup(w http.ResponseWriter, r *http.Request, backupName api.BackupName, params api.RestoreDesktopBackupParams) {
	if !h.enabled(w, r) {
		return
	}
	problem.Write(w, r, problem.NotImplemented)
}
