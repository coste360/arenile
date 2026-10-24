//go:build integration

package acceptance

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/payment/stripe/stripetest"
	"github.com/coste360/arenile/internal/http/api"
)

// M4 extension of the simulated day (spec 28, M4 acceptance "nessuna doppia
// vendita tra cassa e online in un test con carico parallelo"): the public
// page sells the same beach as the front desk. Online customers go through
// the public API only (hold -> details -> pay -> Stripe Checkout); the fake
// Stripe (stripetest, real signatures) plays the provider and the webhook
// is the only online confirmation path (INV-19). Phases:
//
//   - enableOnline: Stripe in live mode (fake), the gazebos become a zone-mode
//     zone (assignment at confirmation) with an online quota;
//   - onlineVsDesk: desk bookings and online checkouts race on one spot and
//     day: exactly one sale;
//   - onlineCheckouts: online customers with every outcome: paid (webhook,
//     delivered twice), abandoned, left to expire, payment left pending;
//   - onlineZoneBurst: online checkouts on the gazebo zone race with desk
//     bookings of its spots: never more sales than spots, the online quota
//     holds and every confirmed online line gets a distinct spot;
//   - onlineRefunds: refunds through the provider from the panel;
//   - onlineLatePayments (after the pending holds expired): Stripe reports
//     the payment of an expired booking: recorded and refunded in full.
//
// checkInvariants then covers the online bookings like any other (INV-01,
// INV-02/03, INV-20, INV-22/24, INV-18, no personal data in the outbox).

// slug of the demo establishment (app.DemoSlug).
const demoSlug = "lido-demo"

// onlineState is what the online customers did.
type onlineState struct {
	pub *apiClient // anonymous browser: public API only, no cookies

	sessions  map[string]onlineSession // booking id -> session
	paid      []string                 // confirmed through the webhook
	pending   []string                 // payment started, never completed
	zoneWon   []string                 // zone-mode bookings confirmed
	raceSpot  string
	webhooks  atomic.Int64 // deliveries (parallel in the zone phases)
	duplicate int
}

type onlineSession struct {
	id, token, providerRef string
	spot                   string
	from                   calendar.Date
	slots                  api.Slots
}

func init() {
	allowedRefusals["createBooking(online race)"] = []string{"spot_unavailable"}
	allowedRefusals["createBooking(zone race)"] = []string{"spot_unavailable", "pool_exhausted"}
	allowedRefusals["createPublicBooking"] = []string{"spot_unavailable", "pool_exhausted", "quota_exceeded", "policy_violation", "no_price"}
}

func (d *day) pubPath(suffix string) string { return "/api/public/v1/" + demoSlug + suffix }

