//go:build integration

package public_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	httpserver "github.com/coste360/arenile/internal/http"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type readiness struct{ db *store.DB }

func (r readiness) Ping(ctx context.Context) error                  { return r.db.PingContext(ctx) }
func (r readiness) MigrationsAligned(context.Context) (bool, error) { return true, nil }

type client struct {
	t    *testing.T
	base string
}

func newServer(t *testing.T, e *checkouttest.Env, creations int) *client {
	t.Helper()
	h := httpserver.New(httpserver.Deps{
		DB: e.DB, Clock: e.F.Clock, Version: "test", Bus: event.NewBroker(nil, 16), Readiness: readiness{e.DB},
		Establishments: establishment.NewService(e.DB), Online: e.Online, Checkout: e.Svc,
		PublicCreationsPerMinute: creations, PublicReadsPerMinute: 1000,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &client{t: t, base: srv.URL + "/api/public/v1/" + checkouttest.Slug}
}

func (c *client) do(method, path string, body any, headers map[string]string) (*http.Response, map[string]any, []byte) {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	url := c.base + path
	if strings.HasPrefix(path, "/") && strings.HasPrefix(path, "//") {
		url = strings.TrimSuffix(c.base, "/api/public/v1/"+checkouttest.Slug) + path[1:]
	}
	req, _ := http.NewRequest(method, url, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if strings.Contains(resp.Header.Get("Content-Type"), "json") && len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			c.t.Fatalf("json %q: %v", raw, err)
		}
	}
	return resp, out, raw
}

func expect(t *testing.T, resp *http.Response, body map[string]any, status int, problemType string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status %d, want %d: %v", resp.StatusCode, status, body)
	}
	if problemType != "" && body["type"] != problemType {
		t.Fatalf("problem %v, want %s", body, problemType)
	}
}

