package desktop

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Autostart identifiers. BundleID matches the macOS bundle identifier of
// the packaged app (scripts/desktop/common.sh).
const (
	BundleID         = "it.coste360.arenile"
	autostartName    = "Arenile"
	linuxDesktopFile = "arenile.desktop"
	// WindowsRunKey is the per-user Run key (no administrator rights).
	WindowsRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
)

// Autostart is the "start at login" strategy (tray: Avvio all'accensione) of one
// operating system: a LaunchAgent on macOS, a Run registry value on
// Windows, an XDG autostart entry on Linux. Per-user, no admin rights.
type Autostart interface {
	Enabled() (bool, error)
	SetEnabled(on bool) error
}

// LaunchAgentPlist returns the macOS LaunchAgent that starts exe at login
// (pure; golden-tested). Inside an .app bundle exe is
// Contents/MacOS/<binary>, which is what os.Executable returns.
func LaunchAgentPlist(exe string) []byte {
	var esc bytes.Buffer
	_ = xml.EscapeText(&esc, []byte(exe))
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + BundleID + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + esc.String() + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
</dict>
</plist>
`)
}

// XDGAutostartEntry returns the Linux autostart desktop entry for exe
// (pure; golden-tested). Exec is quoted per the Desktop Entry spec.
func XDGAutostartEntry(exe string) []byte {
	return []byte(`[Desktop Entry]
Type=Application
Name=Arenile
Comment=Gestionale per stabilimenti balneari
Exec=` + desktopExecQuote(exe) + `
Icon=arenile
Terminal=false
X-GNOME-Autostart-enabled=true
`)
}

// desktopExecQuote quotes an argument of a desktop entry Exec key: the
// quoting rule (escape " ` $ \ inside double quotes) is applied first, then
// the string-value escaping (\ doubled), as the Desktop Entry spec
// requires; % is doubled for the field-code expansion.
func desktopExecQuote(s string) string {
	q := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`, `%`, `%%`).Replace(s)
	return `"` + strings.ReplaceAll(q, `\`, `\\`) + `"`
}

// WindowsRunValue is the Run registry value for exe (pure).
func WindowsRunValue(exe string) string { return `"` + exe + `"` }

// NewAutostart returns the Autostart of goos for the executable exe.
// home is the user home, getenv reads XDG_CONFIG_HOME and APPIMAGE (an
// AppImage must be started from its file, not from its temporary mount).
// The Windows strategy writes the registry through runKey (nil: the
// platform registry, available only on Windows).
func NewAutostart(goos, home, exe string, getenv func(string) string, runKey RunKey) (Autostart, error) {
	if exe == "" {
		return nil, errors.New("desktop: autostart: unknown executable")
	}
	switch goos {
	case "darwin":
		if home == "" {
			return nil, ErrNoDataDir
		}
		return fileAutostart{
			path:    filepath.Join(home, "Library", "LaunchAgents", BundleID+".plist"),
			content: LaunchAgentPlist(exe),
		}, nil
	case "windows":
		if runKey == nil {
			runKey = platformRunKey()
		}
		if runKey == nil {
			return nil, errors.New("desktop: autostart: registry not available")
		}
		return registryAutostart{key: runKey, value: WindowsRunValue(exe)}, nil
	default:
		if p := getenv("APPIMAGE"); p != "" && filepath.IsAbs(p) {
			exe = p
		}
		dir := getenv("XDG_CONFIG_HOME")
		if dir == "" || !filepath.IsAbs(dir) {
			if home == "" {
				return nil, ErrNoDataDir
			}
			dir = filepath.Join(home, ".config")
		}
		return fileAutostart{path: filepath.Join(dir, "autostart", linuxDesktopFile), content: XDGAutostartEntry(exe)}, nil
	}
}

// fileAutostart is the file-based strategy (LaunchAgent, XDG entry): the
// entry exists exactly when autostart is on.
type fileAutostart struct {
	path    string
	content []byte
}

func (f fileAutostart) Enabled() (bool, error) {
	_, err := os.Stat(f.path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	}
	return false, err
}

func (f fileAutostart) SetEnabled(on bool) error {
	if !on {
		if err := os.Remove(f.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil { //nolint:gosec // G301: ~/Library/LaunchAgents and ~/.config/autostart are read by the session manager
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, f.content, 0o644); err != nil { //nolint:gosec // G306: launchd/XDG require a readable entry; it holds no secret
		return err
	}
	return os.Rename(tmp, f.path)
}

// RunKey is the HKCU Run key (port: the registry on Windows, a fake in tests).
type RunKey interface {
	Get(name string) (value string, ok bool, err error)
	Set(name, value string) error
	Delete(name string) error
}

// registryAutostart is the Windows strategy.
type registryAutostart struct {
	key   RunKey
	value string
}

func (r registryAutostart) Enabled() (bool, error) {
	v, ok, err := r.key.Get(autostartName)
	if err != nil {
		return false, err
	}
	return ok && v != "", nil
}

func (r registryAutostart) SetEnabled(on bool) error {
	if on {
		if err := r.key.Set(autostartName, r.value); err != nil {
			return fmt.Errorf("desktop: autostart registry: %w", err)
		}
		return nil
	}
	return r.key.Delete(autostartName)
}
