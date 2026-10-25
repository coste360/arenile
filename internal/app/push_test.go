package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/push"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/pushrelay"
	"github.com/coste360/arenile/internal/store"
)

const pushTestSecret = "relay-secret-0123456789abcdef0123456789"

// pushTestRelay is a fake Coste360 relay: it verifies the key id and the
// signature like the real one and records the bodies.
type pushTestRelay struct {
	t      *testing.T
	mu     sync.Mutex
	bodies [][]byte
}

func (f *pushTestRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.Header.Get(pushrelay.KeyHeader) != "lido-demo" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" ||
		!strings.HasPrefix(r.UserAgent(), "arenile/1.0.0") {
		f.t.Errorf("relay request headers %v", r.Header)
	}
	if err := event.VerifySignature(pushTestSecret, r.Header.Get(pushrelay.SignatureHeader), body, time.Date(2027, 7, 10, 8, 0, 0, 0, time.UTC), 5*time.Minute); err != nil {
		f.t.Errorf("relay signature: %v", err)
	}
	f.mu.Lock()
	f.bodies = append(f.bodies, body)
	f.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}

func (f *pushTestRelay) received() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]byte(nil), f.bodies...)
}

// pushTestClient is a minimal API client: cookies and CSRF for the panel,
// bearer token for the app.
type pushTestClient struct {
	t      *testing.T
	base   string
	http   *http.Client
	bearer string
}