// enableOnline configures the online channel through the admin API.
func (d *day) enableOnline() {
	d.on.pub, d.on.sessions = newClient(d.t, d.e.srv.URL), map[string]onlineSession{}
	r := d.c.must(d.c.get("getPaymentSettings", d.e.path("/payment-settings")), "payment settings")
	sk, wh, live := stripetest.LiveSecretKey, stripetest.WebhookSecret, api.Live
	d.c.must(d.c.do("updatePaymentSettings", http.MethodPut, d.e.path("/payment-settings"), api.PaymentSettingsInput{
		Provider: "stripe", Mode: live, Stripe: &api.StripeSettingsInput{SecretKey: &sk, WebhookSecret: &wh},
	}, map[string]string{"If-Match": r.ETag}), "stripe in live mode (fake; real bookings never pay with test keys, INV-88)")

	z := d.e.gazeboZone
	r = d.c.must(d.c.get("getZone", d.e.path("/zones/"+z.ID)), "gazebo zone")
	zone := decode[api.Zone](d.t, r)
	auto, visible := true, true
	quota := api.OnlineQuota{Type: api.OnlineQuotaTypeCount, Value: len(d.e.gazeboSpots) - 1}
	r = d.c.must(d.c.do("updateZone", http.MethodPut, d.e.path("/zones/"+z.ID), api.ZoneInput{
		AssignmentMode: api.AssignmentMode("zone"), AutoAssign: &auto, Code: zone.Code, MapId: zone.MapId,
		Name: zone.Name, OnlineQuota: &quota, OnlineVisible: &visible, PriceCategory: zone.PriceCategory, SortOrder: &zone.SortOrder,
	}, map[string]string{"If-Match": r.ETag}), "gazebos as a zone-mode zone")
	if got := decode[api.Zone](d.t, r); string(got.AssignmentMode) != "zone" || got.OnlineQuota == nil {
		d.t.Fatalf("gazebo zone not switched: %+v", got)
	}
	// The lido now sells gazebos online without asking for its approval:
	// the pack flag and the template policy "Gazebo: approvazione del
	// gestore" are turned off.
	pk := d.c.must(d.c.get("getPack", d.e.path("/packs/"+d.e.gazeboPack.ID)), "gazebo pack")
	pack := decode[api.Pack](d.t, pk)
	no, active := false, true
	d.c.must(d.c.do("updatePack", http.MethodPut, d.e.path("/packs/"+pack.Id), api.PackInput{
		Active: &active, AllowedFormulas: pack.AllowedFormulas, AllowedZoneIds: pack.AllowedZoneIds, Channels: pack.Channels,
		Code: pack.Code, Composition: pack.Composition, Description: &pack.Description, Image: pack.Image,
		MaxPerBooking: &pack.MaxPerBooking, Name: pack.Name, RequiresApproval: &no, RequiresSpotKind: pack.RequiresSpotKind,
		SortOrder: &pack.SortOrder, VatRateBp: pack.VatRateBp,
	}, map[string]string{"If-Match": pk.ETag}), "gazebo pack without approval")
	pl := d.c.must(d.c.get("listPolicies", d.e.path("/policies?season_id="+d.e.season.ID)), "policies")
	for _, p := range decode[api.BookingPolicyList](d.t, pl).Items {
		if p.Rules.ApprovalRequired == nil || !*p.Rules.ApprovalRequired || !p.Active {
			continue
		}
		off, prio := false, p.Priority
		d.c.must(d.c.do("updatePolicy", http.MethodPut, d.e.path("/policies/"+p.Id), api.BookingPolicyInput{
			Active: &off, Name: p.Name, Priority: &prio, Rules: p.Rules, Scope: p.Scope, SeasonId: p.SeasonId,
		}, ifMatch(p.Version)), "approval policy off")
	}
	prof := d.on.pub.must(d.on.pub.get("getPublicProfile", d.pubPath("")), "public profile")
	if p := decode[api.PublicEstablishment](d.t, prof); p.Online == nil || !p.Online.Payment.Online {
		d.t.Fatalf("online payment not active on the public profile: %s", prof)
	}
}

// publicCreate starts a checkout (hold) for one line.
func (d *day) publicCreate(line api.PublicLineInput, from calendar.Date, slots api.Slots) (onlineSession, result) {
	it := "it"
	r := d.on.pub.post("createPublicBooking", d.pubPath("/bookings"), api.PublicBookingCreate{
		StartsOn: from, EndsOn: from, Slots: slots, Formula: formulaOf(from, from, slots), Lines: []api.PublicLineInput{line}, Locale: &it,
	})
	if !r.ok() {
		return onlineSession{}, r
	}
	cs := decode[api.CheckoutSession](d.t, r)
	if cs.Status != api.BookingStatusHeld || cs.CheckoutToken == "" {
		d.t.Errorf("checkout session %+v", cs)
	}
	s := onlineSession{id: cs.BookingId, token: cs.CheckoutToken, from: from, slots: slots}
	if line.SpotId != nil {
		s.spot = *line.SpotId
	}
	return s, r
}

func (d *day) checkoutHdr(s onlineSession) map[string]string {
	return map[string]string{"X-Arenile-Checkout": s.token}
}

