//go:build !windows

package desktop

// allowForegroundActivation: nothing to do outside Windows (macOS lets the
// app activate itself when its window is shown on request).
func allowForegroundActivation() {}
