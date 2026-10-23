package remote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/desktop"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/http/api"
)

func TestParseInput_Scenarios(t *testing.T) {
	qr, _ := device.EncodeQR(device.QRPayload{URL: "https://192.168.1.20:8443", Code: "abcd-efgh", Fingerprint: "ab:cd", EstablishmentName: "Lido Sole"})
	tests := map[string]struct {
		in      PairInput
		want    Target
		wantErr error
	}{
		"qr":            {PairInput{QR: qr}, Target{URL: "https://192.168.1.20:8443", Code: "ABCDEFGH", Fingerprint: "AB:CD", EstablishmentName: "Lido Sole"}, nil},
		"typed":         {PairInput{URL: "lido.example.it/admin/", Code: "abcd efgh"}, Target{URL: "https://lido.example.it", Code: "ABCDEFGH"}, nil},
		"loopback http": {PairInput{URL: "http://127.0.0.1:8090", Code: "ABCDEFGH"}, Target{URL: "http://127.0.0.1:8090", Code: "ABCDEFGH"}, nil},
		"http on lan":   {PairInput{URL: "http://192.168.1.20", Code: "ABCDEFGH"}, Target{}, ErrInsecureURL},
		"http via vpn":  {PairInput{URL: "http://10.8.0.1", Code: "ABCDEFGH"}, Target{}, ErrInsecureURL},
		"bad qr":        {PairInput{QR: "https://example.com"}, Target{}, ErrInvalidInput},
		"missing code":  {PairInput{URL: "https://x.it"}, Target{}, ErrInvalidInput},
		"bad code":      {PairInput{URL: "https://x.it", Code: "!!"}, Target{}, ErrInvalidInput},
		"ftp":           {PairInput{URL: "ftp://x.it", Code: "ABCDEFGH"}, Target{}, ErrInvalidInput},
		"userinfo":      {PairInput{URL: "https://a:b@x.it", Code: "ABCDEFGH"}, Target{}, ErrInvalidInput},
	}
	for name, tc := range tests {
		got, err := ParseInput(tc.in)
		if !errors.Is(err, tc.wantErr) || got != tc.want {
			t.Errorf("%s: %+v %v, want %+v %v", name, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestVersionAtLeast_Scenarios(t *testing.T) {
	tests := []struct {
		v, min string
		want   bool
	}{
		{"1.2.0", "1.2.0", true},
		{"1.10.0", "1.9.9", true},
		{"v2.0.0", "1.99.0", true},
		{"1.2.0", "1.3.0", false},
		{"0.9.0-rc.1", "0.9.0", true},
		{"dev", "9.0.0", true},
		{"1.2", "1.2.1", false},
		{"1.2.0", "", true},
	}
	for _, tc := range tests {
		if got := VersionAtLeast(tc.v, tc.min); got != tc.want {
			t.Errorf("VersionAtLeast(%q, %q) = %v", tc.v, tc.min, got)
		}
	}
	if Platform("darwin") != "macos" || Platform("windows") != "windows" || Platform("plan9") != "other" {
		t.Fatal("Platform")
	}
}

func TestWindowHelpers(t *testing.T) {
	js := InitScript("1.0.0", true)
	if !strings.Contains(js, `"mode":"remote"`) || !strings.Contains(js, `"kiosk":true`) || !strings.Contains(js, "arenileOpenExternal") {
		t.Fatal(js)
	}
	if !IsExternal("http://127.0.0.1:5/admin/", "https://coste360.it/") || IsExternal("http://127.0.0.1:5/admin/", "http://127.0.0.1:5/x") {
		t.Fatal("IsExternal")
	}
	// The binding opens web links of other origins only: never local
	// programs, files or custom schemes, even though they are "external".
	panel := "http://127.0.0.1:5/admin/"
	for _, ok := range []string{"https://coste360.it/", "mailto:info@lido.example", "http://192.168.1.20:8443/admin/"} {
		if !AllowExternal(panel, ok) {
			t.Errorf("AllowExternal(%q) = false", ok)
		}
	}
	for _, bad := range []string{
		"http://127.0.0.1:5/other", "file:/Applications/Terminal.app", "/Applications/Calculator.app",
		`C:\Windows\System32\calc.exe`, `\\host\share\evil.exe`, "javascript:alert(1)", "arenile://pair?d=x", "-n",
	} {
		if AllowExternal(panel, bad) {
			t.Errorf("AllowExternal(%q) = true", bad)
		}
	}
	args := KioskArgs("http://127.0.0.1:5/admin/", "/tmp/p")
	if args[0] != "--kiosk" || args[1] != "--app=http://127.0.0.1:5/admin/" || args[len(args)-1] != "--user-data-dir=/tmp/p" {
		t.Fatal(args)
	}
	win := KioskCandidates("windows", func(k string) string {
		if k == "ProgramFiles(x86)" {
			return `C:\PF86`
		}
		return ""
	})
	if len(win) != 2 || !strings.Contains(win[0], "msedge.exe") {
		t.Fatal(win)
	}
	if len(KioskCandidates("darwin", nil)) == 0 || len(KioskCandidates("linux", nil)) == 0 {
		t.Fatal("candidates")
	}
}

func TestRewriteResponse(t *testing.T) {
	h := http.Header{}
	h.Set("Location", "https://lido.local:8443/admin/login")
	h.Add("Set-Cookie", "arenile_session=abc; Path=/; HttpOnly; Secure; SameSite=Lax")
	h.Add("Set-Cookie", "arenile_csrf=x; Domain=lido.local; Path=/; secure")
	RewriteResponse(h, "https://lido.local:8443", "http://127.0.0.1:9")
	if h.Get("Location") != "http://127.0.0.1:9/admin/login" {
		t.Fatal(h.Get("Location"))
	}
	c := h.Values("Set-Cookie")
	if len(c) != 2 || strings.Contains(strings.ToLower(strings.Join(c, ";")), "secure") || strings.Contains(c[1], "Domain") || !strings.Contains(c[0], "HttpOnly") {
		t.Fatalf("cookies %v", c)
	}
}

func TestConfigAndTokens(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadConfig(dir); !errors.Is(err, ErrNoConfig) {
		t.Fatal(err)
	}
	m := New(Options{})
	if m.Configured(dir) {
		t.Fatal("configured without file")
	}
	if err := SetMode(dir, desktop.ModeRemote); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(dir)
	if err != nil || c.Mode != desktop.ModeRemote || c.Paired() || c.FiscalDriver != "noop" || !m.Configured(dir) {
		t.Fatalf("config %+v %v", c, err)
	}
	if err := SaveConfig(dir, Config{Mode: "hybrid"}); err == nil {
		t.Fatal("invalid mode saved")
	}
	ts := &SecretTokenStore{Stores: []desktop.SecretStore{desktop.FileSecretStore{Path: filepath.Join(dir, "remote-token")}}, filePath: filepath.Join(dir, "remote-token")}
	if _, err := ts.Get(context.Background()); !errors.Is(err, ErrNoToken) {
		t.Fatal(err)
	}
	if err := ts.Set(context.Background(), "ar_dev_x_y"); err != nil {
		t.Fatal(err)
	}
	if v, err := ts.Get(context.Background()); err != nil || v != "ar_dev_x_y" {
		t.Fatalf("token %q %v", v, err)
	}
	if err := ts.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ts.Delete(context.Background()); err != nil {
		t.Fatalf("delete is idempotent: %v", err)
	}
	if _, err := ts.Get(context.Background()); !errors.Is(err, ErrNoToken) {
		t.Fatal("token survived delete")
	}
}

// memTokens is an in-memory TokenStore.
type memTokens struct {
	mu sync.Mutex
	v  string
}

func (m *memTokens) Get(context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.v == "" {
		return "", ErrNoToken
	}
	return m.v, nil
}

func (m *memTokens) Set(_ context.Context, v string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.v = v
	return nil
}

func (m *memTokens) Delete(context.Context) error { return m.Set(context.Background(), "") }

// fakeInstance is an Arenile instance with a self-signed certificate.
type fakeInstance struct {
	srv         *httptest.Server
	fingerprint string
	minDesktop  string
	revoked     atomic.Bool
	registered  atomic.Int32
	unpaired    atomic.Int32
	pairName    atomic.Value
}

const pairedToken = "ar_dev_abcdefgh_secret"

func newFakeInstance(t *testing.T) *fakeInstance {
	t.Helper()
	f := &fakeInstance{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/admin/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.Meta{Product: api.Arenile, Version: "1.0.0", MinClientVersions: api.MinClientVersions{Desktop: f.minDesktop}, Capabilities: api.MetaCapabilities{FiscalBridge: true}})
	})
	mux.HandleFunc("POST /api/device/v1/pair", func(w http.ResponseWriter, r *http.Request) {
		var req api.DevicePairRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Code != "ABCDEFGH" || req.Kind != "desktop" {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"type":"pairing_code_invalid","status":400}`)
			return
		}
		f.pairName.Store(req.Name)
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(api.DevicePairResult{Token: pairedToken, EstablishmentId: "est-1", Device: api.Device{Id: "dev-1", Name: req.Name}})
	})
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if f.revoked.Load() || r.Header.Get("Authorization") != "Bearer "+pairedToken {
				w.WriteHeader(401)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("PUT /api/device/v1/fiscal-bridge", auth(func(w http.ResponseWriter, _ *http.Request) {
		f.registered.Add(1)
		_, _ = io.WriteString(w, `{"registered":true,"online":true,"pending_jobs":0,"failed_jobs":0}`)
	}))
	mux.HandleFunc("GET /api/device/v1/fiscal-bridge/jobs", auth(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(w, `{"jobs":[]}`)
	}))
	mux.HandleFunc("DELETE /api/device/v1/self", auth(func(w http.ResponseWriter, _ *http.Request) {
		f.unpaired.Add(1)
		w.WriteHeader(204)
	}))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "arenile_csrf", Value: "t", Path: "/", Secure: true})
		_, _ = io.WriteString(w, "panel host="+r.Host)
	})
	f.srv = httptest.NewUnstartedServer(mux)
	f.srv.Config.ErrorLog = log.New(io.Discard, "", 0) // expected handshake failures (wrong pins)
	f.srv.StartTLS()
	t.Cleanup(f.srv.Close)
	f.fingerprint = desktop.Fingerprint(f.srv.Certificate().Raw)
	return f
}

func (f *fakeInstance) qr(t *testing.T, fp string) string {
	t.Helper()
	s, err := device.EncodeQR(device.QRPayload{URL: f.srv.URL, Code: "ABCD-EFGH", Fingerprint: fp, EstablishmentName: "Lido Sole"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPairer_AgainstSelfSignedInstance(t *testing.T) {
	f := newFakeInstance(t)
	ctx := context.Background()
	p := Pairer{Version: "1.0.0", GOOS: "windows"}
	res, err := p.Pair(ctx, PairInput{QR: f.qr(t, f.fingerprint), DeviceName: "Cassa 1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != pairedToken || res.Config.DeviceID != "dev-1" || res.Config.Fingerprint != f.fingerprint || !res.Config.FiscalBridge || res.Config.EstablishmentName != "Lido Sole" {
		t.Fatalf("paired %+v", res.Config)
	}
	if _, err := p.Pair(ctx, PairInput{QR: f.qr(t, strings.Repeat("00:", 31)+"00")}); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("wrong fingerprint must fail: %v", err)
	}
	if _, err := p.Pair(ctx, PairInput{URL: f.srv.URL, Code: "ABCD-EFGH"}); err == nil {
		t.Fatal("self-signed instance without pin must fail")
	}
	if _, err := p.Pair(ctx, PairInput{URL: f.srv.URL, Code: "ZZZZ-ZZZZ", Fingerprint: f.fingerprint}); !errors.Is(err, ErrCodeInvalid) {
		t.Fatalf("invalid code: %v", err)
	}
	f.minDesktop = "2.0.0"
	if _, err := p.Pair(ctx, PairInput{QR: f.qr(t, f.fingerprint)}); !errors.Is(err, ErrClientTooOld) {
		t.Fatalf("old client: %v", err)
	}
}

// fakeUI records the UI calls of the remote mode.
type fakeUI struct {
	mu       sync.Mutex
	opened   []string
	notified []string
	quit     chan struct{}
	once     sync.Once
}

func newFakeUI() *fakeUI { return &fakeUI{quit: make(chan struct{})} }

func (u *fakeUI) Run(ctx context.Context, _ []desktop.MenuItem) error {
	select {
	case <-ctx.Done():
	case <-u.quit:
	}
	return nil
}
func (u *fakeUI) SetMenu([]desktop.MenuItem) {}
func (u *fakeUI) OpenURL(s string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.opened = append(u.opened, s)
	return nil
}
func (u *fakeUI) OpenFolder(string) error { return nil }
func (u *fakeUI) Notify(_, msg string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.notified = append(u.notified, msg)
}
func (u *fakeUI) Quit() { u.once.Do(func() { close(u.quit) }) }

type scriptedForm struct{ inputs []PairInput }

func (s *scriptedForm) Ask(_ context.Context, submit func(PairInput) error) error {
	for _, in := range s.inputs {
		if submit(in) == nil {
			return nil
		}
	}
	return ErrNotPaired
}

// recordingWindow loads the panel through the window URL and waits until
// the bridge registered, like an operator who looks at the panel.
type recordingWindow struct {
	f    *fakeInstance
	opts WindowOptions
	body string
}

func (w *recordingWindow) Open(ctx context.Context, o WindowOptions) error {
	w.opts = o
	resp, err := http.Get(o.URL)
	if err != nil {
		return err
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	w.body = string(b) + "|" + strings.Join(resp.Header.Values("Set-Cookie"), ",")
	deadline := time.Now().Add(5 * time.Second)
	for w.f.registered.Load() == 0 && time.Now().Before(deadline) && ctx.Err() == nil {
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func TestMode_PairRunAndRevoke(t *testing.T) {
	f := newFakeInstance(t)
	dir := t.TempDir()
	if err := SetMode(dir, desktop.ModeRemote); err != nil {
		t.Fatal(err)
	}
	tokens := &memTokens{}
	win := &recordingWindow{f: f}
	m := New(Options{
		Version: "1.0.0", GOOS: "linux", Tokens: func(string) TokenStore { return tokens }, Window: win,
		Pairing:   &scriptedForm{inputs: []PairInput{{URL: f.srv.URL, Code: "WRONG-111", Fingerprint: f.fingerprint}, {QR: f.qr(t, f.fingerprint), DeviceName: "Cassa 2"}}},
		Autostart: &fakeAutostart{},
	})
	ui := newFakeUI()
	if err := m.Run(context.Background(), ui, dir, nil); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil || !cfg.Paired() || cfg.DeviceName != "Cassa 2" || cfg.Mode != desktop.ModeRemote {
		t.Fatalf("config after pairing %+v %v", cfg, err)
	}
	if v, _ := tokens.Get(context.Background()); v != pairedToken {
		t.Fatal("token not stored")
	}
	// The window got the loopback proxy (self-signed instance), the init
	// script, and the panel through the pinned proxy without Secure cookies.
	if !strings.HasPrefix(win.opts.URL, "http://127.0.0.1:") || !strings.HasSuffix(win.opts.URL, "/admin/") {
		t.Fatalf("window url %q", win.opts.URL)
	}
	if !strings.Contains(win.opts.Init, "arenileDesktop") || win.opts.Title != "Arenile — Lido Sole" {
		t.Fatalf("window options %+v", win.opts)
	}
	host := strings.TrimPrefix(f.srv.URL, "https://")
	if !strings.HasPrefix(win.body, "panel host="+host) || strings.Contains(strings.ToLower(win.body), "secure") {
		t.Fatalf("panel through proxy %q", win.body)
	}
	if f.registered.Load() == 0 {
		t.Fatal("fiscal bridge agent did not register")
	}

	// Revoked from the panel: the agent gets 401, the pairing is dropped
	// and the UI quits.
	f.revoked.Store(true)
	ui2 := newFakeUI()
	m2 := New(Options{Version: "1.0.0", GOOS: "linux", Tokens: func(string) TokenStore { return tokens }, Autostart: &fakeAutostart{}})
	m2.o.Window = nil
	done := make(chan error, 1)
	go func() { done <- m2.Run(context.Background(), ui2, dir, nil) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("remote mode did not stop after revocation")
	}
	if v, _ := tokens.Get(context.Background()); v != "" {
		t.Fatal("token kept after revocation")
	}
	if cfg, _ := LoadConfig(dir); cfg.Paired() {
		t.Fatal("pairing kept after revocation")
	}
	if len(ui2.opened) == 0 || len(ui2.notified) == 0 {
		t.Fatalf("browser fallback / notification missing: %v %v", ui2.opened, ui2.notified)
	}
	// Not paired and no form: a clear error.
	if err := New(Options{Tokens: func(string) TokenStore { return tokens }}).Run(context.Background(), newFakeUI(), dir, nil); !errors.Is(err, ErrNotPaired) {
		t.Fatalf("unpaired run: %v", err)
	}
}

func TestMode_Unpair(t *testing.T) {
	f := newFakeInstance(t)
	dir := t.TempDir()
	tokens := &memTokens{v: pairedToken}
	m := New(Options{Tokens: func(string) TokenStore { return tokens }})
	cfg := Config{Mode: desktop.ModeRemote, InstanceURL: f.srv.URL, Fingerprint: f.fingerprint, DeviceID: "dev-1"}
	if err := SaveConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.Unpair(context.Background(), dir, cfg, pairedToken, HTTPClient(f.fingerprint)); err != nil {
		t.Fatal(err)
	}
	c, _ := LoadConfig(dir)
	if f.unpaired.Load() != 1 || tokens.v != "" || c.Paired() || c.Mode != desktop.ModeLocal {
		t.Fatalf("unpair: %d %q %+v", f.unpaired.Load(), tokens.v, c)
	}
}

type fakeAutostart struct{ on bool }

func (a *fakeAutostart) Enabled() (bool, error) { return a.on, nil }
func (a *fakeAutostart) SetEnabled(on bool) error {
	a.on = on
	return nil
}