// publicDetails sends the customer step (fictitious names, tracked as
// personal data that must never reach events).
func (d *day) publicDetails(s onlineSession, n int) {
	first, last := fmt.Sprintf("Onlina%d", n), fmt.Sprintf("Cliente%d", n)
	email := fmt.Sprintf("online.%d@mailbox.test", n)
	d.personalData = append(d.personalData, last, email)
	d.on.pub.must(d.on.pub.do("updatePublicBooking", http.MethodPatch, d.pubPath("/bookings/"+s.id), api.CheckoutCustomerInput{
		FirstName: first, LastName: last, Email: openapi_types.Email(email), AcceptTermsVersion: "", MarketingConsent: n%3 == 0,
	}, d.checkoutHdr(s)), "customer details")
}

// publicPay starts the Stripe payment and returns the session with its
// Checkout Session id.
func (d *day) publicPay(s onlineSession) onlineSession {
	r := d.on.pub.must(d.on.pub.do("payPublicBooking", http.MethodPost, d.pubPath("/bookings/"+s.id+"/pay"), api.CheckoutPayRequest{}, d.checkoutHdr(s)), "pay")
	pr := decode[api.PayResult](d.t, r)
	if pr.Mode != api.PayResultModeRedirect || pr.RedirectUrl == nil {
		d.t.Fatalf("pay %s: %+v", s.id, pr)
	}
	s.providerRef = (*pr.RedirectUrl)[strings.LastIndex(*pr.RedirectUrl, "/")+1:]
	if d.e.stripe.Session(s.providerRef) == nil {
		d.t.Fatalf("pay %s: unknown checkout session %q", s.id, s.providerRef)
	}
	d.on.sessions[s.id] = s
	return s
}

// webhook completes a Checkout Session at the fake Stripe and delivers the
// signed checkout.session.completed event (eventID repeats = redelivery).
func (d *day) webhook(s onlineSession, eventID string) result {
	d.e.stripe.Pay(s.providerRef)
	now := d.e.clock.Now()
	payload := d.e.stripe.SessionEvent(eventID, "checkout.session.completed", s.providerRef, now)
	req, err := http.NewRequest(http.MethodPost, d.e.srv.URL+"/webhooks/payments/stripe/"+d.e.estID, bytes.NewReader(payload))
	if err != nil {
		d.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", stripetest.Sign(payload, now, stripetest.WebhookSecret))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result{Status: -1, Body: []byte(err.Error())}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	d.on.webhooks.Add(1)
	return result{Status: resp.StatusCode, Body: raw}
}

// trackOnline reads an online booking through the admin API into the
// scenario state (so later phases and the invariants see it).
func (d *day) trackOnline(id string) *trackedBooking {
	r := d.c.must(d.c.get("getBooking", d.e.path("/bookings/"+id)), "get online booking")
	return d.track(decode[api.BookingDetail](d.t, r))
}

// confirmOnline completes the payment of s through the webhook and checks
// that the booking is confirmed (the status page only polls).
func (d *day) confirmOnline(s onlineSession) *trackedBooking {
	st := d.on.pub.must(d.on.pub.do("getPublicBooking", http.MethodGet, d.pubPath("/bookings/"+s.id), nil, d.checkoutHdr(s)), "status before the webhook")
	if decode[api.CheckoutState](d.t, st).Status == api.BookingStatusConfirmed {
		d.t.Fatalf("booking %s confirmed before its webhook", s.id)
	}
	if r := d.webhook(s, "evt_"+s.providerRef); r.Status != http.StatusOK {
		d.t.Fatalf("webhook of %s: %s", s.id, r)
	}
	b := d.trackOnline(s.id)
	if b.status != api.BookingStatusConfirmed || b.paid != b.total {
		d.t.Fatalf("online booking %s after the webhook: %s paid %d/%d", s.id, b.status, b.paid, b.total)
	}
	d.on.paid = append(d.on.paid, s.id)
	return b
}

// onlineVsDesk: desk and online race on one spot and day (n attempts,
// alternating): exactly one sale (INV-01, the DB decides), whoever wins.
func (d *day) onlineVsDesk(n int) {
	from := d.date(2)
	spot := d.pickSpot(from, from, api.SlotsFULL)
	if spot == "" {
		d.t.Fatal("no free spot for the online race")
	}
	d.on.raceSpot = spot
	type outcome struct {
		desk *api.BookingDetail
		on   onlineSession
		r    result
	}
	out := make([]outcome, n)
	deskIn := d.bookingInput(spot, from, from, api.SlotsFULL) // the rng stays on this goroutine
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				r := d.c.post("createBooking(online race)", d.e.path("/bookings"), deskIn)
				out[i].r = r
				if r.ok() {
					bd := decode[api.BookingDetail](d.t, r)
					out[i].desk = &bd
				}
				return
			}
			sp := spot
			s, r := d.publicCreate(api.PublicLineInput{PackId: d.e.basePack.ID, SpotId: &sp}, from, api.SlotsFULL)
			out[i] = outcome{on: s, r: r}
		}(i)
	}
	wg.Wait()
	winners := 0
	var online *onlineSession
	for i, o := range out {
		switch {
		case o.r.ok():
			winners++
			if o.desk != nil {
				d.track(*o.desk)
			} else {
				s := o.on
				online = &s
			}
		case i%2 == 0:
			d.refused("createBooking(online race)", o.r)
		default:
			d.refused("createPublicBooking", o.r)
		}
	}
	d.c.failIfNotImplemented()
	if winners != 1 {
		d.t.Fatalf("desk vs online on one spot: %d sales, want exactly 1", winners)
	}
	if online != nil {
		d.publicDetails(*online, 0)
		d.confirmOnline(d.publicPay(*online))
	}
	d.mark(spot, from, from, api.SlotsFULL, true)
	d.t.Logf("online vs desk: 1 sale out of %d attempts (won by %s)", n, map[bool]string{true: "online", false: "desk"}[online != nil])
}

