package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/coste360/arenile/internal/desktop/control"
)

// fakeUI records what the launcher asks of the tray.
type fakeUI struct {
	mu     sync.Mutex
	opened []string
	notes  []string
	menu   []MenuItem
	quit   chan struct{}
	once   sync.Once
}

func newFakeUI() *fakeUI { return &fakeUI{quit: make(chan struct{})} }

func (f *fakeUI) Run(ctx context.Context, menu []MenuItem) error {
	f.SetMenu(menu)
	select {
	case <-ctx.Done():
	case <-f.quit:
	}
	return nil
}

func (f *fakeUI) SetMenu(m []MenuItem) { f.mu.Lock(); f.menu = m; f.mu.Unlock() }
func (f *fakeUI) OpenURL(u string) error {
	f.mu.Lock()
	f.opened = append(f.opened, u)
	f.mu.Unlock()
	return nil
}
func (f *fakeUI) OpenFolder(string) error { return nil }
func (f *fakeUI) Notify(_, m string)      { f.mu.Lock(); f.notes = append(f.notes, m); f.mu.Unlock() }
func (f *fakeUI) Quit()                   { f.once.Do(func() { close(f.quit) }) }

// browserFakeUI is a window UI (desktop.BrowserOpener).
type browserFakeUI struct {
	*fakeUI
	browser []string
}

func (b *browserFakeUI) OpenInBrowser(u string) error {
	b.mu.Lock()
	b.browser = append(b.browser, u)
	b.mu.Unlock()
	return nil
}

func (f *fakeUI) lastOpened() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.opened) == 0 {
		return ""
	}
	return f.opened[len(f.opened)-1]
}

func (f *fakeUI) menuIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for _, m := range f.menu {
		ids = append(ids, m.ID)
	}
	return ids
}

// apiClient is a browser-like client: cookie jar plus the CSRF header.
type apiClient struct {
	t    *testing.T
	base string
	http *http.Client
}

