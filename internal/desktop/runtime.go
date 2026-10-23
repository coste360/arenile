package desktop

import (
	"context"
	"log/slog"
	"sync"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/meta"
)

// RuntimeDeps are the dependencies of Runtime.
type RuntimeDeps struct {
	DataDir  string
	Backuper *Backuper
	Certs    CertStore
	Setup    *SetupToken // nil when users already exist
	Clock    clock.Clock
	Log      *slog.Logger
}

// Runtime is the state of a running desktop edition. It implements
// control.Control (and meta.RuntimeProvider) for the core and exposes
// Restarts to the launcher.
type Runtime struct {
	d RuntimeDeps

	mu       sync.Mutex
	mode     meta.DesktopMode
	lanURL   string
	lanFP    string
	restarts chan string
}

var _ control.Control = (*Runtime)(nil)

// NewRuntime returns a Runtime in local mode.
func NewRuntime(d RuntimeDeps) *Runtime {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Runtime{d: d, mode: meta.DesktopLocal, restarts: make(chan string, 1)}
}

// Runtime implements meta.RuntimeProvider.
func (r *Runtime) Runtime() meta.Runtime {
	r.mu.Lock()
	defer r.mu.Unlock()
	return meta.Runtime{Edition: meta.EditionDesktop, Desktop: &meta.Desktop{Mode: r.mode, LANURL: r.lanURL, LANFingerprint: r.lanFP}}
}

// SetLAN records the LAN listener state (called by the launcher when it
// opens or closes the LAN listener).
func (r *Runtime) SetLAN(url, fingerprint string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if url == "" {
		r.mode, r.lanURL, r.lanFP = meta.DesktopLocal, "", ""
		return
	}
	r.mode, r.lanURL, r.lanFP = meta.DesktopLAN, url, fingerprint
}

// Restarts delivers the backup names the operator asked to restore; the
// launcher stops the app, calls Backuper.Restore and starts it again.
func (r *Runtime) Restarts() <-chan string { return r.restarts }

// ConsumeSetupToken implements control.Control.
func (r *Runtime) ConsumeSetupToken(token string) error { return r.d.Setup.Consume(token) }

// Settings implements control.Control.
func (r *Runtime) Settings(context.Context) (control.Settings, error) {
	return LoadSettings(r.d.DataDir)
}

// UpdateSettings implements control.Control.
func (r *Runtime) UpdateSettings(context.Context, control.Settings) (control.Settings, error) {
	return control.Settings{}, ErrNotImplemented
}

// Backups implements control.Control.
func (r *Runtime) Backups(context.Context) (string, []control.Backup, error) {
	return "", nil, ErrNotImplemented
}

// BackupNow implements control.Control.
func (r *Runtime) BackupNow(context.Context, control.BackupReason) (control.Backup, error) {
	return control.Backup{}, ErrNotImplemented
}

// ScheduleRestore implements control.Control.
func (r *Runtime) ScheduleRestore(context.Context, string) (control.Backup, error) {
	return control.Backup{}, ErrNotImplemented
}