// onlineCheckouts plays n online customers with every outcome.
func (d *day) onlineCheckouts(n int) {
	for i := range n {
		from := d.date(1 + d.rng.IntN(15))
		slots := api.SlotsFULL
		spot := d.pickSpot(from, from, slots)
		if spot == "" {
			continue
		}
		sp := spot
		s, r := d.publicCreate(api.PublicLineInput{PackId: d.e.basePack.ID, SpotId: &sp}, from, slots)
		if !r.ok() {
			d.refused("createPublicBooking", r)
			continue
		}
		d.mark(spot, from, from, slots, true)
		switch i % 6 {
		case 0, 1, 2: // paid; the provider redelivers the event once
			d.publicDetails(s, i+1)
			s = d.publicPay(s)
			d.confirmOnline(s)
			if i%2 == 0 {
				if r := d.webhook(s, "evt_"+s.providerRef); r.Status != http.StatusOK {
					d.t.Fatalf("duplicate webhook: %s", r)
				}
				d.on.duplicate++
				if b := d.trackOnline(s.id); b.paid != b.total {
					d.t.Fatalf("duplicate webhook changed the payment of %s: paid %d total %d", s.id, b.paid, b.total)
				}
			}
		case 3: // back button: the hold is released at once
			d.on.pub.must(d.on.pub.do("abandonPublicBooking", http.MethodDelete, d.pubPath("/bookings/"+s.id), nil, d.checkoutHdr(s)), "abandon")
			b := d.trackOnline(s.id)
			if b.status != api.BookingStatusExpired {
				d.t.Fatalf("abandoned checkout %s: %s", s.id, b.status)
			}
			d.mark(spot, from, from, slots, false)
		case 4: // closes the tab on the details step: the hold expires
			d.publicDetails(s, i+1)
			d.trackOnline(s.id)
			d.expectExpired = append(d.expectExpired, s.id)
		case 5: // reaches Stripe and never pays (hold extended for the payment)
			d.publicDetails(s, i+1)
			s = d.publicPay(s)
			if b := d.trackOnline(s.id); b.status != api.BookingStatusPendingPayment {
				d.t.Fatalf("payment started: %s", b.status)
			}
			d.on.pending = append(d.on.pending, s.id)
		}
	}
	d.c.failIfNotImplemented()
}

