package event

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEgressPolicy_Check(t *testing.T) {
	strict, lan := EgressPolicy{}, EgressPolicy{AllowPrivate: true}
	tests := []struct {
		addr          string
		strictAllowed bool
		lanAllowed    bool
	}{
		// Public.
		{"93.184.216.34", true, true},
		{"2606:4700:4700::1111", true, true},
		{"::ffff:93.184.216.34", true, true},
		// Loopback and private: only for LAN installations.
		{"127.0.0.1", false, true},
		{"127.10.0.1", false, true},
		{"10.1.2.3", false, true},
		{"172.16.0.1", false, true},
		{"172.31.255.255", false, true},
		{"192.168.1.10", false, true},
		{"100.64.0.1", false, true},
		{"198.18.0.1", false, true},
		{"::1", false, true},
		{"fd12:3456:789a::1", false, true},
		{"fec0::1", false, true},
		// IPv4 hidden in IPv6 is checked as IPv4.
		{"::ffff:127.0.0.1", false, true},
		{"::ffff:10.0.0.1", false, true},
		{"64:ff9b::7f00:1", false, true},  // NAT64 of 127.0.0.1
		{"64:ff9b::a00:1", false, true},   // NAT64 of 10.0.0.1
		{"2002:7f00:1::1", false, true},   // 6to4 of 127.0.0.1
		{"2002:5db8:d822::1", true, true}, // 6to4 of 93.184.216.34
		// Always blocked: metadata, link-local, reserved, multicast...
		{"169.254.169.254", false, false},
		{"169.254.1.1", false, false},
		{"fe80::1", false, false},
		{"fe80::1%eth0", false, false},
		{"0.0.0.0", false, false},
		{"0.1.2.3", false, false},
		{"::", false, false},
		{"224.0.0.1", false, false},
		{"ff02::1", false, false},
		{"255.255.255.255", false, false},
		{"240.0.0.1", false, false},
		{"192.0.2.1", false, false},
		{"2001:db8::1", false, false},
		{"2001:0:4136:e378::1", false, false}, // Teredo
		{"::ffff:169.254.169.254", false, false},
		{"64:ff9b::a9fe:a9fe", false, false}, // NAT64 of the metadata address
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			a := netip.MustParseAddr(tt.addr)
			if err := strict.Check(a); (err == nil) != tt.strictAllowed {
				t.Fatalf("strict: allowed=%v want %v (%v)", err == nil, tt.strictAllowed, err)
			} else if err != nil && !errors.Is(err, ErrBlockedDestination) {
				t.Fatalf("strict error kind: %v", err)
			}
			if err := lan.Check(a); (err == nil) != tt.lanAllowed {
				t.Fatalf("lan: allowed=%v want %v (%v)", err == nil, tt.lanAllowed, err)
			}
		})
	}
	if err := strict.Check(netip.Addr{}); err == nil {
		t.Fatal("the zero address must be refused")
	}
}

type fakeResolver struct {
	answers map[string][]netip.Addr
	calls   atomic.Int32
}

