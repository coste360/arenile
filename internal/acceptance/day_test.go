//go:build integration

package acceptance

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
)

// minOperations is the M3 acceptance volume (spec 28).
const minOperations = 300

// TestAcceptance_SimulatedDay is the M3 acceptance criterion (spec 28): a
// simulated beach day of more than 300 operations through the admin API
// leaves allocations, bookings, payments, cash sessions and the
// corrispettivi export consistent, and the SSE stream carries the changes
// without personal data.
func TestAcceptance_SimulatedDay(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		seed := uint64(20270710)
		if v := os.Getenv(envSeed); v != "" {
			if n, err := strconv.ParseUint(v, 10, 64); err == nil {
				seed = n
			}
		}
		t.Logf("random seed %d (replay with %s=%d)", seed, envSeed, seed)

		d := newDay(t, e, seed)
		d.run()
		e.runJobs()

		ops := d.c.mutations.Load() + d.on.pub.mutations.Load()
		t.Logf("operations: %d mutations accepted (%d desk, %d public), %d refused by the server; business refusals: %v",
			ops, d.c.mutations.Load(), d.on.pub.mutations.Load(), d.c.refused.Load()+d.on.pub.refused.Load(), d.refusals)
		t.Log(d.onlineStats())
		if ops < minOperations {
			t.Fatalf("simulated day ran %d accepted operations, want >= %d", ops, minOperations)
		}
		checkInvariants(t, e, d)
		d.checkStream()
	})
}

// day is the scenario state: what the "front desk" did and expects.
type day struct {
	t   *testing.T
	e   *env
	c   *apiClient
	rng *rand.Rand
	sse *streamRecorder

	customers    []string
	personalData []string // names, emails and phones that must never reach the stream
	cash         string   // open cash session
	closedCash   string
	bookings     map[string]*trackedBooking
	order        []string
	occ          map[string]bool // spot|date|slot taken by the scenario (best effort, the server decides)
	holds        []trackedHold
	passes       []*trackedBooking
	refusals     map[string]int

	expectExpired   []string // held bookings left to expire
	expectCancelled []string
	expectCheckedIn []string
	burstWinners    int
	poolBurstOK     int
	poolBurstWant   int
	idemPayments    int

	on onlineState // M4: the public page sells the same beach (online_test.go)
}

type trackedBooking struct {
	id      string
	status  api.BookingStatus
	version int64
	total   int64
	paid    int64
	lineID  string
	spot    string
	from    calendar.Date
	to      calendar.Date
	slots   api.Slots
	cabin   bool
}

type trackedHold struct {
	id    string
	spot  string
	from  calendar.Date
	slots api.Slots
}

func newDay(t *testing.T, e *env, seed uint64) *day {
	c := newClient(t, e.srv.URL)
	c.login(ownerEmail, ownerPassword)
	d := &day{
		t: t, e: e, c: c, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		bookings: map[string]*trackedBooking{}, occ: map[string]bool{}, refusals: map[string]int{},
	}
	d.sse = listen(t, c, e.estID)
	return d
}

// run plays the day. Each phase is a block of front-desk work; the order
// follows a real morning: open the till, sell, hold, expire, modify,
// cancel, passes, bursts, check-in, close the till.
func (d *day) run() {
	d.setVAT(2200)
	d.openCash(20000)
	d.createCustomers(30)
	d.createDeskBookings(80)
	d.takePayments(50)
	d.holdsAndConversions(30, 10, 5)
	d.heldBookings(10, 5)
	d.burstSameSpot(100)
	d.burstPool(50)
	d.burstIdempotentPayment(10)
	d.enableOnline()
	d.onlineVsDesk(120)
	d.onlineCheckouts(36)
	d.onlineZoneBurst(60)
	d.onlineZoneQuota(40)
	d.expireHolds()
	d.rebookExpired(5)
	d.modifyBookings(15)
	d.moveBookings(5)
	d.cancelBookings(20)
	d.explicitRefunds(5)
	d.onlineRefunds(4)
	d.seasonPasses(5, 3)
	d.counterSales(10)
	d.cashMovements(4)
	d.checkIns(10)
	d.closeAndReopenCash()
	d.onlineLatePayments()
	d.readViews()
}

// ---- helpers -----------------------------------------------------------------

func (d *day) date(offset int) calendar.Date { return d.e.today.AddDays(offset) }

func slotsOf(s api.Slots) []string {
	if s == api.SlotsFULL {
		return []string{"AM", "PM"}
	}
	return []string{string(s)}
}

