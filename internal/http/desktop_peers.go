package httpserver

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/middleware"
)

// desktopPeers refuses requests from peers outside the local network on
// the desktop edition (LAN mode, spec 29.1); the server edition is behind
// proxies and keeps its own exposure rules, so it is a no-op there.
func desktopPeers(d Deps) middleware.Middleware {
	if d.Desktop == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return middleware.LocalPeersOnly()
}
