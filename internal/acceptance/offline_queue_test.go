//go:build integration

package acceptance

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/http/api"
)

// Offline queue of the mobile app (spec 21.2, 29.2, M7 acceptance): the app
// queues check-in, day releases of season passes, spot blocks, booking
// notes, cash collections and gate check-ins while offline and replays them
// on reconnection with the Idempotency-Key it chose when the operation was
// queued, authenticated with its device token. A replay must return the
// original response and never repeat the effect.

// deviceClient is a paired app: device token, no cookies, no CSRF.
type deviceClient struct {
	t     *testing.T
	base  string
	token string
}

// deviceResult is one exchange of the app.
type deviceResult struct {
	result
	Replayed bool
}

func (c deviceClient) do(method, path string, in any, hdr map[string]string) deviceResult {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return deviceResult{result: result{Status: -1, Body: []byte(err.Error())}}
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return deviceResult{result: result{Status: -1, Body: []byte(err.Error())}}
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return deviceResult{result: result{Status: -1, Body: []byte(err.Error())}}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	r := deviceResult{result: result{Status: resp.StatusCode, Body: raw, ETag: resp.Header.Get("ETag")}, Replayed: resp.Header.Get("Idempotent-Replayed") == "true"}
	if strings.Contains(resp.Header.Get("Content-Type"), "problem+json") {
		_ = json.Unmarshal(raw, &r.Problem)
	}
	return r
}

// queued sends a queued operation with its key (and optional headers).
func (c deviceClient) queued(path, key string, in any, extra map[string]string) deviceResult {
	hdr := map[string]string{"Idempotency-Key": key}
	for k, v := range extra {
		hdr[k] = v
	}
	return c.do(http.MethodPost, path, in, hdr)
}

// pairDevice pairs an app for the panel user of c (spec 29.2) and returns it.
func pairDevice(t *testing.T, e *env, c *apiClient) deviceClient {
	t.Helper()
	kind := api.DeviceKindMobile
	r := c.must(c.post("createDevicePairingCode", e.path("/devices/pairing-codes"), api.DevicePairingCodeRequest{Kind: &kind}), "pairing code")
	code := decode[api.DevicePairingCode](t, r).Code
	app := newClient(t, e.srv.URL)
	r = app.must(app.post("pairDevice", "/api/device/v1/pair", api.DevicePairRequest{
		Code: code, Name: "Telefono della spiaggia", Platform: api.DevicePlatformAndroid, Kind: api.DeviceKindMobile,
	}), "pair device")
	return deviceClient{t: t, base: e.srv.URL, token: decode[api.DevicePairResult](t, r).Token}
}

// offlineWorld is what the queued operations act on.
type offlineWorld struct {
	panel    *apiClient
	app      deviceClient
	cash     string
	customer string
}

func newOfflineWorld(t *testing.T, e *env) *offlineWorld {
	t.Helper()
	panel := newClient(t, e.srv.URL)
	panel.login(ownerEmail, ownerPassword)
	r := panel.must(panel.post("openCashSession", e.path("/cash-sessions"), api.CashSessionOpen{OpeningFloat: 10000}), "open cash")
	cash := decode[api.CashSession](t, r).Id
	email := openapi_types.Email("coda.offline@example.com")
	r = panel.must(panel.post("createCustomer", e.path("/customers"), api.CustomerInput{LastName: "Coda", Email: &email}), "create customer")
	return &offlineWorld{panel: panel, app: pairDevice(t, e, panel), cash: cash, customer: decode[api.Customer](t, r).Id}
}

// booking creates a confirmed desk booking of today on spot.
func (w *offlineWorld) booking(t *testing.T, e *env, spot string) api.BookingDetail {
	t.Helper()
	force := true
	r := w.panel.must(w.panel.post("createBooking", e.path("/bookings"), api.BookingCreate{
		CustomerId: &w.customer, StartsOn: e.today, EndsOn: e.today, Slots: api.SlotsFULL, Formula: api.FormulaDay,
		Lines: []api.BookingLineInput{{PackId: e.basePack.ID, SpotId: &spot}}, Force: &force,
	}), "create booking")
	b := decode[api.BookingDetail](t, r)
	if b.Status != api.BookingStatusConfirmed {
		t.Fatalf("desk booking is %s, want confirmed", b.Status)
	}
	return b
}