func (d *day) keys(spot string, from, to calendar.Date, slots api.Slots) []string {
	var out []string
	for dt := from; !dt.After(to); dt = dt.AddDays(1) {
		for _, s := range slotsOf(slots) {
			out = append(out, spot+"|"+dt.String()+"|"+s)
		}
	}
	return out
}

func (d *day) free(spot string, from, to calendar.Date, slots api.Slots) bool {
	for _, k := range d.keys(spot, from, to, slots) {
		if d.occ[k] {
			return false
		}
	}
	return true
}

func (d *day) mark(spot string, from, to calendar.Date, slots api.Slots, taken bool) {
	for _, k := range d.keys(spot, from, to, slots) {
		if taken {
			d.occ[k] = true
		} else {
			delete(d.occ, k)
		}
	}
}

// pickSpot returns a daily spot the scenario believes free.
func (d *day) pickSpot(from, to calendar.Date, slots api.Slots) string {
	n := len(d.e.dailySpots)
	start := d.rng.IntN(n)
	for k := range n {
		sp := d.e.dailySpots[(start+k)%n].ID
		if d.free(sp, from, to, slots) {
			return sp
		}
	}
	return ""
}

func (d *day) randomSlots() api.Slots {
	switch d.rng.IntN(6) {
	case 0:
		return api.SlotsAM
	case 1:
		return api.SlotsPM
	}
	return api.SlotsFULL
}

func formulaOf(from, to calendar.Date, slots api.Slots) api.Formula {
	switch {
	case from.Before(to):
		return api.FormulaMultiDay
	case slots != api.SlotsFULL:
		return api.FormulaHalfDay
	}
	return api.FormulaDay
}

// allowedRefusals lists, per scenario step, the problem types the server may
// legitimately answer: the scenario is random and the server decides, but a
// regression that turns another step into a refusal (a payment answering
// 409, a move answering 422...) must fail the day, not be counted as business.
var allowedRefusals = map[string][]string{
	"createBooking":                    {"spot_unavailable", "pool_exhausted", "quota_exceeded", "policy_violation", "no_price"},
	"createBooking(held)":              {"spot_unavailable", "pool_exhausted", "quota_exceeded", "policy_violation", "no_price"},
	"createBooking(burst)":             {"policy_violation"},
	"createBooking(pass spot)":         {"spot_unavailable"},
	"createHold":                       {"spot_unavailable", "pool_exhausted", "quota_exceeded"},
	"createHold(pool burst)":           {"quota_exceeded"},
	"recordBookingPayment":             {"validation_failed", "cash_session_closed", "conflict"},
	"recordBookingPayment(idempotent)": {"validation_failed", "idempotency_key_reused"},
	"updateBooking":                    {"spot_unavailable", "pool_exhausted", "quota_exceeded", "policy_violation", "version_conflict", "conflict", "no_price"},
	"moveBooking":                      {"spot_unavailable", "pool_exhausted", "quota_exceeded", "validation_failed", "conflict", "version_conflict"},
	"cancelBooking":                    {"invalid_transition", "conflict", "validation_failed", "version_conflict"},
	"recordBookingRefund":              {"validation_failed", "cash_session_closed"},
	"recordBookingRefund(over)":        {"validation_failed"},
	"checkInBooking":                   {"invalid_transition", "booking_outside_period", "version_conflict"},
}

// refused records an expected business refusal of a scenario step: only a
// 4xx problem type in allowedRefusals for that step. Any other answer stops
// the test; 501 answers are collected and fail it.
func (d *day) refused(op string, r result) {
	d.t.Helper()
	if r.Status == http.StatusNotImplemented {
		d.c.failIfNotImplemented()
		return
	}
	if r.Status >= 400 && r.Status < 500 && slices.Contains(allowedRefusals[op], r.Problem.Type) {
		d.refusals[op+":"+r.Problem.Type]++
		return
	}
	d.t.Fatalf("%s: unexpected %s", op, r)
}

func (d *day) track(bd api.BookingDetail) *trackedBooking {
	b := d.bookings[bd.Id]
	if b == nil {
		b = &trackedBooking{id: bd.Id}
		d.bookings[bd.Id] = b
		d.order = append(d.order, bd.Id)
	}
	b.status, b.version, b.total, b.paid = bd.Status, bd.Version, bd.TotalAmount, bd.PaidAmount
	b.from, b.to, b.slots = bd.StartsOn, bd.EndsOn, bd.Slots
	if len(bd.Lines) > 0 {
		b.lineID = bd.Lines[0].Id
		if bd.Lines[0].SpotId != nil {
			b.spot = *bd.Lines[0].SpotId
		}
	}
	return b
}

func (d *day) refresh(b *trackedBooking) {
	d.t.Helper()
	r := d.c.must(d.c.get("getBooking", d.e.path("/bookings/"+b.id)), "get booking")
	d.track(decode[api.BookingDetail](d.t, r))
}

