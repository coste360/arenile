package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SQLite backup and restore primitives shared by `arenile backup|restore`
// and the desktop edition (spec 24.6, 29.1): one implementation, so a
// desktop backup is byte-for-byte the file a VPS restores.

// ErrInvalidBackup: the file is not a usable Arenile SQLite backup.
var ErrInvalidBackup = errors.New("store: not an Arenile SQLite backup")

var sqliteHeader = []byte("SQLite format 3\x00")

// BackupSQLite writes a consistent copy of the SQLite database at dbPath to
// out with VACUUM INTO on a dedicated connection (safe while the server is
// running: WAL readers do not block writers). out must not exist; the
// result is 0600.
func BackupSQLite(ctx context.Context, dbPath, out string) error {
	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("store: backup: %s already exists", out)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return fmt.Errorf("store: backup: %w", err)
	}
	sqldb, err := sql.Open("sqlite", SQLiteDSN(dbPath, false))
	if err != nil {
		return fmt.Errorf("store: backup: %w", err)
	}
	defer sqldb.Close()
	sqldb.SetMaxOpenConns(1)
	if _, err := sqldb.ExecContext(ctx, "VACUUM INTO ?", out); err != nil {
		_ = os.Remove(out)
		return fmt.Errorf("store: backup: %w", err)
	}
	return os.Chmod(out, 0o600)
}

// BackupSQLiteDB is BackupSQLite through an already open database (the CLI
// opens it with the configuration).
func BackupSQLiteDB(ctx context.Context, db *DB, out string) error {
	if db.Kind != SQLite {
		return errors.New("store: backup: only SQLite is supported")
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", out); err != nil {
		return fmt.Errorf("store: backup: %w", err)
	}
	return os.Chmod(out, 0o600)
}

// ValidateSQLiteBackup checks that path is a SQLite file that passes
// PRAGMA integrity_check and carries the Arenile schema (goose table and
// establishments). Errors wrap ErrInvalidBackup, except I/O errors.
func ValidateSQLiteBackup(ctx context.Context, path string) error {
	f, err := os.Open(path) //nolint:gosec // G304: path chosen by the operator (CLI) or validated by the caller
	if err != nil {
		return err
	}
	head := make([]byte, len(sqliteHeader))
	_, err = io.ReadFull(f, head)
	_ = f.Close()
	if err != nil || !bytes.Equal(head, sqliteHeader) {
		return fmt.Errorf("%w: %s is not a SQLite database", ErrInvalidBackup, filepath.Base(path))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	sqldb, err := sql.Open("sqlite", "file:"+abs+"?mode=ro")
	if err != nil {
		return err
	}
	defer sqldb.Close()
	var res string
	if err := sqldb.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&res); err != nil {
		return fmt.Errorf("%w: integrity check: %w", ErrInvalidBackup, err)
	}
	if res != "ok" {
		return fmt.Errorf("%w: integrity check: %s", ErrInvalidBackup, res)
	}
	var n int
	if err := sqldb.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name IN ('goose_db_version', 'establishments')").Scan(&n); err != nil || n != 2 {
		return fmt.Errorf("%w: missing Arenile tables", ErrInvalidBackup)
	}
	return nil
}

// RestoreSQLiteFile validates src and copies it over dst atomically,
// removing dst's -wal and -shm files. The server must be stopped; keeping
// the previous dst is the caller's job.
func RestoreSQLiteFile(ctx context.Context, src, dst string) error {
	if err := ValidateSQLiteBackup(ctx, src); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dst + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("store: restore: %w", err)
		}
	}
	return CopyFileAtomic(src, dst)
}

// CopyFileAtomic copies src to dst through a synced temporary file and a
// rename (0600).
func CopyFileAtomic(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // G304: validated backup path
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // G304: data directory path
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
