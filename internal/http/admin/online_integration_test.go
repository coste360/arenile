//go:build integration

package admin_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	httpserver "github.com/coste360/arenile/internal/http"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// newOnlineEnv is newStEnv with the online settings service wired.
func newOnlineEnv(t *testing.T, db *store.DB) *stEnv {
	t.Helper()
	e := newStEnv(t, db)
	e.srv.Close()
	c := clock.System{}
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	as, err := auth.NewService(db, c, nil, aw, nil, auth.Options{Argon2: &stFastArgon})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := crypto.NewKeyRing(make([]byte, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	on := online.NewService(online.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Keys: keys, BaseURL: "https://prenota.example.org"})
	h := httpserver.New(httpserver.Deps{
		DB: db, Clock: c, Version: "test", Auth: as, Bus: event.NewBroker(nil, 16), Readiness: stReadiness{db},
		Establishments: establishment.NewService(db, establishment.WithWriteDeps(establishment.WriteDeps{Clock: c, Audit: aw, Outbox: ob})),
		Online:         on,
	})
	e.srv = httptest.NewServer(h)
	t.Cleanup(e.srv.Close)
	return e
}

func TestOnlineHTTP_SettingsAndTerms(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newOnlineEnv(t, db)
		manager, cashier, viewer, ownerB := e.as("manager@lido-a.it"), e.as("cashier@lido-a.it"), e.as("viewer@lido-a.it"), e.as("owner@lido-b.it")
		base := "/api/admin/v1/establishments/" + e.estA.ID

		resp, body := viewer.do(http.MethodGet, base+"/online-settings", nil, nil)
		stExpect(t, resp, body, 200, "")
		embed := body["embed"].(map[string]any)
		if resp.Header.Get("ETag") != `"1"` || embed["page_url"] != "https://prenota.example.org/b/lido-a" ||
			!strings.Contains(embed["script"].(string), `data-arenile="lido-a"`) || !strings.Contains(embed["script"].(string), "https://prenota.example.org/embed.js") {
			t.Fatalf("settings %v %v", resp.Header, body)
		}
		resp, body = ownerB.do(http.MethodGet, base+"/online-settings", nil, nil)
		stExpect(t, resp, body, 404, "not_found")

		in := map[string]any{
			"booking_enabled": true, "allowed_embed_origins": []string{"https://www.lido-a.it/"},
			"theme": map[string]any{"primary_color": "#0E6BA8", "show_prices_on_map": true, "map_kit": "classic"},
			"flow":  map[string]any{"formulas": []string{"day", "week"}, "slots": []string{"FULL"}, "show_rate_periods": true, "promo_codes_enabled": true, "require_phone": true},
		}
		resp, body = cashier.do(http.MethodPut, base+"/online-settings", in, map[string]string{"If-Match": `"1"`})
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = manager.do(http.MethodPut, base+"/online-settings", in, map[string]string{"If-Match": `"1"`})
		stExpect(t, resp, body, 200, "")
		if resp.Header.Get("ETag") != `"2"` || body["allowed_embed_origins"].([]any)[0] != "https://www.lido-a.it" ||
			body["theme"].(map[string]any)["primary_color"] != "#0e6ba8" || body["flow"].(map[string]any)["require_phone"] != false {
			t.Fatalf("updated %v %v", resp.Header, body)
		}
		resp, body = manager.do(http.MethodPut, base+"/online-settings", in, map[string]string{"If-Match": `"1"`})
		stExpect(t, resp, body, 412, "version_conflict")
		in["allowed_embed_origins"] = []string{"http://insecure.example"}
		resp, body = manager.do(http.MethodPut, base+"/online-settings", in, map[string]string{"If-Match": `W/"2"`})
		stExpect(t, resp, body, 400, "validation_failed")
		// A theme without map_kit keeps the kit chosen in the establishment settings (ADR 0024).
		in["allowed_embed_origins"] = []string{"https://www.lido-a.it"}
		in["theme"] = map[string]any{"primary_color": "#0E6BA8"}
		resp, body = manager.do(http.MethodPut, base+"/online-settings", in, map[string]string{"If-Match": `"2"`})
		stExpect(t, resp, body, 200, "")
		if body["theme"].(map[string]any)["map_kit"] != "classic" {
			t.Fatalf("map kit lost: %v", body["theme"])
		}

		terms := map[string]any{"version": "2027.1", "texts": map[string]any{"it": map[string]any{"terms": "Condizioni", "marketing": "Newsletter"}}}
		resp, body = manager.do(http.MethodPost, base+"/terms-versions", terms, nil)
		stExpect(t, resp, body, 201, "")
		if body["current"] != true || body["texts"].(map[string]any)["it"].(map[string]any)["marketing"] != "Newsletter" {
			t.Fatalf("terms %v", body)
		}
		resp, body = manager.do(http.MethodPost, base+"/terms-versions", terms, nil)
		stExpect(t, resp, body, 409, "conflict")
		resp, body = viewer.do(http.MethodGet, base+"/terms-versions", nil, nil)
		stExpect(t, resp, body, 200, "")
		if items := body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["current"] != true {
			t.Fatalf("terms list %v", body)
		}
		resp, body = viewer.do(http.MethodGet, base+"/online-settings", nil, nil)
		if body["current_terms"].(map[string]any)["version"] != "2027.1" || resp.Header.Get("ETag") != `"4"` {
			t.Fatalf("current terms %v %v", resp.Header, body)
		}
		// The public profile publishes the online configuration.
		pub, err := http.Get(e.srv.URL + "/api/public/v1/lido-a")
		if err != nil {
			t.Fatal(err)
		}
		pub.Body.Close()
		if pub.StatusCode != 200 {
			t.Fatalf("public profile %d", pub.StatusCode)
		}
	})
}