// withStatus lists tracked bookings in creation order with one of the
// statuses.
func (d *day) withStatus(statuses ...api.BookingStatus) []*trackedBooking {
	var out []*trackedBooking
	for _, id := range d.order {
		b := d.bookings[id]
		for _, s := range statuses {
			if b.status == s {
				out = append(out, b)
				break
			}
		}
	}
	return out
}

func (d *day) customer() string { return d.customers[d.rng.IntN(len(d.customers))] }

func (d *day) bookingInput(spot string, from, to calendar.Date, slots api.Slots) api.BookingCreate {
	cust := d.customer()
	force := true
	return api.BookingCreate{
		CustomerId: &cust, StartsOn: from, EndsOn: to, Slots: slots, Formula: formulaOf(from, to, slots),
		Lines: []api.BookingLineInput{{PackId: d.e.basePack.ID, SpotId: &spot}}, Force: &force,
	}
}

func (d *day) createBooking(op string, in api.BookingCreate) (*trackedBooking, result) {
	d.t.Helper()
	r := d.c.post(op, d.e.path("/bookings"), in)
	if !r.ok() {
		return nil, r
	}
	b := d.track(decode[api.BookingDetail](d.t, r))
	if in.Extras != nil {
		b.cabin = true
	}
	if b.status != api.BookingStatusDraft {
		d.mark(b.spot, b.from, b.to, b.slots, true)
	}
	return b, r
}

// ---- phases ------------------------------------------------------------------

// setVAT sets the establishment default VAT (spec 8.5) through the M1 API,
// so that the corrispettivi split taxable and tax.
func (d *day) setVAT(bp int64) {
	r := d.c.must(d.c.get("getEstablishment", "/api/admin/v1/establishments/"+d.e.estID), "get establishment")
	est := decode[api.Establishment](d.t, r)
	settings := est.Settings
	settings.Pricing.DefaultVatRateBp = bp
	r = d.c.do("updateEstablishment", http.MethodPatch, "/api/admin/v1/establishments/"+d.e.estID, api.EstablishmentUpdate{Settings: &settings}, ifMatch(est.Version))
	d.c.must(r, "set default VAT")
}

func (d *day) openCash(float int64) {
	r := d.c.must(d.c.post("openCashSession", d.e.path("/cash-sessions"), api.CashSessionOpen{OpeningFloat: float}), "open cash")
	cs := decode[api.CashSession](d.t, r)
	if cs.Status != api.CashSessionStatusOpen || cs.OpeningFloat != float {
		d.t.Fatalf("opened session %+v", cs)
	}
	d.cash = cs.Id
	// A second session on the same register is refused (one open per register).
	again := d.c.post("openCashSession", d.e.path("/cash-sessions"), api.CashSessionOpen{OpeningFloat: 1})
	if again.Status != http.StatusConflict || again.Problem.Type != "cash_session_already_open" {
		d.c.must(again, "second open") // anything unexpected is reported below
		d.t.Fatalf("second open on the same register: %s", again)
	}
}

func (d *day) createCustomers(n int) {
	for i := range n {
		first, last := fmt.Sprintf("Ospite%02d", i), fmt.Sprintf("Bagnante%02d", i)
		email := openapi_types.Email(fmt.Sprintf("ospite%02d@example.com", i))
		phone := fmt.Sprintf("+3900011%05d", i)
		r := d.c.must(d.c.post("createCustomer", d.e.path("/customers"), api.CustomerInput{
			FirstName: &first, LastName: last, Email: &email, Phone: &phone,
		}), "create customer")
		d.customers = append(d.customers, decode[api.Customer](d.t, r).Id)
		d.personalData = append(d.personalData, last, string(email), phone)
	}
}

func (d *day) createDeskBookings(n int) {
	for i := range n {
		from := d.date(d.rng.IntN(7))
		to := from.AddDays(d.rng.IntN(3))
		slots := d.randomSlots()
		if from.Before(to) {
			slots = api.SlotsFULL
		}
		spot := d.pickSpot(from, to, slots)
		if spot == "" {
			continue
		}
		in := d.bookingInput(spot, from, to, slots)
		if i%6 == 0 { // a cabin (pool resource, capacity 10)
			in.Extras = &[]api.SimulateExtra{{ExtraId: d.e.cabinExtra.ID, Quantity: 1}}
		}
		if _, r := d.createBooking("createBooking", in); !r.ok() {
			d.refused("createBooking", r)
		}
	}
}

