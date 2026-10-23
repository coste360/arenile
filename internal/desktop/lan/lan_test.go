package lan

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop"
	"github.com/coste360/arenile/internal/http/middleware"
)

var t0 = time.Date(2027, 6, 1, 9, 0, 0, 0, time.UTC)

func testNames() ([]string, []netip.Addr) {
	return []string{"cassa.local", "arenile.local", "localhost"}, []netip.Addr{
		netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("192.168.1.20"), netip.MustParseAddr("fd00::20"),
	}
}

func TestCertStore_GenerationAndStableFingerprint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "lan")
	clk := clock.NewFake(t0)
	s := &CertStore{Dir: dir, Clock: clk, Names: testNames}
	c1, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	leaf := c1.Leaf
	if pk, ok := leaf.PublicKey.(*ecdsa.PublicKey); !ok || pk.Curve.Params().Name != "P-256" {
		t.Fatalf("not ECDSA: %v", leaf.PublicKeyAlgorithm)
	}
	if !leaf.NotAfter.Equal(t0.Add(CertValidity)) || len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Fatalf("validity/EKU %v %v", leaf.NotAfter, leaf.ExtKeyUsage)
	}
	if err := leaf.VerifyHostname("cassa.local"); err != nil {
		t.Fatal(err)
	}
	if err := leaf.VerifyHostname("192.168.1.20"); err != nil {
		t.Fatal(err)
	}
	if err := leaf.VerifyHostname("fd00::20"); err != nil {
		t.Fatal(err)
	}
	if c1.Fingerprint != desktop.Fingerprint(leaf.Raw) || len(c1.Fingerprint) != 95 {
		t.Fatalf("fingerprint %q", c1.Fingerprint)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(filepath.Join(dir, keyFile))
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("key permissions %v %v", st.Mode(), err)
		}
	}

	// Restart, a year later, on another DHCP address: same certificate.
	clk.Advance(365 * 24 * time.Hour)
	s2 := &CertStore{Dir: dir, Clock: clk, Names: func() ([]string, []netip.Addr) {
		return []string{"cassa.local"}, []netip.Addr{netip.MustParseAddr("10.0.0.7")}
	}}
	c2, err := s2.Load(context.Background())
	if err != nil || c2.Fingerprint != c1.Fingerprint {
		t.Fatalf("fingerprint changed on restart: %v %v", err, c2.Fingerprint)
	}

	// Close to expiry: renewed, new fingerprint, new validity.
	clk.Set(leaf.NotAfter.Add(-RenewBefore + time.Hour))
	c3, err := s.Load(context.Background())
	if err != nil || c3.Fingerprint == c1.Fingerprint || !c3.Leaf.NotAfter.After(leaf.NotAfter) {
		t.Fatalf("not renewed near expiry: %v", err)
	}
	// Corrupted files are replaced.
	if err := os.WriteFile(filepath.Join(dir, certFile), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	c4, err := s.Load(context.Background())
	if err != nil || c4.Fingerprint == c3.Fingerprint {
		t.Fatalf("corrupted certificate not replaced: %v", err)
	}
	// Explicit rotation.
	c5, err := s.Rotate(context.Background())
	if err != nil || c5.Fingerprint == c4.Fingerprint {
		t.Fatalf("rotate: %v", err)
	}
	if tc, err := s.LoadOrCreate(context.Background()); err != nil || desktop.Fingerprint(tc.Certificate[0]) != c5.Fingerprint {
		t.Fatalf("LoadOrCreate: %v", err)
	}
}

