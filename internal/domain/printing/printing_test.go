package printing

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/printing/printertest"
)

func TestAllowedIP_Scenarios(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.50", true},
		{"10.0.0.7", true},
		{"172.16.4.4", true},
		{"172.31.255.1", true},
		{"100.64.1.1", true},
		{"127.0.0.1", true},
		{"169.254.10.1", true},
		{"fd12:3456::1", true},
		{"fe80::1", true},
		{"fe80::1%en0", true},
		{"::1", true},
		{"::ffff:192.168.1.5", true},
		{"8.8.8.8", false},
		{"172.32.0.1", false},
		{"100.128.0.1", false},
		{"0.0.0.0", false},
		{"::", false},
		{"224.0.0.1", false},
		{"2001:4860:4860::8888", false},
		{"::ffff:8.8.8.8", false},
		{"255.255.255.255", false},
	}
	for _, tt := range tests {
		if got := AllowedIP(netip.MustParseAddr(tt.ip)); got != tt.want {
			t.Errorf("AllowedIP(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestNormalizeHost_Scenarios(t *testing.T) {
	ok := map[string]string{
		" 192.168.1.50 ": "192.168.1.50", "[fe80::1]": "fe80::1", "Stampante-Bar.local": "stampante-bar.local",
		"printer": "printer",
	}
	for in, want := range ok {
		got, err := NormalizeHost(in)
		if err != nil || got != want {
			t.Errorf("NormalizeHost(%q) = %q, %v; want %q", in, got, err, want)
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
	}
	ctx := context.Background()
	for host, want := range map[string]error{
		"192.168.1.50": nil, "bar.local": nil,
		"8.8.8.8": ErrAddressNotAllowed, "mixed.lan": ErrAddressNotAllowed, "public.com": ErrAddressNotAllowed,
		"missing.lan": configerr.ErrValidation,
	} {
		err := CheckHost(ctx, r, host)
		if want == nil && err != nil || want != nil && !errors.Is(err, want) {
			t.Errorf("CheckHost(%s) = %v, want %v", host, err, want)
		}
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
	d := NetworkDriver{}
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
	st, err := NetworkDriver{StatusTimeout: 100 * time.Millisecond}.Check(ctx, Target{Host: silent.Host(), Port: silent.Port()})
	if err != nil || !st.Reachable || st.PaperLow || st.CoverOpen {
		t.Fatalf("silent printer: %+v %v", st, err)
	}
	if len(silent.Jobs()) != 0 {
		t.Fatal("a check must not print")
	}
	// 0x1e: fixed bits 0x12 + 0x0c (cover open bit 2 on n=2, paper near end on n=4).
	answering := printertest.Start(t, printertest.WithStatus(0x1e))
	st, err = NetworkDriver{}.Check(ctx, Target{Host: answering.Host(), Port: answering.Port()})
	if err != nil || !st.Reachable || !st.PaperLow || !st.CoverOpen {
		t.Fatalf("answering printer: %+v %v", st, err)
	}
}

func TestNetworkDriver_Errors(t *testing.T) {
	ctx := context.Background()
	fp := printertest.Start(t)
	port := fp.Port()
	fp.Close()
	err := NetworkDriver{}.Send(ctx, Target{Host: "127.0.0.1", Port: port}, []byte("x"))
	if err == nil || errors.Is(err, ErrAddressNotAllowed) {
		t.Fatalf("closed port: %v", err)
	}
	if _, err := (NetworkDriver{}).Check(ctx, Target{Host: "127.0.0.1", Port: port}); err == nil {
		t.Fatal("closed port must be unreachable")
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
