//go:build desktop && cgo && windows

package shell

/*
// Exported by github.com/webview/webview_go (webview.h, compiled there with
// WEBVIEW_STATIC, C linkage); declared here instead of including the
// header, which lives in the module cache. kind 2 is
// WEBVIEW_NATIVE_HANDLE_KIND_BROWSER_CONTROLLER: the
// ICoreWebView2Controller* of the window on Windows.
void *webview_get_native_handle(void *w, int kind);
*/
import "C"

import (
	"unsafe"

	webview "github.com/webview/webview_go"
)

const (
	nativeHandleKindBrowserController = 2
	gwlpUserData                      = ^uintptr(20) // GWLP_USERDATA (-21)
)

var procGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")

// browserController returns the ICoreWebView2Controller of w, or nil when
// it cannot be obtained safely. The webview_t handle is read from
// webview_go's wrapper (webviewHandle) and used only when it equals the
// engine pointer the library stores in its window (GWLP_USERDATA of the
// top-level window it created, webview.h win32_edge_engine): two
// independent derivations must agree before a C function is called with
// it, so a future version of the library with another layout disables the
// hooks instead of crashing.
func browserController(w webview.WebView, hwnd uintptr) unsafe.Pointer {
	h, ok := webviewHandle(w)
	if !ok || hwnd == 0 {
		return nil
	}
	stored, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlpUserData)
	if stored == 0 || stored != uintptr(h) {
		return nil
	}
	return C.webview_get_native_handle(h, C.int(nativeHandleKindBrowserController))
}
