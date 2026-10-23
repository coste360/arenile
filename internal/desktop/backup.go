package desktop

import (
	"context"
	"log/slog"
	"regexp"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop/control"
)

// BackupNamePattern is the shape of backup file names (mirrors the
// BackupName parameter of api/openapi.yaml): no separators, no "..".
var BackupNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// BackupFileName returns the file name of a new backup (pure), e.g.
// arenile-20270701T091244Z-daily.db.
func BackupFileName(at time.Time, reason control.BackupReason) string {
	return "arenile-" + at.UTC().Format("20060102T150405Z") + "-" + string(reason) + ".db"
}

// Backuper makes, lists, prunes and validates backups of the SQLite
// database at DBPath into the configured directory.
type Backuper struct {
	DBPath string
	Clock  clock.Clock
	Log    *slog.Logger
}

// Backup writes a consistent copy (VACUUM INTO on a separate connection,
// 0600) into dir and returns it.
func (b *Backuper) Backup(ctx context.Context, dir string, reason control.BackupReason) (control.Backup, error) {
	return control.Backup{}, ErrNotImplemented
}

// List returns the backups in dir, newest first (files matching
// BackupFileName; unknown files are ignored).
func (b *Backuper) List(ctx context.Context, dir string) ([]control.Backup, error) {
	return nil, ErrNotImplemented
}

// Prune deletes the automatic backups (daily, exit) beyond retention;
// manual and pre_restore backups are kept.
func (b *Backuper) Prune(ctx context.Context, dir string, retention int) error {
	return ErrNotImplemented
}

// Validate checks that path is a SQLite database with Arenile migrations
// (same rules as `arenile restore`).
func (b *Backuper) Validate(ctx context.Context, path string) error {
	return ErrNotImplemented
}

// Restore replaces the database with the backup while the server is
// stopped: the current file becomes a pre_restore backup, -wal/-shm are
// removed. Called by the launcher between two app runs.
func (b *Backuper) Restore(ctx context.Context, backupPath, backupDir string) error {
	return ErrNotImplemented
}

// Scheduler runs the daily backup (first run 5 minutes after start, then
// every 24 hours) while the settings enable it.
type Scheduler struct {
	Runtime *Runtime
	Clock   clock.Clock
	Log     *slog.Logger
}

// Run blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
