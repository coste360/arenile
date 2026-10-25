//go:build desktop && cgo && windows

package shell

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The native link policy on Windows: two WebView2 event handlers on the
// ICoreWebView2 of the window, written as COM objects in plain Go
// (syscall callbacks; only the handle lookup uses cgo), so that what the injected script cannot see
// follows the same rule as on macOS (NavigationAction, NewWindowAction):
//
//   - NavigationStarting fires for every top-level navigation, HTTP
//     redirects included (the Navigation API's navigate event of the
//     script fires for the first URL only): one that leaves the panel is
//     cancelled and sent to the default browser;
//   - NewWindowRequested fires for window.open, target=_blank and the
//     context menu's "Open link in new window", which bypasses the page's
//     scripts: a target that leaves the panel goes to the default browser,
//     a panel page is shown in the window instead of a second one.
//
// The ICoreWebView2Controller comes from webview_go's
// webview_get_native_handle (webview2_cgo_windows.go). The vtable indices
// below are those of WebView2.h vendored by webview_go
// (libs/mswebview2/include); the interfaces are frozen by Microsoft, new
// methods go to new interfaces (ICoreWebView2_2, ...).

// Vtable indices (IUnknown is 0..2).
const (
	vtRelease = 2

	// ICoreWebView2Controller.
	vtControllerGetCoreWebView2 = 25

	// ICoreWebView2.
	vtWebViewGetSource                = 4
	vtWebViewAddNavigationStarting    = 7
	vtWebViewRemoveNavigationStarting = 8
	vtWebViewAddNewWindowRequested    = 44
	vtWebViewRemoveNewWindowRequested = 45

	// ICoreWebView2NavigationStartingEventArgs.
	vtNavArgsGetURI    = 3
	vtNavArgsPutCancel = 8

	// ICoreWebView2NewWindowRequestedEventArgs.
	vtNewWindowArgsGetURI     = 3
	vtNewWindowArgsPutHandled = 6
)

const (
	sOK          = 0
	eNoInterface = 0x80004002
	ePointer     = 0x80004003
)

var (
	iidIUnknown                  = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidNavigationStartingHandler = windows.GUID{Data1: 0x9adbe429, Data2: 0xf36d, Data3: 0x432b, Data4: [8]byte{0x9d, 0xdc, 0xf8, 0x88, 0x1f, 0xbd, 0x76, 0xe3}}
	iidNewWindowRequestedHandler = windows.GUID{Data1: 0xd4c185fe, Data2: 0xc81c, Data3: 0x4989, Data4: [8]byte{0x97, 0xaf, 0x2d, 0x3f, 0xa7, 0xab, 0x56, 0x51}}
)

// comObject is any COM interface pointer: its first word is the vtable.
type comObject struct{ vtbl *[64]uintptr }

// call invokes a method whose arguments are plain values (BOOL, an
// EventRegistrationToken), never Go pointers.
func (o *comObject) call(method int, args ...uintptr) uintptr {
	all := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	r, _, _ := syscall.SyscallN(o.vtbl[method], all...)
	return r
}

// callPtr and callPtr2 invoke a method with pointer arguments (out
// parameters, handlers). The pointers stay typed until the SyscallN call
// expression converts them, so a Go stack that moves in between cannot
// leave a stale address (unsafe.Pointer rule 4).
func (o *comObject) callPtr(method int, p unsafe.Pointer) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[method], uintptr(unsafe.Pointer(o)), uintptr(p))
	return r
}

func (o *comObject) callPtr2(method int, p1, p2 unsafe.Pointer) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[method], uintptr(unsafe.Pointer(o)), uintptr(p1), uintptr(p2))
	return r
}

func (o *comObject) release() { o.call(vtRelease) }

// eventHandler is a COM event handler implemented in Go, one per
// registration: it carries the controller of its window (no process-wide
// state), lives in the linkHooks that registered it (heap memory, which
// the Go GC never moves, kept reachable until remove) and does not count
// references, WebView2 releasing it no later than remove_*.
type eventHandler struct {
	vtbl *eventHandlerVtbl
	iid  *windows.GUID
	ctl  *Controller
}

type eventHandlerVtbl struct{ queryInterface, addRef, release, invoke uintptr }

// The vtables are immutable after the first window and shared by all the
// handlers of a kind (syscall.NewCallback slots are limited and never
// freed, so they are created once).
var (
	handlerOnce     sync.Once
	navStartingVtbl eventHandlerVtbl
	newWindowVtbl   eventHandlerVtbl
)

// The callbacks run on the UI thread, inside the window's message loop.