// onlineZoneBurst: online checkouts on the gazebo zone (zone mode, online
// quota = spots - 1) race with desk bookings of each gazebo spot on one day.
// The winners online are confirmed in parallel through their webhooks:
// each gets its own spot at confirmation (auto-assignment, spec 7.5).
func (d *day) onlineZoneBurst(n int) {
	from := d.date(6)
	spots := d.e.gazeboSpots
	quota := len(spots) - 1
	type outcome struct {
		desk *api.BookingDetail
		on   onlineSession
		r    result
	}
	out := make([]outcome, n)
	customers := make([]string, n)
	for i := range customers {
		customers[i] = d.customer() // the rng stays on this goroutine
	}
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%3 == 0 {
				sp := spots[(i/3)%len(spots)].ID
				cust := customers[i]
				force := true
				zone := d.e.gazeboZone.ID
				r := d.c.post("createBooking(zone race)", d.e.path("/bookings"), api.BookingCreate{
					CustomerId: &cust, StartsOn: from, EndsOn: from, Slots: api.SlotsFULL, Formula: api.FormulaDay,
					Lines: []api.BookingLineInput{{PackId: d.e.gazeboPack.ID, SpotId: &sp, ZoneId: &zone}}, Force: &force,
				})
				out[i].r = r
				if r.ok() {
					bd := decode[api.BookingDetail](d.t, r)
					out[i].desk = &bd
				}
				return
			}
			zone := d.e.gazeboZone.ID
			s, r := d.publicCreate(api.PublicLineInput{PackId: d.e.gazeboPack.ID, ZoneId: &zone}, from, api.SlotsFULL)
			out[i] = outcome{on: s, r: r}
		}(i)
	}
	wg.Wait()
	var desk, online int
	var won []onlineSession
	for i, o := range out {
		switch {
		case o.r.ok() && o.desk != nil:
			desk++
			d.track(*o.desk)
		case o.r.ok():
			online++
			won = append(won, o.on)
		case i%3 == 0:
			d.refused("createBooking(zone race)", o.r)
		default:
			d.refused("createPublicBooking", o.r)
		}
	}
	d.c.failIfNotImplemented()
	if online > quota || desk+online > len(spots) {
		d.t.Fatalf("zone race: %d desk + %d online sales for %d gazebos (online quota %d)", desk, online, len(spots), quota)
	}
	for i, s := range won {
		d.publicDetails(s, 1000+i)
		won[i] = d.publicPay(s)
	}
	errs := make([]error, len(won))
	for i, s := range won {
		wg.Add(1)
		go func(i int, s onlineSession) {
			defer wg.Done()
			if r := d.webhook(s, "evt_"+s.providerRef); r.Status != http.StatusOK {
				errs[i] = fmt.Errorf("webhook %s: %s", s.id, r)
			}
		}(i, s)
	}
	wg.Wait()
	assigned := map[string]string{}
	for _, b := range d.withStatus(api.BookingStatusConfirmed) {
		if b.from.Equal(from) && b.spot != "" {
			for _, sp := range spots {
				if sp.ID == b.spot {
					assigned[b.spot] = b.id
				}
			}
		}
	}
	for i, s := range won {
		if errs[i] != nil {
			d.t.Fatal(errs[i])
		}
		b := d.trackOnline(s.id)
		if b.status != api.BookingStatusConfirmed || b.spot == "" {
			d.t.Fatalf("zone booking %s after its webhook: %s, spot %q", s.id, b.status, b.spot)
		}
		if other, dup := assigned[b.spot]; dup && other != b.id {
			d.t.Fatalf("gazebo %s sold twice: %s and %s", b.spot, other, b.id)
		}
		assigned[b.spot] = b.id
		d.on.zoneWon = append(d.on.zoneWon, s.id)
	}
	d.t.Logf("zone race on %d gazebos: %d desk + %d online sales (quota %d) out of %d attempts, %d distinct spots", len(spots), desk, online, quota, n, len(assigned))
}

