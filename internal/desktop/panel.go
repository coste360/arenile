package desktop

import (
	"net/url"
	"strings"
)

// The desktop window (local shell and remote mode) shows the panel only.
// A link that leaves the panel goes to the default browser, where the
// operator has the back button, tabs and the public pages with their own
// layout; panel pages stay in the window.
//
// The panel is the SPA under /admin and the API it calls under /api: the
// API paths stay in the app because they need the window's session
// (exports downloaded through navigation, server documents such as the Z
// report opened with window.open). Everything else on the same origin
// (the public booking page /b/<slug>, the embed widget, routes added by
// extensions through ext.HTTPRoutes such as an SSO callback) and every
// other origin leaves the panel.
//
// The same rule is mirrored in LeavesPanelJS (the injected script); the
// macOS shim asks ClassifyLink through cgo. Go follows what the browser
// does with the URL so that both sides agree: dot segments are resolved
// ("/admin/../b" is "/b", also when written "%2e%2e"), nothing else is
// decoded or collapsed ("//admin" and "/admin%2Fx" are not panel paths),
// and default ports are implied ("http://h:80" is "http://h").

// panelPrefixes are the path prefixes that stay in the window: exactly the
// prefix or the prefix followed by "/".
var panelPrefixes = []string{"/admin", "/api"}

// IsPanelPath reports whether the escaped URL path p belongs to the panel
// (pure). "/admin", "/admin/oggi" and "/api/admin/v1/x" do; "/", "/b/lido",
// "/administrator", "//admin" and "/admin/../b" do not.
func IsPanelPath(p string) bool {
	p = removeDotSegments(p)
	for _, pre := range panelPrefixes {
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
}

// isDot and isDotDot recognise the dot segments as the WHATWG URL parser
// does, percent-encoded included.
func isDot(seg string) bool { return seg == "." || strings.EqualFold(seg, "%2e") }

func isDotDot(seg string) bool {
	switch strings.ToLower(seg) {
	case "..", ".%2e", "%2e.", "%2e%2e":
		return true
	}
	return false
}

// removeDotSegments resolves "." and ".." in an escaped absolute path
// (RFC 3986 5.2.4) without touching empty segments or escapes.
func removeDotSegments(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	segs := strings.Split(p[1:], "/")
	out := make([]string, 0, len(segs))
	for i, seg := range segs {
		last := i == len(segs)-1
		switch {
		case isDot(seg):
			if last {
				out = append(out, "")
			}
		case isDotDot(seg):
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			if last {
				out = append(out, "")
			}
		default:
			out = append(out, seg)
		}
	}
	return "/" + strings.Join(out, "/")
}

// origin is scheme://host:port with the default port made explicit and the
// host lowercased, as the browser compares origins.
func origin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[scheme]
	}
	return scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port
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
	if origin(t) != origin(p) {
		return true
	}
	return !IsPanelPath(t.EscapedPath())
}

// RebasePanelPage moves the panel page target (an absolute URL whose path
// is under /admin) onto the origin of base, the panel URL of the running
// app (pure). Notifications carry absolute URLs, and one clicked after a
// restart names the old port (the local server listens on a free port).
// ok is false when target is not a panel page or base has no origin.
func RebasePanelPage(base, target string) (string, bool) {
	b, err1 := url.Parse(base)
	t, err2 := url.Parse(target)
	if err1 != nil || err2 != nil || b.Host == "" || t.Host == "" || !isWebScheme(b.Scheme) || !isWebScheme(t.Scheme) {
		return "", false
	}
	if !isPagePath(t.EscapedPath()) {
		return "", false
	}
	out := url.URL{Scheme: b.Scheme, Host: b.Host, Path: t.Path, RawPath: t.RawPath, RawQuery: t.RawQuery, Fragment: t.Fragment}
	return out.String(), true
}

func isWebScheme(s string) bool {
	s = strings.ToLower(s)
	return s == "http" || s == "https"
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
	// as a page that replaces the panel (a server document, JSON).
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
	if p := t.EscapedPath(); IsPanelPath(p) && !isPagePath(p) {
		return LinkPanelResource
	}
	return LinkPanelPage
}

// isPagePath reports whether the panel path p is a page of the SPA
// (/admin) rather than an API URL (/api).
func isPagePath(p string) bool {
	p = removeDotSegments(p)
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
// bound arenileOpenExternal: clicks and middle clicks on <a href>
// (target=_blank included), window.open and, where the webview has the
// Navigation API (WebView2 on Windows), every other navigation the page
// starts (location.href and location.assign, form submissions: a POST
// reaches the browser as a GET of its URL, no panel form leaves it). <a
// download> is never intercepted (the window saves the file). Not covered
// on Windows: HTTP redirects (the navigate event fires for the first URL
// only; the panel's server redirects only within /admin) and WebView2's
// own "open in new window" of the context menu. On macOS the navigation
// delegate of the shim applies the same rule natively to all of them
// (cocoa_darwin.m). The binding's Go policy (AllowExternal) is the
// security boundary, not this script.
const LinkScript = `var leavesPanel = ` + LeavesPanelJS + `;
  var onLink = function (e) {
    if (e.type === "auxclick" && e.button !== 1) return;
    var a = e.target && e.target.closest ? e.target.closest("a[href]") : null;
    if (!a || a.hasAttribute("download") || typeof window.arenileOpenExternal !== "function") return;
    var u;
    try { u = new URL(a.href, location.href); } catch (_) { return; }
    if (leavesPanel(u)) {
      e.preventDefault();
      window.arenileOpenExternal(u.href);
    }
  };
  document.addEventListener("click", onLink, true);
  document.addEventListener("auxclick", onLink, true);
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
