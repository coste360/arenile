package lan

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop"
)

// Certificate policy.
const (
	// CertValidity is the lifetime of the self-signed certificate: 825 days
	// is the maximum Apple platforms accept for TLS server certificates.
	CertValidity = 825 * 24 * time.Hour
	// RenewBefore: the certificate is regenerated this long before expiry.
	RenewBefore = 30 * 24 * time.Hour
	certFile    = "cert.pem"
	keyFile     = "key.pem"
)

// Cert is the LAN certificate with its fingerprint (desktop.Fingerprint
// of the DER, the form shown in the QR and in the panel).
type Cert struct {
	TLS         tls.Certificate
	Leaf        *x509.Certificate
	Fingerprint string
}

// CertStore keeps the LAN certificate in Dir (<data dir>/lan). It
// implements desktop.CertStore.
type CertStore struct {
	Dir   string
	Clock clock.Clock
	// Names returns the DNS names and IP addresses to put in a new
	// certificate (default: SystemNames).
	Names func() ([]string, []netip.Addr)

	mu sync.Mutex
}

var _ desktop.CertStore = (*CertStore)(nil)

// LoadOrCreate implements desktop.CertStore.
func (s *CertStore) LoadOrCreate(ctx context.Context) (tls.Certificate, error) {
	c, err := s.Load(ctx)
	if err != nil {
		return tls.Certificate{}, err
	}
	return c.TLS, nil
}

// Load returns the stored certificate, creating it when it is missing,
// unreadable, not matching its key or close to expiry.
func (s *CertStore) Load(_ context.Context) (*Cert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if c, err := s.read(); err == nil && now.Add(RenewBefore).Before(c.Leaf.NotAfter) && !now.Before(c.Leaf.NotBefore.Add(-time.Hour)) {
		return c, nil
	}
	return s.create(now)
}

// Rotate replaces the certificate unconditionally (paired devices must be
// paired again: their pinned fingerprint changes).
func (s *CertStore) Rotate(_ context.Context) (*Cert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.create(s.now())
}

func (s *CertStore) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *CertStore) read() (*Cert, error) {
	certPEM, err := os.ReadFile(filepath.Join(s.Dir, certFile))
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(filepath.Join(s.Dir, keyFile))
	if err != nil {
		return nil, err
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	pair.Leaf = leaf
	return &Cert{TLS: pair, Leaf: leaf, Fingerprint: desktop.Fingerprint(leaf.Raw)}, nil
}

func (s *CertStore) create(now time.Time) (*Cert, error) {
	names, ips := SystemNames()
	if s.Names != nil {
		names, ips = s.Names()
	}
	certPEM, keyPEM, err := GenerateCert(names, ips, now, CertValidity)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("lan: certificate dir: %w", err)
	}
	// Key first: a crash between the two writes leaves a mismatching pair,
	// which read() rejects and create() replaces.
	if err := writeAtomic(filepath.Join(s.Dir, keyFile), keyPEM, 0o600); err != nil {
		return nil, err
	}
	if err := writeAtomic(filepath.Join(s.Dir, certFile), certPEM, 0o644); err != nil {
		return nil, err
	}
	return s.read()
}

func writeAtomic(path string, b []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return fmt.Errorf("lan: write %s: %w", filepath.Base(path), err)
	}
	return os.Rename(tmp, path)
}

// GenerateCert creates a self-signed ECDSA P-256 server certificate valid
// from now (minus one hour of clock skew) for validity, with the given
// SANs. It returns the certificate and the PKCS#8 key in PEM.
func GenerateCert(dnsNames []string, ips []netip.Addr, now time.Time, validity time.Duration) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("lan: generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, nil, err
	}
	cn := "Arenile"
	if len(dnsNames) > 0 {
		cn = "Arenile " + dnsNames[0]
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn, Organization: []string{"Arenile desktop (self-signed)"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
	}
	for _, a := range ips {
		tmpl.IPAddresses = append(tmpl.IPAddresses, net.IP(a.Unmap().AsSlice()))
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("lan: create certificate: %w", err)
	}
	pk, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk}), nil
}

// SystemNames returns the SANs of a new certificate on this machine:
// <hostname>.local, arenile.local, localhost and the loopback plus private
// addresses of the interfaces that are up.
func SystemNames() ([]string, []netip.Addr) {
	names := []string{}
	if h := LocalHostname(); h != "" {
		names = append(names, h)
	}
	names = append(names, "arenile.local", "localhost")
	ips := []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.IPv6Loopback()}
	addrs, _ := InterfaceAddrs()
	for _, a := range addrs {
		if a.Zone() == "" { // link-local IPv6 needs a zone: not usable in a URL or SAN
			ips = append(ips, a)
		}
	}
	return names, ips
}

// LocalHostname returns "<hostname>.local" (lowercase, DNS-safe), or "".
func LocalHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return MDNSName(h)
}

// MDNSName turns an OS hostname into its .local name (pure): the first
// label, lowercase, characters outside [a-z0-9-] replaced by '-'.
func MDNSName(host string) string {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	host = strings.TrimSuffix(host, ".local")
	if i := strings.IndexByte(host, '.'); i >= 0 {
		host = host[:i]
	}
	var b strings.Builder
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" || len(s) > 63 {
		return ""
	}
	return s + ".local"
}