// onlineZoneQuota: only online customers on the gazebo zone (n parallel
// checkouts on one day): exactly the online quota is sold, the winners are
// confirmed in parallel and get distinct spots; then the desk sells every
// gazebo in parallel and gets exactly the ones left (no double sale across
// channels, spec 7.5 and 7.6).
func (d *day) onlineZoneQuota(n int) {
	from := d.date(7)
	spots := d.e.gazeboSpots
	quota := len(spots) - 1
	zone := d.e.gazeboZone.ID
	sessions := make([]onlineSession, n)
	results := make([]result, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sessions[i], results[i] = d.publicCreate(api.PublicLineInput{PackId: d.e.gazeboPack.ID, ZoneId: &zone}, from, api.SlotsFULL)
		}(i)
	}
	wg.Wait()
	var won []onlineSession
	for i, r := range results {
		if r.ok() {
			won = append(won, sessions[i])
			continue
		}
		d.refused("createPublicBooking", r)
	}
	if len(won) != quota {
		d.t.Fatalf("online only on the gazebo zone: %d sales, want the online quota %d", len(won), quota)
	}
	for i, s := range won {
		d.publicDetails(s, 2000+i)
		won[i] = d.publicPay(s)
	}
	errs := make([]error, len(won))
	for i, s := range won {
		wg.Add(1)
		go func(i int, s onlineSession) {
			defer wg.Done()
			if r := d.webhook(s, "evt_"+s.providerRef); r.Status != http.StatusOK {
				errs[i] = fmt.Errorf("webhook %s: %s", s.id, r)
			}
		}(i, s)
	}
	wg.Wait()
	taken := map[string]string{}
	for i, s := range won {
		if errs[i] != nil {
			d.t.Fatal(errs[i])
		}
		b := d.trackOnline(s.id)
		if b.status != api.BookingStatusConfirmed || b.spot == "" {
			d.t.Fatalf("zone booking %s after its webhook: %s, spot %q", s.id, b.status, b.spot)
		}
		if other, dup := taken[b.spot]; dup {
			d.t.Fatalf("auto-assignment gave gazebo %s to %s and %s", b.spot, other, b.id)
		}
		taken[b.spot] = b.id
		d.on.zoneWon = append(d.on.zoneWon, s.id)
	}
	deskResults := make([]result, len(spots)*3)
	customers := make([]string, len(deskResults))
	for i := range customers {
		customers[i] = d.customer()
	}
	for i := range deskResults {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sp, cust, force := spots[i%len(spots)].ID, customers[i], true
			deskResults[i] = d.c.post("createBooking(zone race)", d.e.path("/bookings"), api.BookingCreate{
				CustomerId: &cust, StartsOn: from, EndsOn: from, Slots: api.SlotsFULL, Formula: api.FormulaDay,
				Lines: []api.BookingLineInput{{PackId: d.e.gazeboPack.ID, SpotId: &sp, ZoneId: &zone}}, Force: &force,
			})
		}(i)
	}
	wg.Wait()
	desk := 0
	for _, r := range deskResults {
		if !r.ok() {
			d.refused("createBooking(zone race)", r)
			continue
		}
		desk++
		b := d.track(decode[api.BookingDetail](d.t, r))
		if other, dup := taken[b.spot]; dup {
			d.t.Fatalf("desk sold gazebo %s already assigned online to %s", b.spot, other)
		}
		taken[b.spot] = b.id
	}
	if desk != len(spots)-quota {
		d.t.Fatalf("desk after the online quota: %d gazebos sold, want %d", desk, len(spots)-quota)
	}
	d.t.Logf("online quota on %d gazebos: %d online sales out of %d attempts confirmed in parallel on distinct spots, then %d desk sale(s)", len(spots), quota, n, desk)
}

