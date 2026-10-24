package printing

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/domain/configerr"
)

// Resolver resolves printer host names (net.DefaultResolver in production).
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// cgnat is the shared address space of RFC 6598 (some LANs and VPNs use it).
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// Cloud metadata endpoints live in link-local space: never a printer.
var metadata = []netip.Addr{
	netip.MustParseAddr("169.254.169.254"), // AWS, GCP, Azure, OpenStack
	netip.MustParseAddr("fd00:ec2::254"),   // AWS IPv6
}

// AddressPolicy decides which addresses a printer may have. The zero value
// is the production policy: LAN addresses only. AllowLoopback admits
// 127/8 and ::1 for development and tests (a fake printer on this PC,
// ARENILE_PRINTERS_ALLOW_LOOPBACK), never by default: a printer is another
// device, and loopback would expose local services to LF-terminated bytes.
type AddressPolicy struct {
	AllowLoopback bool
}

// Allowed reports whether ip is a private or local-network address: RFC
// 1918, RFC 6598 (100.64/10), link-local, IPv6 ULA (fc00::/7) and
// link-local (fe80::/10), plus loopback with AllowLoopback. Unspecified,
// multicast, public and cloud-metadata addresses are refused.
func (p AddressPolicy) Allowed(ip netip.Addr) bool {
	ip = ip.Unmap().WithZone("")
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	for _, m := range metadata {
		if ip == m {
			return false
		}
	}
	if ip.IsLoopback() {
		return p.AllowLoopback
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || (ip.Is4() && cgnat.Contains(ip))
}

// AllowedIP is AddressPolicy{}.Allowed: the production rule.
func AllowedIP(ip netip.Addr) bool { return AddressPolicy{}.Allowed(ip) }

// NormalizeHost trims and validates the syntax of a printer host: an IPv4
// or IPv6 literal (brackets removed, zone kept) or a DNS name.
func NormalizeHost(host string) (string, error) {
	h := strings.TrimSpace(host)
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	if h == "" {
		return "", configerr.Invalid("host", "Obbligatorio.")
	}
	if len(h) > 253 {
		return "", configerr.Invalid("host", "Al massimo 253 caratteri.")
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return h, nil
	}
	for _, label := range strings.Split(strings.TrimSuffix(h, "."), ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", configerr.Invalid("host", "Indirizzo IP o nome host non valido.")
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				return "", configerr.Invalid("host", "Indirizzo IP o nome host non valido.")
			}
		}
	}
	return strings.ToLower(h), nil
}

// CheckHost verifies that host is (or resolves only to) addresses allowed
// by pol: ErrAddressNotAllowed otherwise, a validation error when a name
// does not resolve. Called on create/update; the network driver checks
// again the address it actually dials (DNS may change).
func CheckHost(ctx context.Context, r Resolver, host string, pol AddressPolicy) error {
	if ip, err := netip.ParseAddr(host); err == nil {
		if !pol.Allowed(ip) {
			return ErrAddressNotAllowed
		}
		return nil
	}
	if r == nil {
		r = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, err := r.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return configerr.Invalid("host", "Nome host non trovato nella rete locale: usa l'indirizzo IP della stampante.")
	}
	for _, a := range addrs {
		if !pol.Allowed(a) {
			return fmt.Errorf("%w: %s resolves to a public address", ErrAddressNotAllowed, host)
		}
	}
	return nil
}
