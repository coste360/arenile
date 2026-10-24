package printing

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/printing/printertest"
)

func TestAllowedIP_Scenarios(t *testing.T) {
	tests := []struct {
		ip       string
		want     bool // production policy
		loopback bool // with AllowLoopback
	}{
		{"192.168.1.50", true, true},
		{"10.0.0.7", true, true},
		{"172.16.4.4", true, true},
		{"172.31.255.1", true, true},
		{"100.64.1.1", true, true},
		{"169.254.10.1", true, true},
		{"fd12:3456::1", true, true},
		{"fe80::1", true, true},
		{"fe80::1%en0", true, true},
		{"::ffff:192.168.1.5", true, true},
		// Loopback only behind the development/test flag.
		{"127.0.0.1", false, true},
		{"127.0.0.53", false, true},
		{"::1", false, true},
		{"::ffff:127.0.0.1", false, true},
		// Cloud metadata endpoints are link-local but never a printer.
		{"169.254.169.254", false, false},
		{"fd00:ec2::254", false, false},
		{"8.8.8.8", false, false},
		{"172.32.0.1", false, false},
		{"100.128.0.1", false, false},
		{"0.0.0.0", false, false},
		{"::", false, false},
		{"224.0.0.1", false, false},
		{"2001:4860:4860::8888", false, false},
		{"::ffff:8.8.8.8", false, false},
		{"255.255.255.255", false, false},
	}
	for _, tt := range tests {
		ip := netip.MustParseAddr(tt.ip)
		if got := AllowedIP(ip); got != tt.want {
			t.Errorf("AllowedIP(%s) = %v, want %v", tt.ip, got, tt.want)
		}
		if got := (AddressPolicy{AllowLoopback: true}).Allowed(ip); got != tt.loopback {
			t.Errorf("Allowed(%s) with loopback = %v, want %v", tt.ip, got, tt.loopback)
		}
	}
}

