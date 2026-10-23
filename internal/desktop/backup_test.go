package desktop

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/store"
)

func TestBackupFileName_RoundTrip(t *testing.T) {
	at := time.Date(2027, 7, 1, 9, 12, 44, 0, time.UTC)
	for _, r := range []control.BackupReason{control.BackupManual, control.BackupDaily, control.BackupExit, control.BackupPreRestore} {
		name := BackupFileName(at, r)
		if !BackupNamePattern.MatchString(name) {
			t.Fatalf("%s does not match the API pattern", name)
		}
		got, reason, ok := ParseBackupFileName(name)
		if !ok || !got.Equal(at) || reason != r {
			t.Fatalf("parse %s = %v %v %v", name, got, reason, ok)
		}
	}
	for _, bad := range []string{"arenile.db", "arenile-20270701T091244Z-weekly.db", "x-20270701T091244Z-daily.db", "arenile-20271301T091244Z-daily.db", "../arenile-20270701T091244Z-daily.db"} {
		if _, _, ok := ParseBackupFileName(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, r, ok := ParseBackupFileName("arenile-20270701T091244Z-manual-2.db"); !ok || r != control.BackupManual {
		t.Fatal("suffixed name rejected")
	}
}

func TestPruneCandidates_KeepsManualAndNewest(t *testing.T) {
	at := func(d int) time.Time { return time.Date(2027, 7, d, 0, 0, 0, 0, time.UTC) }
	items := []control.Backup{ // newest first
		{Name: "e5", Reason: control.BackupExit, CreatedAt: at(5)},
		{Name: "m4", Reason: control.BackupManual, CreatedAt: at(4)},
		{Name: "d3", Reason: control.BackupDaily, CreatedAt: at(3)},
		{Name: "p2", Reason: control.BackupPreRestore, CreatedAt: at(2)},
		{Name: "d1", Reason: control.BackupDaily, CreatedAt: at(1)},
	}
	got := PruneCandidates(items, 1)
	if len(got) != 2 || got[0].Name != "d3" || got[1].Name != "d1" {
		t.Fatalf("prune(1) = %+v", got)
	}
	if got := PruneCandidates(items, 3); len(got) != 0 {
		t.Fatalf("prune(3) = %+v", got)
	}
}

func TestDailyDue_Scenarios(t *testing.T) {
	start := time.Date(2027, 7, 1, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name                  string
		now, last, lastFailed time.Time
		enabled, want         bool
	}{
		{"disabled", start.Add(time.Hour), time.Time{}, time.Time{}, false, false},
		{"too early after start", start.Add(4 * time.Minute), time.Time{}, time.Time{}, true, false},
		{"first backup", start.Add(5 * time.Minute), time.Time{}, time.Time{}, true, true},
		{"recent daily", start.Add(time.Hour), start.Add(-22 * time.Hour), time.Time{}, true, false},
		{"daily is 24h old", start.Add(time.Hour), start.Add(-23 * time.Hour).Add(-time.Hour), time.Time{}, true, true},
		{"retry after failure too soon", start.Add(time.Hour), time.Time{}, start.Add(30 * time.Minute), true, false},
		{"retry after one hour", start.Add(2 * time.Hour), time.Time{}, start.Add(time.Hour), true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DailyDue(tt.now, start, tt.last, tt.lastFailed, tt.enabled); got != tt.want {
				t.Fatalf("DailyDue = %v, want %v", got, tt.want)
			}
		})
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBackuper_ListAndPrune_Files(t *testing.T) {
	dir := t.TempDir()
	at := func(d int) time.Time { return time.Date(2027, 7, d, 9, 0, 0, 0, time.UTC) }
	names := []string{
		BackupFileName(at(1), control.BackupDaily),
		BackupFileName(at(2), control.BackupDaily),
		BackupFileName(at(3), control.BackupExit),
		BackupFileName(at(4), control.BackupManual),
		BackupFileName(at(5), control.BackupDaily),
	}
	for _, n := range names {
		touch(t, filepath.Join(dir, n))
	}
	touch(t, filepath.Join(dir, "notes.txt"))
	if err := os.Mkdir(filepath.Join(dir, BackupFileName(at(6), control.BackupDaily)), 0o700); err != nil {
		t.Fatal(err)
	}
	b := &Backuper{}
	ctx := context.Background()
	items, err := b.List(ctx, dir)
	if err != nil || len(items) != 5 || items[0].Name != names[4] || items[4].Name != names[0] {
		t.Fatalf("list = %+v %v", items, err)
	}
	if err := b.Prune(ctx, dir, 2); err != nil {
		t.Fatal(err)
	}
	items, _ = b.List(ctx, dir)
	var left []string
	for _, it := range items {
		left = append(left, it.Name)
	}
	want := []string{names[4], names[3], names[2]} // 2 automatic + the manual one
	if len(left) != 3 || left[0] != want[0] || left[1] != want[1] || left[2] != want[2] {
		t.Fatalf("after prune %v, want %v", left, want)
	}
	if items, err := b.List(ctx, filepath.Join(dir, "missing")); err != nil || len(items) != 0 {
		t.Fatalf("missing dir: %v %v", items, err)
	}
}

// newMigratedSQLite creates an Arenile database (all migrations) at path.
func newMigratedSQLite(t *testing.T, path string) *store.DB {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, store.Options{SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	m, err := db.Migrator()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func countOrgs(t *testing.T, path string) int {
	t.Helper()
	sqldb, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer sqldb.Close()
	var n int
	if err := sqldb.QueryRow("SELECT count(*) FROM organizations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBackuper_BackupValidateRestore_SQLite(t *testing.T) {
	ctx := context.Background()
	data := filepath.Join(t.TempDir(), "Application Support") // a space, as on macOS
	dbPath := filepath.Join(data, "arenile.db")
	db := newMigratedSQLite(t, dbPath)
	now := time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC)
	if _, _, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", now); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewFake(now)
	b := &Backuper{DBPath: dbPath, Clock: clk}
	dir := filepath.Join(data, "backups")
	first, err := b.Backup(ctx, dir, control.BackupManual)
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(dir, first.Name)); err != nil || st.Mode().Perm() != 0o600 || first.SizeBytes == 0 {
		t.Fatalf("backup file %v %v %+v", st, err, first)
	}
	// Same second: a distinct name, never an overwrite.
	second, err := b.Backup(ctx, dir, control.BackupManual)
	if err != nil || second.Name == first.Name {
		t.Fatalf("second backup %+v %v", second, err)
	}
	if err := b.Validate(ctx, filepath.Join(dir, first.Name)); err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(dir, "junk.db")
	touch(t, junk)
	if err := b.Validate(ctx, junk); !errors.Is(err, store.ErrInvalidBackup) {
		t.Fatalf("junk validated: %v", err)
	}

	// Write after the backup, stop the "server", restore: the write is gone
	// and the replaced database is kept as pre_restore.
	if _, _, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Minute)
	if err := b.Restore(ctx, filepath.Join(dir, first.Name), dir); err != nil {
		t.Fatal(err)
	}
	if n := countOrgs(t, dbPath); n != 1 {
		t.Fatalf("restored orgs = %d, want 1", n)
	}
	items, _ := b.List(ctx, dir)
	if items[0].Reason != control.BackupPreRestore || countOrgs(t, filepath.Join(dir, items[0].Name)) != 2 {
		t.Fatalf("pre_restore backup missing: %+v", items)
	}
	if err := b.Restore(ctx, junk, dir); !errors.Is(err, store.ErrInvalidBackup) {
		t.Fatalf("restore of junk: %v", err)
	}
}