func newPushTestClient(t *testing.T, base string) *pushTestClient {
	jar, _ := cookiejar.New(nil)
	return &pushTestClient{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *pushTestClient) do(method, path string, in any, hdr map[string]string) (int, http.Header, map[string]any) {
	c.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	} else if u, _ := url.Parse(c.base); method != http.MethodGet {
		for _, ck := range c.http.Jar.Cookies(u) {
			if ck.Name == "arenile_csrf" {
				req.Header.Set("X-CSRF-Token", ck.Value)
			}
		}
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, resp.Header, out
}

// pushTestApp boots the app with the relay (or without a key) and the demo
// establishment, an owner logged in on the panel and a paired Android app.
func pushTestApp(t *testing.T, relayURL, relayKey string) (a *App, estID string, panel, app *pushTestClient) {
	t.Helper()
	cfg := devConfig(t)
	cfg.PushRelayURL = relayURL
	key, err := pushrelay.ParseKey(relayKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg.PushRelayKey = key
	c := clock.NewFake(time.Date(2027, 7, 10, 8, 0, 0, 0, time.UTC))
	a, err = New(cfg, WithVersion("v1.0.0"), WithClock(c), WithoutSignalHandling(), WithLogger(slog.New(slog.DiscardHandler)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	ctx := context.Background()
	seed, err := SeedDemo(ctx, a.DB(), c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Auth.CreateOwner(ctx, auth.CreateOwnerInput{
		Email: "titolare@push.example", Name: "Titolare", Password: "una-password-lunga-per-i-test",
		OrganizationName: DemoOrganizationName, Source: "seed",
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	panel = newPushTestClient(t, srv.URL)
	if code, _, body := panel.do(http.MethodPost, "/api/admin/v1/auth/login", map[string]string{"email": "titolare@push.example", "password": "una-password-lunga-per-i-test"}, nil); code != http.StatusOK {
		t.Fatalf("login %d %v", code, body)
	}
	estID = seed.EstablishmentID
	code, _, body := panel.do(http.MethodPost, "/api/admin/v1/establishments/"+estID+"/devices/pairing-codes", map[string]any{"kind": "mobile"}, nil)
	if code != http.StatusCreated {
		t.Fatalf("pairing code %d %v", code, body)
	}
	app = newPushTestClient(t, srv.URL)
	code, _, body = app.do(http.MethodPost, "/api/device/v1/pair", map[string]any{"code": body["code"], "name": "Pixel", "platform": "android", "kind": "mobile"}, nil)
	if code != http.StatusCreated {
		t.Fatalf("pair %d %v", code, body)
	}
	app.bearer = body["token"].(string)
	return a, estID, panel, app
}

// TestPush_EndToEndThroughFakeRelay: relay key configured, manager opt-in
// and user registration, then an online booking reaches the relay as a
// signed, generic notification without personal data (ADR 0027, INV-100).
func TestPush_EndToEndThroughFakeRelay(t *testing.T) {
	relay := &pushTestRelay{t: t}
	rs := httptest.NewServer(relay)
	defer rs.Close()
	a, estID, panel, app := pushTestApp(t, rs.URL+"/v1/notifications", "lido-demo."+pushTestSecret)
	ctx := context.Background()
	settingsPath := "/api/admin/v1/establishments/" + estID + "/push-settings"

	if code, _, body := panel.do(http.MethodGet, "/api/admin/v1/meta", nil, nil); code != http.StatusOK || body["capabilities"].(map[string]any)["push_notifications"] != true {
		t.Fatalf("meta %d %v", code, body)
	}
	code, hdr, body := panel.do(http.MethodGet, settingsPath, nil, nil)
	if code != http.StatusOK || body["enabled"] != false || body["relay_configured"] != true || hdr.Get("ETag") != `"1"` {
		t.Fatalf("settings %d %v", code, body)
	}
	// The app cannot register before the manager turns push on.
	reg := map[string]any{"token": "fcm-native-token-0123456789abcdef", "locale": "it", "topics": []string{"booking_new_online"}}
	if code, _, body := app.do(http.MethodPut, "/api/device/v1/push", reg, nil); code != http.StatusConflict || body["type"] != "push_unavailable" {
		t.Fatalf("register before opt-in %d %v", code, body)
	}
	events := map[string]any{"booking_new_online": true, "booking_approval_requested": true, "payment_failed": true}
	if code, _, body := panel.do(http.MethodPut, settingsPath, map[string]any{"enabled": true, "events": events}, nil); code != http.StatusPreconditionRequired {
		t.Fatalf("update without If-Match %d %v", code, body)
	}
	if code, _, body := panel.do(http.MethodPut, settingsPath, map[string]any{"enabled": true, "events": events}, map[string]string{"If-Match": `"1"`}); code != http.StatusOK || body["enabled"] != true {
		t.Fatalf("update %d %v", code, body)
	}
	code, _, body = app.do(http.MethodPut, "/api/device/v1/push", reg, nil)
	if code != http.StatusOK || body["available"] != true || body["registered"] != true || body["token"] != nil {
		t.Fatalf("register %d %v", code, body)
	}
	if code, _, body := app.do(http.MethodGet, "/api/device/v1/push", nil, nil); code != http.StatusOK || body["registered"] != true {
		t.Fatalf("status %d %v", code, body)
	}

	// An online booking confirmed: outbox -> sink -> deliverer -> relay.
	err := store.RunInTx(ctx, a.DB(), func(ctx context.Context, tx bun.Tx) error {
		_, err := a.Outbox.Write(ctx, tx, event.BookingConfirmed, estID, 1, event.BookingChange{
			BookingID: store.NewID(), Code: "LD-2027-000777", Status: "confirmed", Channel: store.ChannelOnline,
			CustomerID: store.NewID(), TotalAmount: 9900,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Jobs().RunNow(ctx, jobs.JobOutboxDispatch); err != nil {
		t.Fatal(err)
	}
	if err := a.Jobs().RunNow(ctx, push.JobDeliver); err != nil {
		t.Fatal(err)
	}
	got := relay.received()
	if len(got) != 1 {
		t.Fatalf("relay received %d notifications", len(got))
	}
	var p map[string]any
	if err := json.Unmarshal(got[0], &p); err != nil {
		t.Fatal(err)
	}
	if p["device_push_id"] != "fcm-native-token-0123456789abcdef" || p["platform"] != "android" || p["environment"] != "production" ||
		p["message"].(map[string]any)["key"] != "booking_new_online" || len(p) != 7 {
		t.Fatalf("payload %s", got[0])
	}
	for _, leak := range []string{"LD-2027-000777", "9900", DemoOrganizationName, "titolare", estID} {
		if strings.Contains(string(got[0]), leak) {
			t.Fatalf("payload leaks %q: %s", leak, got[0])
		}
	}

	// The user turns notifications off: nothing more is sent.
	if code, _, _ := app.do(http.MethodDelete, "/api/device/v1/push", nil, nil); code != http.StatusNoContent {
		t.Fatalf("unregister %d", code)
	}
	if code, _, body := app.do(http.MethodGet, "/api/device/v1/push", nil, nil); code != http.StatusOK || body["registered"] != false {
		t.Fatalf("status after unregister %d %v", code, body)
	}
}

// TestPush_NoRelayKeyNoNetwork: without ARENILE_PUSH_RELAY_KEY the
// capability is off, registration is refused and the relay is never
// contacted, even with push turned on by the manager.
func TestPush_NoRelayKeyNoNetwork(t *testing.T) {
	relay := &pushTestRelay{t: t}
	rs := httptest.NewServer(relay)
	defer rs.Close()
	a, estID, panel, app := pushTestApp(t, rs.URL, "")
	if code, _, body := panel.do(http.MethodGet, "/api/admin/v1/meta", nil, nil); code != http.StatusOK || body["capabilities"].(map[string]any)["push_notifications"] != false {
		t.Fatalf("meta %d %v", code, body)
	}
	settingsPath := "/api/admin/v1/establishments/" + estID + "/push-settings"
	events := map[string]any{"booking_new_online": true, "booking_approval_requested": true, "payment_failed": true}
	if code, _, body := panel.do(http.MethodPut, settingsPath, map[string]any{"enabled": true, "events": events}, map[string]string{"If-Match": "*"}); code != http.StatusOK || body["relay_configured"] != false {
		t.Fatalf("update %d %v", code, body)
	}
	reg := map[string]any{"token": "fcm-native-token-0123456789abcdef", "locale": "it", "topics": []string{}}
	if code, _, body := app.do(http.MethodPut, "/api/device/v1/push", reg, nil); code != http.StatusConflict || body["type"] != "push_unavailable" {
		t.Fatalf("register %d %v", code, body)
	}
	ctx := context.Background()
	if err := a.Jobs().RunNow(ctx, push.JobDeliver); err != nil {
		t.Fatal(err)
	}
	if n := len(relay.received()); n != 0 {
		t.Fatalf("relay contacted %d times", n)
	}
}