func (w *offlineWorld) getBooking(t *testing.T, e *env, id string) api.BookingDetail {
	t.Helper()
	return decode[api.BookingDetail](t, w.panel.must(w.panel.get("getBooking", e.path("/bookings/"+id)), "get booking"))
}

// replayTwice sends a queued operation, then replays it with the same key
// (the reconnection after a lost response): the replay is the stored
// response, byte for byte, marked Idempotent-Replayed.
func replayTwice(t *testing.T, app deviceClient, what, path, key string, in any, hdr map[string]string, want int) deviceResult {
	t.Helper()
	first := app.queued(path, key, in, hdr)
	if first.Status != want || first.Replayed {
		t.Fatalf("%s: first send %s (replayed %v), want %d", what, first.result, first.Replayed, want)
	}
	again := app.queued(path, key, in, hdr)
	if again.Status != first.Status || !bytes.Equal(again.Body, first.Body) || !again.Replayed {
		t.Fatalf("%s: replay %s (replayed %v), want the original %s", what, again.result, again.Replayed, first.result)
	}
	return first
}

// TestAcceptance_OfflineQueueReplay replays every operation of the offline
// queue of the app with its Idempotency-Key: same response, one effect.
// The gate check-in of entry tickets goes through the same middleware; its
// contract is guarded by TestOfflineQueueContract_IdempotencyKey.
func TestAcceptance_OfflineQueueReplay(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		w := newOfflineWorld(t, e)
		app := w.app

		// Check-in (If-Match of the cached version, as queued offline).
		b := w.booking(t, e, e.dailySpots[0].ID)
		checkIn := e.path("/bookings/" + b.Id + "/check-in")
		r := replayTwice(t, app, "check-in", checkIn, "queue-checkin-0001", api.BookingActionRequest{}, ifMatch(b.Version), http.StatusOK)
		if decode[api.BookingDetail](t, r.result).Status != api.BookingStatusCheckedIn {
			t.Fatalf("check-in: %s", r.result)
		}
		// A new key is a new operation: the transition is refused, proving
		// the replay did not run the handler again.
		if again := app.queued(checkIn, "queue-checkin-0002", api.BookingActionRequest{}, ifMatch(b.Version)); again.Status < 400 {
			t.Fatalf("second check-in with a new key: %s", again.result)
		}

		// Booking note: appended once.
		note := "Ombrellone da spostare (coda offline)"
		replayTwice(t, app, "note", e.path("/bookings/"+b.Id+"/notes"), "queue-note-00001", api.BookingNoteInput{Text: note}, nil, http.StatusOK)
		if got := w.getBooking(t, e, b.Id).NotesInternal; strings.Count(got, note) != 1 {
			t.Fatalf("note appended %d times: %q", strings.Count(got, note), got)
		}
		// The same key with another body is refused, never replayed.
		reused := app.queued(e.path("/bookings/"+b.Id+"/notes"), "queue-note-00001", api.BookingNoteInput{Text: "altro"}, nil)
		if reused.Status != http.StatusUnprocessableEntity || reused.Problem.Type != "idempotency_key_reused" {
			t.Fatalf("reused key: %s", reused.result)
		}

		// Cash collection on an existing booking: one payment.
		pb := w.booking(t, e, e.dailySpots[1].ID)
		cash := w.cash
		r = replayTwice(t, app, "cash payment", e.path("/bookings/"+pb.Id+"/payments"), "queue-pay-000001",
			api.ManualPaymentInput{Amount: 1000, Method: api.PaymentMethodCash, CashSessionId: &cash}, nil, http.StatusCreated)
		payment := decode[api.Payment](t, r.result)
		if got := w.getBooking(t, e, pb.Id); len(got.Payments) != 1 || got.Payments[0].Id != payment.Id || got.PaidAmount != 1000 {
			t.Fatalf("payments after replay: %d paid %d", len(got.Payments), got.PaidAmount)
		}

		// Spot block: one block.
		spot := e.dailySpots[2].ID
		replayTwice(t, app, "spot block", e.path("/spot-blocks"), "queue-block-00001", api.SpotBlockCreate{
			SpotIds: []string{spot}, StartsOn: e.today.AddDays(1), EndsOn: e.today.AddDays(1), Slots: api.SlotsFULL,
		}, nil, http.StatusCreated)
		blocks := decode[api.SpotBlockList](t, w.panel.must(w.panel.get("listSpotBlocks", e.path("/spot-blocks?spot_id="+spot)), "list blocks"))
		if len(blocks.Items) != 1 {
			t.Fatalf("blocks after replay: %d", len(blocks.Items))
		}

		// Day release of a season pass: released once.
		pr := w.panel.must(w.panel.post("createSeasonPass", e.path("/season-passes"), api.SeasonPassCreate{
			CustomerId: w.customer, PackId: e.basePack.ID, SeasonId: e.season.ID, SpotId: e.passSpots[0].ID,
		}), "create season pass")
		pass := decode[api.SeasonPassDetail](t, pr).Pass
		full := api.SlotsFULL
		releases := e.path("/season-passes/" + pass.Id + "/releases")
		r = replayTwice(t, app, "release", releases, "queue-release-001", api.SeasonReleaseCreate{Dates: []calendar.Date{e.today.AddDays(2)}, Slots: &full}, nil, http.StatusCreated)
		created := decode[api.SeasonReleaseList](t, r.result).Items
		rel := decode[api.SeasonReleaseList](t, w.panel.must(w.panel.get("listSeasonPassReleases", releases), "list releases"))
		if len(created) == 0 || len(rel.Items) != len(created) {
			t.Fatalf("releases after replay: %d, created by the first send: %d", len(rel.Items), len(created))
		}
	})
}

