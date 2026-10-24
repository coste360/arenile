package remote

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/coste360/arenile/internal/desktop"
)

// WindowOptions configure the panel window of the remote mode.
type WindowOptions struct {
	URL   string
	Title string
	// Kiosk: full screen, no browser chrome (cash desk PC).
	Kiosk bool
	// Init is injected before the panel scripts (window.arenileDesktop).
	Init string
	// OpenExternal opens links to other origins in the default browser.
	OpenExternal func(url string) error
	Width        int
	Height       int
}

// Window shows the panel of the instance. Open blocks until the window is
// closed (nil) or ctx is done. Implementations: the system webview behind
// the "desktop" build tag (window_webview.go) and BrowserWindow.
type Window interface {
	Open(ctx context.Context, o WindowOptions) error
}

// nativeWindow builds the webview window when the binary is built with
// the "desktop" tag and CGO (see window_webview.go); nil otherwise.
var nativeWindow func() Window

// InitScript is the script injected in the panel window (pure): the panel
// detects the remote mode from window.arenileDesktop (web/admin lib/meta.ts)
// and links that leave the panel (desktop.LinkScript) open in the default
// browser through the bound arenileOpenExternal function.
func InitScript(version string, kiosk bool) string {
	desc, _ := json.Marshal(map[string]any{"mode": "remote", "version": version, "kiosk": kiosk})
	return `(function(){
  Object.defineProperty(window, "arenileDesktop", {value: Object.freeze(` + string(desc) + `), writable: false});
  ` + desktop.LinkScript + `
})();`
}

// IsExternal reports whether target leaves the panel (pure,
// desktop.LeavesPanel): another origin, or a same-origin path outside the
// panel (/admin, /api).
func IsExternal(panelURL, target string) bool {
	return desktop.LeavesPanel(panelURL, target)
}

// AllowExternal is the policy of the arenileOpenExternal binding (pure):
// the window hands to the default browser only web links (http, https,
// mailto: desktop.IsWebLink) that leave the panel (IsExternal). The binding stays
// callable by any script in the window, including a page the window was
// navigated to, so this check, not the injected click handler, is the
// security boundary: no file:, path, UNC path or custom scheme ever
// reaches the operating system.
func AllowExternal(panelURL, target string) bool {
	return desktop.IsWebLink(target) && IsExternal(panelURL, target)
}

// BrowserWindow is the fallback Window: the default browser, or, in kiosk
// mode, a Chromium-family browser (Edge on Windows, Chrome, Chromium) in
// --kiosk --app mode with a dedicated profile in the data dir. It cannot
// inject InitScript, so the panel shows its browser layout.
type BrowserWindow struct {
	// OpenURL opens the default browser (desktop.UI.OpenURL).
	OpenURL func(string) error
	// ProfileDir is the browser profile of the kiosk window.
	ProfileDir string
	GOOS       string
	// LookPath overrides exec.LookPath (tests).
	LookPath func(string) (string, error)
}

// Open implements Window: it opens the browser and waits for ctx.
func (b BrowserWindow) Open(ctx context.Context, o WindowOptions) error {
	if err := b.open(ctx, o); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

// open starts the browser without waiting (the tray UI then blocks).
func (b BrowserWindow) open(ctx context.Context, o WindowOptions) error {
	if o.Kiosk {
		if bin := b.kioskBrowser(); bin != "" {
			cmd := exec.CommandContext(ctx, bin, KioskArgs(o.URL, b.ProfileDir)...) //nolint:gosec // G204: fixed browser binary, URL as a single argument
			if err := cmd.Start(); err == nil {
				go func() { _ = cmd.Wait() }()
				return nil
			}
		}
	}
	if b.OpenURL == nil {
		return nil
	}
	return b.OpenURL(o.URL)
}

// KioskArgs are the Chromium flags of the kiosk window (pure).
func KioskArgs(u, profileDir string) []string {
	args := []string{"--kiosk", "--app=" + u, "--no-first-run", "--disable-features=Translate"}
	if profileDir != "" {
		args = append(args, "--user-data-dir="+profileDir)
	}
	return args
}

func (b BrowserWindow) kioskBrowser() string {
	goos := b.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	look := b.LookPath
	if look == nil {
		look = exec.LookPath
	}
	for _, c := range KioskCandidates(goos, os.Getenv) {
		if filepath.IsAbs(c) {
			if _, err := os.Stat(c); err == nil {
				return c
			}
			continue
		}
		if p, err := look(c); err == nil {
			return p
		}
	}
	return ""
}

// KioskCandidates lists the Chromium-family browsers to try (pure).
func KioskCandidates(goos string, getenv func(string) string) []string {
	switch goos {
	case "windows":
		var out []string
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
			if d := getenv(env); d != "" {
				out = append(out,
					filepath.Join(d, "Microsoft", "Edge", "Application", "msedge.exe"),
					filepath.Join(d, "Google", "Chrome", "Application", "chrome.exe"))
			}
		}
		return out
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	default:
		return []string{"google-chrome", "chromium", "chromium-browser", "microsoft-edge"}
	}
}
