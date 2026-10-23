package shell

import (
	"encoding/json"
	"net/url"

	"github.com/coste360/arenile/internal/desktop"
)

// InitScript is injected in the window before the panel scripts (pure).
// window.arenileDesktop tells the panel it runs in the desktop shell
// ({mode: "local", shell: "window"}: the panel still reads local/LAN from
// /meta, lib/meta.ts); links to other origins go to the default browser
// through the bound arenileOpenExternal (AllowExternal is the security
// boundary, not this handler). With nativePrint false (macOS: WKWebView
// ignores window.print) window.print calls the bound arenilePrint, which
// opens the system print panel on the page.
func InitScript(version string, nativePrint bool) string {
	desc, _ := json.Marshal(map[string]any{"mode": "local", "shell": "window", "version": version})
	s := `(function(){
  Object.defineProperty(window, "arenileDesktop", {value: Object.freeze(` + string(desc) + `), writable: false});
  document.addEventListener("click", function (e) {
    var a = e.target && e.target.closest ? e.target.closest("a[href]") : null;
    if (!a || typeof window.arenileOpenExternal !== "function") return;
    var u;
    try { u = new URL(a.href, location.href); } catch (_) { return; }
    if (u.origin !== location.origin && (u.protocol === "https:" || u.protocol === "http:" || u.protocol === "mailto:")) {
      e.preventDefault();
      window.arenileOpenExternal(u.href);
    }
  }, true);
  var open = window.open;
  window.open = function (u) {
    try {
      var t = new URL(u, location.href);
      if (t.origin !== location.origin && typeof window.arenileOpenExternal === "function") {
        window.arenileOpenExternal(t.href);
        return null;
      }
    } catch (_) {}
    return open.apply(window, arguments);
  };`
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

// IsExternal reports whether target leaves the origin of panelURL (pure).
func IsExternal(panelURL, target string) bool {
	p, err1 := url.Parse(panelURL)
	t, err2 := url.Parse(target)
	if err1 != nil || err2 != nil {
		return false
	}
	return t.Scheme+"://"+t.Host != p.Scheme+"://"+p.Host
}

// AllowExternal is the policy of the arenileOpenExternal binding (pure):
// only web links (http, https, mailto: desktop.IsWebLink) to another origin
// reach the default browser; no file:, path or custom scheme ever reaches
// the operating system, whatever script calls the binding.
func AllowExternal(panelURL, target string) bool {
	return desktop.IsWebLink(target) && IsExternal(panelURL, target)
}
