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
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/auth"
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
//	CompleteFirstRun      -> Control.ConsumeSetupToken(Auth.CreateOwner),
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

// owner checks that the request comes from a logged-in owner (the desktop
// settings act on the whole installation).
func (h *Handlers) owner(w http.ResponseWriter, r *http.Request) bool {
	if !h.enabled(w, r) {
		return false
	}
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		problem.Write(w, r, problem.Unauthorized)
		return false
	}
	if p.Kind != auth.PrincipalUser || !p.HasRole(auth.RoleOwner) {
		problem.Write(w, r, problem.Forbidden)
		return false
	}
	return true
}

// decode reads a JSON body into v, writing a problem on failure.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			problem.Write(w, r, problem.BadRequest, problem.Detail("Corpo della richiesta troppo grande."))
			return false
		}
		problem.Write(w, r, problem.BadRequest, problem.Detail("JSON non valido o campi con formato errato."))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
