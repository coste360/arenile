package desktop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/meta"
	"github.com/coste360/arenile/internal/store"
)

// LANMode is the port to the LAN mode (spec 29.1, "Modalità rete locale").
// The adapter lives in internal/desktop/lan; the runtime only switches it
// on and off and reports its address to /meta. Enable must be idempotent
// and return the https URL and the certificate fingerprint shown in the
// pairing QR.
type LANMode interface {
	Enable(ctx context.Context, port int) (url, fingerprint string, err error)
	Disable(ctx context.Context) error
}

// BackupStore makes and restores backups (Backuper; fakes in tests).
type BackupStore interface {
	Backup(ctx context.Context, dir string, reason control.BackupReason) (control.Backup, error)
	List(ctx context.Context, dir string) ([]control.Backup, error)
	Prune(ctx context.Context, dir string, retention int) error
	Validate(ctx context.Context, path string) error
	Restore(ctx context.Context, backupPath, backupDir string) error
}

var _ BackupStore = (*Backuper)(nil)

// RuntimeDeps are the dependencies of Runtime.
type RuntimeDeps struct {
	DataDir  string
	Backuper BackupStore
	Certs    CertStore
	Setup    *SetupToken // nil when users already exist
	Clock    clock.Clock
	Log      *slog.Logger
	// Autostart applies the "start at login" setting (nil: unsupported).
	Autostart Autostart
	// LAN switches the LAN mode (nil: not available in this build).
	LAN LANMode
	// OnSettingsChanged is called after a successful UpdateSettings (the
	// launcher refreshes the tray menu).
	OnSettingsChanged func(control.Settings)
	// OnSetupCompleted is called once the first owner exists.
	OnSetupCompleted func()
}

// Runtime is the state of a running desktop edition. It implements
// control.Control (and meta.RuntimeProvider) for the core and exposes
// Restarts to the launcher. It outlives the app instances: the launcher
// passes the same Runtime to every app.New (after a restore too).
type Runtime struct {
	d RuntimeDeps

	mu       sync.Mutex
	mode     meta.DesktopMode
	lanURL   string
	lanFP    string
	restarts chan string

	// settingsMu serialises UpdateSettings (side effects + file write).
	settingsMu sync.Mutex
	// backupMu guards backups and restores (TryLock: ErrBackupInProgress).
	backupMu sync.Mutex
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
	if d.Backuper == nil {
		d.Backuper = &Backuper{DBPath: filepath.Join(d.DataDir, "arenile.db"), Clock: d.Clock, Log: d.Log}
	}
	return &Runtime{d: d, mode: meta.DesktopLocal, restarts: make(chan string, 1)}
}

// Runtime implements meta.RuntimeProvider.
func (r *Runtime) Runtime() meta.Runtime {
	r.mu.Lock()
	defer r.mu.Unlock()
	return meta.Runtime{Edition: meta.EditionDesktop, Desktop: &meta.Desktop{Mode: r.mode, LANURL: r.lanURL, LANFingerprint: r.lanFP}}
}

// SetLAN records the LAN listener state (called when the LAN listener is
// opened or closed).
func (r *Runtime) SetLAN(url, fingerprint string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if url == "" {
		r.mode, r.lanURL, r.lanFP = meta.DesktopLocal, "", ""
		return
	}
	r.mode, r.lanURL, r.lanFP = meta.DesktopLAN, url, fingerprint
}

// SetSetupToken installs the first-run token of this start (nil once an
// owner exists).
func (r *Runtime) SetSetupToken(t *SetupToken) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.d.Setup = t
}

// LANAvailable reports whether this build can switch the LAN mode.
func (r *Runtime) LANAvailable() bool { return r.d.LAN != nil }

// Restarts delivers the backup names the operator asked to restore; the
// launcher stops the app, calls Backuper.Restore and starts it again.
func (r *Runtime) Restarts() <-chan string { return r.restarts }

// ConsumeSetupToken implements control.Control.
func (r *Runtime) ConsumeSetupToken(token string) error {
	r.mu.Lock()
	tok := r.d.Setup
	r.mu.Unlock()
	if err := tok.Consume(token); err != nil {
		return err
	}
	if r.d.OnSetupCompleted != nil {
		r.d.OnSetupCompleted()
	}
	return nil
}

// Settings implements control.Control. The autostart flag reflects the
// operating system (the user may have removed the entry by hand).
func (r *Runtime) Settings(context.Context) (control.Settings, error) {
	s, err := LoadSettings(r.d.DataDir)
	if err != nil {
		r.d.Log.Warn("desktop settings unreadable, using defaults", slog.String("error", err.Error()))
	}
	if r.d.Autostart != nil {
		if on, aerr := r.d.Autostart.Enabled(); aerr == nil {
			s.Autostart = on
		}
	}
	return s, nil
}

