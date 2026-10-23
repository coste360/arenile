package desktop

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// IsPrivateAddr reports whether a peer address belongs to a local network:
// loopback, RFC 1918 (10/8, 172.16/12, 192.168/16), RFC 6598 is excluded
// (carrier-grade NAT is not the lido Wi-Fi), IPv6 ULA (fc00::/7) and
// link-local (169.254/16, fe80::/10). IPv4-mapped IPv6 is unmapped first.
func IsPrivateAddr(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsValid() && (a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast())
}

// PrivateListener wraps a listener and closes, before any byte is read,
// the connections whose peer is not on a local network (spec 29.1: "Le
// connessioni dall'esterno della rete locale sono rifiutate"). Wrap the
// raw TCP listener, then tls.NewListener on top.
type PrivateListener struct {
	net.Listener
	// Rejected, when set, is called with each refused peer (for a counter
	// or a debug log; never log more than the address).
	Rejected func(addr net.Addr)
}

// Accept implements net.Listener.
func (l *PrivateListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if ap, err := netip.ParseAddrPort(c.RemoteAddr().String()); err == nil && IsPrivateAddr(ap.Addr()) {
			return c, nil
		}
		if l.Rejected != nil {
			l.Rejected(c.RemoteAddr())
		}
		_ = c.Close()
	}
}

// Fingerprint is the SHA-256 of a DER certificate, uppercase hex separated
// by ':' (the form shown in the QR and in the tray menu).
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	var b strings.Builder
	for i, v := range sum {
		if i > 0 {
			b.WriteByte(':')
		}
		fmt.Fprintf(&b, "%02X", v)
	}
	return b.String()
}

// CertStore provides the self-signed LAN certificate (implementation:
// internal/desktop/lan.CertStore).
type CertStore interface {
	// LoadOrCreate returns the certificate in <data dir>/lan/ (ECDSA P-256,
	// 825 days, SANs: hostname.local, arenile.local, localhost and the
	// private addresses at creation time), creating it when missing or
	// corrupted and regenerating it 30 days before expiry (lan.RenewBefore).
	// The fingerprint is stable across restarts and address changes: a
	// changed address is not a reason to renew (clients pin the
	// fingerprint, browsers remember the exception).
	LoadOrCreate(ctx context.Context) (tls.Certificate, error)
}

// LANAddress picks the address shown in the QR: the first private IPv4 of
// an up, non-loopback interface (pure over the given addresses).
func LANAddress(addrs []netip.Addr) (netip.Addr, bool) {
	for _, a := range addrs {
		a = a.Unmap()
		if a.Is4() && a.IsPrivate() {
			return a, true
		}
	}
	return netip.Addr{}, false
}

// PinnedClientConfig returns a TLS config that accepts exactly the server
// certificate with fingerprint (desktop remote mode and tests against a LAN
// instance): chain verification is replaced by the pin.
func PinnedClientConfig(fingerprint string) *tls.Config {
	want := strings.ToUpper(fingerprint)
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // G402: replaced by certificate pinning in VerifyConnection
		// VerifyConnection also runs on resumed sessions.
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 || Fingerprint(cs.PeerCertificates[0].Raw) != want {
				return errors.New("desktop: certificate fingerprint mismatch")
			}
			return nil
		},
	}
}