func handlerQueryInterface(this *eventHandler, riid *windows.GUID, ppv *uintptr) uintptr {
	if ppv == nil {
		return ePointer
	}
	if riid != nil && (*riid == iidIUnknown || *riid == *this.iid) {
		*ppv = uintptr(unsafe.Pointer(this))
		return sOK
	}
	*ppv = 0
	return eNoInterface
}

func handlerAddRef(*eventHandler) uintptr  { return 1 }
func handlerRelease(*eventHandler) uintptr { return 1 }

func installHandlerVtables() {
	handlerOnce.Do(func() {
		qi, add, rel := syscall.NewCallback(handlerQueryInterface), syscall.NewCallback(handlerAddRef), syscall.NewCallback(handlerRelease)
		navStartingVtbl = eventHandlerVtbl{qi, add, rel, syscall.NewCallback(onNavigationStarting)}
		newWindowVtbl = eventHandlerVtbl{qi, add, rel, syscall.NewCallback(onNewWindowRequested)}
	})
}

// getString reads a string property (an LPWSTR out parameter allocated
// by WebView2), "" on failure.
func getString(o *comObject, method int) string {
	var p *uint16
	if o.callPtr(method, unsafe.Pointer(&p)) != sOK || p == nil {
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(p))
	return windows.UTF16PtrToString(p)
}

// shownURL is the page the sender webview shows (get_Source: the URL as
// WebView2 canonicalised it, about:blank before the first load), the
// reference of the link rule as webView.URL is on macOS.
func shownURL(sender *comObject) string {
	if sender == nil {
		return ""
	}
	return getString(sender, vtWebViewGetSource)
}

func onNavigationStarting(this *eventHandler, sender, args *comObject) uintptr {
	if this == nil || this.ctl == nil || args == nil {
		return sOK
	}
	target := getString(args, vtNavArgsGetURI)
	if NavigationAction(shownURL(sender), target) == LinkExternal {
		args.call(vtNavArgsPutCancel, 1)
		go this.ctl.OpenExternal(target)
	}
	return sOK
}

func onNewWindowRequested(this *eventHandler, sender, args *comObject) uintptr {
	if this == nil || this.ctl == nil || args == nil {
		return sOK
	}
	target := getString(args, vtNewWindowArgsGetURI)
	switch NewWindowAction(shownURL(sender), target) {
	case LinkExternal:
		// Handled without a NewWindow: no window opens (window.open
		// returns null, as with the injected script and on macOS).
		args.call(vtNewWindowArgsPutHandled, 1)
		go this.ctl.OpenExternal(target)
	case LinkInPlace:
		args.call(vtNewWindowArgsPutHandled, 1)
		go this.ctl.OpenPage(target)
	case LinkAllow:
	}
	return sOK
}

// linkHooks are the handlers registered on one ICoreWebView2.
type linkHooks struct {
	webview         *comObject
	nav, newWin     *eventHandler
	navToken, nwTok int64
	navOK, newWinOK bool
}

// installLinkHooks registers the handlers on the ICoreWebView2 of
// controller (an ICoreWebView2Controller, UI thread) for c; nil when
// nothing could be registered.
func installLinkHooks(controller unsafe.Pointer, c *Controller) *linkHooks {
	if controller == nil || c == nil {
		return nil
	}
	installHandlerVtables()
	var wv *comObject
	ctl := (*comObject)(controller)
	if ctl.callPtr(vtControllerGetCoreWebView2, unsafe.Pointer(&wv)) != sOK || wv == nil {
		return nil
	}
	h := &linkHooks{
		webview: wv,
		nav:     &eventHandler{vtbl: &navStartingVtbl, iid: &iidNavigationStartingHandler, ctl: c},
		newWin:  &eventHandler{vtbl: &newWindowVtbl, iid: &iidNewWindowRequestedHandler, ctl: c},
	}
	h.navOK = wv.callPtr2(vtWebViewAddNavigationStarting, unsafe.Pointer(h.nav), unsafe.Pointer(&h.navToken)) == sOK
	h.newWinOK = wv.callPtr2(vtWebViewAddNewWindowRequested, unsafe.Pointer(h.newWin), unsafe.Pointer(&h.nwTok)) == sOK
	if !h.navOK && !h.newWinOK {
		wv.release()
		return nil
	}
	return h
}

// remove unregisters the handlers (UI thread, before the webview is
// destroyed or adopted by another controller).
func (h *linkHooks) remove() {
	if h == nil {
		return
	}
	if h.navOK {
		h.webview.call(vtWebViewRemoveNavigationStarting, uintptr(h.navToken))
	}
	if h.newWinOK {
		h.webview.call(vtWebViewRemoveNewWindowRequested, uintptr(h.nwTok))
	}
	h.webview.release()
}
