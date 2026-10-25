package shell

import (
	"net/url"
	"reflect"
	"strings"
	"unsafe"

	"github.com/coste360/arenile/internal/desktop"
)

// The native link policy of the window where the webview reports
// navigations and new-window requests to Go (WebView2 on Windows:
// NavigationStarting and NewWindowRequested, webview2_windows.go). It
// is the same rule as the macOS shim (cocoa_darwin.m) and the injected
// script (desktop.LinkScript): the window shows the panel only.

// LinkAction is what the window does with a navigation or a new-window
// request (NavigationAction, NewWindowAction).
type LinkAction int

const (
	// LinkAllow keeps the webview's default behaviour.
	LinkAllow LinkAction = iota
	// LinkInPlace shows the target in the window instead of a new window
	// (a panel page opened with target=_blank or "open in new window").
	LinkInPlace
	// LinkExternal cancels the request and hands the target to the
	// default browser (Controller.OpenExternal, whose AllowExternal
	// refuses anything that is not a web link).
	LinkExternal
)

// NavigationAction decides a top-level navigation of the window towards
// target (pure); panelURL is the page the window shows (as the webview
// reports it, so canonicalised like target): links, location changes, form
// submissions and HTTP redirects. A navigation that leaves the panel
// goes to the default browser; everything else proceeds. With no web
// page shown yet (panelURL empty or not http(s), the first load, the
// about:blank of the quit) nothing is diverted.
func NavigationAction(panelURL, target string) LinkAction {
	if !isWebPage(panelURL) {
		return LinkAllow
	}
	if desktop.ClassifyLink(panelURL, target) == desktop.LinkLeaves {
		return LinkExternal
	}
	return LinkAllow
}

// NewWindowAction decides a request to open target in a new window
// (window.open, target=_blank, the context menu's "open link in new
// window") from the window shown at panelURL (pure). A target that leaves
// the panel goes to the default browser; a panel page is shown in place
// (a second window would be a panel without the app's menu, close
// handling or link policy); a panel API URL (a server document) and
// non-web targets (blob:, about:blank) keep the webview's own popup.
func NewWindowAction(panelURL, target string) LinkAction {
	if !isWebPage(panelURL) {
		return LinkAllow
	}
	switch desktop.ClassifyLink(panelURL, target) {
	case desktop.LinkLeaves:
		return LinkExternal
	case desktop.LinkPanelPage:
		return LinkInPlace
	default:
		return LinkAllow
	}
}

func isWebPage(u string) bool {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return false
	}
	s := strings.ToLower(p.Scheme)
	return s == "http" || s == "https"
}

// webviewHandle returns the C handle (webview_t) held by a webview_go
// WebView (pure apart from reflection): webview_go wraps it in an
// unexported struct with that single pointer field and has no accessor.
// ok is false when the value does not have that shape (another version
// of the library): the caller then skips the native hooks instead of
// guessing. The Windows layer also checks the handle against the one the
// library stores in the window (webview2_cgo_windows.go) before using it.
func webviewHandle(w any) (unsafe.Pointer, bool) {
	v := reflect.ValueOf(w)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return nil, false
	}
	s := v.Elem()
	if s.NumField() != 1 || s.Field(0).Kind() != reflect.UnsafePointer {
		return nil, false
	}
	h := s.Field(0).UnsafePointer()
	return h, h != nil
}