// TestConcurrency_OfflineQueueReplay: the app (or two retries racing after
// a reconnection) sends the same queued note and cash collection 100 times
// in parallel with one key: the effect happens once; every answer is the
// stored response or 409 idempotency_in_progress (to retry).
func TestConcurrency_OfflineQueueReplay(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		w := newOfflineWorld(t, e)
		b := w.booking(t, e, e.dailySpots[0].ID)
		cash := w.cash
		note := "Nota inviata cento volte"
		ops := []struct {
			name, path, key string
			in              any
			want            int
		}{
			{"note", e.path("/bookings/" + b.Id + "/notes"), "burst-note-000001", api.BookingNoteInput{Text: note}, http.StatusOK},
			{"payment", e.path("/bookings/" + b.Id + "/payments"), "burst-pay-0000001", api.ManualPaymentInput{Amount: 500, Method: api.PaymentMethodCash, CashSessionId: &cash}, http.StatusCreated},
		}
		for _, op := range ops {
			const n = 100
			results := make([]deviceResult, n)
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := range n {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					results[i] = w.app.queued(op.path, op.key, op.in, nil)
				}()
			}
			close(start)
			wg.Wait()
			var body []byte
			for _, r := range results {
				switch {
				case r.Status == op.want:
					if body == nil {
						body = r.Body
					} else if !bytes.Equal(body, r.Body) {
						t.Fatalf("%s: two different successful answers:\n%s\n%s", op.name, body, r.Body)
					}
				case r.Status == http.StatusConflict && r.Problem.Type == "idempotency_in_progress":
				default:
					t.Fatalf("%s: unexpected answer %s", op.name, r.result)
				}
			}
			if body == nil {
				t.Fatalf("%s: no request succeeded", op.name)
			}
		}
		got := w.getBooking(t, e, b.Id)
		if strings.Count(got.NotesInternal, note) != 1 {
			t.Fatalf("note appended %d times", strings.Count(got.NotesInternal, note))
		}
		if len(got.Payments) != 1 || got.PaidAmount != 500 {
			t.Fatalf("payments %d, paid %d: want one payment of 500", len(got.Payments), got.PaidAmount)
		}
	})
}
