package desktop

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/meta"
)

// fakeBackups is an in-memory BackupStore driven by the fake clock.
type fakeBackups struct {
	mu      sync.Mutex
	clk     clock.Clock
	items   []control.Backup // newest first
	fail    error
	block   chan struct{} // when set, Backup waits on it
	entered chan struct{}
	pruned  []int
	invalid map[string]bool
}

func (f *fakeBackups) Backup(_ context.Context, _ string, reason control.BackupReason) (control.Backup, error) {
	if f.block != nil {
		f.entered <- struct{}{}
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return control.Backup{}, f.fail
	}
	b := control.Backup{Name: BackupFileName(f.clk.Now(), reason), CreatedAt: f.clk.Now(), Reason: reason, SizeBytes: 1}
	f.items = append([]control.Backup{b}, f.items...)
	return b, nil
}

func (f *fakeBackups) List(context.Context, string) ([]control.Backup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]control.Backup(nil), f.items...), nil
}

func (f *fakeBackups) Prune(_ context.Context, _ string, retention int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruned = append(f.pruned, retention)
	return nil
}

func (f *fakeBackups) Validate(_ context.Context, path string) error {
	if f.invalid[filepath.Base(path)] {
		return errors.New("store: not an Arenile SQLite backup")
	}
	return nil
}

func (f *fakeBackups) Restore(context.Context, string, string) error { return nil }

func (f *fakeBackups) count(reason control.BackupReason) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, b := range f.items {
		if b.Reason == reason {
			n++
		}
	}
	return n
}

type fakeAutostart struct {
	on    bool
	calls int
	err   error
}

func (a *fakeAutostart) Enabled() (bool, error) { return a.on, nil }
func (a *fakeAutostart) SetEnabled(on bool) error {
	a.calls++
	if a.err != nil {
		return a.err
	}
	a.on = on
	return nil
}

type fakeLAN struct {
	enabled bool
	port    int
}

func (l *fakeLAN) Enable(_ context.Context, port int) (string, string, error) {
	l.enabled, l.port = true, port
	return "https://192.168.1.20:8443", "AA:BB", nil
}

func (l *fakeLAN) Disable(context.Context) error { l.enabled = false; return nil }

func newTestRuntime(t *testing.T, mut func(*RuntimeDeps)) (*Runtime, *fakeBackups, *clock.Fake) {
	t.Helper()
	clk := clock.NewFake(time.Date(2027, 7, 1, 8, 0, 0, 0, time.UTC))
	fb := &fakeBackups{clk: clk}
	d := RuntimeDeps{DataDir: t.TempDir(), Backuper: fb, Clock: clk}
	if mut != nil {
		mut(&d)
	}
	return NewRuntime(d), fb, clk
}