func TestNormalizeHost_Scenarios(t *testing.T) {
	ok := []struct{ in, want string }{
		{" 192.168.1.50 ", "192.168.1.50"},
		{"[fe80::1]", "fe80::1"},
		{"Stampante-Bar.local", "stampante-bar.local"},
		{"printer", "printer"},
	}
	for _, tt := range ok {
		got, err := NormalizeHost(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("NormalizeHost(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"", "bad host", "-x.local", "a..b", "http://192.168.1.5", "x_y.lan"} {
		if _, err := NormalizeHost(in); !errors.Is(err, configerr.ErrValidation) {
			t.Errorf("NormalizeHost(%q): err = %v, want validation", in, err)
		}
	}
}

type fakeResolver map[string][]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	a, ok := f[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return a, nil
}

func TestCheckHost_Scenarios(t *testing.T) {
	r := fakeResolver{
		"bar.local":  {netip.MustParseAddr("192.168.1.60")},
		"mixed.lan":  {netip.MustParseAddr("192.168.1.61"), netip.MustParseAddr("93.184.216.34")},
		"public.com": {netip.MustParseAddr("93.184.216.34")},
		"localhost":  {netip.MustParseAddr("127.0.0.1")},
		"metadata":   {netip.MustParseAddr("169.254.169.254")},
	}
	ctx := context.Background()
	for host, want := range map[string]error{
		"192.168.1.50": nil, "bar.local": nil,
		"8.8.8.8": ErrAddressNotAllowed, "mixed.lan": ErrAddressNotAllowed, "public.com": ErrAddressNotAllowed,
		"127.0.0.1": ErrAddressNotAllowed, "localhost": ErrAddressNotAllowed, "169.254.169.254": ErrAddressNotAllowed,
		"metadata":    ErrAddressNotAllowed,
		"missing.lan": configerr.ErrValidation,
	} {
		err := CheckHost(ctx, r, host, AddressPolicy{})
		if want == nil && err != nil || want != nil && !errors.Is(err, want) {
			t.Errorf("CheckHost(%s) = %v, want %v", host, err, want)
		}
	}
	dev := AddressPolicy{AllowLoopback: true}
	if err := CheckHost(ctx, r, "127.0.0.1", dev); err != nil {
		t.Errorf("loopback with the flag: %v", err)
	}
	if err := CheckHost(ctx, r, "localhost", dev); err != nil {
		t.Errorf("localhost with the flag: %v", err)
	}
	if err := CheckHost(ctx, r, "169.254.169.254", dev); !errors.Is(err, ErrAddressNotAllowed) {
		t.Errorf("metadata with the flag: %v", err)
	}
}

func TestBackoff(t *testing.T) {
	want := []time.Duration{5 * time.Second, 15 * time.Second, time.Minute, 5 * time.Minute, 5 * time.Minute}
	for i, w := range want {
		if got := Backoff(i + 1); got != w {
			t.Errorf("Backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
}

func TestNetworkDriver_Send(t *testing.T) {
	fp := printertest.Start(t)
	payload := append([]byte{0x1b, '@'}, bytes.Repeat([]byte("Lido Azzurro\n"), 2000)...)
	d := NetworkDriver{AllowLoopback: true}
	if err := d.Send(context.Background(), Target{Host: fp.Host(), Port: fp.Port()}, payload); err != nil {
		t.Fatal(err)
	}
	jobs := fp.WaitJobs(1, 5*time.Second)
	if len(jobs) != 1 || !bytes.Equal(jobs[0], payload) {
		t.Fatalf("printer got %d jobs (len %d), want the payload of %d bytes", len(jobs), len(jobs[0]), len(payload))
	}
}

func TestNetworkDriver_Check(t *testing.T) {
	ctx := context.Background()
	silent := printertest.Start(t)
	st, err := NetworkDriver{StatusTimeout: 100 * time.Millisecond, AllowLoopback: true}.Check(ctx, Target{Host: silent.Host(), Port: silent.Port()})
	if err != nil || !st.Reachable || st.PaperLow || st.CoverOpen {
		t.Fatalf("silent printer: %+v %v", st, err)
	}
	if len(silent.Jobs()) != 0 {
		t.Fatal("a check must not print")
	}
	// 0x1e: fixed bits 0x12 + 0x0c (cover open bit 2 on n=2, paper near end on n=4).
	answering := printertest.Start(t, printertest.WithStatus(0x1e))
	st, err = NetworkDriver{AllowLoopback: true}.Check(ctx, Target{Host: answering.Host(), Port: answering.Port()})
	if err != nil || !st.Reachable || !st.PaperLow || !st.CoverOpen {
		t.Fatalf("answering printer: %+v %v", st, err)
	}
}

func TestNetworkDriver_Errors(t *testing.T) {
	ctx := context.Background()
	fp := printertest.Start(t)
	port := fp.Port()
	fp.Close()
	dev := NetworkDriver{AllowLoopback: true}
	err := dev.Send(ctx, Target{Host: "127.0.0.1", Port: port}, []byte("x"))
	if err == nil || errors.Is(err, ErrAddressNotAllowed) {
		t.Fatalf("closed port: %v", err)
	}
	if _, err := dev.Check(ctx, Target{Host: "127.0.0.1", Port: port}); err == nil {
		t.Fatal("closed port must be unreachable")
	}
	// Production policy: loopback refused at dial time.
	if err := (NetworkDriver{}).Send(ctx, Target{Host: "127.0.0.1", Port: port}, []byte("x")); !errors.Is(err, ErrAddressNotAllowed) {
		t.Fatalf("loopback without the flag: %v", err)
	}
	// Public address: refused before any packet leaves (Dialer.Control).
	start := time.Now()
	err = NetworkDriver{DialTimeout: 2 * time.Second}.Send(ctx, Target{Host: "8.8.8.8", Port: 9100}, []byte("x"))
	if !errors.Is(err, ErrAddressNotAllowed) {
		t.Fatalf("public address: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("public address must be refused immediately")
	}
}

func TestTechnical_Bounded(t *testing.T) {
	long := errors.New(string(bytes.Repeat([]byte("x"), 1000)))
	if got := technical(long); len(got) != maxErrorLen {
		t.Fatalf("len %d", len(got))
	}
	if got := technical(ErrAddressNotAllowed); got == ErrAddressNotAllowed.Error() {
		t.Fatal("address error must be rephrased")
	}
}

func TestStripDrawerKick(t *testing.T) {
	body := []byte("RICEVUTA\n")
	for _, pin := range []int{2, 5} {
		kick := []byte{0x1b, 'p', byte(pin / 5), 25, 250}
		payload := append(append(append([]byte{}, body...), kick...), body...)
		got := stripDrawerKick(payload)
		if bytes.Contains(got, kick) || !bytes.Equal(got, append(append([]byte{}, body...), body...)) {
			t.Fatalf("pin %d: %q", pin, got)
		}
	}
	if got := stripDrawerKick(body); !bytes.Equal(got, body) {
		t.Fatal("payload without kick changed")
	}
}

func TestCheckLogoRedirect(t *testing.T) {
	req := func(u string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if err := checkLogoRedirect(req("https://cdn.example/logo.png"), []*http.Request{req("https://lido.example/logo")}); err != nil {
		t.Fatalf("https redirect: %v", err)
	}
	if err := checkLogoRedirect(req("http://cdn.example/logo.png"), nil); !errors.Is(err, errLogoRedirect) {
		t.Fatalf("http downgrade: %v", err)
	}
	if err := checkLogoRedirect(req("file:///etc/passwd"), nil); !errors.Is(err, errLogoRedirect) {
		t.Fatalf("other scheme: %v", err)
	}
	via := []*http.Request{req("https://a"), req("https://b"), req("https://c")}
	if err := checkLogoRedirect(req("https://d"), via); !errors.Is(err, errLogoRedirect) {
		t.Fatalf("too many hops: %v", err)
	}
}