func (c *apiClient) do(method, path string, body any, hdr map[string]string) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	u, _ := url.Parse(c.base)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == "arenile_csrf" {
			req.Header.Set("X-CSRF-Token", ck.Value)
		}
	}
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (c *apiClient) json(method, path string, body any, want int) map[string]any {
	c.t.Helper()
	code, b := c.do(method, path, body, nil)
	if code != want {
		c.t.Fatalf("%s %s = %d, want %d: %s", method, path, code, want, b)
	}
	out := map[string]any{}
	if len(b) > 0 {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func startLauncher(t *testing.T, o Options) (*fakeUI, StartInfo, chan error) {
	t.Helper()
	ready := make(chan StartInfo, 1)
	o.OnReady = func(si StartInfo) { ready <- si }
	ui := newFakeUI()
	errc := make(chan error, 1)
	go func() { errc <- NewLauncher(o).Run(context.Background(), ui) }()
	select {
	case si := <-ready:
		return ui, si, errc
	case err := <-errc:
		t.Fatalf("launcher stopped: %v", err)
	case <-time.After(60 * time.Second):
		t.Fatal("launcher not ready")
	}
	return nil, StartInfo{}, nil
}

func stopLauncher(t *testing.T, ui *fakeUI, errc chan error) {
	t.Helper()
	ui.Quit()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("launcher exit: %v", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("launcher did not stop")
	}
}

// TestSmoke_DesktopLauncherHeadless boots the desktop edition in a temp
// data directory (with a space in the path, as on macOS) without a tray:
// first owner from the browser endpoint, guided setup and a booking
// through the API, backup, restore, second launch, exit backup, restart.
func TestSmoke_DesktopLauncherHeadless(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test")
	}
	keyring.MockInit()
	dataDir := filepath.Join(t.TempDir(), "Application Support", "Arenile")
	never := make(chan time.Time)
	auto := &fakeAutostart{}
	opts := Options{
		Version: "1.0.0", DataDir: dataDir, Log: slog.New(slog.DiscardHandler),
		Autostart: auto, ScheduleTick: never, 
	}
	ui, si, errc := startLauncher(t, opts)

	// The browser is sent to the first-run page with the token in the fragment.
	u, err := url.Parse(si.OpenURL)
	if err != nil || u.Path != "/admin/first-run" || !strings.HasPrefix(u.Fragment, "token=") || ui.lastOpened() != si.OpenURL {
		t.Fatalf("open url %q (%v)", si.OpenURL, ui.opened)
	}
	token := strings.TrimPrefix(u.Fragment, "token=")
	if !strings.HasPrefix(si.BaseURL, "http://127.0.0.1:") {
		t.Fatalf("base url %q", si.BaseURL)
	}
	for i := 0; len(ui.menuIDs()) == 0 && i < 100; i++ {
		time.Sleep(20 * time.Millisecond) // OnReady runs just before the tray
	}
	if ids := strings.Join(ui.menuIDs(), ","); ids != "open,backup,data_dir,autostart,restore,quit" {
		t.Fatalf("menu %s", ids)
	}
	info, err := ReadInstanceInfo(dataDir)
	if err != nil || info.OpenURL != si.OpenURL {
		t.Fatalf("instance file %+v %v", info, err)
	}

	jar, _ := cookiejar.New(nil)
	c := &apiClient{t: t, base: si.BaseURL, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}

	// /meta reports the desktop edition.
	m := c.json(http.MethodGet, "/api/admin/v1/meta", nil, http.StatusOK)
	if m["edition"] != "desktop" {
		t.Fatalf("meta %v", m)
	}
	// Loopback only: a rebinding Host header is refused before the handler.
	if code, b := c.do(http.MethodGet, "/api/desktop/v1/first-run", nil, map[string]string{"Host": "evil.example:80"}); code != http.StatusForbidden || !bytes.Contains(b, []byte("loopback_required")) {
		t.Fatalf("rebinding host: %d %s", code, b)
	}
	if r := c.json(http.MethodGet, "/api/desktop/v1/first-run", nil, http.StatusOK); r["required"] != true {
		t.Fatalf("first-run status %v", r)
	}
	owner := map[string]any{
		"setup_token": "wrong", "email": "anna@lido-test.it", "name": "Anna Rossi",
		"password": "spiaggia-sicura-2027", "organization_name": "Lido Test",
	}
	if code, b := c.do(http.MethodPost, "/api/desktop/v1/first-run", owner, nil); code != http.StatusForbidden || !bytes.Contains(b, []byte("setup_token_invalid")) {
		t.Fatalf("wrong token: %d %s", code, b)
	}
	owner["setup_token"], owner["password"] = token, "corta"
	if code, b := c.do(http.MethodPost, "/api/desktop/v1/first-run", owner, nil); code != http.StatusBadRequest || !bytes.Contains(b, []byte(`"password"`)) {
		t.Fatalf("weak password: %d %s", code, b)
	}
	owner["password"] = "spiaggia-sicura-2027" // the token survived the validation error
	me := c.json(http.MethodPost, "/api/desktop/v1/first-run", owner, http.StatusCreated)
	if me["principal_type"] != "user" {
		t.Fatalf("me %v", me)
	}
	if code, b := c.do(http.MethodPost, "/api/desktop/v1/first-run", owner, nil); code != http.StatusConflict || !bytes.Contains(b, []byte("setup_completed")) {
		t.Fatalf("second first-run: %d %s", code, b)
	}
	if info, _ := ReadInstanceInfo(dataDir); info.OpenURL != si.BaseURL+"/admin/" {
		t.Fatalf("instance file still carries the token: %+v", info)
	}

	// Guided setup and a booking, as the panel does.
	est := c.json(http.MethodPost, "/api/admin/v1/establishments", map[string]any{"name": "Lido Test", "slug": "lido-test"}, http.StatusCreated)
	estID, _ := est["id"].(string)
	base := "/api/admin/v1/establishments/" + estID
	applied := c.json(http.MethodPost, base+"/setup/apply-template", map[string]any{"template": "classic_rows", "rows": 8, "spots_per_row": 15}, http.StatusCreated)
	seasonID, _ := applied["season_id"].(string)
	var prices []map[string]any
	for _, cat := range []string{"row1", "row2", "standard", "gazebo"} {
		for _, p := range []string{"low", "mid", "high", "peak"} {
			prices = append(prices, map[string]any{"price_category": cat, "rate_period_code": p, "amount": 2500})
		}
	}
	c.json(http.MethodPost, base+"/setup/quick-pricelist", map[string]any{"season_id": seasonID, "prices": prices}, http.StatusCreated)
	season := c.json(http.MethodGet, base+"/seasons/"+seasonID, nil, http.StatusOK)
	day, _ := season["opens_on"].(string)
	doc := c.json(http.MethodGet, base+"/maps/"+applied["map_id"].(string), nil, http.StatusOK)
	spots, _ := doc["spots"].([]any)
	packs, _ := applied["pack_ids"].([]any)
	book := func(spotIdx int, last string) map[string]any {
		t.Helper()
		for _, p := range packs {
			sp := spots[spotIdx].(map[string]any)
			req := map[string]any{
				"starts_on": day, "ends_on": day, "formula": "day", "slots": "FULL", "force": true,
				"customer": map[string]any{"last_name": last},
				"lines":    []map[string]any{{"pack_id": p, "spot_id": sp["id"]}},
			}
			code, b := c.do(http.MethodPost, base+"/bookings", req, nil)
			if code == http.StatusCreated {
				out := map[string]any{}
				_ = json.Unmarshal(b, &out)
				return out
			}
			t.Logf("pack %v: %d %s", p, code, b)
		}
		t.Fatal("no pack could book the spot")
		return nil
	}
	first := book(0, "Bianchi")
	if first["id"] == nil {
		t.Fatalf("booking %v", first)
	}

	// Backup from the panel, then a second booking, then restore: the
	// second booking disappears and the server comes back on the same URL.
	bk := c.json(http.MethodPost, "/api/desktop/v1/backups", nil, http.StatusCreated)
	if bk["reason"] != "manual" {
		t.Fatalf("backup %v", bk)
	}
	book(1, "Verdi")
	countBookings := func() int {
		res, err := c.http.Get(c.base + base + "/bookings") // the server restarts during a restore
		if err != nil {
			return -1
		}
		b, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return -1
		}
		var l struct {
			Items []any `json:"items"`
		}
		_ = json.Unmarshal(b, &l)
		return len(l.Items)
	}
	if n := countBookings(); n != 2 {
		t.Fatalf("bookings before restore = %d", n)
	}
	s := c.json(http.MethodGet, "/api/desktop/v1/settings", nil, http.StatusOK)
	s["autostart"] = true
	c.json(http.MethodPut, "/api/desktop/v1/settings", s, http.StatusOK)
	if !auto.on {
		t.Fatal("autostart not applied")
	}
	r := c.json(http.MethodPost, "/api/desktop/v1/backups/"+bk["name"].(string)+"/restore", nil, http.StatusAccepted)
	if r["restart_required"] != true {
		t.Fatalf("restore %v", r)
	}
	deadline := time.Now().Add(30 * time.Second)
	for countBookings() != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("restore not applied (bookings=%d)", countBookings())
		}
		time.Sleep(100 * time.Millisecond)
	}
	list := c.json(http.MethodGet, "/api/desktop/v1/backups", nil, http.StatusOK)
	if items, _ := list["items"].([]any); len(items) != 2 || items[0].(map[string]any)["reason"] != string(control.BackupPreRestore) {
		t.Fatalf("backups after restore %v", list)
	}

	// Second launch: asks the running instance to show its panel (window
	// to the front) through the activation listener and returns.
	ui2 := newFakeUI()
	ui.mu.Lock()
	before := len(ui.opened)
	ui.mu.Unlock()
	if err := NewLauncher(opts).Run(context.Background(), ui2); err != nil {
		t.Fatalf("second launch: %v", err)
	}
	if len(ui2.opened) != 0 {
		t.Fatalf("second launch opened its own panel %v", ui2.opened)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		ui.mu.Lock()
		n := len(ui.opened)
		ui.mu.Unlock()
		if n > before && ui.lastOpened() == si.BaseURL+"/admin/" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("running instance not activated: %v", ui.opened)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// An instance without the activation listener (older launcher): the
	// panel opens in the second launch, in the browser for a window UI.
	running, _ := ReadInstanceInfo(dataDir)
	legacy := running
	legacy.ActivateAddr, legacy.ActivateToken = "", ""
	if err := WriteInstanceInfo(dataDir, legacy); err != nil {
		t.Fatal(err)
	}
	winUI := &browserFakeUI{fakeUI: newFakeUI()}
	if err := NewLauncher(opts).Run(context.Background(), winUI); err != nil {
		t.Fatalf("third launch: %v", err)
	}
	if len(winUI.browser) != 1 || winUI.browser[0] != si.BaseURL+"/admin/" || len(winUI.opened) != 0 {
		t.Fatalf("legacy fallback: browser=%v window=%v", winUI.browser, winUI.opened)
	}
	if err := WriteInstanceInfo(dataDir, running); err != nil {
		t.Fatal(err)
	}

	// Quit: exit backup, instance file removed.
	stopLauncher(t, ui, errc)
	if _, err := os.Stat(filepath.Join(dataDir, instanceFile)); !os.IsNotExist(err) {
		t.Fatal("instance file left behind")
	}
	items, _ := (&Backuper{}).List(context.Background(), filepath.Join(dataDir, "backups"))
	reasons := map[control.BackupReason]int{}
	for _, it := range items {
		reasons[it.Reason]++
	}
	if len(items) != 3 || reasons[control.BackupExit] != 1 || reasons[control.BackupPreRestore] != 1 || reasons[control.BackupManual] != 1 {
		t.Fatalf("exit backup missing: %+v", items)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "secret.key")); !os.IsNotExist(err) {
		t.Fatal("the key went to the file although the keyring works")
	}

	// Restart: same key (keyring), same port, no first run, data kept.
	ui3, si3, errc3 := startLauncher(t, opts)
	if si3.OpenURL != si.BaseURL+"/admin/" || si3.BaseURL != si.BaseURL {
		t.Fatalf("restart: %+v", si3)
	}
	jar3, _ := cookiejar.New(nil)
	c3 := &apiClient{t: t, base: si3.BaseURL, http: &http.Client{Jar: jar3, Timeout: 30 * time.Second}}
	c3.json(http.MethodPost, "/api/admin/v1/auth/login", map[string]any{"email": "anna@lido-test.it", "password": "spiaggia-sicura-2027"}, http.StatusOK)
	if code, b := c3.do(http.MethodGet, base+"/bookings", nil, nil); code != http.StatusOK || !bytes.Contains(b, []byte("Bianchi")) {
		t.Fatalf("data after restart: %d %s", code, b)
	}
	stopLauncher(t, ui3, errc3)
	fmt.Fprintln(io.Discard, ui3.notes)
}
