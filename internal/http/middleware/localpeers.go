package middleware

import (
	"net/http"
	"net/netip"

	"github.com/coste360/arenile/internal/http/problem"
)

// LocalPeersOnly is the HTTP-level half of the desktop LAN mode guard
// (spec 29.1: "Le connessioni dall'esterno della rete locale sono
// rifiutate"). The desktop edition serves the panel on 127.0.0.1 and, in
// LAN mode, on a TLS listener bound to every interface; that listener
// already closes connections from non-local peers before TLS
// (desktop.PrivateListener). This middleware checks the TCP peer of every
// request again (r.RemoteAddr, never X-Forwarded-For, which the desktop
// edition does not trust), so a listener opened without the filter, a
// port forward, UPnP or a public IPv6 address on the cash desk PC can
// never expose the archive to the internet. Anything that is not
// loopback, RFC 1918, link-local or IPv6 ULA gets 403 forbidden.
func LocalPeersOnly() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ap, err := netip.ParseAddrPort(r.RemoteAddr)
			if err != nil || !IsLocalPeer(ap.Addr()) {
				problem.Write(w, r, problem.Forbidden, problem.Detail("Accesso consentito solo dalla rete locale."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IsLocalPeer reports whether a peer address is on the local network:
// loopback, RFC 1918, link-local (169.254/16, fe80::/10) or IPv6 ULA
// (fc00::/7); IPv4-mapped IPv6 is unmapped first. Carrier-grade NAT
// (100.64/10) and every public address are refused. It must agree with
// desktop.IsPrivateAddr (cross-checked by the lan package tests).
func IsLocalPeer(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsValid() && (a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast())
}
