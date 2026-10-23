//go:build desktop && cgo && darwin

package shell

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
#include "cocoa_darwin.h"
*/
import "C"

import (
	"encoding/json"
	"sync"
	"unsafe"

	webview "github.com/webview/webview_go"
)

// The macOS layer is a small Objective-C shim (cocoa_darwin.m) on the
// NSWindow and WKWebView that webview_go creates. webview_go sets no main
// menu, destroys the window on close and has no download or print support;
// the shim adds, with public AppKit/WebKit API only (macOS 13+):
//
//   - the main menu built from AppMenu (JSON): native entries go to the
//     responder chain (copy:, paste:, undo:, hide:, performMiniaturize:,
//     ...), the others call arenileShellMenuAction;
//   - windowShouldClose: (close button, Cmd+W) -> Controller.OnClose, which
//     hides the window: the window is never destroyed until Terminate;
//   - applicationShouldHandleReopen: (Dock click) -> show the window;
//     applicationShouldTerminate: (Dock "Esci", logout) -> the launcher's
//     quit (exit backup, LAN stop) instead of an immediate exit;
//   - a WKNavigationDelegate turning attachments, non-displayable types and
//     <a download> (blob URLs included) into WKDownloads saved where the
//     controller says (~/Downloads, unique name), with a notification;
//   - links to other origins opened in a new window -> default browser;
//   - window.print -> NSPrintOperation of the WKWebView (print panel);
//   - window frame (get/set, kept on screen) and page zoom.
//
// One window per process: the callbacks reach the controller through a
// package variable.

var (
	activeMu sync.Mutex
	active   *Controller
)

func activeController() *Controller {
	activeMu.Lock()
	defer activeMu.Unlock()
	return active
}

type platform struct {
	w webview.WebView
	c *Controller
}

func platformAvailable() bool { return true }

func newPlatform(w webview.WebView, c *Controller) *platform {
	return &platform{w: w, c: c}
}

func (p *platform) install(st WindowState) {
	activeMu.Lock()
	active = p.c
	activeMu.Unlock()
	menu, _ := json.Marshal(AppMenu())
	cm := C.CString(string(menu))
	defer C.free(unsafe.Pointer(cm))
	C.arenile_shell_install(p.w.Window(), cm)
	if st.Placed {
		C.arenile_shell_set_frame(C.double(st.X), C.double(st.Y), C.double(st.Width), C.double(st.Height))
	}
	if st.Maximized {
		C.arenile_shell_set_zoomed()
	}
	if st.Zoom != 1 {
		p.setZoom(st.Zoom)
	}
}

func (p *platform) uninstall() {
	C.arenile_shell_uninstall()
	activeMu.Lock()
	active = nil
	activeMu.Unlock()
}

func (p *platform) show()             { C.arenile_shell_show() }
func (p *platform) hide()             { C.arenile_shell_hide() }
func (p *platform) setZoom(z float64) { C.arenile_shell_set_zoom(C.double(z)) }
func (p *platform) print()            { C.arenile_shell_print() }
func (p *platform) frame() (WindowState, bool) {
	var x, y, w, h C.double
	var zoomed C.int
	if C.arenile_shell_get_frame(&x, &y, &w, &h, &zoomed) == 0 {
		return WindowState{}, false
	}
	return WindowState{X: int(x), Y: int(y), Width: int(w), Height: int(h), Placed: true, Maximized: zoomed != 0}, true
}

// SetAccessoryActivation hides the Dock icon (tray-only fallback: the
// panel is in the browser, the app has no window to show from the Dock).
func SetAccessoryActivation() { C.arenile_shell_set_accessory() }

//export arenileShellMenuAction
func arenileShellMenuAction(action *C.char) {
	if c := activeController(); c != nil {
		a := Action(C.GoString(action))
		go c.OnMenuAction(a)
	}
}

// The callbacks return 1 when a controller handled the event, 0 to keep
// the default AppKit behaviour (no shell window attached).

//export arenileShellWindowShouldClose
func arenileShellWindowShouldClose() C.int {
	if c := activeController(); c != nil {
		c.OnClose()
		return 1
	}
	return 0
}

//export arenileShellReopen
func arenileShellReopen() C.int {
	if c := activeController(); c != nil {
		c.OnReopen()
		return 1
	}
	return 0
}

//export arenileShellQuitRequested
func arenileShellQuitRequested() C.int {
	if c := activeController(); c != nil {
		go c.quit()
		return 1
	}
	return 0
}

//export arenileShellOpenExternal
func arenileShellOpenExternal(u *C.char) {
	if c := activeController(); c != nil {
		s := C.GoString(u)
		go c.OpenExternal(s)
	}
}

// arenileShellDownloadPath returns the destination of a download (malloc'd
// C string, freed by the caller) or NULL to cancel it.
//
//export arenileShellDownloadPath
func arenileShellDownloadPath(suggested *C.char) *C.char {
	c := activeController()
	if c == nil {
		return nil
	}
	p, err := c.DownloadDestination(C.GoString(suggested))
	if err != nil {
		c.o.Log.Warn("desktop window: download refused (no free file name or no downloads folder)")
		return nil
	}
	return C.CString(p)
}

//export arenileShellDownloadDone
func arenileShellDownloadDone(path *C.char, failed C.int) {
	if c := activeController(); c != nil {
		p := C.GoString(path)
		go c.DownloadFinished(p, failed != 0)
	}
}