func (d *day) pay(b *trackedBooking, amount int64, method api.PaymentMethod) result {
	cs := d.cash
	return d.c.post("recordBookingPayment", d.e.path("/bookings/"+b.id+"/payments"), api.ManualPaymentInput{
		Amount: amount, Method: method, CashSessionId: &cs,
	})
}

func (d *day) takePayments(n int) {
	done := 0
	for _, b := range d.withStatus(api.BookingStatusConfirmed) {
		if done == n {
			break
		}
		due := b.total - b.paid
		if due <= 0 {
			continue
		}
		amount, method := due, api.PaymentMethodCash
		if done%3 == 1 {
			method = api.PaymentMethodCard
		}
		if done%5 == 2 {
			amount = max(due/2, 1) // deposit, balance at the desk later
		}
		r := d.pay(b, amount, method)
		if !r.ok() {
			d.refused("recordBookingPayment", r)
			continue
		}
		done++
		d.refresh(b)
	}
}

func (d *day) holdsAndConversions(n, convert, release int) {
	ttl := 600
	for range n {
		from := d.date(d.rng.IntN(5))
		slots := d.randomSlots()
		spot := d.pickSpot(from, from, slots)
		if spot == "" {
			continue
		}
		sp := spot
		r := d.c.post("createHold", d.e.path("/holds"), api.HoldCreate{
			StartsOn: from, EndsOn: from, Slots: slots, Items: []api.HoldItem{{SpotId: &sp}}, TtlSeconds: &ttl,
		})
		if !r.ok() {
			d.refused("createHold", r)
			continue
		}
		h := decode[api.Hold](d.t, r)
		if h.Status != api.HoldStatusActive {
			d.t.Fatalf("new hold %+v", h)
		}
		d.holds = append(d.holds, trackedHold{id: h.Id, spot: spot, from: from, slots: slots})
		d.mark(spot, from, from, slots, true)
	}
	// Convert the first ones into bookings (same selection, hold_id).
	var left []trackedHold
	for i, h := range d.holds {
		switch {
		case i < convert:
			in := d.bookingInput(h.spot, h.from, h.from, h.slots)
			id := h.id
			in.HoldId = &id
			d.mark(h.spot, h.from, h.from, h.slots, false)
			if _, r := d.createBooking("createBooking(hold)", in); !r.ok() {
				d.t.Fatalf("convert hold %s: %s", h.id, r)
			}
		case i < convert+release:
			r := d.c.do("releaseHold", http.MethodDelete, d.e.path("/holds/"+h.id), nil, nil)
			d.c.must(r, "release hold")
			d.mark(h.spot, h.from, h.from, h.slots, false)
		default:
			left = append(left, h) // will expire
		}
	}
	d.holds = left
}

func (d *day) heldBookings(n, confirm int) {
	ttl := 600
	held := api.BookingCreateStatus("held")
	var created []*trackedBooking
	for range n {
		from := d.date(1 + d.rng.IntN(5))
		spot := d.pickSpot(from, from, api.SlotsFULL)
		if spot == "" {
			continue
		}
		in := d.bookingInput(spot, from, from, api.SlotsFULL)
		in.Status, in.HoldTtlSeconds = &held, &ttl
		b, r := d.createBooking("createBooking(held)", in)
		if !r.ok() {
			d.refused("createBooking(held)", r)
			continue
		}
		if b.status != api.BookingStatusHeld {
			d.t.Fatalf("held booking status %s", b.status)
		}
		created = append(created, b)
	}
	for i, b := range created {
		if i < confirm { // manual confirmation from the panel (manual driver)
			r := d.c.must(d.c.post("confirmBooking", d.e.path("/bookings/"+b.id+"/confirm"), api.BookingActionRequest{}), "confirm held")
			d.track(decode[api.BookingDetail](d.t, r))
			if b.status != api.BookingStatusConfirmed {
				d.t.Fatalf("confirmed booking status %s", b.status)
			}
			continue
		}
		d.expectExpired = append(d.expectExpired, b.id)
	}
}

// burstSameSpot fires parallel creations on one spot and day: exactly one
// wins, every other gets 409 (INV-01, availability enforced by the DB).
func (d *day) burstSameSpot(n int) {
	from := d.date(3)
	spot := d.pickSpot(from, from, api.SlotsFULL)
	if spot == "" {
		d.t.Fatal("no free spot for the burst")
	}
	in := d.bookingInput(spot, from, from, api.SlotsFULL)
	var wg sync.WaitGroup
	results := make([]result, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = d.c.post("createBooking(burst)", d.e.path("/bookings"), in)
		}(i)
	}
	wg.Wait()
	for _, r := range results {
		switch {
		case r.ok():
			d.burstWinners++
			d.track(decode[api.BookingDetail](d.t, r))
		case r.Status == http.StatusConflict:
			d.refusals["createBooking(burst):"+r.Problem.Type]++
		default:
			d.refused("createBooking(burst)", r)
		}
	}
	d.c.failIfNotImplemented()
	if d.burstWinners != 1 {
		d.t.Fatalf("burst on one spot: %d winners, want exactly 1", d.burstWinners)
	}
	d.mark(spot, from, from, api.SlotsFULL, true)
}

