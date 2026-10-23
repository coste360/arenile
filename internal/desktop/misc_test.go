package desktop

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/config"
)

func TestRotatingFile_Rotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", LogFileName)
	r, err := OpenRotatingFile(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	line := []byte(strings.Repeat("a", 59) + "\n") // 60 bytes
	for i := 0; i < 5; i++ {
		if _, err := r.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	_ = r.Close()
	for _, p := range []string{path, path + ".1", path + ".2"} {
		st, err := os.Stat(p)
		if err != nil || st.Size() != 60 || st.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", p, st, err)
		}
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Fatal("kept more files than Keep")
	}
	if _, err := r.Write(line); err == nil {
		t.Fatal("write after close")
	}
	// Reopen appends to the existing file.
	r2, _ := OpenRotatingFile(path, 1000, 2)
	_, _ = r2.Write(line)
	_ = r2.Close()
	if st, _ := os.Stat(path); st.Size() != 120 {
		t.Fatalf("append after reopen: %d", st.Size())
	}
}

func TestOpenCommand_PerOS(t *testing.T) {
	tests := []struct{ goos, target, name, first string }{
		{"darwin", "http://127.0.0.1:5/admin/", "open", "http://127.0.0.1:5/admin/"},
		{"linux", "/home/anna/.local/share/arenile", "xdg-open", "/home/anna/.local/share/arenile"},
		{"windows", "http://127.0.0.1:5/admin/", "rundll32", "url.dll,FileProtocolHandler"},
		{"windows", `C:\Users\anna\AppData\Roaming\Arenile`, "explorer", `C:\Users\anna\AppData\Roaming\Arenile`},
	}
	for _, tt := range tests {
		name, args := OpenCommand(tt.goos, tt.target)
		if name != tt.name || args[0] != tt.first || args[len(args)-1] != tt.target {
			t.Errorf("%s %s: %s %v", tt.goos, tt.target, name, args)
		}
	}
}

// TestOpenURL_OnlyWebLinks: the opener reachable from the remote-mode
// window (arenileOpenExternal) refuses everything that could start a local
// program or file: file: URLs, absolute paths, UNC paths, flags, custom
// schemes and credentials in the URL.
func TestOpenURL_OnlyWebLinks(t *testing.T) {
	bad := []string{
		"file:///etc/passwd", "file:/Applications/Terminal.app", "javascript://x", "-n", "",
		"/Applications/Calculator.app", `C:\Windows\System32\calc.exe`, `\\host\share\evil.exe`,
		"arenile://pair?d=x", "ftp://host/x", "http://", "http:///path", "https://user:pw@host/", "mailto:",
		"vbscript:msgbox(1)", "data:text/html,<script>1</script>",
	}
	for _, s := range bad {
		if IsWebLink(s) {
			t.Errorf("IsWebLink(%q) = true", s)
		}
		for _, goos := range []string{"darwin", "windows", "linux"} {
			if err := OpenURL(context.Background(), goos, s); !errors.Is(err, ErrUnsafeTarget) {
				t.Errorf("OpenURL(%s, %q): %v", goos, s, err)
			}
		}
	}
	for _, s := range []string{"https://www.coste360.it/", "http://127.0.0.1:5/admin/", "mailto:info@lido.example", "HTTPS://Lido.Example/x?y=1#f"} {
		if !IsWebLink(s) {
			t.Errorf("IsWebLink(%q) = false", s)
		}
	}
}

// TestOpenFolder_OnlyExistingDirectories: the tray "data folder" opener
// accepts an existing absolute directory and nothing else.
func TestOpenFolder_OnlyExistingDirectories(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "relative/dir", file, filepath.Join(dir, "missing"), "https://www.coste360.it/", `\\host\share`, "-" + dir} {
		if err := OpenFolder(context.Background(), "linux", bad); !errors.Is(err, ErrUnsafeTarget) {
			t.Errorf("OpenFolder(%q): %v", bad, err)
		}
	}
	if err := checkFolder(dir); err != nil {
		t.Errorf("checkFolder(%q): %v", dir, err)
	}
}

func TestNotifyCommand_Escaping(t *testing.T) {
	name, args, ok := NotifyCommand("darwin", `Arenile`, `Backup "ok" \ fatto`)
	if !ok || name != "osascript" || args[1] != `display notification "Backup \"ok\" \\ fatto" with title "Arenile"` {
		t.Fatalf("%s %q", name, args)
	}
	if _, args, ok := NotifyCommand("linux", "Arenile", "-x"); !ok || args[1] != "--" {
		t.Fatalf("linux %q", args)
	}
	if _, _, ok := NotifyCommand("windows", "a", "b"); ok {
		t.Fatal("windows has no command")
	}
}