// onlineRefunds refunds n paid online bookings through the provider from
// the panel (spec 13.4): pending until Stripe confirms, then in the ledger.
func (d *day) onlineRefunds(n int) {
	done := 0
	for _, id := range d.on.paid {
		if done == n {
			break
		}
		b := d.bookings[id]
		d.refresh(b)
		if b.status != api.BookingStatusConfirmed || b.paid <= 0 {
			continue
		}
		r := d.c.must(d.c.get("listBookingPayments", d.e.path("/bookings/"+id+"/payments")), "payments")
		list := decode[struct {
			Items []api.Payment `json:"items"`
		}](d.t, r)
		var pay *api.Payment
		for i := range list.Items {
			if list.Items[i].Provider == "stripe" && list.Items[i].Direction == api.PaymentDirectionIn {
				pay = &list.Items[i]
			}
		}
		if pay == nil {
			d.t.Fatalf("online booking %s without its stripe payment", id)
		}
		via := true
		amount := b.paid / 2
		r = d.c.post("recordBookingRefund(provider)", d.e.path("/bookings/"+id+"/refunds"), api.RefundInput{
			Amount: amount, Method: pay.Method, PaymentId: &pay.Id, ViaProvider: &via,
		})
		d.c.must(r, "refund via provider")
		d.e.runJobs()
		d.refresh(b)
		if b.paid != b.total-amount {
			d.t.Fatalf("provider refund of %s not in the ledger: paid %d, want %d", id, b.paid, b.total-amount)
		}
		done++
	}
	if done < n {
		d.t.Fatalf("refunded %d online bookings, want %d", done, n)
	}
}

// onlineLatePayments: the payments left pending expire with their hold
// (the clock passes the 30 minutes of the payment hold); then Stripe
// reports half of them as paid: the money is recorded and refunded in full
// (ADR 0014), the booking stays expired and its spot free.
func (d *day) onlineLatePayments() {
	d.e.clock.Advance(31 * time.Minute)
	d.e.runJobs()
	for i, id := range d.on.pending {
		b := d.trackOnline(id)
		if b.status != api.BookingStatusExpired {
			d.t.Fatalf("pending payment %s after 31 minutes: %s", id, b.status)
		}
		d.mark(b.spot, b.from, b.to, b.slots, false)
		if i%2 == 1 {
			continue
		}
		if r := d.webhook(d.on.sessions[id], "evt_late_"+id); r.Status != http.StatusOK {
			d.t.Fatalf("late webhook %s: %s", id, r)
		}
	}
	d.e.runJobs()
	for i, id := range d.on.pending {
		if i%2 == 1 {
			continue
		}
		b := d.trackOnline(id)
		if b.status != api.BookingStatusExpired || b.paid != 0 {
			d.t.Fatalf("late payment of %s: %s, paid %d (want expired, refunded)", id, b.status, b.paid)
		}
		if refunds := d.e.stripe.RefundsOf(d.e.stripe.Session(d.on.sessions[id].providerRef).PaymentIntent); len(refunds) != 1 {
			d.t.Fatalf("late payment of %s: %d provider refunds, want 1", id, len(refunds))
		}
	}
}

// onlineStats reports the online part of the day.
func (d *day) onlineStats() string {
	return fmt.Sprintf("online: %d accepted public calls, %d paid through the webhook (%d redelivered), %d zone sales, %d pending then expired, %d webhooks",
		d.on.pub.mutations.Load(), len(d.on.paid), d.on.duplicate, len(d.on.zoneWon), len(d.on.pending), d.on.webhooks.Load())
}