// burstPool fires parallel holds on the cabin pool: exactly the remaining
// capacity is granted (INV-02, conditional update on pool_usage).
func (d *day) burstPool(n int) {
	ctx := context.Background()
	from := d.date(4)
	remaining := d.e.cabinPool.Capacity
	var usage []store.PoolUsage
	if err := d.e.db.NewSelect().Model(&usage).Where("pu.pool_id = ?", d.e.cabinPool.ID).Where("pu.date = ?", from).Scan(ctx); err != nil {
		d.t.Fatal(err)
	}
	for _, u := range usage {
		remaining = min(remaining, d.e.cabinPool.Capacity-u.Used-u.Held)
	}
	d.poolBurstWant = min(max(remaining, 0), n)
	pool, qty, ttl := d.e.cabinPool.ID, 1, 600
	in := api.HoldCreate{StartsOn: from, EndsOn: from, Slots: api.SlotsFULL, Items: []api.HoldItem{{PoolId: &pool, Quantity: &qty}}, TtlSeconds: &ttl}
	var wg sync.WaitGroup
	results := make([]result, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = d.c.post("createHold(pool burst)", d.e.path("/holds"), in)
		}(i)
	}
	wg.Wait()
	for _, r := range results {
		switch {
		case r.ok():
			d.poolBurstOK++
		case r.Status == http.StatusConflict:
			d.refusals["createHold(pool burst):"+r.Problem.Type]++
		default:
			d.refused("createHold(pool burst)", r)
		}
	}
	d.c.failIfNotImplemented()
	if d.poolBurstOK != d.poolBurstWant {
		d.t.Fatalf("pool burst: %d holds granted, want %d (capacity %d)", d.poolBurstOK, d.poolBurstWant, d.e.cabinPool.Capacity)
	}
}

// burstIdempotentPayment retries one payment in parallel with the same
// Idempotency-Key: a single payment is recorded (INV-31).
func (d *day) burstIdempotentPayment(n int) {
	var target *trackedBooking
	for _, b := range d.withStatus(api.BookingStatusConfirmed) {
		if b.total-b.paid > 0 {
			target = b
			break
		}
	}
	if target == nil {
		d.t.Fatal("no booking with balance due for the idempotency burst")
	}
	key := newKey()
	cs := d.cash
	in := api.ManualPaymentInput{Amount: target.total - target.paid, Method: api.PaymentMethodCash, CashSessionId: &cs}
	var wg sync.WaitGroup
	results := make([]result, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = d.c.do("recordBookingPayment(idempotent)", http.MethodPost, d.e.path("/bookings/"+target.id+"/payments"), in, map[string]string{"Idempotency-Key": key})
		}(i)
	}
	wg.Wait()
	ids := map[string]bool{}
	for _, r := range results {
		switch {
		case r.ok():
			ids[decode[api.Payment](d.t, r).Id] = true
		case r.Status == http.StatusConflict: // replay still in flight
			d.refusals["recordBookingPayment(idempotent):"+r.Problem.Type]++
		default:
			d.refused("recordBookingPayment(idempotent)", r)
		}
	}
	d.c.failIfNotImplemented()
	d.idemPayments = len(ids)
	if len(ids) != 1 {
		d.t.Fatalf("idempotent payment created %d payments, want 1", len(ids))
	}
	d.refresh(target)
}

// expireHolds lets the TTL pass on the fake clock and runs the jobs:
// unconverted holds and held bookings expire and free their resources.
func (d *day) expireHolds() {
	d.e.clock.Advance(11 * time.Minute)
	d.e.runJobs()
	for _, h := range d.holds {
		r := d.c.must(d.c.get("getHold", d.e.path("/holds/"+h.id)), "get hold")
		if st := decode[api.Hold](d.t, r).Status; st != api.HoldStatusExpired {
			d.t.Fatalf("hold %s after its ttl: %s", h.id, st)
		}
	}
	for _, id := range d.expectExpired {
		b := d.bookings[id]
		d.refresh(b)
		if b.status != api.BookingStatusExpired {
			d.t.Fatalf("held booking %s after its ttl: %s", id, b.status)
		}
		d.mark(b.spot, b.from, b.to, b.slots, false)
	}
	for _, h := range d.holds {
		d.mark(h.spot, h.from, h.from, h.slots, false)
	}
}