// TestPublicHTTP_CheckoutFlow drives the public API end to end over HTTP:
// profile with the online configuration, quote, idempotent creation,
// header-bound session, details, payment with the manual driver, QR and
// .ics, manage area by magic link, new link request.
func TestPublicHTTP_CheckoutFlow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		c := newServer(t, e, 100)
		spot := e.F.Spots[0].ID

		resp, body, _ := c.do(http.MethodGet, "", nil, nil)
		expect(t, resp, body, 200, "")
		on, _ := body["online"].(map[string]any)
		if on == nil || len(on["zones"].([]any)) != 2 || on["hold_minutes"].(float64) != 10 || on["payment"].(map[string]any)["provider"] != "manual" {
			t.Fatalf("profile online config %v", body["online"])
		}
		resp2, _, _ := c.do(http.MethodGet, "", nil, map[string]string{"If-None-Match": resp.Header.Get("ETag")})
		if resp2.StatusCode != 304 {
			t.Fatalf("profile 304: %d", resp2.StatusCode)
		}

		sel := map[string]any{
			"starts_on": "2027-07-10", "ends_on": "2027-07-10", "slots": "FULL", "formula": "day",
			"lines": []map[string]any{{"pack_id": e.F.Base.ID, "spot_id": spot}},
		}
		resp, body, _ = c.do(http.MethodPost, "/quote", sel, nil)
		expect(t, resp, body, 200, "")
		if body["total"].(float64) != 2000 || body["available"] != true || body["payment_plan"] != "at_desk" {
			t.Fatalf("quote %v", body)
		}
		resp, _, _ = c.do(http.MethodPost, "/availability", nil, nil)
		if resp.StatusCode != 405 && resp.StatusCode != 404 {
			t.Fatalf("availability is GET only: %d", resp.StatusCode)
		}
		resp, body, _ = c.do(http.MethodGet, "/availability?from=2027-07-10&to=2027-07-10&slots=FULL", nil, nil)
		expect(t, resp, body, 200, "")
		if len(body["zones"].([]any)) != 2 {
			t.Fatalf("availability %v", body)
		}

		create := map[string]any{
			"starts_on": "2027-07-10", "ends_on": "2027-07-10", "slots": "FULL", "formula": "day",
			"lines": []map[string]any{{"pack_id": e.F.Base.ID, "spot_id": spot}}, "locale": "it",
		}
		key := map[string]string{"Idempotency-Key": "tap-0123456789"}
		resp, body, _ = c.do(http.MethodPost, "/bookings", create, key)
		expect(t, resp, body, 201, "")
		token, id := body["checkout_token"].(string), body["booking_id"].(string)
		if len(token) != 43 || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("session %v %v", body, resp.Header)
		}
		resp, body, _ = c.do(http.MethodPost, "/bookings", create, key)
		expect(t, resp, body, 201, "")
		if body["booking_id"] != id || resp.Header.Get("Idempotent-Replayed") != "true" {
			t.Fatalf("double tap created another hold: %v", body)
		}
		resp, body, _ = c.do(http.MethodPost, "/bookings", create, nil)
		expect(t, resp, body, 409, "spot_unavailable")

		hdr := map[string]string{"X-Arenile-Checkout": token}
		resp, body, _ = c.do(http.MethodGet, "/bookings/"+id, nil, map[string]string{"X-Arenile-Checkout": strings.Repeat("Q", 43)})
		expect(t, resp, body, 404, "checkout_not_found")
		resp, body, _ = c.do(http.MethodGet, "/bookings/"+id, nil, hdr)
		expect(t, resp, body, 200, "")
		if body["next_action"] != "details" || body["customer"] != nil {
			t.Fatalf("state %v", body)
		}
		if _, ok := body["summary"].(map[string]any)["code"]; ok {
			t.Fatal("booking code exposed before the confirmation")
		}
		resp, body, _ = c.do(http.MethodPatch, "/bookings/"+id, map[string]any{
			"first_name": "Luca", "last_name": "Verdi", "email": "luca@example.org", "phone": "+39 340 1234567",
			"accept_terms_version": "", "marketing_consent": false,
		}, hdr)
		expect(t, resp, body, 200, "")
		if body["next_action"] != "pay" || body["customer"].(map[string]any)["email"] != "luca@example.org" {
			t.Fatalf("details %v", body)
		}
		resp, body, _ = c.do(http.MethodPost, "/bookings/"+id+"/pay", nil, hdr)
		expect(t, resp, body, 200, "")
		if body["mode"] != "none" || body["status"] != "confirmed" {
			t.Fatalf("pay %v", body)
		}
		_, body, _ = c.do(http.MethodGet, "/bookings/"+id, nil, hdr)
		conf := body["confirmation"].(map[string]any)
		if body["next_action"] != "done" || conf["code"] == "" || conf["email_masked"] == "luca@example.org" {
			t.Fatalf("confirmed %v", body)
		}
		resp, _, raw := c.do(http.MethodGet, "/bookings/"+id+"/qr", nil, hdr)
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/svg+xml" || !bytes.HasPrefix(raw, []byte("<svg>")) {
			t.Fatalf("qr %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		resp, _, _ = c.do(http.MethodGet, "/bookings/"+id+"/ics", nil, hdr)
		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") {
			t.Fatalf("ics %d %v", resp.StatusCode, resp.Header)
		}

		link, err := e.Notify.IssueMagicLink(e.Ctx, db, e.ES, id, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, body, _ = c.do(http.MethodGet, "/manage/"+link.Token, nil, nil)
		expect(t, resp, body, 200, "")
		if resp.Header.Get("Referrer-Policy") != "no-referrer" || resp.Header.Get("ETag") == "" || body["allowed"].(map[string]any)["cancel"] != true ||
			strings.Contains(body["customer"].(map[string]any)["email_masked"].(string), "luca@") {
			t.Fatalf("manage %v %v", body, resp.Header)
		}
		resp, body, _ = c.do(http.MethodPost, "/manage/"+link.Token+"/cancel", map[string]any{}, map[string]string{"If-Match": resp.Header.Get("ETag")})
		expect(t, resp, body, 200, "")
		if body["applied"] != false || body["refund_status"] != "none" {
			t.Fatalf("cancel preview %v", body)
		}
		resp, body, _ = c.do(http.MethodGet, "/manage/"+strings.Repeat("W", 43), nil, nil)
		expect(t, resp, body, 404, "not_found")
		resp, body, _ = c.do(http.MethodPost, "/manage-links", map[string]any{"email": "nobody@example.org", "code": "XX-0000"}, nil)
		if resp.StatusCode != 202 {
			t.Fatalf("manage link %d %v", resp.StatusCode, body)
		}
		e.Svc.Wait()
	})
}

// TestPublicHTTP_FrameAncestorsAndLimits: the public page answers with a
// frame-ancestors CSP built from the allowed embedding origins; public
// writes are rate limited per IP.
func TestPublicHTTP_FrameAncestorsAndLimits(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		if _, err := e.Online.Update(e.Ctx, e.F.Owner, e.F.Est.ID, nil, online.Input{
			BookingEnabled: true, AllowedEmbedOrigins: []string{"https://www.lido-delfino.it"},
		}); err != nil {
			t.Fatal(err)
		}
		c := newServer(t, e, 2)
		resp, _, _ := c.do(http.MethodGet, "//b/"+checkouttest.Slug+"?embed=1", nil, nil)
		csp := resp.Header.Get("Content-Security-Policy")
		if !strings.Contains(csp, "frame-ancestors 'self' https://www.lido-delfino.it") {
			t.Fatalf("csp %q", csp)
		}
		for i := range 3 {
			resp, body, _ := c.do(http.MethodPost, "/bookings", map[string]any{
				"starts_on": "2027-07-10", "ends_on": "2027-07-10", "slots": "FULL", "formula": "day",
				"lines": []map[string]any{{"pack_id": e.F.Base.ID, "spot_id": e.F.Spots[i].ID}},
			}, nil)
			want := 201
			if i == 2 {
				want = 429
			}
			if resp.StatusCode != want {
				t.Fatalf("creation %d: %d %v", i, resp.StatusCode, body)
			}
		}
		// Quotes count as reads.
		resp, body, _ := c.do(http.MethodPost, "/quote", map[string]any{
			"starts_on": "2027-07-11", "ends_on": "2027-07-11", "slots": "FULL", "formula": "day",
			"lines": []map[string]any{{"pack_id": e.F.Base.ID, "spot_id": e.F.Spots[3].ID}},
		}, nil)
		expect(t, resp, body, 200, "")
	})
}