func TestRuntime_UpdateSettings_SideEffects(t *testing.T) {
	ctx := context.Background()
	auto := &fakeAutostart{}
	lan := &fakeLAN{}
	var notified []control.Settings
	rt, _, _ := newTestRuntime(t, func(d *RuntimeDeps) {
		d.Autostart, d.LAN = auto, lan
		d.OnSettingsChanged = func(s control.Settings) { notified = append(notified, s) }
	})
	s, err := rt.Settings(ctx)
	if err != nil || s.Autostart || s.LANEnabled || s.UpdateCheck || !s.BackupDaily {
		t.Fatalf("defaults %+v %v", s, err)
	}
	s.Autostart, s.LANEnabled, s.BackupRetention = true, true, 7
	s.BackupDir = filepath.Join(t.TempDir(), "usb", "arenile")
	got, err := rt.UpdateSettings(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if !auto.on || !lan.enabled || lan.port != DefaultLANPort || got.BackupRetention != 7 || len(notified) != 1 {
		t.Fatalf("side effects: auto=%v lan=%v port=%d %+v", auto.on, lan.enabled, lan.port, got)
	}
	if m := rt.Runtime(); m.Edition != meta.EditionDesktop || m.Desktop.Mode != meta.DesktopLAN || m.Desktop.LANURL == "" {
		t.Fatalf("meta runtime %+v", m.Desktop)
	}
	reloaded, _ := rt.Settings(ctx)
	if reloaded != got {
		t.Fatalf("not persisted: %+v vs %+v", reloaded, got)
	}
	// Unchanged autostart: the OS entry is not touched again.
	got.LANEnabled = false
	if _, err := rt.UpdateSettings(ctx, got); err != nil {
		t.Fatal(err)
	}
	if auto.calls != 1 || lan.enabled || rt.Runtime().Desktop.Mode != meta.DesktopLocal {
		t.Fatalf("second update: calls=%d lan=%v", auto.calls, lan.enabled)
	}
	// The OS state wins over desktop.json (entry removed by hand).
	auto.on = false
	if s, _ := rt.Settings(ctx); s.Autostart {
		t.Fatal("autostart must reflect the OS entry")
	}
}

func TestRuntime_UpdateSettings_Rejections(t *testing.T) {
	ctx := context.Background()
	rt, _, _ := newTestRuntime(t, nil) // no LAN adapter, no autostart
	s, _ := rt.Settings(ctx)
	s.LANEnabled = true
	if _, err := rt.UpdateSettings(ctx, s); !errors.Is(err, control.ErrInvalidSettings) {
		t.Fatalf("LAN without adapter: %v", err)
	}
	s.LANEnabled = false
	s.BackupDir = "relative/path"
	if _, err := rt.UpdateSettings(ctx, s); !errors.Is(err, control.ErrInvalidSettings) {
		t.Fatalf("relative dir: %v", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	touch(t, file)
	s.BackupDir = filepath.Join(file, "sub") // parent is a file: not creatable
	if _, err := rt.UpdateSettings(ctx, s); !errors.Is(err, control.ErrInvalidSettings) {
		t.Fatalf("unwritable dir: %v", err)
	}
	auto := &fakeAutostart{err: errors.New("denied")}
	rt2, _, _ := newTestRuntime(t, func(d *RuntimeDeps) { d.Autostart = auto })
	s2, _ := rt2.Settings(ctx)
	s2.Autostart = true
	if _, err := rt2.UpdateSettings(ctx, s2); err == nil {
		t.Fatal("autostart failure must be reported")
	}
}

func TestRuntime_BackupNow_InProgressAndPrune(t *testing.T) {
	ctx := context.Background()
	rt, fb, _ := newTestRuntime(t, nil)
	fb.block, fb.entered = make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := rt.BackupNow(ctx, control.BackupManual)
		done <- err
	}()
	<-fb.entered
	if _, err := rt.BackupNow(ctx, control.BackupManual); !errors.Is(err, control.ErrBackupInProgress) {
		t.Fatalf("concurrent backup: %v", err)
	}
	close(fb.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(fb.pruned) != 1 || fb.pruned[0] != DefaultBackupRetention {
		t.Fatalf("prune calls %v", fb.pruned)
	}
}

func TestRuntime_ScheduleRestore(t *testing.T) {
	ctx := context.Background()
	rt, fb, _ := newTestRuntime(t, nil)
	b, _ := fb.Backup(ctx, "", control.BackupDaily)
	if _, err := rt.ScheduleRestore(ctx, "../etc/passwd"); !errors.Is(err, control.ErrBackupNotFound) {
		t.Fatalf("traversal: %v", err)
	}
	if _, err := rt.ScheduleRestore(ctx, "arenile-20990101T000000Z-daily.db"); !errors.Is(err, control.ErrBackupNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	fb.invalid = map[string]bool{b.Name: true}
	if _, err := rt.ScheduleRestore(ctx, b.Name); err == nil {
		t.Fatal("damaged backup scheduled")
	}
	fb.invalid = nil
	got, err := rt.ScheduleRestore(ctx, b.Name)
	if err != nil || got.Name != b.Name {
		t.Fatalf("schedule: %+v %v", got, err)
	}
	if _, err := rt.ScheduleRestore(ctx, b.Name); !errors.Is(err, control.ErrBackupInProgress) {
		t.Fatalf("second schedule: %v", err)
	}
	if name := <-rt.Restarts(); name != b.Name {
		t.Fatalf("restart %q", name)
	}
}

func TestRuntime_SetupTokenCompletion(t *testing.T) {
	completed := 0
	rt, _, _ := newTestRuntime(t, func(d *RuntimeDeps) { d.OnSetupCompleted = func() { completed++ } })
	if err := rt.ConsumeSetupToken("x"); !errors.Is(err, control.ErrSetupTokenInvalid) {
		t.Fatalf("no token installed: %v", err)
	}
	tok, _ := NewSetupToken()
	rt.SetSetupToken(tok)
	if err := rt.ConsumeSetupToken(tok.Value()); err != nil || completed != 1 {
		t.Fatalf("consume: %v %d", err, completed)
	}
}

func TestScheduler_DailyWithFakeClock(t *testing.T) {
	ctx := context.Background()
	rt, fb, clk := newTestRuntime(t, nil)
	var failures int
	s := &Scheduler{Runtime: rt, Clock: clk, OnFailure: func(error) { failures++ }}
	s.Start()
	clk.Advance(4 * time.Minute)
	if s.Step(ctx) {
		t.Fatal("backup before the start delay")
	}
	clk.Advance(time.Minute)
	if !s.Step(ctx) || fb.count(control.BackupDaily) != 1 {
		t.Fatal("first daily backup not made")
	}
	clk.Advance(23 * time.Hour)
	if s.Step(ctx) {
		t.Fatal("second backup within 24h")
	}
	clk.Advance(time.Hour)
	if !s.Step(ctx) || fb.count(control.BackupDaily) != 2 {
		t.Fatal("second daily backup not made")
	}
	// Failure: retried after an hour, not at every tick.
	fb.fail = errors.New("usb stick missing")
	clk.Advance(24 * time.Hour)
	if s.Step(ctx) || failures != 1 {
		t.Fatalf("failure not reported: %d", failures)
	}
	clk.Advance(10 * time.Minute)
	s.Step(ctx)
	if failures != 1 {
		t.Fatal("retried too early")
	}
	fb.fail = nil
	clk.Advance(time.Hour)
	if !s.Step(ctx) {
		t.Fatal("retry after one hour failed")
	}
	// Disabled in the settings: nothing.
	st, _ := rt.Settings(ctx)
	st.BackupDaily = false
	if _, err := rt.UpdateSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	clk.Advance(48 * time.Hour)
	if s.Step(ctx) {
		t.Fatal("backup while disabled")
	}
}

func TestScheduler_RestartReadsLastDaily(t *testing.T) {
	ctx := context.Background()
	rt, fb, clk := newTestRuntime(t, nil)
	if _, err := fb.Backup(ctx, "", control.BackupDaily); err != nil { // made by the previous run
		t.Fatal(err)
	}
	clk.Advance(2 * time.Hour)
	s := &Scheduler{Runtime: rt, Clock: clk}
	s.Start()
	clk.Advance(10 * time.Minute)
	if s.Step(ctx) {
		t.Fatal("a restart must not cause an extra daily backup")
	}
	clk.Advance(22 * time.Hour)
	if !s.Step(ctx) {
		t.Fatal("daily backup not made 24h after the previous one")
	}
}

func TestScheduler_RunStopsOnCancel(t *testing.T) {
	rt, fb, clk := newTestRuntime(t, nil)
	tick := make(chan time.Time)
	s := &Scheduler{Runtime: rt, Clock: clk, Tick: tick}
	s.Start()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = s.Run(ctx); close(done) }()
	clk.Advance(time.Hour)
	tick <- clk.Now()
	tick <- clk.Now() // the second send waits for the first Step to finish
	cancel()
	<-done
	if fb.count(control.BackupDaily) != 1 {
		t.Fatalf("daily backups = %d", fb.count(control.BackupDaily))
	}
}
