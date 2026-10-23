package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/http/api"
)

// Pairing errors (messages in Italian, shown to the operator as they are).
var (
	ErrInvalidInput   = errors.New("inserisci il testo del QR oppure indirizzo e codice di abbinamento")
	ErrInsecureURL    = errors.New("l'indirizzo dell'istanza deve usare https")
	ErrNotArenile     = errors.New("a questo indirizzo non risponde un'istanza Arenile")
	ErrClientTooOld   = errors.New("questa versione dell'app è troppo vecchia per l'istanza: aggiornala dal portale Coste360")
	ErrCodeInvalid    = errors.New("codice di abbinamento non valido, scaduto o già usato: generane uno nuovo dal pannello")
	ErrPairingLimited = errors.New("troppi tentativi di abbinamento: riprova tra un minuto")
)

// PairInput is what the operator provides: the QR text, or the instance
// address and the code shown by the panel (Configurazione -> Dispositivi).
type PairInput struct {
	QR          string
	URL         string
	Code        string
	Fingerprint string
	DeviceName  string
}

// Target is a parsed pairing request.
type Target struct {
	URL               string
	Code              string
	Fingerprint       string
	EstablishmentName string
}

// ParseInput validates the operator input (pure).
func ParseInput(in PairInput) (Target, error) {
	if qr := strings.TrimSpace(in.QR); qr != "" {
		p, err := device.DecodeQR(qr)
		if err != nil {
			return Target{}, ErrInvalidInput
		}
		u, err := NormalizeInstanceURL(p.URL)
		if err != nil {
			return Target{}, err
		}
		code, ok := device.NormalizePairingCode(p.Code)
		if !ok {
			return Target{}, ErrInvalidInput
		}
		return Target{URL: u, Code: code, Fingerprint: strings.ToUpper(p.Fingerprint), EstablishmentName: p.EstablishmentName}, nil
	}
	if strings.TrimSpace(in.URL) == "" || strings.TrimSpace(in.Code) == "" {
		return Target{}, ErrInvalidInput
	}
	u, err := NormalizeInstanceURL(in.URL)
	if err != nil {
		return Target{}, err
	}
	code, ok := device.NormalizePairingCode(in.Code)
	if !ok {
		return Target{}, ErrInvalidInput
	}
	return Target{URL: u, Code: code, Fingerprint: strings.ToUpper(strings.TrimSpace(in.Fingerprint))}, nil
}

// NormalizeInstanceURL returns scheme://host[:port] of an instance
// address (pure). A bare host gets https. Plain http is accepted only on
// loopback (development): the device token must never cross a network in
// clear, including a VPN.
func NormalizeInstanceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return "", ErrInvalidInput
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return "", ErrInsecureURL
		}
	default:
		return "", ErrInvalidInput
	}
	return u.Scheme + "://" + u.Host, nil
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	a, err := netip.ParseAddr(h)
	return err == nil && a.Unmap().IsLoopback()
}

// HTTPClient returns the client for an instance: it pins fingerprint when
// set (self-signed certificate), else uses the system trust store. No
// timeout: callers bound each request with a context (long-poll).
func HTTPClient(fingerprint string) *http.Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment, ForceAttemptHTTP2: true, MaxIdleConns: 10,
		IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ExpectContinueTimeout: time.Second,
	}
	if fingerprint != "" {
		tr.TLSClientConfig = desktop.PinnedClientConfig(fingerprint)
	}
	tr.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	return &http.Client{Transport: tr}
}

// Platform maps GOOS to api.DevicePlatform (pure).
func Platform(goos string) string {
	switch goos {
	case "darwin":
		return "macos"
	case "windows", "linux":
		return goos
	default:
		return "other"
	}
}

// Pairer exchanges a pairing code for a device token.
type Pairer struct {
	// Version of the app (semver, "dev" skips the minimum version check).
	Version string
	GOOS    string
	Clock   clock.Clock
	// Client overrides HTTPClient (tests).
	Client func(fingerprint string) *http.Client
}