func (r *fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	r.calls.Add(1)
	a, ok := r.answers[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return a, nil
}

func addrs(ss ...string) []netip.Addr {
	out := make([]netip.Addr, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParseAddr(s)
	}
	return out
}

// TestEgressDialer_PinsResolvedAddress: the name is resolved once, every
// answer is checked and the connection goes to the checked IP literal, so a
// second (rebinding) DNS answer can never be used.
func TestEgressDialer_PinsResolvedAddress(t *testing.T) {
	r := &fakeResolver{answers: map[string][]netip.Addr{
		"hooks.example.com": addrs("93.184.216.34"),
		"rebind.example":    addrs("93.184.216.34", "127.0.0.1"),
		"internal.example":  addrs("10.0.0.7"),
		"meta.example":      addrs("169.254.169.254"),
		"v6.example":        addrs("::ffff:192.168.0.1"),
	}}
	var dialed []string
	fakeDial := func(_ context.Context, _, address string) (net.Conn, error) {
		dialed = append(dialed, address)
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}
	tests := []struct {
		name    string
		policy  EgressPolicy
		address string
		dialed  string // "" = refused
	}{
		{"public name", EgressPolicy{}, "hooks.example.com:443", "93.184.216.34:443"},
		{"mixed answers refused", EgressPolicy{}, "rebind.example:443", ""},
		{"private name refused", EgressPolicy{}, "internal.example:443", ""},
		{"private name on LAN", EgressPolicy{AllowPrivate: true}, "internal.example:8443", "10.0.0.7:8443"},
		{"metadata refused on LAN", EgressPolicy{AllowPrivate: true}, "meta.example:80", ""},
		{"mapped v6 private refused", EgressPolicy{}, "v6.example:443", ""},
		{"loopback literal refused", EgressPolicy{}, "127.0.0.1:8080", ""},
		{"v6 loopback literal refused", EgressPolicy{}, "[::1]:8080", ""},
		{"public literal", EgressPolicy{}, "93.184.216.34:443", "93.184.216.34:443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialed = nil
			before := r.calls.Load()
			conn, err := tt.policy.dialer(r, fakeDial)(context.Background(), "tcp", tt.address)
			if tt.dialed == "" {
				if err == nil || !errors.Is(err, ErrBlockedDestination) || len(dialed) != 0 {
					t.Fatalf("want refusal before dialing, got err=%v dialed=%v", err, dialed)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			if len(dialed) != 1 || dialed[0] != tt.dialed {
				t.Fatalf("dialed %v, want [%s]", dialed, tt.dialed)
			}
			if host, _, _ := net.SplitHostPort(tt.address); net.ParseIP(host) == nil && r.calls.Load()-before != 1 {
				t.Fatalf("resolver called %d times, want exactly 1", r.calls.Load()-before)
			}
		})
	}
}

// TestEgressClient_BlocksLoopbackAndRedirects: the default delivery client
// refuses a loopback receiver unless the LAN flag is set, and never follows
// a redirect (which could point at a blocked address).
func TestEgressClient_BlocksLoopbackAndRedirects(t *testing.T) {
	var internalHits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer internal.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/admin", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	post := func(c *http.Client, url string) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader([]byte("{}")))
		return c.Do(req)
	}

	strict := NewEgressClient(EgressPolicy{}, nil, 2*time.Second)
	if resp, err := post(strict, internal.URL); err == nil {
		_ = resp.Body.Close()
		t.Fatal("loopback receiver must be refused by default")
	} else if !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("error kind: %v", err)
	}
	// "localhost" resolves to loopback: refused as well.
	if resp, err := post(strict, strings.Replace(internal.URL, "127.0.0.1", "localhost", 1)); err == nil {
		_ = resp.Body.Close()
		t.Fatal("localhost must be refused by default")
	}

	lan := NewEgressClient(EgressPolicy{AllowPrivate: true}, nil, 2*time.Second)
	resp, err := post(lan, internal.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || internalHits.Load() != 1 {
		t.Fatalf("LAN delivery: %d hits=%d", resp.StatusCode, internalHits.Load())
	}
	resp, err = post(lan, redirector.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect || internalHits.Load() != 1 {
		t.Fatalf("redirect followed: status=%d hits=%d", resp.StatusCode, internalHits.Load())
	}
}

func TestSignAll_VerifiesWithEitherSecret(t *testing.T) {
	at := time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC)
	body := []byte(`{"id":"evt_1"}`)
	h := SignAll([]string{"whsec_new", "", "whsec_old"}, body, at)
	if strings.Count(h, "v1=") != 2 || !strings.HasPrefix(h, "t=1814432400,v1=") {
		t.Fatalf("header %q", h)
	}
	for _, s := range []string{"whsec_new", "whsec_old"} {
		if err := VerifySignature(s, h, body, at.Add(time.Minute), 5*time.Minute); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	if err := VerifySignature("whsec_other", h, body, at, 5*time.Minute); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("foreign secret: %v", err)
	}
	if err := VerifySignature("whsec_new", h, []byte(`{"id":"evt_2"}`), at, 5*time.Minute); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered body: %v", err)
	}
	if err := VerifySignature("whsec_new", h, body, at.Add(10*time.Minute), 5*time.Minute); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("replay outside tolerance: %v", err)
	}
	if Sign("whsec_new", body, at) != SignAll([]string{"whsec_new"}, body, at) {
		t.Fatal("Sign must equal SignAll with one secret")
	}
}
