//go:build windows

package desktop

import "golang.org/x/sys/windows"

var procAllowSetForegroundWindow = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

// asfwAny is ASFW_ANY: any process may take the foreground.
const asfwAny = ^uintptr(0)

// allowForegroundActivation lets the running instance bring its window to
// the front: Windows refuses SetForegroundWindow to a background process
// unless the foreground one (this second launch, just started by the
// operator) allows it.
func allowForegroundActivation() {
	if procAllowSetForegroundWindow.Find() == nil {
		_, _, _ = procAllowSetForegroundWindow.Call(asfwAny)
	}
}
