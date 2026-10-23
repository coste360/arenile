// Package shell is the native window of the desktop edition (spec 29.1,
// local and LAN mode): the panel runs inside the app's own window, not in
// the default browser. The browser stays available ("Apri nel browser" in
// the tray and in the Vista menu, flag -browser).
//
// # Layers (functional core / imperative shell)
//
//   - Pure, tested on every OS (no tags, no CGO): WindowState and its file
//     <data dir>/window.json (LoadState, SaveState, Normalize, NextZoom),
//     the macOS application menu as data (AppMenu, Menu, MenuEntry) and its
//     wiring to Go handlers (Commands), CloseAction (hide or quit when the
//     window is closed), the download file naming (SanitizeFileName,
//     UniquePath, DownloadsDir), the external-link policy (AllowExternal)
//     and the script injected in the window (InitScript).
//   - Controller holds the window behaviour over the Native port (navigate,
//     show, hide, reload, zoom, terminate): which URL is shown, what closing
//     does, where a download goes, what a menu action runs.
//   - Native implementations, behind the build tags "desktop && cgo &&
//     !linux": window_webview.go (github.com/webview/webview_go: WKWebView on
//     macOS, WebView2 on Windows), cocoa_darwin.{go,m} (a small Objective-C
//     shim: main menu, close-to-hide, Dock reopen, Cmd+Q, WKDownload to
//     ~/Downloads, printing, window frame and zoom) and win_windows.go
//     (Win32: WM_CLOSE subclass, window placement, focus, WebView2 presence).
//     WebView2 handles downloads, printing, zoom and the edit shortcuts by
//     itself; WKWebView needs the shim for each of them.
//
// Linux has no native window: webview_go links WebKitGTK 4.0, no longer
// shipped by current distributions (ADR 0015 point 9); the Linux build opens
// the panel in the default browser as before.
package shell