// TestPeerFilter_Scenarios: the listener filter (desktop.IsPrivateAddr) and
// the HTTP middleware (middleware.IsLocalPeer) agree on every case.
func TestPeerFilter_Scenarios(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.10.0.1", true},
		{"::1", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.254", true},
		{"172.15.255.255", false},
		{"172.32.0.1", false},
		{"192.168.0.1", true},
		{"192.169.0.1", false},
		{"169.254.10.10", true},
		{"100.64.0.1", false},    // carrier-grade NAT
		{"100.127.255.1", false}, // carrier-grade NAT
		{"8.8.8.8", false},
		{"192.0.2.1", false}, // TEST-NET (httptest default peer)
		{"0.0.0.0", false},
		{"255.255.255.255", false},
		{"224.0.0.251", false}, // mDNS multicast
		{"fc00::1", true},
		{"fd12:3456:789a::1", true},
		{"fe80::1", true},
		{"fe80::1%en0", true},
		{"febf::1", true},
		{"fec0::1", false}, // deprecated site-local
		{"2001:db8::1", false},
		{"2a00:1450:4002::1", false},
		{"::", false},
		{"ff02::fb", false},
		{"::ffff:192.168.1.5", true},
		{"::ffff:8.8.8.8", false},
		{"64:ff9b::808:808", false}, // NAT64 of a public address
	}
	for _, tc := range tests {
		a := netip.MustParseAddr(tc.addr)
		if got := desktop.IsPrivateAddr(a); got != tc.want {
			t.Errorf("listener filter %s = %v, want %v", tc.addr, got, tc.want)
		}
		if got := middleware.IsLocalPeer(a); got != tc.want {
			t.Errorf("middleware filter %s = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func TestLocalPeersOnly_Middleware(t *testing.T) {
	h := middleware.LocalPeersOnly()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	for remote, want := range map[string]int{
		"192.168.1.30:51000": 204, "[fd00::5]:443": 204, "127.0.0.1:1": 204,
		"203.0.113.9:4000": 403, "[2001:db8::1]:443": 403, "garbage": 403,
	} {
		r := httptest.NewRequest(http.MethodGet, "/admin/", nil)
		r.RemoteAddr = remote
		r.Header.Set("X-Forwarded-For", "192.168.1.1") // never trusted
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("%s: %d, want %d", remote, w.Code, want)
		}
	}
}

// fakeConn reports a chosen peer address.
type fakeConn struct {
	net.Conn
	remote net.Addr
	closed chan struct{}
	once   sync.Once
}

func (c *fakeConn) RemoteAddr() net.Addr { return c.remote }
func (c *fakeConn) Close() error         { c.once.Do(func() { close(c.closed) }); return nil }

type scriptedListener struct {
	conns chan net.Conn
}

func (l *scriptedListener) Accept() (net.Conn, error) {
	c, ok := <-l.conns
	if !ok {
		return nil, net.ErrClosed
	}
	return c, nil
}
func (l *scriptedListener) Close() error   { return nil }
func (l *scriptedListener) Addr() net.Addr { return &net.TCPAddr{} }

func TestPrivateListener_RejectsPublicPeersBeforeTLS(t *testing.T) {
	sl := &scriptedListener{conns: make(chan net.Conn, 3)}
	var rejected []string
	pl := &desktop.PrivateListener{Listener: sl, Rejected: func(a net.Addr) { rejected = append(rejected, a.String()) }}
	public := &fakeConn{remote: &net.TCPAddr{IP: net.ParseIP("203.0.113.5"), Port: 1}, closed: make(chan struct{})}
	public6 := &fakeConn{remote: &net.TCPAddr{IP: net.ParseIP("2001:db8::7"), Port: 1}, closed: make(chan struct{})}
	lan := &fakeConn{remote: &net.TCPAddr{IP: net.ParseIP("192.168.1.40"), Port: 1}, closed: make(chan struct{})}
	sl.conns <- public
	sl.conns <- public6
	sl.conns <- lan
	c, err := pl.Accept()
	if err != nil || c != lan {
		t.Fatalf("accepted %v %v", c, err)
	}
	for _, pc := range []*fakeConn{public, public6} {
		select {
		case <-pc.closed:
		default:
			t.Fatalf("public peer %v not closed", pc.remote)
		}
	}
	if len(rejected) != 2 {
		t.Fatalf("rejected %v", rejected)
	}
}

// TestMode_TLSListener: Enable serves the app over TLS on the LAN port; a
// client pinning the fingerprint connects, a client pinning another one is
// refused; Enable is idempotent and Disable closes the port.
func TestMode_TLSListener(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var servers []*http.Server
	m := NewMode(Deps{
		LANDeps: desktop.LANDeps{
			DataDir: t.TempDir(),
			Serve: func(ln net.Listener) func(context.Context) error {
				srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, ErrorLog: log.New(io.Discard, "", 0), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, "panel "+r.Proto)
				})}
				mu.Lock()
				servers = append(servers, srv)
				mu.Unlock()
				go func() { _ = srv.Serve(ln) }()
				return srv.Shutdown
			},
		},
		Clock: clock.NewFake(t0),
		Listen: func(network, addr string) (net.Listener, error) {
			return net.Listen(network, "127.0.0.1:"+strings.TrimPrefix(addr, ":"))
		},
		Addrs:    func() ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr("192.168.1.20")}, nil },
		Hostname: func() string { return "cassa.local" },
	})
	port := freePort(t)
	url, fp, err := m.Enable(ctx, port)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://192.168.1.20:"+itoa(port) || len(fp) != 95 {
		t.Fatalf("url %q fp %q", url, fp)
	}
	if u2, fp2, err := m.Enable(ctx, port); err != nil || u2 != url || fp2 != fp || len(servers) != 1 {
		t.Fatalf("Enable is not idempotent: %v %d", err, len(servers))
	}
	if _, _, err := m.Enable(ctx, 80); !errors.Is(err, ErrInvalidPort) {
		t.Fatalf("privileged port: %v", err)
	}
	target := "https://127.0.0.1:" + itoa(port) + "/admin/"

	pinned := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: desktop.PinnedClientConfig(fp), ForceAttemptHTTP2: true}}
	resp, err := pinned.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(string(body), "panel HTTP/") {
		t.Fatalf("pinned client: %d %q", resp.StatusCode, body)
	}
	other, _, _ := GenerateCert([]string{"x"}, nil, t0, time.Hour)
	block, _ := pemDecode(other)
	wrong := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: desktop.PinnedClientConfig(desktop.Fingerprint(block))}}
	if _, err := wrong.Get(target); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("wrong pin accepted: %v", err)
	}
	// A browser without the pin sees an untrusted certificate (the warning
	// the QR page explains), never plain HTTP.
	if _, err := (&http.Client{Timeout: 5 * time.Second}).Get(target); err == nil {
		t.Fatal("self-signed certificate trusted by default")
	}
	if c, err := tls.Dial("tcp", "127.0.0.1:"+itoa(port), &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS11}); err == nil { //nolint:gosec // G402: checks that old TLS is refused
		_ = c.Close()
		t.Fatal("TLS 1.1 accepted")
	}
	info := m.Info()
	if !info.Enabled || info.Fingerprint != fp || info.Hostname != "cassa.local" || len(info.Addresses) != 1 {
		t.Fatalf("info %+v", info)
	}

	if err := m.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Disable(ctx); err != nil {
		t.Fatalf("Disable is not idempotent: %v", err)
	}
	if _, err := net.DialTimeout("tcp", "127.0.0.1:"+itoa(port), time.Second); err == nil {
		t.Fatal("port still open after Disable")
	}
	// Re-enable: same certificate, same fingerprint.
	if _, fp3, err := m.Enable(ctx, port); err != nil || fp3 != fp {
		t.Fatalf("re-enable: %v %q", err, fp3)
	}
	_ = m.Disable(ctx)
}

