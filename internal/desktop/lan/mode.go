package lan

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop"
)

// Tray integration: the launcher adds a menu item with this id and label
// that opens QRPagePath on the loopback panel (the page that shows the
// address, the QR and the fingerprint).
const (
	MenuShowQR      = "lan_qr"
	MenuShowQRLabel = "Mostra QR rete locale"
	QRPagePath      = "/admin/rete-locale"
)

// Info describes the running LAN listener.
type Info struct {
	Enabled     bool
	URL         string
	Fingerprint string
	Port        int
	Addresses   []netip.Addr
	Hostname    string
	// Rejected counts the connections closed because the peer was not on
	// the local network (since Enable).
	Rejected int64
}

// Deps configure a Mode (desktop.LANDeps plus test seams).
type Deps struct {
	desktop.LANDeps
	Clock clock.Clock
	// Certs overrides the certificate store (default <DataDir>/lan).
	Certs *CertStore
	// Listen overrides net.Listen (tests bind 127.0.0.1).
	Listen func(network, addr string) (net.Listener, error)
	// Addrs overrides InterfaceAddrs (tests).
	Addrs func() ([]netip.Addr, error)
	// Hostname overrides LocalHostname (tests).
	Hostname func() string
}

// Mode implements desktop.LANMode: it opens and closes the filtered TLS
// listener of the LAN mode. Safe for concurrent use; Enable and Disable
// are idempotent.
type Mode struct {
	d Deps

	mu       sync.Mutex
	stop     func(context.Context) error
	info     Info
	rejected atomic.Int64
}

var _ desktop.LANMode = (*Mode)(nil)

// New returns the LAN mode adapter for desktop.Options.NewLAN.
func New(d desktop.LANDeps) desktop.LANMode { return NewMode(Deps{LANDeps: d}) }

// NewMode returns a Mode with explicit dependencies.
func NewMode(d Deps) *Mode {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Certs == nil {
		d.Certs = &CertStore{Dir: d.DataDir + "/lan", Clock: d.Clock}
	}
	if d.Listen == nil {
		d.Listen = net.Listen
	}
	if d.Addrs == nil {
		d.Addrs = InterfaceAddrs
	}
	if d.Hostname == nil {
		d.Hostname = LocalHostname
	}
	return &Mode{d: d}
}

// ErrInvalidPort: the port is outside 1024..65535.
var ErrInvalidPort = errors.New("lan: port must be between 1024 and 65535")

// Enable implements desktop.LANMode: it starts the listener on port (or
// keeps the running one when the port is unchanged) and returns the URL
// and certificate fingerprint for the pairing QR.
func (m *Mode) Enable(ctx context.Context, port int) (string, string, error) {
	if port < 1024 || port > 65535 {
		return "", "", ErrInvalidPort
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stop != nil && m.info.Port == port {
		m.refreshAddrs()
		return m.info.URL, m.info.Fingerprint, nil
	}
	if err := m.disableLocked(ctx); err != nil {
		return "", "", err
	}
	cert, err := m.d.Certs.Load(ctx)
	if err != nil {
		return "", "", err
	}
	raw, err := m.d.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return "", "", fmt.Errorf("lan: listen on port %d: %w", port, err)
	}
	m.rejected.Store(0)
	filtered := &desktop.PrivateListener{Listener: raw, Rejected: func(a net.Addr) {
		m.rejected.Add(1)
		m.d.Log.Debug("lan: connection from outside the local network refused", slog.String("peer", a.String()))
	}}
	ln := tls.NewListener(filtered, ServerTLSConfig(cert.TLS))
	if m.d.Serve == nil {
		_ = ln.Close()
		return "", "", errors.New("lan: no app to serve")
	}
	m.stop = m.d.Serve(ln)
	m.info = Info{Enabled: true, Fingerprint: cert.Fingerprint, Port: port}
	m.refreshAddrs()
	m.d.Log.Info("lan mode enabled", slog.String("url", m.info.URL), slog.Int("port", port))
	return m.info.URL, m.info.Fingerprint, nil
}

// refreshAddrs re-reads the interface addresses (DHCP may have changed).
func (m *Mode) refreshAddrs() {
	addrs, err := m.d.Addrs()
	if err != nil {
		m.d.Log.Warn("lan: cannot list network interfaces", slog.String("error", err.Error()))
	}
	m.info.Addresses, m.info.Hostname = addrs, m.d.Hostname()
	m.info.URL = PanelURL(addrs, m.info.Hostname, m.info.Port)
	if m.info.URL == "" {
		m.info.URL = "https://" + net.JoinHostPort("localhost", strconv.Itoa(m.info.Port))
	}
}

// Disable implements desktop.LANMode.
func (m *Mode) Disable(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.disableLocked(ctx)
}

func (m *Mode) disableLocked(ctx context.Context) error {
	if m.stop == nil {
		return nil
	}
	err := m.stop(ctx)
	m.stop, m.info = nil, Info{}
	m.d.Log.Info("lan mode disabled")
	return err
}

// Info returns the state of the listener.
func (m *Mode) Info() Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.info
	i.Addresses = append([]netip.Addr(nil), i.Addresses...)
	i.Rejected = m.rejected.Load()
	return i
}

// ServerTLSConfig is the TLS configuration of the LAN listener: TLS 1.2+,
// HTTP/2 and HTTP/1.1.
func ServerTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}
}