func TestCompareVersions_Scenarios(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.4.0", "1.3.9", 1},
		{"v1.10.0", "1.9.0", 1},
		{"1.4", "1.4.0", 0},
		{"1.4.0-rc.1", "1.4.0", -1},
		{"v1.4.0", "1.4.0-rc.1", 1},
		{"1.4.0-rc.1", "1.4.0-rc.2", 0},
		{"0.6.0-dev.187+835d645", "v0.6.0", -1},
		{"0.6.0-dev.187+835d645.dirty", "0.5.9", 1},
		{"0.6.0+build.7", "0.6.0", 0},
		{"1.3.0", "v1.4.0", -1},
		{"1.0.0", "dev", 1},
		{"dev", "dev", 0},
		{"x.y", "1.0", -1},
	}
	for _, tt := range tests {
		if got := CompareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRunUpdateChecks_NoNetworkWhenDisabled(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Cookie") != "" || !strings.HasPrefix(r.UserAgent(), "arenile-desktop/") {
			t.Errorf("unexpected request headers %v", r.Header)
		}
		fmt.Fprint(w, `{"version":"1.5.0"}`)
	}))
	defer srv.Close()
	u := UpdateChecker{URL: srv.URL, Current: "1.4.0", Client: srv.Client()}
	var enabled atomic.Bool
	// Every check reads enabled once: the test waits for that read after
	// each tick, so flipping the switch never races with a check in flight.
	reads := make(chan bool, 16)
	isEnabled := func() bool {
		v := enabled.Load()
		reads <- v
		return v
	}
	notified := make(chan string, 4)
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunUpdateChecks(ctx, u, isEnabled, func(v string) { notified <- v }, tick)
		close(done)
	}()
	<-reads // the initial check
	for range 2 {
		tick <- time.Now()
		<-reads
	}
	if hits.Load() != 0 {
		t.Fatalf("network call while disabled: %d", hits.Load())
	}
	enabled.Store(true)
	for range 3 { // same version every time: notified once
		tick <- time.Now()
		<-reads
	}
	// One more tick is received only after the previous check finished.
	enabled.Store(false)
	tick <- time.Now()
	<-reads
	cancel()
	<-done
	if hits.Load() != 3 || len(notified) != 1 || <-notified != "1.5.0" {
		t.Fatalf("hits=%d notified=%d", hits.Load(), len(notified))
	}
}

func TestTrayIcon_PNGAndICO(t *testing.T) {
	for _, tc := range []struct {
		size     int
		template bool
		want     int
	}{
		{16, false, 16},
		{32, false, 32},
		{44, false, 64},
		{200, false, 64},
		{18, true, 18},
		{32, true, 36},
		{44, true, 36},
	} {
		b := TrayIconPNG(tc.size, tc.template)
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil || img.Bounds().Dx() != tc.want || img.Bounds().Dy() != tc.want {
			t.Fatalf("size %d template=%v: got %v, %v", tc.size, tc.template, img, err)
		}
		// Transparent corners; the template image is a pure black silhouette.
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Fatalf("size %d template=%v: corner alpha %d", tc.size, tc.template, a)
		}
		c := tc.want / 2
		r, g, bl, a := img.At(c, c*3/2).RGBA()
		if tc.template && a > 0 && (r|g|bl) != 0 {
			t.Fatalf("template pixel not black: %d %d %d", r, g, bl)
		}
		if !tc.template && a == 0 {
			t.Fatalf("colour tile has a transparent centre")
		}
	}
	p16, p32 := TrayIconPNG(16, false), TrayIconPNG(32, false)
	ico := ICOFromPNG(p16, p32)
	if binary.LittleEndian.Uint16(ico[2:]) != 1 || binary.LittleEndian.Uint16(ico[4:]) != 2 || ico[6] != 16 || ico[22] != 32 {
		t.Fatalf("ico header % x", ico[:38])
	}
	off := binary.LittleEndian.Uint32(ico[6+12:])
	if !bytes.Equal(ico[off:off+uint32(len(p16))], p16) {
		t.Fatal("first image offset")
	}
}

func TestDesktopConfig_LoopbackProfile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := DesktopConfig(dir, 51234, bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:51234" || cfg.BaseURL != "http://127.0.0.1:51234" || cfg.SQLitePath() != filepath.Join(dir, "arenile.db") {
		t.Fatalf("cfg %+v", cfg)
	}
	if cfg.SecureCookies() || cfg.Telemetry || !cfg.AutoMigrate || cfg.UsesPostgres() || cfg.ValidateServe() != nil {
		t.Fatalf("profile %+v", cfg)
	}
	// Desktop is an edition, not development: Dev stays off.
	if cfg.Dev || !cfg.Desktop() || cfg.Edition != config.EditionDesktop {
		t.Fatalf("edition %q dev=%v", cfg.Edition, cfg.Dev)
	}
	t.Setenv("DATABASE_URL", "postgres://elsewhere")
	t.Setenv("ARENILE_TELEMETRY", "true")
	cfg2, _ := DesktopConfig(dir, 1, bytes.Repeat([]byte{7}, 32))
	if cfg2.UsesPostgres() || cfg2.Telemetry {
		t.Fatal("the process environment must not change the desktop configuration")
	}
}
