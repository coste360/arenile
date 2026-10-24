package desktop

import (
	"net/url"
	"path"
	"strings"
)

// The desktop window (local shell and remote mode) shows the panel only.
// A link that leaves the panel goes to the default browser, where the
// operator has the back button, tabs and the public pages with their own
// layout; panel pages stay in the window.
//
// The panel is the SPA under /admin and the API it calls under /api: the
// API paths stay in the window because they need the window's session
// (inline documents such as the Z report PDF opened with window.open,
// exports downloaded through navigation). Everything else on the same
// origin (the public booking page /b/<slug>, the embed widget, the
// documentation) and every other origin leaves the panel.
//
// The same rule is mirrored in LeavesPanelJS (the injected click handler
// and window.open override) and in the macOS shim (cocoa_darwin.m,
// createWebView and the navigation delegate): keep the three in sync.

// panelPrefixes are the path prefixes that stay in the window: exactly the
// prefix or the prefix followed by "/".
var panelPrefixes = []string{"/admin", "/api"}

// IsPanelPath reports whether the URL path p belongs to the panel (pure).
// "/admin", "/admin/oggi" and "/api/admin/v1/x" do; "/", "/b/lido" and
// "/administrator" do not. The path is cleaned first ("/admin/../b/x" is
// "/b/x"), as a browser resolves it.
func IsPanelPath(p string) bool {
	if p == "" {
		p = "/"
	}
	p = path.Clean("/" + p)
	for _, pre := range panelPrefixes {
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
}

// LeavesPanel reports whether target, an absolute URL, leaves the panel
// shown at panelURL (pure): another origin (scheme, host or port), or the
// same origin with a path outside the panel (IsPanelPath). Relative or
// unparsable targets do not leave the panel. It says nothing about safety:
// callers that hand the target to the operating system also require
// IsWebLink.
func LeavesPanel(panelURL, target string) bool {
	p, err1 := url.Parse(panelURL)
	t, err2 := url.Parse(target)
	if err1 != nil || err2 != nil || t.Scheme == "" {
		return false
	}
	if !strings.EqualFold(t.Scheme, p.Scheme) || !strings.EqualFold(t.Host, p.Host) {
		return true
	}
	return !IsPanelPath(t.Path)
}

// LinkKind classifies a URL the window is asked to show (ClassifyLink).
type LinkKind int

const (
	// LinkOther is not a web URL (about:, blob:, data:, a relative or
	// unparsable target): the webview's default behaviour applies.
	LinkOther LinkKind = iota
	// LinkPanelPage is a page of the panel SPA (/admin): shown in place.
	LinkPanelPage
	// LinkPanelResource is an API URL of the panel origin (/api): it needs
	// the window's session, so it stays in the app, but as a download, never
	// as a page that replaces the panel (inline PDF, JSON).
	LinkPanelResource
	// LinkLeaves leaves the panel: the default browser (with AllowExternal).
	LinkLeaves
)

// ClassifyLink classifies target relative to the panel shown at panelURL
// (pure). The macOS shim asks it for every new-window request and every
// top-level navigation (cocoa_darwin.go, arenileShellClassifyLink).
func ClassifyLink(panelURL, target string) LinkKind {
	t, err := url.Parse(target)
	if err != nil {
		return LinkOther
	}
	switch strings.ToLower(t.Scheme) {
	case "mailto":
		return LinkLeaves
	case "http", "https":
	default:
		return LinkOther
	}
	if LeavesPanel(panelURL, target) {
		return LinkLeaves
	}
	if IsPanelPath(t.Path) && !isPagePath(t.Path) {
		return LinkPanelResource
	}
	return LinkPanelPage
}

// isPagePath reports whether the panel path p is a page of the SPA
// (/admin) rather than an API URL (/api).
func isPagePath(p string) bool {
	p = path.Clean("/" + p)
	return p == "/admin" || strings.HasPrefix(p, "/admin/")
}

// LeavesPanelJS is the JavaScript mirror of LeavesPanel for the injected
// scripts: a function expression taking a URL object and returning true
// when it leaves the panel of the current page. Only http(s) and mailto:
// links are handed over; blob: and data: (downloads built by the panel)
// never are.
const LeavesPanelJS = `function (u) {
    if (u.protocol === "mailto:") return true;
    if (u.protocol !== "https:" && u.protocol !== "http:") return false;
    if (u.origin !== location.origin) return true;
    var p = u.pathname;
    return !(p === "/admin" || p.indexOf("/admin/") === 0 || p === "/api" || p.indexOf("/api/") === 0);
  }`

// LinkScript is the part of the injected scripts that sends the links
// leaving the panel (LeavesPanelJS) to the default browser through the
// bound arenileOpenExternal: clicks on <a href> (target=_blank included),
// window.open and, where the webview has the Navigation API (WebView2 on
// Windows), every other navigation the page starts (location.href and
// location.assign, form submissions). <a download> is never intercepted
// (the window saves the file). On macOS the navigation delegate of the
// shim applies the same rule natively (cocoa_darwin.m). The binding's Go
// policy (AllowExternal) is the security boundary, not this script.
const LinkScript = `var leavesPanel = ` + LeavesPanelJS + `;
  document.addEventListener("click", function (e) {
    var a = e.target && e.target.closest ? e.target.closest("a[href]") : null;
    if (!a || a.hasAttribute("download") || typeof window.arenileOpenExternal !== "function") return;
    var u;
    try { u = new URL(a.href, location.href); } catch (_) { return; }
    if (leavesPanel(u)) {
      e.preventDefault();
      window.arenileOpenExternal(u.href);
    }
  }, true);
  var open = window.open;
  window.open = function (u) {
    try {
      var t = new URL(u, location.href);
      if (leavesPanel(t) && typeof window.arenileOpenExternal === "function") {
        window.arenileOpenExternal(t.href);
        return null;
      }
    } catch (_) {}
    return open.apply(window, arguments);
  };
  if (window.navigation && typeof window.navigation.addEventListener === "function") {
    window.navigation.addEventListener("navigate", function (e) {
      if (!e.cancelable || e.downloadRequest || e.navigationType === "traverse" || typeof window.arenileOpenExternal !== "function") return;
      var d;
      try { d = new URL(e.destination.url); } catch (_) { return; }
      if (leavesPanel(d)) {
        e.preventDefault();
        window.arenileOpenExternal(d.href);
      }
    });
  }`
