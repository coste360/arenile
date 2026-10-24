//go:build desktop && cgo && windows

package shell

import (
	"sync"
	"syscall"
	"unsafe"

	webview "github.com/webview/webview_go"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// The Windows layer is plain Win32 through golang.org/x/sys (no C): the
// WebView2 control already handles downloads (Downloads folder, its own
// download flyout), window.print (print preview), the edit shortcuts
// (Ctrl+C/V/X/Z/A), reload (F5, Ctrl+R) and zoom (Ctrl +/-/0). This file
// adds the close behaviour (WM_CLOSE subclass: hide or quit, never destroy
// before the state is saved), the window placement and the focus on a
// second launch. Links and navigations that leave the panel are sent to
// the default browser by the injected script (desktop.LinkScript: clicks,
// window.open and the Navigation API's navigate event, which WebView2
// supports), so no WebView2 COM handler is needed here; a same-origin
// panel URL opened in a new window (Z report PDF) gets WebView2's own
// popup, which the operator closes to return to the panel.

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSetWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW     = user32.NewProc("CallWindowProcW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetWindowPlacement  = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement  = user32.NewProc("SetWindowPlacement")
)

const (
	gwlpWndProc       = ^uintptr(3) // GWLP_WNDPROC (-4)
	wmClose           = 0x0010
	swHide            = 0
	swShowNormal      = 1
	swShowMaximized   = 3
	swShow            = 5
	swRestore         = 9
	webView2ClientKey = `Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
)

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

type windowPlacement struct {
	Length         uint32
	Flags          uint32
	ShowCmd        uint32
	MinPosition    point
	MaxPosition    point
	NormalPosition rect
}

var (
	wndProcOnce sync.Once
	wndProcCB   uintptr
	origMu      sync.Mutex
	origProc    uintptr
	activeCtl   *Controller
)

type platform struct {
	w    webview.WebView
	c    *Controller
	hwnd uintptr
}

// platformAvailable reports whether the Evergreen WebView2 runtime is
// installed (Microsoft's documented check: the "pv" value of the runtime
// client key, per machine or per user). Windows 11 ships it; on Windows 10
// without it the launcher opens the browser instead.
func platformAvailable() bool {
	for _, k := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\` + webView2ClientKey},
		{registry.LOCAL_MACHINE, `SOFTWARE\` + webView2ClientKey},
		{registry.CURRENT_USER, `Software\` + webView2ClientKey},
	} {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := key.GetStringValue("pv")
		_ = key.Close()
		if err == nil && v != "" && v != "0.0.0.0" {
			return true
		}
	}
	return false
}

func newPlatform(w webview.WebView, c *Controller) *platform {
	return &platform{w: w, c: c, hwnd: uintptr(w.Window())}
}

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	origMu.Lock()
	orig, c := origProc, activeCtl
	origMu.Unlock()
	if msg == wmClose && c != nil {
		c.OnClose()
		return 0
	}
	r, _, _ := procCallWindowProcW.Call(orig, hwnd, msg, wp, lp)
	return r
}

func (p *platform) install(st WindowState) {
	wndProcOnce.Do(func() { wndProcCB = syscall.NewCallback(wndProc) })
	origMu.Lock()
	activeCtl = p.c
	prev, _, _ := procSetWindowLongPtrW.Call(p.hwnd, gwlpWndProc, wndProcCB)
	if prev == 0 {
		// Not subclassed: webview_go's own close (destroy, loop ends) stays.
		activeCtl = nil
		p.c.o.Log.Warn("desktop window: close handling not installed")
	}
	origProc = prev
	origMu.Unlock()
	if st.Placed || st.Maximized {
		wp := windowPlacement{ShowCmd: swShowNormal}
		wp.Length = uint32(unsafe.Sizeof(wp))
		if st.Maximized {
			wp.ShowCmd = swShowMaximized
		}
		if st.Placed {
			wp.NormalPosition = rect{Left: int32(st.X), Top: int32(st.Y), Right: int32(st.X + st.Width), Bottom: int32(st.Y + st.Height)}
		} else {
			cur := windowPlacement{}
			cur.Length = uint32(unsafe.Sizeof(cur))
			_, _, _ = procGetWindowPlacement.Call(p.hwnd, uintptr(unsafe.Pointer(&cur)))
			wp.NormalPosition = cur.NormalPosition
		}
		// SetWindowPlacement moves a window that would be off every
		// monitor back onto the work area.
		_, _, _ = procSetWindowPlacement.Call(p.hwnd, uintptr(unsafe.Pointer(&wp)))
	}
}

func (p *platform) uninstall() {
	origMu.Lock()
	if origProc != 0 {
		_, _, _ = procSetWindowLongPtrW.Call(p.hwnd, gwlpWndProc, origProc)
	}
	origProc, activeCtl = 0, nil
	origMu.Unlock()
}

func (p *platform) show() {
	if r, _, _ := procIsIconic.Call(p.hwnd); r != 0 {
		_, _, _ = procShowWindow.Call(p.hwnd, swRestore)
	} else {
		_, _, _ = procShowWindow.Call(p.hwnd, swShow)
	}
	_, _, _ = procSetForegroundWindow.Call(p.hwnd)
}

func (p *platform) hide() { _, _, _ = procShowWindow.Call(p.hwnd, swHide) }

// setZoom: WebView2 has its own zoom (Ctrl +/-/0), kept per origin.
func (p *platform) setZoom(float64) {}

// print: WebView2 implements window.print (print preview).
func (p *platform) print() {}

func (p *platform) frame() (WindowState, bool) {
	wp := windowPlacement{}
	wp.Length = uint32(unsafe.Sizeof(wp))
	if r, _, _ := procGetWindowPlacement.Call(p.hwnd, uintptr(unsafe.Pointer(&wp))); r == 0 {
		return WindowState{}, false
	}
	n := wp.NormalPosition
	return WindowState{
		X: int(n.Left), Y: int(n.Top), Width: int(n.Right - n.Left), Height: int(n.Bottom - n.Top),
		Placed: true, Maximized: wp.ShowCmd == swShowMaximized,
	}, true
}

// SetAccessoryActivation is a macOS concept (no Dock icon): nothing here.
func SetAccessoryActivation() {}
