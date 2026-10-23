package shell

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWindowState_Normalize(t *testing.T) {
	cases := []struct {
		name string
		in   WindowState
		want WindowState
	}{
		{"zero gives defaults", WindowState{}, WindowState{Width: DefaultWidth, Height: DefaultHeight, Zoom: 1}},
		{"below minimum", WindowState{Width: 300, Height: 200, Zoom: 1}, WindowState{Width: MinWidth, Height: MinHeight, Zoom: 1}},
		{"huge", WindowState{Width: 99999, Height: 99999, Zoom: 1}, WindowState{Width: maxSide, Height: maxSide, Zoom: 1}},
		{"unplaced drops coordinates", WindowState{X: 10, Y: 20, Width: 1200, Height: 800, Zoom: 1}, WindowState{Width: 1200, Height: 800, Zoom: 1}},
		{"placed keeps coordinates", WindowState{X: -1500, Y: 20, Width: 1200, Height: 800, Placed: true, Zoom: 1.25}, WindowState{X: -1500, Y: 20, Width: 1200, Height: 800, Placed: true, Zoom: 1.25}},
		{"absurd coordinates", WindowState{X: 1 << 30, Y: 0, Width: 1200, Height: 800, Placed: true, Zoom: 1}, WindowState{Width: 1200, Height: 800, Zoom: 1}},
		{"zoom snapped", WindowState{Width: 1200, Height: 800, Zoom: 1.12}, WindowState{Width: 1200, Height: 800, Zoom: 1.1}},
		{"zoom invalid", WindowState{Width: 1200, Height: 800, Zoom: -3}, WindowState{Width: 1200, Height: 800, Zoom: 1}},
		{"maximized kept", WindowState{Width: 1200, Height: 800, Maximized: true, Zoom: 1}, WindowState{Width: 1200, Height: 800, Maximized: true, Zoom: 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.in.Normalize(); got != c.want {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestWindowState_PersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := LoadState(dir); got != DefaultState() {
		t.Fatalf("missing file: %+v", got)
	}
	s := WindowState{X: 40, Y: 60, Width: 1400, Height: 900, Placed: true, Maximized: true, Zoom: 1.25}
	if err := SaveState(dir, s); err != nil {
		t.Fatal(err)
	}
	if got := LoadState(dir); got != s {
		t.Fatalf("round trip: %+v", got)
	}
	st, err := os.Stat(filepath.Join(dir, stateFileName))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("file mode: %v %v", st, err)
	}
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadState(dir); got != DefaultState() {
		t.Fatalf("corrupted file: %+v", got)
	}
	if err := SaveState("", s); err == nil {
		t.Fatal("empty data dir accepted")
	}
}

func TestNextZoom_Steps(t *testing.T) {
	cases := []struct {
		cur  float64
		dir  int
		want float64
	}{
		{1, 1, 1.1}, {1, -1, 0.9}, {1.75, 0, 1}, {2, 1, 2}, {0.5, -1, 0.5}, {0, 1, 1.1}, {1.12, 1, 1.25},
	}
	for _, c := range cases {
		if got := NextZoom(c.cur, c.dir); got != c.want {
			t.Errorf("NextZoom(%v,%d)=%v want %v", c.cur, c.dir, got, c.want)
		}
	}
}

func TestCloseAction_PerPlatform(t *testing.T) {
	cases := []struct {
		goos          string
		keep, hasTray bool
		want          CloseBehaviour
	}{
		{"darwin", false, true, CloseHides},
		{"darwin", false, false, CloseHides},
		{"windows", false, true, CloseQuits},
		{"windows", true, true, CloseHides},
		{"windows", true, false, CloseQuits},
		{"linux", true, true, CloseHides},
	}
	for _, c := range cases {
		if got := CloseAction(c.goos, c.keep, c.hasTray); got != c.want {
			t.Errorf("%s keep=%v tray=%v: %v", c.goos, c.keep, c.hasTray, got)
		}
	}
}

func TestAppMenu_EveryEntryIsWired(t *testing.T) {
	calls := map[string]int{}
	cmd := Commands{
		Quit:          func() { calls["quit"]++ },
		OpenSettings:  func() { calls["settings"]++ },
		OpenInBrowser: func() { calls["browser"]++ },
		Reload:        func() { calls["reload"]++ },
		Zoom:          func(d int) { calls["zoom"] += 10 + d },
	}
	roles := map[string]bool{}
	keys := map[string]Action{}
	for _, m := range AppMenu() {
		roles[m.Role] = true
		for _, e := range m.Entries {
			if e.Separator {
				continue
			}
			if e.Label == "" || strings.HasPrefix(e.Label, "app.") || strings.HasPrefix(e.Label, "menu.") {
				t.Errorf("%s: missing translation %q", e.Action, e.Label)
			}
			if e.Selector == "" {
				if _, ok := cmd.Handler(e.Action); !ok {
					t.Errorf("%s: neither native selector nor Go handler", e.Action)
				}
			} else if !strings.HasSuffix(e.Selector, ":") {
				t.Errorf("%s: selector %q", e.Action, e.Selector)
			}
			if e.Key != "" {
				combo := e.Key
				if e.Shift {
					combo += "+shift"
				}
				if e.Option {
					combo += "+option"
				}
				if e.Control {
					combo += "+control"
				}
				if prev, dup := keys[combo]; dup {
					t.Errorf("shortcut %s on %s and %s", combo, prev, e.Action)
				}
				keys[combo] = e.Action
			}
		}
	}
	for _, r := range []string{RoleApp, RoleEdit, RoleView, RoleWindow} {
		if !roles[r] {
			t.Errorf("menu %s missing", r)
		}
	}
	// The shortcuts the operator expects.
	for combo, a := range map[string]Action{"q": ActionQuit, "c": ActionCopy, "v": ActionPaste, "x": ActionCut, "z": ActionUndo, "z+shift": ActionRedo, "a": ActionSelectAll, "r": ActionReload, "m": ActionMinimize, ",": ActionSettings, "+": ActionZoomIn, "-": ActionZoomOut, "0": ActionZoomReset} {
		if keys[combo] != a {
			t.Errorf("Cmd+%s = %q, want %s", combo, keys[combo], a)
		}
	}
	for _, a := range []Action{ActionQuit, ActionSettings, ActionOpenInBrowser, ActionReload, ActionZoomIn, ActionZoomOut, ActionZoomReset} {
		if !cmd.Dispatch(a) {
			t.Errorf("%s not dispatched", a)
		}
	}
	if cmd.Dispatch(ActionCopy) || cmd.Dispatch("nope") {
		t.Fatal("native or unknown action dispatched to Go")
	}
	if calls["quit"] != 1 || calls["settings"] != 1 || calls["browser"] != 1 || calls["reload"] != 1 || calls["zoom"] != 30 {
		t.Fatalf("calls: %v", calls)
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"corrispettivi-2027-07.csv":     "corrispettivi-2027-07.csv",
		"../../etc/passwd":              "passwd",
		`..\..\Windows\win.ini`:         "win.ini",
		".bashrc":                       "bashrc",
		"":                              "download",
		"...":                           "download",
		"a<b>c:d\"e|f?g*h.csv":          "a_b_c_d_e_f_g_h.csv",
		"line\nbreak\x00.ics":           "line_break_.ics",
		"CON.txt":                       "_CON.txt",
		"fattura\u202efdp.exe":          "fattura_fdp.exe",
		"com3":                          "_com3",
		"trailing dots. . ":             "trailing dots",
		"prenotazioni estate 2027.ics ": "prenotazioni estate 2027.ics",
	}
	for in, want := range cases {
		if got := SanitizeFileName(in); got != want {
			t.Errorf("SanitizeFileName(%q)=%q want %q", in, got, want)
		}
	}
	long := strings.Repeat("è", 300) + ".csv"
	got := SanitizeFileName(long)
	if len(got) > maxFileNameBytes || !strings.HasSuffix(got, ".csv") || !strings.HasPrefix(got, "è") {
		t.Fatalf("long name: %d %q", len(got), got[len(got)-8:])
	}
}

func TestUniquePath_NeverOverwrites(t *testing.T) {
	taken := map[string]bool{"/d/a.csv": true, "/d/a (1).csv": true, "/d/noext": true}
	exists := func(p string) bool { return taken[filepath.ToSlash(p)] }
	for name, want := range map[string]string{"a.csv": "/d/a (2).csv", "b.csv": "/d/b.csv", "noext": "/d/noext (1)"} {
		got, err := UniquePath("/d", name, exists)
		if err != nil || filepath.ToSlash(got) != want {
			t.Errorf("%s: %q %v", name, got, err)
		}
	}
	if _, err := UniquePath("/d", "x", func(string) bool { return true }); err == nil {
		t.Fatal("no error when every name is taken")
	}
	dir := filepath.Join(t.TempDir(), "Downloads")
	p, err := DownloadPath(dir, "../export.csv")
	if err != nil || p != filepath.Join(dir, "export.csv") {
		t.Fatalf("DownloadPath: %q %v", p, err)
	}
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if p2, _ := DownloadPath(dir, "export.csv"); p2 != filepath.Join(dir, "export (1).csv") {
		t.Fatalf("second download: %q", p2)
	}
	if _, err := DownloadPath("", "x"); err == nil {
		t.Fatal("empty downloads dir accepted")
	}
	home := t.TempDir()
	if DownloadsDir(home) != filepath.Join(home, "Downloads") || DownloadsDir("") != "" {
		t.Fatal("DownloadsDir")
	}
}

func TestAllowExternal_Policy(t *testing.T) {
	panel := "http://127.0.0.1:5173/admin/"
	for target, want := range map[string]bool{
		"https://dashboard.stripe.com/":    true,
		"https://docs.arenile.it/guida":    true,
		"mailto:info@example.it":           true,
		"http://127.0.0.1:5173/admin/oggi": false,
		"http://127.0.0.1:9999/":           true,
		"file:///etc/passwd":               false,
		"javascript:alert(1)":              false,
		"arenile://pair?d=x":               false,
		`\\server\share`:                   false,
		"https://user:pass@example.it/":    false,
		"/admin/desktop":                   false,
	} {
		if got := AllowExternal(panel, target); got != want {
			t.Errorf("AllowExternal(%q)=%v", target, got)
		}
	}
}

func TestInitScript_DesktopGlobalAndPrint(t *testing.T) {
	mac := InitScript("v1.2.3", false)
	for _, want := range []string{`"mode":"local"`, `"shell":"window"`, `"version":"v1.2.3"`, "arenileOpenExternal", "window.print = function", "arenilePrint"} {
		if !strings.Contains(mac, want) {
			t.Errorf("macOS script lacks %q", want)
		}
	}
	if win := InitScript("v1", true); strings.Contains(win, "arenilePrint") {
		t.Error("WebView2 script overrides window.print")
	}
}

// fakeNative records the calls of the controller.
type fakeNative struct {
	mu    sync.Mutex
	calls []string
	frame WindowState
	zoom  float64
}

func (f *fakeNative) rec(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }

func (f *fakeNative) Navigate(u string) { f.rec("navigate " + u) }
func (f *fakeNative) Show()             { f.rec("show") }
func (f *fakeNative) Hide()             { f.rec("hide") }
func (f *fakeNative) Reload()           { f.rec("reload") }
func (f *fakeNative) SetZoom(z float64) { f.mu.Lock(); f.zoom = z; f.mu.Unlock(); f.rec("zoom") }
func (f *fakeNative) Terminate()        { f.rec("terminate") }
func (f *fakeNative) Frame() (WindowState, bool) {
	return f.frame, f.frame.Width > 0
}

func (f *fakeNative) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls
	f.calls = nil
	return c
}

func eq(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("calls %q, want %q", got, want)
	}
}

func TestController_PanelNavigationAndClose(t *testing.T) {
	dir := t.TempDir()
	var quits, opened []string
	keep := false
	var notes []string
	c := NewController(Options{
		GOOS: "windows", DataDir: dir, HasTray: true,
		KeepInTray:   func() bool { return keep },
		Quit:         func() { quits = append(quits, "quit") },
		OpenBrowser:  func(u string) error { opened = append(opened, u); return nil },
		Notify:       func(_, m string) { notes = append(notes, m) },
		DownloadsDir: filepath.Join(dir, "Downloads"),
	})
	// Before the window exists the launcher's URL is remembered.
	c.ShowPanel("http://127.0.0.1:1/admin/first-run#token=x")
	n := &fakeNative{frame: WindowState{X: 5, Y: 6, Width: 1300, Height: 850, Placed: true}}
	if u := c.Attach(n); u != "http://127.0.0.1:1/admin/first-run#token=x" {
		t.Fatalf("attach: %q", u)
	}
	eq(t, n.take(), "navigate http://127.0.0.1:1/admin/first-run#token=x")

	// Second launch / tray "Apri pannello" with the same URL: focus only.
	c.ShowPanel("http://127.0.0.1:1/admin/first-run#token=x")
	eq(t, n.take(), "show")
	// A new URL (setup done, LAN QR page) navigates.
	c.ShowPanel("http://127.0.0.1:1/admin/")
	eq(t, n.take(), "navigate http://127.0.0.1:1/admin/", "show")

	// Menu wiring through the controller.
	c.OnMenuAction(ActionSettings)
	eq(t, n.take(), "navigate http://127.0.0.1:1/admin/desktop", "show")
	c.OnMenuAction(ActionReload)
	eq(t, n.take(), "reload")
	c.OnMenuAction(ActionZoomIn)
	c.OnMenuAction(ActionZoomIn)
	eq(t, n.take(), "zoom", "zoom")
	if n.zoom != 1.25 || LoadState(dir).Zoom != 1.25 {
		t.Fatalf("zoom %v, saved %v", n.zoom, LoadState(dir).Zoom)
	}
	c.OnMenuAction(ActionOpenInBrowser)
	if len(opened) != 1 || opened[0] != "http://127.0.0.1:1/admin/desktop" {
		t.Fatalf("browser: %v", opened)
	}

	// Windows, default setting: closing quits and saves the geometry.
	if b := c.OnClose(); b != CloseQuits || len(quits) != 1 {
		t.Fatalf("close: %v %v", b, quits)
	}
	if s := LoadState(dir); s.X != 5 || s.Width != 1300 || s.Zoom != 1.25 {
		t.Fatalf("saved state: %+v", s)
	}
	// keep_running_in_tray: closing hides.
	keep = true
	if b := c.OnClose(); b != CloseHides {
		t.Fatalf("close with tray: %v", b)
	}
	eq(t, n.take(), "hide")
	c.OnReopen()
	eq(t, n.take(), "show")
	c.OnMenuAction(ActionQuit)
	if len(quits) != 2 {
		t.Fatalf("quit action: %v", quits)
	}

	// External links: only other origins, only web links.
	c.OpenExternal("https://dashboard.stripe.com/")
	c.OpenExternal("file:///etc/passwd")
	c.OpenExternal("http://127.0.0.1:1/admin/oggi")
	if len(opened) != 2 || opened[1] != "https://dashboard.stripe.com/" {
		t.Fatalf("external: %v", opened)
	}

	// Downloads: sanitized unique path, notification with the file name.
	p, err := c.DownloadDestination("../corrispettivi.csv")
	if err != nil || p != filepath.Join(dir, "Downloads", "corrispettivi.csv") {
		t.Fatalf("download path: %q %v", p, err)
	}
	c.DownloadFinished(p, false)
	c.DownloadFinished(p, true)
	if len(notes) != 2 || !strings.Contains(notes[0], "corrispettivi.csv") || !strings.Contains(notes[1], "corrispettivi.csv") || notes[0] == notes[1] {
		t.Fatalf("notes: %v", notes)
	}

	c.Detach()
	c.ShowPanel("http://127.0.0.1:1/admin/x")
	if c.Current() != "http://127.0.0.1:1/admin/desktop" {
		t.Fatalf("current after detach: %q", c.Current())
	}
}

func TestController_MacClosesToMenuBar(t *testing.T) {
	quit := 0
	c := NewController(Options{GOOS: "darwin", DataDir: t.TempDir(), Quit: func() { quit++ }})
	n := &fakeNative{}
	c.Attach(n)
	if b := c.OnClose(); b != CloseHides || quit != 0 {
		t.Fatalf("mac close: %v quit=%d", b, quit)
	}
	eq(t, n.take(), "hide")
	// No frame from the platform: nothing saved, no error.
	c.SaveFrame()
	if _, err := os.Stat(filepath.Join(c.o.DataDir, stateFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state written without a frame: %v", err)
	}
}
