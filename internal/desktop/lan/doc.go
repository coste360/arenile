// Package lan is the LAN mode of the desktop edition (spec 29.1,
// "Modalità rete locale"): the cash desk PC exposes its single server on
// the lido Wi-Fi so that other PCs, tablets and phones work on the same
// archive from the browser (and, from M7, from the mobile app) without
// internet. It implements desktop.LANMode and is wired by
// cmd/arenile-desktop through desktop.Options.NewLAN.
//
// Ownership (M6): the "LAN and remote mode" builder.
//
// # What Enable does
//
//  1. CertStore loads or creates a self-signed ECDSA P-256 certificate in
//     <data dir>/lan (cert.pem, key.pem 0600). Its SHA-256 fingerprint is
//     stable across restarts: it is regenerated only when missing,
//     unreadable or within 30 days of its 825-day expiry (the longest
//     validity Apple platforms accept), never because the DHCP address
//     changed, so paired devices that pin the fingerprint keep working.
//     SANs: the private addresses at creation time, <hostname>.local,
//     arenile.local and localhost.
//  2. It listens on every interface (":<port>", default 8443) and wraps the
//     listener in desktop.PrivateListener, which closes connections from
//     peers outside the local network before the TLS handshake.
//  3. TLS 1.2+ on top, then the panel of the running app (LANDeps.Serve,
//     i.e. App.ServeListener).
//  4. It returns the https URL (first private IPv4, else <hostname>.local)
//     and the fingerprint; the runtime reports them to /meta and the
//     pairing QR (internal/domain/device) embeds both.
//
// # Why peers are filtered twice
//
// The server holds personal data of customers and staff (spec 23) and the
// desktop edition has no rate limiting proxy in front of it. A cash desk
// PC can become reachable from the internet without the operator knowing:
// a router port forward or UPnP rule, a public IPv6 address with a
// permissive router firewall, a hotel-style network. So the LAN mode
// accepts loopback, RFC 1918, link-local and IPv6 ULA peers only:
//
//   - at the listener (desktop.PrivateListener): the connection is closed
//     before TLS, nothing is disclosed, not even the certificate;
//   - at the HTTP layer (middleware.LocalPeersOnly, enabled on the desktop
//     edition by internal/http): defence in depth if a listener is ever
//     opened without the filter. Both use r.RemoteAddr / the TCP peer,
//     never X-Forwarded-For.
//
// Carrier-grade NAT (100.64/10, also used by some VPNs) is refused: the
// LAN mode is for the lido network; reaching the desktop from elsewhere
// means moving to an online instance (spec 29.0).
//
// # Browsers and the certificate warning
//
// The certificate is self-signed, so browsers on other devices show a
// warning the first time. The panel page "Rete locale" (web/admin
// features/devices/lan, opened by the tray item "Mostra QR rete locale",
// QRPagePath) shows the address, the fingerprint and the pairing QR,
// and explains how to compare the fingerprint in the browser certificate
// details before accepting. The mobile app and the desktop app in remote
// mode do not need the warning: they pin the fingerprint from the QR.
//
// # Availability guarantees
//
// There is still exactly one server and one SQLite database: the LAN
// clients use the same HTTP API, so the double booking guarantees of
// section 7 (DB constraints on spot_allocations and pool_usage) are
// unchanged. No synchronisation is involved.
//
// # Not included
//
// mDNS/Bonjour announcement: skipped to avoid a new dependency. macOS and
// Windows 10+ answer <hostname>.local themselves, which is in the SANs;
// elsewhere the QR carries the IP address.
package lan
