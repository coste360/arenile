package desktop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/store"
)

// BackupNamePattern is the shape of backup file names (mirrors the
// BackupName parameter of api/openapi.yaml): no separators, no "..".
var BackupNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// backupFilePattern recognises the files written by Backuper (the optional
// -N suffix disambiguates two backups in the same second).
var backupFilePattern = regexp.MustCompile(`^arenile-(\d{8}T\d{6}Z)-(manual|daily|exit|pre_restore)(?:-(\d{1,3}))?\.db$`)

// BackupFileName returns the file name of a new backup (pure), e.g.
// arenile-20270701T091244Z-daily.db.
func BackupFileName(at time.Time, reason control.BackupReason) string {
	return "arenile-" + at.UTC().Format("20060102T150405Z") + "-" + string(reason) + ".db"
}

// ParseBackupFileName extracts time and reason from a backup file name
// (pure). ok is false for files Backuper did not write.
func ParseBackupFileName(name string) (at time.Time, reason control.BackupReason, ok bool) {
	m := backupFilePattern.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, "", false
	}
	t, err := time.Parse("20060102T150405Z", m[1])
	if err != nil {
		return time.Time{}, "", false
	}
	return t.UTC(), control.BackupReason(m[2]), true
}

// IsAutomatic reports whether the retention applies to backups made for
// reason (daily and on exit); manual and pre_restore ones are kept.
func IsAutomatic(reason control.BackupReason) bool {
	return reason == control.BackupDaily || reason == control.BackupExit
}

// PruneCandidates returns the automatic backups beyond the newest
// retention ones (pure; items newest first, as List returns them).
func PruneCandidates(items []control.Backup, retention int) []control.Backup {
	var out []control.Backup
	kept := 0
	for _, b := range items {
		if !IsAutomatic(b.Reason) {
			continue
		}
		if kept < retention {
			kept++
			continue
		}
		out = append(out, b)
	}
	return out
}

// Backuper makes, lists, prunes and validates backups of the SQLite
// database at DBPath into the configured directory. The copy is the same
// file `arenile backup` writes, restorable with `arenile restore` on a VPS
// (spec 29.1, "Passaggio a un'istanza online").
type Backuper struct {
	DBPath string
	Clock  clock.Clock
	Log    *slog.Logger
}

func (b *Backuper) now() time.Time {
	if b.Clock == nil {
		return time.Now().UTC()
	}
	return b.Clock.Now()
}

func (b *Backuper) log() *slog.Logger {
	if b.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return b.Log
}

// Backup writes a consistent copy (VACUUM INTO on a separate connection,
// 0600) into dir and returns it.
func (b *Backuper) Backup(ctx context.Context, dir string, reason control.BackupReason) (control.Backup, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return control.Backup{}, fmt.Errorf("desktop: backup dir: %w", err)
	}
	at := b.now().Truncate(time.Second)
	base := BackupFileName(at, reason)
	name := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			break
		}
		if i > 99 {
			return control.Backup{}, control.ErrBackupInProgress
		}
		name = base[:len(base)-len(".db")] + "-" + strconv.Itoa(i) + ".db"
	}
	path := filepath.Join(dir, name)
	if err := store.BackupSQLite(ctx, b.DBPath, path); err != nil {
		return control.Backup{}, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return control.Backup{}, err
	}
	b.log().Info("desktop backup written", slog.String("name", name), slog.Int64("size_bytes", st.Size()))
	return control.Backup{Name: name, SizeBytes: st.Size(), CreatedAt: at, Reason: reason}, nil
}

// List returns the backups in dir, newest first (files matching
// BackupFileName; unknown files are ignored). A missing dir is empty.
func (b *Backuper) List(_ context.Context, dir string) ([]control.Backup, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []control.Backup{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("desktop: list backups: %w", err)
	}
	items := []control.Backup{}
	mtime := map[string]time.Time{} // tie-break inside the same second
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		at, reason, ok := ParseBackupFileName(e.Name())
		if !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, control.Backup{Name: e.Name(), SizeBytes: info.Size(), CreatedAt: at, Reason: reason})
		mtime[e.Name()] = info.ModTime()
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		if ma, mb := mtime[a.Name], mtime[b.Name]; !ma.Equal(mb) {
			return ma.After(mb)
		}
		return a.Name > b.Name
	})
	return items, nil
}