// TestPublicAPI_NoCustomerData (INV-28): after a desk booking and an online
// booking with real contact details, none of the public read endpoints
// (profile, calendar, map, availability, catalog, quote) carries a name,
// an email, a phone number or a customer id; the customer's own details
// come back only through the session token (and the manage token).
func TestPublicAPI_NoCustomerData(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		c := newServer(t, e, 100)

		// A desk booking with full contact details on spot A1 (confirmed).
		in := e.F.Input(e.F.Spots[0], "2027-07-10", "2027-07-10")
		in.Customer.FirstName, in.Customer.LastName = "Giulia", "Barbieri"
		in.Customer.Email, in.Customer.Phone = "giulia.barbieri@example.org", "+393330000021"
		desk := e.F.MustCreate(in)
		// An online booking confirmed through the public flow on spot A2.
		s := e.Create(e.Seat(e.F.Spots[1], "2027-07-10"))
		st, err := e.Svc.UpdateCustomer(e.Ctx, checkouttest.Slug, s.BookingID, s.Token, checkout.CustomerInput{
			FirstName: "Luca", LastName: "Verdi", Email: "luca.verdi@example.org", Phone: "+39 340 1234567", Locale: "it",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Svc.Pay(e.Ctx, checkouttest.Slug, s.BookingID, s.Token, checkout.PayInput{}); err != nil {
			t.Fatal(err)
		}
		secrets := []string{
			"Giulia", "Barbieri", "giulia.barbieri", "3330000021", desk.Customer.ID, desk.Booking.Code,
			"Luca", "Verdi", "luca.verdi", "3401234567", st.Booking.Customer.ID, s.Token,
		}
		sel := map[string]any{
			"starts_on": "2027-07-10", "ends_on": "2027-07-10", "slots": "FULL", "formula": "day",
			"lines": []map[string]any{{"pack_id": e.F.Base.ID, "spot_id": e.F.Spots[2].ID}},
		}
		reads := []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, "", nil},
			{http.MethodGet, "/calendar?month=2027-07", nil},
			{http.MethodGet, "/maps/" + e.F.Spots[0].MapID + "?from=2027-07-10&to=2027-07-10&slots=FULL", nil},
			{http.MethodGet, "/availability?from=2027-07-10&to=2027-07-10&slots=FULL", nil},
			{http.MethodGet, "/catalog?from=2027-07-10&to=2027-07-10&slots=FULL", nil},
			{http.MethodPost, "/quote", sel},
			{http.MethodPost, "/quote", map[string]any{
				"starts_on": "2027-07-10", "ends_on": "2027-07-10", "slots": "FULL", "formula": "day",
				"lines": []map[string]any{{"pack_id": e.F.Base.ID, "spot_id": e.F.Spots[0].ID}},
			}},
		}
		for _, rd := range reads {
			resp, _, raw := c.do(rd.method, rd.path, rd.body, nil)
			if resp.StatusCode != 200 {
				t.Fatalf("%s %s: %d %s", rd.method, rd.path, resp.StatusCode, raw)
			}
			body := strings.ToLower(string(raw))
			for _, sec := range secrets {
				if strings.Contains(body, strings.ToLower(sec)) {
					t.Fatalf("%s %s leaks %q: %s", rd.method, rd.path, sec, raw)
				}
			}
		}
		// Only the session token reads the customer, and only its own.
		_, body, raw := c.do(http.MethodGet, "/bookings/"+s.BookingID, nil, map[string]string{"X-Arenile-Checkout": s.Token})
		if body["customer"].(map[string]any)["email"] != "luca.verdi@example.org" {
			t.Fatalf("own details through the token: %s", raw)
		}
		if strings.Contains(strings.ToLower(string(raw)), "barbieri") {
			t.Fatalf("other customer in the session: %s", raw)
		}
		resp, _, raw := c.do(http.MethodGet, "/bookings/"+desk.Booking.ID, nil, map[string]string{"X-Arenile-Checkout": s.Token})
		if resp.StatusCode != 404 || strings.Contains(string(raw), "Barbieri") {
			t.Fatalf("desk booking through another session: %d %s", resp.StatusCode, raw)
		}
	})
}