func TestPanelURLAndNames(t *testing.T) {
	addrs := []netip.Addr{netip.MustParseAddr("fd00::1"), netip.MustParseAddr("169.254.1.1"), netip.MustParseAddr("10.0.0.5")}
	SortAddrs(addrs)
	if addrs[0].String() != "10.0.0.5" || addrs[2].String() != "fd00::1" {
		t.Fatalf("sort %v", addrs)
	}
	if got := PanelURL(addrs, "cassa.local", 8443); got != "https://10.0.0.5:8443" {
		t.Fatal(got)
	}
	if got := PanelURL(nil, "cassa.local", 8443); got != "https://cassa.local:8443" {
		t.Fatal(got)
	}
	if got := PanelURL(nil, "", 8443); got != "" {
		t.Fatal(got)
	}
	for in, want := range map[string]string{
		"Cassa-PC": "cassa-pc.local", "MacBook Pro di Anna.local": "macbook-pro-di-anna.local",
		"host.example.com": "host.local", "": "", "___": "",
	} {
		if got := MDNSName(in); got != want {
			t.Errorf("MDNSName(%q) = %q, want %q", in, got, want)
		}
	}
	names, ips := SystemNames()
	if len(names) < 2 || len(ips) < 2 {
		t.Fatalf("system names %v %v", names, ips)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	p := l.Addr().(*net.TCPAddr).Port
	if p < 1024 {
		t.Skip("ephemeral port below 1024")
	}
	return p
}
