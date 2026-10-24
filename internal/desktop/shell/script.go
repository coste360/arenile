package shell

import (
	"encoding/json"

	"github.com/coste360/arenile/internal/desktop"
)

// InitScript is injected in the window before the panel scripts (pure).
// window.arenileDesktop tells the panel it runs in the desktop shell
// ({mode: "local", shell: "window"}: the panel still reads local/LAN from
// /meta, lib/meta.ts); links that leave the panel (desktop.LinkScript:
// another origin, or a same-origin page outside /admin such as the public
// booking page) go to the default browser through the bound
// arenileOpenExternal (AllowExternal is the security boundary, not this
// handler). With nativePrint false (macOS: WKWebView
// ignores window.print) window.print calls the bound arenilePrint, which
// opens the system print panel on the page.
func InitScript(version string, nativePrint bool) string {
	desc, _ := json.Marshal(map[string]any{"mode": "local", "shell": "window", "version": version})
	s := `(function(){
  Object.defineProperty(window, "arenileDesktop", {value: Object.freeze(` + string(desc) + `), writable: false});
  ` + desktop.LinkScript
	if !nativePrint {
		s += "\n  " + printOverride
	}
	return s + `
})();`
}

// printOverride routes window.print to the bound arenilePrint (macOS).
const printOverride = `window.print = function () { if (typeof window.arenilePrint === "function") window.arenilePrint(); };`

// PrintScript is the print override alone, for windows that bring their
// own init script (remote mode); empty where the webview prints natively.
func PrintScript(nativePrint bool) string {
	if nativePrint {
		return ""
	}
	return "(function(){ " + printOverride + " })();"
}

// IsExternal reports whether target leaves the panel shown at panelURL
// (pure, desktop.LeavesPanel): another origin, or a same-origin path
// outside the panel (/admin, /api).
func IsExternal(panelURL, target string) bool {
	return desktop.LeavesPanel(panelURL, target)
}

// AllowExternal is the policy of the arenileOpenExternal binding (pure):
// only web links (http, https, mailto: desktop.IsWebLink) that leave the
// panel (another origin, or a same-origin page outside the panel such as
// /b/<slug>) reach the default browser; no file:, path or custom scheme
// ever reaches the operating system, whatever script calls the binding.
func AllowExternal(panelURL, target string) bool {
	return desktop.IsWebLink(target) && IsExternal(panelURL, target)
}
