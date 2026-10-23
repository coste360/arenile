package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/coste360/arenile/internal/http/problem"
)

// LoopbackOnly restricts a route group to the local machine (desktop
// edition, /api/desktop/v1): the TCP peer (r.RemoteAddr, never
// X-Forwarded-For) must be a loopback address and the Host header must be
// a loopback name, so that a page on another origin cannot reach the
// endpoints through DNS rebinding. Anything else is 403 loopback_required.
func LoopbackOnly() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isLoopbackPeer(r.RemoteAddr) || !isLoopbackHost(r.Host) {
				problem.Write(w, r, problem.LoopbackRequired)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isLoopbackPeer(remoteAddr string) bool {
	ap, err := netip.ParseAddrPort(remoteAddr)
	if err != nil {
		return false
	}
	return ap.Addr().Unmap().IsLoopback()
}

func isLoopbackHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	a, err := netip.ParseAddr(host)
	return err == nil && a.Unmap().IsLoopback()
}