// Prune deletes the automatic backups (daily, exit) beyond retention;
// manual and pre_restore backups are kept.
func (b *Backuper) Prune(ctx context.Context, dir string, retention int) error {
	items, err := b.List(ctx, dir)
	if err != nil {
		return err
	}
	var errs []error
	for _, old := range PruneCandidates(items, retention) {
		if err := os.Remove(filepath.Join(dir, old.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
			continue
		}
		b.log().Info("desktop backup pruned", slog.String("name", old.Name))
	}
	return errors.Join(errs...)
}

// Validate checks that path is a SQLite database with Arenile migrations
// (same rules as `arenile restore`).
func (b *Backuper) Validate(ctx context.Context, path string) error {
	return store.ValidateSQLiteBackup(ctx, path)
}

// Restore replaces the database with the backup while the server is
// stopped: the current file becomes a pre_restore backup, -wal/-shm are
// removed. Called by the launcher between two app runs.
func (b *Backuper) Restore(ctx context.Context, backupPath, backupDir string) error {
	if err := b.Validate(ctx, backupPath); err != nil {
		return err
	}
	if _, err := os.Stat(b.DBPath); err == nil {
		if _, err := b.Backup(ctx, backupDir, control.BackupPreRestore); err != nil {
			return fmt.Errorf("desktop: keep the current database before restoring: %w", err)
		}
	}
	if err := store.RestoreSQLiteFile(ctx, backupPath, b.DBPath); err != nil {
		return fmt.Errorf("desktop: restore: %w", err)
	}
	b.log().Info("desktop backup restored", slog.String("name", filepath.Base(backupPath)))
	return nil
}

// Scheduler timings (spec 29.1: daily automatic backup).
const (
	// DailyStartDelay keeps the first daily backup away from the start-up.
	DailyStartDelay = 5 * time.Minute
	// DailyInterval is the minimum distance between two daily backups.
	DailyInterval = 24 * time.Hour
	// DailyRetryAfter spaces the attempts after a failed backup (e.g. the
	// USB stick of the backup folder is not plugged in).
	DailyRetryAfter = time.Hour
)

// DailyDue reports whether a daily backup is due (pure): enabled, at least
// DailyStartDelay after the start, DailyInterval after the last daily
// backup and DailyRetryAfter after the last failed attempt.
func DailyDue(now, started, lastDaily, lastFailure time.Time, enabled bool) bool {
	switch {
	case !enabled:
		return false
	case now.Before(started.Add(DailyStartDelay)):
		return false
	case !lastFailure.IsZero() && now.Before(lastFailure.Add(DailyRetryAfter)):
		return false
	}
	return lastDaily.IsZero() || !now.Before(lastDaily.Add(DailyInterval))
}

// Scheduler runs the daily backup while the settings enable it. The last
// daily backup is read from the backup folder, so a restart does not cause
// an extra backup.
type Scheduler struct {
	Runtime *Runtime
	Clock   clock.Clock
	Log     *slog.Logger
	// Tick drives Step (nil: every minute). Tests call Step directly.
	Tick <-chan time.Time
	// OnFailure is called after a failed backup (tray notification).
	OnFailure func(err error)

	started     time.Time
	lastDaily   time.Time
	lastFailure time.Time
	loaded      bool
}

// Run blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	tick := s.Tick
	if tick == nil {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		tick = t.C
	}
	s.Start()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick:
			s.Step(ctx)
		}
	}
}

// Start records the start time (Run calls it; idempotent).
func (s *Scheduler) Start() {
	if s.started.IsZero() {
		s.started = s.Clock.Now()
	}
}

// Step makes the daily backup when it is due and reports whether it ran.
func (s *Scheduler) Step(ctx context.Context) bool {
	s.Start()
	settings, err := s.Runtime.Settings(ctx)
	if err != nil {
		return false
	}
	if !s.loaded {
		if items, err := s.Runtime.d.Backuper.List(ctx, settings.BackupDir); err == nil {
			for _, b := range items {
				if b.Reason == control.BackupDaily {
					s.lastDaily = b.CreatedAt
					break
				}
			}
		}
		s.loaded = true
	}
	now := s.Clock.Now()
	if !DailyDue(now, s.started, s.lastDaily, s.lastFailure, settings.BackupDaily) {
		return false
	}
	if _, err := s.Runtime.BackupNow(ctx, control.BackupDaily); err != nil {
		s.lastFailure = now
		if s.Log != nil {
			s.Log.Warn("desktop daily backup failed", slog.String("error", err.Error()))
		}
		if s.OnFailure != nil {
			s.OnFailure(err)
		}
		return false
	}
	s.lastDaily, s.lastFailure = now, time.Time{}
	return true
}