// rebookExpired sells the spots of expired holds: they must be free
// (INV-05).
func (d *day) rebookExpired(n int) {
	for i, h := range d.holds {
		if i == n {
			break
		}
		if _, r := d.createBooking("createBooking(after expiry)", d.bookingInput(h.spot, h.from, h.from, h.slots)); !r.ok() {
			d.t.Fatalf("spot of an expired hold not sellable: %s", r)
		}
	}
}

func (d *day) modifyBookings(n int) {
	done := 0
	for _, b := range d.withStatus(api.BookingStatusConfirmed) {
		if done == n {
			break
		}
		if b.slots != api.SlotsFULL || b.spot == "" {
			continue
		}
		newTo := b.to.AddDays(1)
		if !d.free(b.spot, newTo, newTo, api.SlotsFULL) {
			continue
		}
		formula := api.FormulaMultiDay
		force := true
		r := d.c.do("updateBooking", http.MethodPatch, d.e.path("/bookings/"+b.id), api.BookingPatch{EndsOn: &newTo, Formula: &formula, Force: &force}, ifMatch(b.version))
		if !r.ok() {
			d.refused("updateBooking", r)
			continue
		}
		res := decode[api.BookingChangeResult](d.t, r)
		if !res.Applied {
			d.t.Fatalf("patch not applied: %s", r)
		}
		d.track(res.Booking)
		d.mark(b.spot, b.from, b.to, b.slots, true)
		done++
	}
	// A stale version is refused (optimistic concurrency).
	if len(d.order) > 0 {
		b := d.bookings[d.order[0]]
		note := "versione vecchia"
		r := d.c.do("updateBooking", http.MethodPatch, d.e.path("/bookings/"+b.id), api.BookingPatch{NotesInternal: &note}, ifMatch(b.version+1000))
		if r.Status != http.StatusPreconditionFailed {
			d.c.must(r, "stale patch")
			d.t.Fatalf("stale If-Match accepted: %s", r)
		}
	}
}

func (d *day) moveBookings(n int) {
	done := 0
	for _, b := range d.withStatus(api.BookingStatusConfirmed) {
		if done == n {
			break
		}
		if b.lineID == "" || b.spot == "" {
			continue
		}
		target := d.pickSpot(b.from, b.to, b.slots)
		if target == "" {
			continue
		}
		r := d.c.post("moveBooking", d.e.path("/bookings/"+b.id+"/move"), api.BookingMoveRequest{LineId: b.lineID, SpotId: target})
		if !r.ok() {
			d.refused("moveBooking", r)
			continue
		}
		d.mark(b.spot, b.from, b.to, b.slots, false)
		d.refresh(b)
		if b.spot != target {
			d.t.Fatalf("moved booking on %s, want %s", b.spot, target)
		}
		d.mark(b.spot, b.from, b.to, b.slots, true)
		done++
	}
}

func (d *day) cancelBookings(n int) {
	settlements := []api.BookingCancelRequestSettlement{
		api.BookingCancelRequestSettlementRefund, api.BookingCancelRequestSettlementCredit,
		api.BookingCancelRequestSettlementNone, api.BookingCancelRequestSettlementPolicy,
	}
	done := 0
	for i, b := range d.withStatus(api.BookingStatusConfirmed) {
		if done == n {
			break
		}
		if i%3 != 0 {
			continue
		}
		st := settlements[done%len(settlements)]
		method := api.PaymentMethodCash
		reason := "Cambio programma"
		in := api.BookingCancelRequest{Settlement: &st, Reason: &reason}
		if st == api.BookingCancelRequestSettlementRefund && b.paid > 0 {
			amount := b.paid
			in.RefundAmount, in.RefundMethod = &amount, &method
		}
		r := d.c.post("cancelBooking", d.e.path("/bookings/"+b.id+"/cancel"), in)
		if !r.ok() {
			d.refused("cancelBooking", r)
			continue
		}
		res := decode[api.BookingCancelResult](d.t, r)
		d.track(res.Booking)
		if b.status != api.BookingStatusCancelled {
			d.t.Fatalf("cancelled booking status %s", b.status)
		}
		d.mark(b.spot, b.from, b.to, b.slots, false)
		d.expectCancelled = append(d.expectCancelled, b.id)
		done++
	}
	// Cancelling twice is an invalid transition.
	if len(d.expectCancelled) > 0 {
		r := d.c.post("cancelBooking", d.e.path("/bookings/"+d.expectCancelled[0]+"/cancel"), api.BookingCancelRequest{})
		if r.Status != http.StatusConflict || r.Problem.Type != "invalid_transition" {
			d.c.must(r, "second cancel")
			d.t.Fatalf("second cancel: %s", r)
		}
	}
}

