package desktop

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden files in testdata/")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "autostart", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v (run go test -run Autostart -update)", name, err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s differs from the golden file:\n%s", name, got)
	}
}

func TestAutostart_GoldenPerOS(t *testing.T) {
	golden(t, "darwin.plist", LaunchAgentPlist("/Applications/Arenile.app/Contents/MacOS/arenile-desktop"))
	golden(t, "darwin-escaped.plist", LaunchAgentPlist("/Users/anna & co/<Arenile>.app/Contents/MacOS/arenile-desktop"))
	golden(t, "linux.desktop", XDGAutostartEntry("/home/anna/Applicazioni/Arenile.AppImage"))
	golden(t, "linux-quoted.desktop", XDGAutostartEntry(`/home/anna/My "Apps"/$HOME/100%/arenile`))
	golden(t, "windows-run.txt", []byte(WindowsRunValue(`C:\Program Files\Arenile\arenile-desktop.exe`)+"\n"))
}

func TestAutostart_FileStrategies(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	home := t.TempDir()
	tests := []struct {
		name, goos string
		env        map[string]string
		wantPath   string
		wantExe    string
	}{
		{"macos", "darwin", nil, filepath.Join(home, "Library", "LaunchAgents", "it.coste360.arenile.plist"), "/opt/arenile"},
		{"linux", "linux", nil, filepath.Join(home, ".config", "autostart", "arenile.desktop"), "/opt/arenile"},
		{"linux xdg", "linux", map[string]string{"XDG_CONFIG_HOME": filepath.Join(home, "xdg")}, filepath.Join(home, "xdg", "autostart", "arenile.desktop"), "/opt/arenile"},
		{"linux appimage", "linux", map[string]string{"APPIMAGE": "/home/anna/Arenile.AppImage"}, filepath.Join(home, ".config", "autostart", "arenile.desktop"), "/home/anna/Arenile.AppImage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewAutostart(tt.goos, home, "/opt/arenile", env(tt.env), nil)
			if err != nil {
				t.Fatal(err)
			}
			if on, err := a.Enabled(); err != nil || on {
				t.Fatalf("initially enabled: %v %v", on, err)
			}
			if err := a.SetEnabled(true); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(tt.wantPath)
			if err != nil {
				t.Fatalf("entry not at %s: %v", tt.wantPath, err)
			}
			if !strings.Contains(string(b), tt.wantExe) {
				t.Fatalf("entry does not start %s:\n%s", tt.wantExe, b)
			}
			if on, _ := a.Enabled(); !on {
				t.Fatal("not enabled after SetEnabled(true)")
			}
			if err := a.SetEnabled(false); err != nil {
				t.Fatal(err)
			}
			if err := a.SetEnabled(false); err != nil { // idempotent
				t.Fatal(err)
			}
			if on, _ := a.Enabled(); on {
				t.Fatal("still enabled")
			}
		})
	}
}

type fakeRunKey struct {
	values map[string]string
	err    error
}

func (k *fakeRunKey) Get(name string) (string, bool, error) {
	v, ok := k.values[name]
	return v, ok, k.err
}

func (k *fakeRunKey) Set(name, value string) error {
	if k.err != nil {
		return k.err
	}
	k.values[name] = value
	return nil
}

func (k *fakeRunKey) Delete(name string) error { delete(k.values, name); return nil }

func TestAutostart_WindowsRegistry(t *testing.T) {
	key := &fakeRunKey{values: map[string]string{}}
	a, err := NewAutostart("windows", `C:\Users\anna`, `C:\Program Files\Arenile\arenile-desktop.exe`, os.Getenv, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if key.values["Arenile"] != `"C:\Program Files\Arenile\arenile-desktop.exe"` {
		t.Fatalf("run value %q", key.values["Arenile"])
	}
	if on, _ := a.Enabled(); !on {
		t.Fatal("not enabled")
	}
	if err := a.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if on, _ := a.Enabled(); on {
		t.Fatal("still enabled")
	}
	key.err = errors.New("access denied")
	if err := a.SetEnabled(true); err == nil {
		t.Fatal("registry error swallowed")
	}
	if _, err := NewAutostart("darwin", "", "/x", os.Getenv, nil); err == nil {
		t.Fatal("macOS without home accepted")
	}
	if _, err := NewAutostart("linux", "/h", "", os.Getenv, nil); err == nil {
		t.Fatal("empty executable accepted")
	}
}
