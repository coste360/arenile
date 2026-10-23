package desktop

import (
	"net"
	"net/netip"
	"strings"
	"testing"
)

func TestIsPrivateAddr_Scenarios(t *testing.T) {
	tests := map[string]bool{
		"127.0.0.1":          true,
		"::1":                true,
		"10.1.2.3":           true,
		"172.16.0.1":         true,
		"172.32.0.1":         false,
		"192.168.1.20":       true,
		"169.254.1.1":        true,
		"fd12:3456::1":       true,
		"fe80::1":            true,
		"::ffff:192.168.1.5": true,
		"100.64.0.1":         false,
		"8.8.8.8":            false,
		"2001:4860::8888":    false,
	}
	for s, want := range tests {
		if got := IsPrivateAddr(netip.MustParseAddr(s)); got != want {
			t.Errorf("IsPrivateAddr(%s) = %v, want %v", s, got, want)
		}
	}
	if IsPrivateAddr(netip.Addr{}) {
		t.Error("zero address accepted")
	}
}

func TestPrivateListener_AcceptsLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	pl := &PrivateListener{Listener: ln}
	defer pl.Close()
	go func() {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err == nil {
			_ = c.Close()
		}
	}()
	c, err := pl.Accept()
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
}

func TestFingerprintAndLANAddress(t *testing.T) {
	fp := Fingerprint([]byte("cert"))
	if len(fp) != 95 || strings.ToUpper(fp) != fp || strings.Count(fp, ":") != 31 {
		t.Fatalf("fingerprint %q", fp)
	}
	a, ok := LANAddress([]netip.Addr{netip.MustParseAddr("fe80::1"), netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("192.168.1.20")})
	if !ok || a.String() != "192.168.1.20" {
		t.Fatalf("LANAddress = %v %v", a, ok)
	}
	if _, ok := LANAddress(nil); ok {
		t.Fatal("no address expected")
	}
}
