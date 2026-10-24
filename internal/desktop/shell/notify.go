package shell

import (
	"context"
	"sync"

	"github.com/coste360/arenile/internal/desktop"
)

// Native notifications. On macOS desktop builds (notify_darwin.go) they
// go through UNUserNotificationCenter: macOS shows them with the name and
// icon of Arenile.app (osascript's "display notification" is attributed to
// Script Editor) and a click reaches the app. Everywhere else, and on
// macOS when that is not possible (a development binary outside the app
// bundle, notifications not allowed), desktop.Notify is used (osascript,
// notify-send; nothing on Windows).

// nativeNotify posts a native notification; nil where there is none. It
// returns false when the notification must go through the fallback; the
// fallback it is given runs later when the native path fails
// asynchronously (authorization denied).
//
// The platform hook and the click handler are process-wide because the
// operating system delivers notification clicks to the process, not to a
// window (same reason as the controller of cocoa_darwin.go).
var nativeNotify func(title, message, target string, fallback func()) bool

var (
	clickMu sync.Mutex
	onClick func(target string)
)

// SetNotificationHandler sets what a click on a native notification does:
// target is the panel URL given to Notify ("" = just show the app). The
// UIs set it (cmd/arenile-desktop): show the window at target, or the
// browser in the tray-only build.
func SetNotificationHandler(fn func(target string)) {
	clickMu.Lock()
	onClick = fn
	clickMu.Unlock()
}

// notificationClicked runs the click handler (native callback).
func notificationClicked(target string) {
	clickMu.Lock()
	fn := onClick
	clickMu.Unlock()
	if fn != nil {
		go fn(target)
	}
}

// Notify shows a desktop notification on goos, best effort: native when
// available (macOS: clickable, a click opens target), otherwise
// desktop.Notify.
func Notify(ctx context.Context, goos, title, message, target string) error {
	return notifyWith(nativeNotify, func() error { return desktop.Notify(ctx, goos, title, message) }, title, message, target)
}

// notifyWith picks the native path or the fallback (pure apart from the
// two functions, tested with fakes).
func notifyWith(native func(title, message, target string, fallback func()) bool, fallback func() error, title, message, target string) error {
	if native != nil && native(title, message, target, func() { _ = fallback() }) {
		return nil
	}
	return fallback()
}
