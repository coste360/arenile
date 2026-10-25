//go:build desktop && cgo && darwin

package shell

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#include <stdint.h>
#include <stdlib.h>
#include "notify_darwin.h"
*/
import "C"

import (
	"runtime/cgo"
	"unsafe"
)

// UNUserNotificationCenter needs a real app bundle: the shim reports
// failure (and the caller uses osascript) when the process has no bundle
// identifier, i.e. a development binary run outside Arenile.app. Local
// notifications need no entitlement in a non-sandboxed Developer ID app.
func init() {
	nativeNotify = postNative
	nativeInit = func() { C.arenile_notify_init() }
}

// postNative posts the notification through notify_darwin.m. The shim
// runs fallback (through arenileNotifyFallback) when the authorization
// turns out to be denied; it reports false at once when there is no
// bundle, and the caller then runs the fallback itself.
func postNative(title, message, target string, fallback func()) bool {
	ct, cm, cu := C.CString(title), C.CString(message), C.CString(target)
	defer func() {
		C.free(unsafe.Pointer(ct))
		C.free(unsafe.Pointer(cm))
		C.free(unsafe.Pointer(cu))
	}()
	h := cgo.NewHandle(fallback)
	if C.arenile_notify_post(ct, cm, cu, C.uintptr_t(h)) == 0 {
		h.Delete()
		return false
	}
	return true
}

// arenileNotifyFallback runs the fallback of a notification the native
// path could not show (authorization denied or failed) and releases it.
//
//export arenileNotifyFallback
func arenileNotifyFallback(h C.uintptr_t) {
	hd := cgo.Handle(h)
	fn, _ := hd.Value().(func())
	hd.Delete()
	if fn != nil {
		go fn()
	}
}

// arenileNotifyDone releases the fallback of a notification shown natively.
//
//export arenileNotifyDone
func arenileNotifyDone(h C.uintptr_t) { cgo.Handle(h).Delete() }

// arenileNotifyClicked is the click on a native notification: target is
// the panel URL it carries ("" = just show the app).
//
//export arenileNotifyClicked
func arenileNotifyClicked(target *C.char) { notificationClicked(C.GoString(target)) }