// UpdateSettings implements control.Control: it validates, applies the
// side effects (autostart entry, LAN listener) and persists desktop.json.
func (r *Runtime) UpdateSettings(ctx context.Context, s control.Settings) (control.Settings, error) {
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	if s.LANPort == 0 {
		s.LANPort = DefaultLANPort
	}
	s.BackupDir = filepath.Clean(s.BackupDir)
	if err := ValidateSettings(s); err != nil {
		return control.Settings{}, err
	}
	if err := checkWritableDir(s.BackupDir); err != nil {
		return control.Settings{}, fmt.Errorf("%w: backup_dir is not writable", control.ErrInvalidSettings)
	}
	if s.LANEnabled && r.d.LAN == nil {
		return control.Settings{}, fmt.Errorf("%w: lan_enabled: the LAN mode is not available in this build", control.ErrInvalidSettings)
	}
	prev, _ := r.Settings(ctx)
	if r.d.Autostart != nil && s.Autostart != prev.Autostart {
		if err := r.d.Autostart.SetEnabled(s.Autostart); err != nil {
			return control.Settings{}, fmt.Errorf("desktop: autostart: %w", err)
		}
	}
	if err := r.applyLAN(ctx, prev, s); err != nil {
		return control.Settings{}, err
	}
	if err := SaveSettings(r.d.DataDir, s); err != nil {
		return control.Settings{}, err
	}
	r.d.Log.Info("desktop settings updated",
		slog.Bool("backup_daily", s.BackupDaily), slog.Bool("backup_on_exit", s.BackupOnExit),
		slog.Int("backup_retention", s.BackupRetention), slog.Bool("update_check", s.UpdateCheck),
		slog.Bool("autostart", s.Autostart), slog.Bool("lan_enabled", s.LANEnabled))
	if r.d.OnSettingsChanged != nil {
		r.d.OnSettingsChanged(s)
	}
	return s, nil
}

// applyLAN switches the LAN listener when the setting (or its port) changed.
func (r *Runtime) applyLAN(ctx context.Context, prev, next control.Settings) error {
	if r.d.LAN == nil {
		return nil
	}
	switch {
	case next.LANEnabled && (!r.lanOn() || prev.LANPort != next.LANPort):
		return r.StartLAN(ctx, next.LANPort)
	case !next.LANEnabled && r.lanOn():
		return r.StopLAN(ctx)
	}
	return nil
}

func (r *Runtime) lanOn() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mode == meta.DesktopLAN
}

// StartLAN opens the LAN listener and records its address (also used by
// the launcher at start-up when the setting is on).
func (r *Runtime) StartLAN(ctx context.Context, port int) error {
	if r.d.LAN == nil {
		return nil
	}
	url, fp, err := r.d.LAN.Enable(ctx, port)
	if err != nil {
		return fmt.Errorf("desktop: LAN mode: %w", err)
	}
	r.SetLAN(url, fp)
	return nil
}

// StopLAN closes the LAN listener.
func (r *Runtime) StopLAN(ctx context.Context) error {
	if r.d.LAN == nil {
		return nil
	}
	err := r.d.LAN.Disable(ctx)
	r.SetLAN("", "")
	return err
}

// Backups implements control.Control.
func (r *Runtime) Backups(ctx context.Context) (string, []control.Backup, error) {
	s, err := r.Settings(ctx)
	if err != nil {
		return "", nil, err
	}
	items, err := r.d.Backuper.List(ctx, s.BackupDir)
	return s.BackupDir, items, err
}

// BackupNow implements control.Control.
func (r *Runtime) BackupNow(ctx context.Context, reason control.BackupReason) (control.Backup, error) {
	if !r.backupMu.TryLock() {
		return control.Backup{}, control.ErrBackupInProgress
	}
	defer r.backupMu.Unlock()
	return r.backupLocked(ctx, reason)
}

func (r *Runtime) backupLocked(ctx context.Context, reason control.BackupReason) (control.Backup, error) {
	s, err := r.Settings(ctx)
	if err != nil {
		return control.Backup{}, err
	}
	b, err := r.d.Backuper.Backup(ctx, s.BackupDir, reason)
	if err != nil {
		return control.Backup{}, err
	}
	if err := r.d.Backuper.Prune(ctx, s.BackupDir, s.BackupRetention); err != nil {
		r.d.Log.Warn("desktop backup prune failed", slog.String("error", err.Error()))
	}
	return b, nil
}

// ScheduleRestore implements control.Control.
func (r *Runtime) ScheduleRestore(ctx context.Context, name string) (control.Backup, error) {
	b, err := r.findBackup(ctx, name)
	if err != nil {
		return control.Backup{}, err
	}
	s, _ := r.Settings(ctx)
	if err := r.d.Backuper.Validate(ctx, filepath.Join(s.BackupDir, b.Name)); err != nil {
		if errors.Is(err, store.ErrInvalidBackup) {
			return control.Backup{}, fmt.Errorf("%w: the backup file is damaged or is not an Arenile backup", control.ErrInvalidSettings)
		}
		return control.Backup{}, err
	}
	select {
	case r.restarts <- b.Name:
		r.d.Log.Info("desktop restore scheduled", slog.String("name", b.Name))
		return b, nil
	default:
		return control.Backup{}, control.ErrBackupInProgress
	}
}

// findBackup resolves a bare backup name in the backup folder.
func (r *Runtime) findBackup(ctx context.Context, name string) (control.Backup, error) {
	if !BackupNamePattern.MatchString(name) {
		return control.Backup{}, control.ErrBackupNotFound
	}
	_, items, err := r.Backups(ctx)
	if err != nil {
		return control.Backup{}, err
	}
	for _, b := range items {
		if b.Name == name {
			return b, nil
		}
	}
	return control.Backup{}, control.ErrBackupNotFound
}

// RestoreNamed restores a backup of the backup folder (the launcher calls
// it with the app stopped).
func (r *Runtime) RestoreNamed(ctx context.Context, name string) error {
	if !r.backupMu.TryLock() {
		return control.ErrBackupInProgress
	}
	defer r.backupMu.Unlock()
	b, err := r.findBackup(ctx, name)
	if err != nil {
		return err
	}
	s, _ := r.Settings(ctx)
	return r.d.Backuper.Restore(ctx, filepath.Join(s.BackupDir, b.Name), s.BackupDir)
}

// checkWritableDir creates dir if needed and checks that a file can be
// written in it (a read-only or missing USB stick is rejected early).
func checkWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".arenile-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}