// Paired is the outcome of Pair: Token goes to the TokenStore, Config to
// remote.json.
type Paired struct {
	Token  string
	Config Config
	Meta   api.Meta
}

// Pair checks the instance (/meta: product, minimum desktop version) and
// pairs this PC as a desktop device.
func (p Pairer) Pair(ctx context.Context, in PairInput) (*Paired, error) {
	t, err := ParseInput(in)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.DeviceName)
	if name == "" {
		name = "PC cassa"
	}
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	hc := HTTPClient(t.Fingerprint)
	if p.Client != nil {
		hc = p.Client(t.Fingerprint)
	}
	meta, err := p.meta(ctx, hc, t.URL)
	if err != nil {
		return nil, err
	}
	goos := p.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	req := api.DevicePairRequest{Code: t.Code, Name: name, Kind: api.DeviceKind("desktop"), Platform: api.DevicePlatform(Platform(goos))}
	if p.Version != "" {
		v := p.Version
		req.AppVersion = &v
	}
	var res api.DevicePairResult
	status, problemType, err := doJSON(ctx, hc, http.MethodPost, t.URL+"/api/device/v1/pair", "", req, &res)
	switch {
	case err != nil:
		return nil, fmt.Errorf("remote: pairing request: %w", err)
	case status == http.StatusTooManyRequests:
		return nil, ErrPairingLimited
	case status == http.StatusBadRequest && problemType == "pairing_code_invalid":
		return nil, ErrCodeInvalid
	case status != http.StatusCreated:
		return nil, fmt.Errorf("remote: pairing refused (%d %s)", status, problemType)
	}
	if !strings.HasPrefix(res.Token, "ar_dev_") {
		return nil, errors.New("remote: the instance returned no device token")
	}
	now := time.Now().UTC()
	if p.Clock != nil {
		now = p.Clock.Now().UTC()
	}
	return &Paired{Token: res.Token, Meta: meta, Config: Config{
		Mode: desktop.ModeRemote, InstanceURL: t.URL, Fingerprint: t.Fingerprint,
		DeviceID: res.Device.Id, DeviceName: res.Device.Name, EstablishmentID: res.EstablishmentId,
		EstablishmentName: t.EstablishmentName, PairedAt: now, FiscalBridge: meta.Capabilities.FiscalBridge,
	}}, nil
}

func (p Pairer) meta(ctx context.Context, hc *http.Client, base string) (api.Meta, error) {
	var m api.Meta
	status, _, err := doJSON(ctx, hc, http.MethodGet, base+"/api/admin/v1/meta", "", nil, &m)
	if err != nil {
		return m, fmt.Errorf("remote: instance not reachable: %w", err)
	}
	if status != http.StatusOK || string(m.Product) != "arenile" {
		return m, ErrNotArenile
	}
	if !VersionAtLeast(p.Version, m.MinClientVersions.Desktop) {
		return m, ErrClientTooOld
	}
	return m, nil
}

// VersionAtLeast reports whether version >= minimum (pure, semver
// major.minor.patch, "v" prefix and pre-release suffix ignored). A
// development build ("dev", empty) or an empty minimum always passes.
func VersionAtLeast(version, minimum string) bool {
	v, ok1 := parseSemver(version)
	m, ok2 := parseSemver(minimum)
	if !ok1 || !ok2 {
		return true
	}
	for i := range 3 {
		if v[i] != m[i] {
			return v[i] > m[i]
		}
	}
	return true
}

func parseSemver(s string) ([3]int, bool) {
	var out [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if s == "" || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// doJSON performs a JSON request and returns the status and, for problem
// responses, the problem type.
func doJSON(ctx context.Context, hc *http.Client, method, u, token string, in, out any) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, "", err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, "", err
	}
	if resp.StatusCode/100 != 2 {
		var p struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw, &p)
		return resp.StatusCode, p.Type, nil
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, "", fmt.Errorf("remote: unexpected answer: %w", err)
		}
	}
	return resp.StatusCode, "", nil
}