func (d *day) explicitRefunds(n int) {
	done := 0
	for _, b := range d.withStatus(api.BookingStatusConfirmed) {
		if done == n {
			break
		}
		if b.paid < 200 {
			continue
		}
		cs := d.cash
		reason := "Rimborso parziale lettino"
		r := d.c.post("recordBookingRefund", d.e.path("/bookings/"+b.id+"/refunds"), api.RefundInput{
			Amount: b.paid / 4, Method: api.PaymentMethodCash, CashSessionId: &cs, Reason: &reason,
		})
		if !r.ok() {
			d.refused("recordBookingRefund", r)
			continue
		}
		d.refresh(b)
		done++
	}
	// A refund above the paid amount is refused (INV-24).
	for _, b := range d.withStatus(api.BookingStatusConfirmed) {
		if b.paid > 0 {
			cs := d.cash
			r := d.c.post("recordBookingRefund", d.e.path("/bookings/"+b.id+"/refunds"), api.RefundInput{Amount: b.paid + 1, Method: api.PaymentMethodCash, CashSessionId: &cs})
			if r.ok() {
				d.t.Fatalf("refund above paid accepted for %s", b.id)
			}
			d.refused("recordBookingRefund(over)", r)
			break
		}
	}
}

func (d *day) seasonPasses(n, release int) {
	for i := range n {
		r := d.c.must(d.c.post("createSeasonPass", d.e.path("/season-passes"), api.SeasonPassCreate{
			CustomerId: d.customer(), PackId: d.e.basePack.ID, SeasonId: d.e.season.ID, SpotId: d.e.passSpots[i].ID,
		}), "create season pass")
		p := decode[api.SeasonPassDetail](d.t, r).Pass
		tb := &trackedBooking{id: p.Id, spot: p.SpotId}
		d.passes = append(d.passes, tb)
		d.bookings[p.Id] = tb
		d.order = append(d.order, p.Id)
		d.refresh(tb)
	}
	// Release two days of some passes and resell one of them.
	for i := 0; i < release && i < len(d.passes); i++ {
		p := d.passes[i]
		days := []calendar.Date{d.date(2), d.date(3)}
		full := api.SlotsFULL
		r := d.c.must(d.c.post("createSeasonPassReleases", d.e.path("/season-passes/"+p.id+"/releases"), api.SeasonReleaseCreate{Dates: days, Slots: &full}), "release days")
		rel := decode[api.SeasonReleaseList](d.t, r).Items
		if len(rel) == 0 {
			d.t.Fatalf("release created nothing: %s", r)
		}
		switch i {
		case 0, 1: // resell the first released day at the desk
			if _, r := d.createBooking("createBooking(resale)", d.bookingInput(p.spot, days[0], days[0], api.SlotsFULL)); !r.ok() {
				d.t.Fatalf("resale of a released day: %s", r)
			}
		case 2: // the holder changes plans: revoke an unsold release
			r := d.c.do("revokeSeasonPassRelease", http.MethodDelete, d.e.path("/season-passes/"+p.id+"/releases/"+rel[0].Id), nil, nil)
			d.c.must(r, "revoke release")
		}
	}
	// A released and resold day cannot be revoked; a pass spot is not
	// sellable on a day that was not released.
	if len(d.passes) > 3 {
		p := d.passes[3]
		if _, r := d.createBooking("createBooking(pass spot)", d.bookingInput(p.spot, d.date(2), d.date(2), api.SlotsFULL)); r.ok() {
			d.t.Fatalf("sold a pass spot on a day that was not released")
		} else {
			d.refused("createBooking(pass spot)", r)
		}
	}
}

func (d *day) counterSales(n int) {
	vat := api.BasisPoints(2200)
	for i := range n {
		method := api.PaymentMethodCash
		if i%3 == 0 {
			method = api.PaymentMethodCard
		}
		cs := d.cash
		d.c.must(d.c.post("recordCounterSale", d.e.path("/payments"), api.CounterSaleInput{
			Amount: int64(500 + 100*i), Description: "Lettino extra al volo", Method: method, VatRateBp: &vat, CashSessionId: &cs,
		}), "counter sale")
	}
}

func (d *day) cashMovements(n int) {
	for i := range n {
		kind := api.CashMovementInputKindIn
		if i%2 == 1 {
			kind = api.CashMovementInputKindOut
		}
		d.c.must(d.c.post("addCashMovement", d.e.path("/cash-sessions/"+d.cash+"/movements"), api.CashMovementInput{
			Amount: int64(1000 + 250*i), Kind: kind, Reason: "Versamento/prelievo di prova",
		}), "cash movement")
	}
}

