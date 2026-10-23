// Package control is the port between the HTTP layer of the core and the
// desktop launcher runtime (spec 29.1). It has no dependencies beyond the
// standard library and internal/domain/meta, so the server binary can carry
// the /api/desktop/v1 handlers without importing the launcher (keyring,
// TLS certificate generation, tray).
//
// The implementation is internal/desktop.Runtime, passed to the core with
// app.WithDesktop. On the server edition no Control is wired and the
// /api/desktop/v1 operations answer 404 not_found.
package control

import (
	"context"
	"errors"
	"time"

	"github.com/coste360/arenile/internal/domain/meta"
)

// Errors mapped by internal/http/problem.
var (
	// ErrLoopbackRequired: request not from 127.0.0.1/::1 (403 loopback_required).
	ErrLoopbackRequired = errors.New("desktop: loopback required")
	// ErrSetupTokenInvalid: missing, wrong or already used setup token (403 setup_token_invalid).
	ErrSetupTokenInvalid = errors.New("desktop: setup token invalid")
	// ErrSetupCompleted: users already exist (409 setup_completed).
	ErrSetupCompleted = errors.New("desktop: setup already completed")
	// ErrBackupNotFound: unknown backup name (404 not_found).
	ErrBackupNotFound = errors.New("desktop: backup not found")
	// ErrBackupInProgress: another backup or restore is running (409 conflict).
	ErrBackupInProgress = errors.New("desktop: backup in progress")
	// ErrInvalidSettings: settings rejected (400 validation_failed).
	ErrInvalidSettings = errors.New("desktop: invalid settings")
)

// Settings are the desktop app settings (api.DesktopSettings), persisted by
// the launcher in <data dir>/desktop.json.
type Settings struct {
	BackupDir       string `json:"backup_dir"`
	BackupRetention int    `json:"backup_retention"`
	BackupDaily     bool   `json:"backup_daily"`
	BackupOnExit    bool   `json:"backup_on_exit"`
	UpdateCheck     bool   `json:"update_check"`
	Autostart       bool   `json:"autostart"`
	LANEnabled      bool   `json:"lan_enabled"`
	LANPort         int    `json:"lan_port"`
}

// BackupReason tells why a backup was made (api.DesktopBackupReason).
type BackupReason string

// Backup reasons.
const (
	BackupManual     BackupReason = "manual"
	BackupDaily      BackupReason = "daily"
	BackupExit       BackupReason = "exit"
	BackupPreRestore BackupReason = "pre_restore"
)

// Backup is a backup file in the backup directory (api.DesktopBackup).
// Name is a bare file name, never a path.
type Backup struct {
	Name      string
	SizeBytes int64
	CreatedAt time.Time
	Reason    BackupReason
}

// Control is implemented by the desktop runtime.
type Control interface {
	// RuntimeProvider reports edition desktop and the local/LAN mode.
	meta.RuntimeProvider
	// ConsumeSetupToken checks the one-time first-run token in constant
	// time and invalidates it on success (ErrSetupTokenInvalid otherwise).
	ConsumeSetupToken(token string) error
	// Settings returns the current settings.
	Settings(ctx context.Context) (Settings, error)
	// UpdateSettings validates, persists and applies the settings (starts
	// or stops the LAN listener, toggles autostart).
	UpdateSettings(ctx context.Context, s Settings) (Settings, error)
	// Backups lists the backups, newest first, and the backup directory.
	Backups(ctx context.Context) (dir string, items []Backup, err error)
	// BackupNow makes a consistent backup (VACUUM INTO) and prunes the
	// automatic ones beyond the retention.
	BackupNow(ctx context.Context, reason BackupReason) (Backup, error)
	// ScheduleRestore validates the backup and asks the launcher to restart
	// the internal server with it (the current database is kept as a
	// pre_restore backup).
	ScheduleRestore(ctx context.Context, name string) (Backup, error)
}
