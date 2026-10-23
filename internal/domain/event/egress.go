package event

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// ErrBlockedDestination is returned when a webhook URL resolves to an
// address the EgressPolicy does not allow (SSRF protection, spec 22).
var ErrBlockedDestination = errors.New("event: webhook destination not allowed")

// EgressPolicy decides which network addresses outgoing webhook deliveries
// may reach. The endpoint URL is chosen by the establishment owner, so
// without a policy the server could be used to probe its own network
// (SSRF): cloud metadata services, the database, the admin port of a router.
//
// Always blocked, whatever the policy: unspecified, multicast, broadcast,
// link-local (169.254.0.0/16 with the cloud metadata address, fe80::/10),
// "this network" 0.0.0.0/8, reserved 240.0.0.0/4, documentation ranges,
// Teredo. Blocked unless AllowPrivate: loopback, RFC 1918, unique local
// fc00::/7, deprecated site-local fec0::/10, CGNAT 100.64.0.0/10, IETF
// 192.0.0.0/24, benchmarking 198.18.0.0/15. IPv4 addresses embedded in IPv6
// (mapped ::ffff:a.b.c.d, NAT64 64:ff9b::/96 and 64:ff9b:1::/48, 6to4
// 2002::/16) are checked as the IPv4 address they carry.
type EgressPolicy struct {
	// AllowPrivate permits loopback and private targets, for self-hosted
	// installations whose receivers live on the same LAN
	// (ARENILE_WEBHOOKS_ALLOW_PRIVATE). Link-local and the other always
	// blocked ranges stay blocked.
	AllowPrivate bool
}

var (
	alwaysBlocked = prefixes(
		"0.0.0.0/8", "169.254.0.0/16", "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24",
		"224.0.0.0/4", "240.0.0.0/4", "255.255.255.255/32",
		"::/128", "fe80::/10", "ff00::/8", "2001:db8::/32", "2001::/32",
	)
	privateRanges = prefixes(
		"10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "172.16.0.0/12", "192.0.0.0/24",
		"192.168.0.0/16", "198.18.0.0/15",
		"::1/128", "fc00::/7", "fec0::/10",
	)
	nat64Prefixes = prefixes("64:ff9b::/96", "64:ff9b:1::/48")
	sixToFour     = netip.MustParsePrefix("2002::/16")
)

func prefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

func inAny(a netip.Addr, ps []netip.Prefix) bool {
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Check reports whether a may be contacted (nil) or why not.
func (p EgressPolicy) Check(a netip.Addr) error {
	if !a.IsValid() {
		return fmt.Errorf("%w: invalid address", ErrBlockedDestination)
	}
	a = a.Unmap().WithZone("")
	if a.Is6() {
		if v4, ok := embeddedIPv4(a); ok {
			if err := p.Check(v4); err != nil {
				return fmt.Errorf("%w (embedded in %s)", err, a)
			}
		}
	}
	if inAny(a, alwaysBlocked) || a.IsUnspecified() || a.IsMulticast() || a.IsLinkLocalUnicast() {
		return fmt.Errorf("%w: %s is a reserved address", ErrBlockedDestination, a)
	}
	if !p.AllowPrivate && (inAny(a, privateRanges) || a.IsLoopback() || a.IsPrivate()) {
		return fmt.Errorf("%w: %s is a private or loopback address (set ARENILE_WEBHOOKS_ALLOW_PRIVATE=true for receivers on the local network)", ErrBlockedDestination, a)
	}
	return nil
}

// embeddedIPv4 extracts the IPv4 address carried by NAT64 and 6to4 IPv6
// addresses.
func embeddedIPv4(a netip.Addr) (netip.Addr, bool) {
	b := a.As16()
	switch {
	case inAny(a, nat64Prefixes):
		return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}), true
	case sixToFour.Contains(a):
		return netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}), true
	}
	return netip.Addr{}, false
}

// Resolver resolves host names (net.DefaultResolver satisfies it; tests
// inject a fake).
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// dialer returns a DialContext that resolves the host once, refuses the
// connection when any resolved address is not allowed (a name answering
// both a public and a private address is not trusted at all), then connects
// to the checked IP literals only: the name is never resolved again by the
// transport, so a DNS rebinding answer between check and connect cannot
// redirect the request. The low-level dial additionally re-checks the
// address actually connected (Control hook) as a second line of defence.
func (p EgressPolicy) dialer(r Resolver, dial dialFunc) dialFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrBlockedDestination, err)
		}
		var addrs []netip.Addr
		if a, perr := netip.ParseAddr(host); perr == nil {
			addrs = []netip.Addr{a}
		} else {
			addrs, err = r.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve %s: %w", host, err)
			}
		}
		if len(addrs) == 0 {
			return nil, fmt.Errorf("resolve %s: no addresses", host)
		}
		for _, a := range addrs {
			if err := p.Check(a); err != nil {
				return nil, err
			}
		}
		var lastErr error
		for _, a := range addrs {
			conn, err := dial(ctx, network, net.JoinHostPort(a.Unmap().String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

// control is the net.Dialer Control hook: it runs with the address the
// socket is about to connect to.
func (p EgressPolicy) control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrBlockedDestination, err)
	}
	return p.Check(ap.Addr())
}

// NewEgressClient returns the HTTP client used for webhook deliveries: the
// EgressPolicy enforced at dial time, no proxy from the environment (a proxy
// would bypass the address check), no redirects (a 3xx is a failed attempt:
// following it could reach a blocked address and the receiver should fix its
// URL), TLS 1.2+, bounded handshake, header and total time. resolver may be
// nil (net.DefaultResolver).
func NewEgressClient(policy EgressPolicy, resolver Resolver, timeout time.Duration) *http.Client {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	nd := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second, Control: policy.control}
	tr := &http.Transport{
		Proxy:                  nil,
		DialContext:            policy.dialer(resolver, nd.DialContext),
		ForceAttemptHTTP2:      true,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  timeout,
		ExpectContinueTimeout:  time.Second,
		MaxIdleConns:           32,
		MaxIdleConnsPerHost:    2,
		IdleConnTimeout:        90 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
	}
	return &http.Client{
		Transport:     tr,
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