func (d *day) checkIns(n int) {
	d.e.clock.Advance(2 * time.Hour) // mid-morning
	done := 0
	for _, b := range d.withStatus(api.BookingStatusConfirmed) {
		if done == n {
			break
		}
		if !b.from.Equal(d.e.today) {
			continue
		}
		r := d.c.post("checkInBooking", d.e.path("/bookings/"+b.id+"/check-in"), api.BookingActionRequest{})
		if !r.ok() {
			d.refused("checkInBooking", r)
			continue
		}
		d.track(decode[api.BookingDetail](d.t, r))
		if b.status != api.BookingStatusCheckedIn {
			d.t.Fatalf("checked-in status %s", b.status)
		}
		d.expectCheckedIn = append(d.expectCheckedIn, b.id)
		done++
	}
	if done == 0 {
		d.t.Fatal("no booking checked in today")
	}
}

// closeAndReopenCash closes the session counting 1 euro less cash than
// expected, then opens the afternoon session with one sale.
func (d *day) closeAndReopenCash() {
	r := d.c.must(d.c.get("getCashSession", d.e.path("/cash-sessions/"+d.cash)), "get cash session")
	detail := decode[api.CashSessionDetail](d.t, r)
	counted := api.MethodTotals{}
	var expected int64
	for m, v := range detail.Session.ExpectedTotals {
		counted[m] = v
		expected += v
	}
	counted[string(api.PaymentMethodCash)] -= 100
	notes := "Chiusura di metà giornata"
	r = d.c.must(d.c.post("closeCashSession", d.e.path("/cash-sessions/"+d.cash+"/close"), api.CashSessionClose{CountedTotals: counted, Notes: &notes}), "close cash")
	closed := decode[api.CashSession](d.t, r)
	if closed.Status != api.CashSessionStatusClosed || closed.Difference == nil || *closed.Difference != -100 {
		d.t.Fatalf("closed session %+v (expected total %d)", closed, expected)
	}
	// Payments on a closed session are refused.
	vat := api.BasisPoints(2200)
	cs := d.cash
	late := d.c.post("recordCounterSale", d.e.path("/payments"), api.CounterSaleInput{Amount: 300, Description: "Tardiva", Method: api.PaymentMethodCash, VatRateBp: &vat, CashSessionId: &cs})
	if late.Status != http.StatusConflict || late.Problem.Type != "cash_session_closed" {
		d.c.must(late, "payment on a closed session")
		d.t.Fatalf("payment on a closed session: %s", late)
	}
	d.closedCash = d.cash
	d.openCashAfternoon()
}

func (d *day) openCashAfternoon() {
	r := d.c.must(d.c.post("openCashSession", d.e.path("/cash-sessions"), api.CashSessionOpen{OpeningFloat: 15000}), "reopen cash")
	d.cash = decode[api.CashSession](d.t, r).Id
	vat := api.BasisPoints(2200)
	d.c.must(d.c.post("recordCounterSale", d.e.path("/payments"), api.CounterSaleInput{Amount: 700, Description: "Telo mare", Method: api.PaymentMethodCash, VatRateBp: &vat}), "afternoon sale")
}

// readViews loads the read models of the panel after the day: operational
// map, planning, availability, bookings and cash lists.
func (d *day) readViews() {
	today, week := d.e.today.String(), d.date(6).String()
	for op, path := range map[string]string{
		"getOperationsDay": "/operations/day?date=" + today,
		"getPlanning":      "/planning?from=" + today + "&to=" + week,
		"getAvailability":  "/availability?from=" + today + "&to=" + week,
		"listBookings":     "/bookings?from=" + today + "&to=" + week,
		"listCashSessions": "/cash-sessions",
		"listPayments":     "/payments",
	} {
		d.c.must(d.c.get(op, d.e.path(path)), op)
	}
}

// checkStream verifies that the SSE stream carried booking, allocation,
// hold and cash changes (published after commit by the outbox
// dispatcher), with no event twice and no personal data (spec 21.1, 23).
func (d *day) checkStream() {
	d.t.Helper()
	d.sse.waitTopics(d.t, "booking.changed", "allocation.changed", "hold.changed", "cash.changed", "customer.changed")
	raw := d.sse.text()
	for _, pii := range d.personalData {
		if strings.Contains(raw, pii) {
			d.t.Fatalf("personal data %q in the SSE stream", pii)
		}
	}
	if dup := d.sse.duplicates(); len(dup) > 0 {
		d.t.Fatalf("SSE events delivered twice: %v", dup[:min(len(dup), 5)])
	}
}
