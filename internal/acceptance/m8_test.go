//go:build integration

package acceptance

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
)

// M8 acceptance (spec 28, spec 30): advanced cash desk and agreements,
// driven only through the admin API of the real application.
//
//   - With the beach full on 15 August (no spot of the zone free for the
//     whole week), find spot sells the 10-16 August week on two spots in
//     one call sequence (search, book), priced day by day (ADR 0028).
//   - No overbooking path: a desk booking, a move from a date, a stale
//     proposal and a swap onto the full day never place a second booking
//     on a spot-day (INV-01, INV-102).
//   - At season end the agreements register totals match the cash reports
//     (booked and collected revenue of the same bookings).
//
// TestConcurrency_AcceptanceM8Swaps adds a burst of 160 parallel swaps,
// moves and bookings on the full week (run with -race by make test-race).

// m8 is the M8 scenario on an env.
type m8 struct {
	t *testing.T
	e *env
	c *apiClient

	zoneID string
	spots  []store.Spot // umbrella spots of the zone, label order
}

func newM8(t *testing.T, e *env) *m8 {
	t.Helper()
	c := newClient(t, e.srv.URL)
	c.login(ownerEmail, ownerPassword)
	m := &m8{t: t, e: e, c: c}
	// The smallest zone of daily umbrellas with at least four spots keeps
	// the scenario short.
	byZone := map[string][]store.Spot{}
	for _, s := range e.dailySpots {
		byZone[s.ZoneID] = append(byZone[s.ZoneID], s)
	}
	for z, ss := range byZone {
		if len(ss) >= 4 && (m.zoneID == "" || len(ss) < len(m.spots) || (len(ss) == len(m.spots) && z < m.zoneID)) {
			m.zoneID, m.spots = z, ss
		}
	}
	if m.zoneID == "" {
		t.Fatal("no zone with four daily umbrellas")
	}
	sort.Slice(m.spots, func(i, j int) bool { return m.spots[i].Label < m.spots[j].Label })
	t.Logf("zone %s: %d umbrellas", m.zoneID, len(m.spots))
	return m
}

func aug(day int) calendar.Date { return calendar.MustParse(fmt.Sprintf("2027-08-%02d", day)) }

func (m *m8) customer(last string) *api.CustomerInput {
	first := "Ospite"
	return &api.CustomerInput{FirstName: &first, LastName: last}
}

// book creates a confirmed desk booking of spot (or returns the refusal).
func (m *m8) book(spotID string, from, to calendar.Date, agreementID string) (api.BookingDetail, result) {
	m.t.Helper()
	in := api.BookingCreate{
		Customer: m.customer("Cliente" + spotID[:4]), StartsOn: from, EndsOn: to, Slots: api.SlotsFULL,
		Formula: formulaOf(from, to, api.SlotsFULL), Lines: []api.BookingLineInput{{PackId: m.e.basePack.ID, SpotId: &spotID}},
	}
	if agreementID != "" {
		in.AgreementId = &agreementID
		in.Customer.AgreementId = &agreementID
	}
	r := m.c.post("createBooking", m.e.path("/bookings"), in)
	if !r.ok() {
		return api.BookingDetail{}, r
	}
	return decode[api.BookingDetail](m.t, r), r
}

func (m *m8) mustBook(spotID string, from, to calendar.Date, agreementID string) api.BookingDetail {
	m.t.Helper()
	d, r := m.book(spotID, from, to, agreementID)
	m.c.must(r, "desk booking")
	return d
}

func (m *m8) findSpot(from, to calendar.Date, maxChanges int) api.FindSpotResult {
	m.t.Helper()
	pack := m.e.basePack.ID
	r := m.c.must(m.c.post("findSpot", m.e.path("/find-spot"), api.FindSpotRequest{
		StartsOn: from, EndsOn: to, Slots: api.SlotsFULL, PackId: &pack, ZoneIds: &[]string{m.zoneID}, MaxSpotChanges: &maxChanges,
	}), "find spot")
	return decode[api.FindSpotResult](m.t, r)
}

func (m *m8) bookProposal(p api.FindSpotProposal, from, to calendar.Date) result {
	var segs []api.BookingSegmentInput
	for _, s := range p.Lines[0].Segments {
		segs = append(segs, api.BookingSegmentInput{SpotId: s.SpotId, StartsOn: s.StartsOn, EndsOn: s.EndsOn})
	}
	return m.c.post("bookFindSpotProposal", m.e.path("/find-spot/book"), api.FindSpotBookRequest{
		ProposalId: &p.Id, ExpectedTotal: p.Total,
		Booking: api.BookingCreate{
			Customer: m.customer("Ferragosto"), StartsOn: from, EndsOn: to, Slots: api.SlotsFULL,
			Formula: formulaOf(from, to, api.SlotsFULL), Lines: []api.BookingLineInput{{PackId: m.e.basePack.ID, Segments: &segs}},
		},
	})
}

func (m *m8) detail(id string) api.BookingDetail {
	m.t.Helper()
	return decode[api.BookingDetail](m.t, m.c.must(m.c.get("getBooking", m.e.path("/bookings/"+id)), "get booking"))
}

// expectRefused checks that r is a refusal (4xx, never 5xx or success).
func (m *m8) expectRefused(r result, what string) {
	m.t.Helper()
	if r.ok() || r.Status >= 500 || r.Status < 400 {
		m.t.Fatalf("%s: want a refusal, got %s", what, r)
	}
}

// fullWeek books the zone so that no spot is free for the whole 10-16
// August week and 15 August has one free spot left (B), then sells the
// week on A (10-14) and B (15-16) through find spot. It returns the split
// booking.
func (m *m8) fullWeek() api.BookingDetail {
	t := m.t
	a, b := m.spots[0], m.spots[1]
	for _, s := range m.spots[2:] {
		m.mustBook(s.ID, aug(10), aug(16), "")
	}
	m.mustBook(a.ID, aug(15), aug(16), "")
	m.mustBook(b.ID, aug(10), aug(14), "")

	if res := m.findSpot(aug(10), aug(16), 0); len(res.Proposals) != 0 {
		t.Fatalf("a single spot is free for the week: %+v", res.Proposals)
	}
	res := m.findSpot(aug(10), aug(16), 1)
	var p *api.FindSpotProposal
	for i := range res.Proposals {
		if res.Proposals[i].SpotChanges == 1 {
			p = &res.Proposals[i]
			break
		}
	}
	if p == nil || len(p.Lines) != 1 || len(p.Lines[0].Segments) != 2 || p.Total == nil {
		t.Fatalf("no proposal on two spots: %+v", res.Proposals)
	}
	s0, s1 := p.Lines[0].Segments[0], p.Lines[0].Segments[1]
	if s0.SpotId != a.ID || !s0.StartsOn.Equal(aug(10)) || !s0.EndsOn.Equal(aug(14)) ||
		s1.SpotId != b.ID || !s1.StartsOn.Equal(aug(15)) || !s1.EndsOn.Equal(aug(16)) {
		t.Fatalf("proposal segments %+v", p.Lines[0].Segments)
	}
	r := m.c.must(m.bookProposal(*p, aug(10), aug(16)), "book proposal")
	d := decode[api.BookingDetail](t, r)
	if d.Status != api.BookingStatusConfirmed || d.TotalAmount != *p.Total || len(d.Lines) != 1 ||
		d.Lines[0].Segments == nil || len(*d.Lines[0].Segments) != 2 {
		t.Fatalf("split booking %+v", d)
	}
	// The same proposal again: the spots are gone (INV-01).
	m.expectRefused(m.bookProposal(*p, aug(10), aug(16)), "stale proposal")
	return d
}

// noOverbooking tries every desk path onto the full 15 August.
func (m *m8) noOverbooking(split api.BookingDetail) {
	t := m.t
	for _, s := range m.spots {
		_, r := m.book(s.ID, aug(15), aug(15), "")
		m.expectRefused(r, "desk booking on a full day")
	}
	if res := m.findSpot(aug(15), aug(15), 2); len(res.Proposals) != 0 {
		t.Fatalf("find spot on a full day: %+v", res.Proposals)
	}
	// Move from a date onto an occupied spot.
	from := aug(15)
	other := m.spots[3].ID
	r := m.c.do("moveBooking", http.MethodPost, m.e.path("/bookings/"+split.Id+"/move"),
		api.BookingMoveRequest{LineId: split.Lines[0].Id, SpotId: other, From: &from}, ifMatch(split.Version))
	m.expectRefused(r, "move onto an occupied spot")
}

// swapAndUndo swaps the split booking with a whole-week booking from 12
// August (preview first), checks both and undoes it.
func (m *m8) swapAndUndo(split api.BookingDetail) {
	t := m.t
	week := m.weekBookingOn(m.spots[2].ID)
	req := api.BookingSwapRequest{
		A: api.BookingSwapSide{BookingId: split.Id, LineId: split.Lines[0].Id},
		B: api.BookingSwapSide{BookingId: week.Id, LineId: week.Lines[0].Id}, From: aug(12),
	}
	dry := true
	req.DryRun = &dry
	pv := decode[api.BookingSwapResult](t, m.c.must(m.c.post("swapBookings", m.e.path("/bookings/swap"), req), "swap preview"))
	if pv.Applied {
		t.Fatal("swap preview applied")
	}
	req.DryRun = nil
	sw := decode[api.BookingSwapResult](t, m.c.must(m.c.post("swapBookings", m.e.path("/bookings/swap"), req), "swap"))
	if !sw.Applied || sw.Undo == nil || !sw.To.Equal(aug(16)) {
		t.Fatalf("swap %+v", sw)
	}
	m.checkAllocations("after swap")
	r := m.c.post("undoMove", m.e.path("/moves/"+sw.Undo.MoveId+"/undo"), nil)
	m.c.must(r, "undo swap")
	m.checkAllocations("after undo")
	got := m.detail(split.Id)
	if got.Lines[0].Segments == nil || len(*got.Lines[0].Segments) != 2 || (*got.Lines[0].Segments)[0].SpotId != m.spots[0].ID {
		t.Fatalf("undo did not restore the split stay: %+v", got.Lines[0])
	}
}

// weekBookingOn returns the confirmed 10-16 August booking on spot.
func (m *m8) weekBookingOn(spotID string) api.BookingDetail {
	m.t.Helper()
	var rows []store.BookingLine
	if err := m.e.db.NewSelect().Model(&rows).Where("bl.establishment_id = ?", m.e.estID).Where("bl.spot_id = ?", spotID).
		Scan(context.Background()); err != nil {
		m.t.Fatal(err)
	}
	for _, l := range rows {
		d := m.detail(l.BookingID)
		if d.StartsOn.Equal(aug(10)) && d.EndsOn.Equal(aug(16)) && d.Status == api.BookingStatusConfirmed {
			return d
		}
	}
	m.t.Fatalf("no week booking on %s", spotID)
	return api.BookingDetail{}
}

// checkAllocations verifies on the database that no spot-day is allocated
// twice (INV-01) and that every live booking line holds each half-day of
// its stay exactly once, on one spot at a time (INV-102).
func (m *m8) checkAllocations(when string) {
	m.t.Helper()
	ctx := context.Background()
	var allocs []store.SpotAllocation
	var lines []store.BookingLine
	var bookings []store.Booking
	if err := m.e.db.NewSelect().Model(&allocs).Where("sa.establishment_id = ?", m.e.estID).Scan(ctx); err != nil {
		m.t.Fatal(err)
	}
	if err := m.e.db.NewSelect().Model(&lines).Where("bl.establishment_id = ?", m.e.estID).Scan(ctx); err != nil {
		m.t.Fatal(err)
	}
	if err := m.e.db.NewSelect().Model(&bookings).Where("bk.establishment_id = ?", m.e.estID).Scan(ctx); err != nil {
		m.t.Fatal(err)
	}
	var errs []string
	seen := map[string]bool{}
	byLine := map[string]map[string]int{}
	for _, a := range allocs {
		k := a.SpotID + "|" + a.Date.String() + "|" + string(a.Slot)
		if seen[k] {
			errs = append(errs, "INV-01: "+k+" allocated twice")
		}
		seen[k] = true
		if a.BookingLineID != nil {
			if byLine[*a.BookingLineID] == nil {
				byLine[*a.BookingLineID] = map[string]int{}
			}
			byLine[*a.BookingLineID][a.Date.String()+"|"+string(a.Slot)]++
		}
	}
	bk := map[string]store.Booking{}
	for _, b := range bookings {
		bk[b.ID] = b
	}
	for _, l := range lines {
		b := bk[l.BookingID]
		if (b.Status != "confirmed" && b.Status != "checked_in") || b.IsSeasonPass || l.AssignmentStatus == store.AssignmentPending {
			continue
		}
		got := byLine[l.ID]
		want := 0
		for d := b.StartsOn; !d.After(b.EndsOn); d = d.AddDays(1) {
			for _, sl := range b.Slots.Expand() {
				want++
				if got[d.String()+"|"+string(sl)] != 1 {
					errs = append(errs, fmt.Sprintf("INV-102: booking %s line %s holds %s %s %d times", b.Code, l.ID, d, sl, got[d.String()+"|"+string(sl)]))
				}
			}
		}
		if len(got) != want {
			errs = append(errs, fmt.Sprintf("INV-102: booking %s line %s has %d half-days, want %d", b.Code, l.ID, len(got), want))
		}
	}
	if len(errs) > 0 {
		if len(errs) > 20 {
			errs = errs[:20]
		}
		m.t.Fatalf("%s:\n  %s", when, strings.Join(errs, "\n  "))
	}
}

// agreementsAtSeasonEnd sells with two agreements on one day, collects
// everything in cash, closes the season and compares the register with the
// cash reports of that day.
func (m *m8) agreementsAtSeasonEnd() {
	t := m.t
	e := m.e
	e.clock.Advance(24 * time.Hour) // a day with agreement sales only
	day := calendar.In(e.clock.Now(), e.loc)
	cs := decode[api.CashSession](t, m.c.must(m.c.post("openCashSession", e.path("/cash-sessions"), api.CashSessionOpen{OpeningFloat: 10000}), "open cash"))

	mk := func(name string, cat api.RegisterCategory, percent string) string {
		target := api.AgreementTargetIndividualCustomers
		ch := []api.AgreementChannel{api.AgreementChannelDesk}
		r := m.c.must(m.c.post("createAgreement", e.path("/agreements"), api.AgreementInput{
			Name: name, Action: api.PriceAction{Type: api.PercentOff, Percent: &percent}, Target: &target,
			RegisterCategory: &cat, Channels: &ch,
		}), "create agreement "+name)
		return decode[api.Agreement](t, r).Id
	}
	residents := mk("Residenti", api.RegisterCategoryResidents, "20")
	protected := mk("Categorie tutelate", api.RegisterCategoryProtected, "50")

	// July: no conflict with the full August week.
	free := e.dailySpots
	type sale struct {
		agreement string
		days      int
	}
	sales := []sale{{residents, 3}, {residents, 1}, {protected, 5}, {protected, 2}, {residents, 7}}
	var total int64
	spotDays := 0
	start := calendar.MustParse("2027-07-20")
	for i, s := range sales {
		d := m.mustBook(free[i].ID, start, start.AddDays(s.days-1), s.agreement)
		if d.AgreementId == nil || *d.AgreementId != s.agreement {
			t.Fatalf("booking without its agreement: %+v", d)
		}
		total += d.TotalAmount
		spotDays += s.days
		cash := cs.Id
		m.c.must(m.c.post("recordBookingPayment", e.path("/bookings/"+d.Id+"/payments"), api.ManualPaymentInput{
			Amount: d.TotalAmount, Method: api.PaymentMethodCash, CashSessionId: &cash,
		}), "pay agreement booking")
	}

	// Season end: the bookings are completed by the jobs.
	e.clock.Set(time.Date(e.season.ClosesOn.Year(), e.season.ClosesOn.Month(), e.season.ClosesOn.Day(), 22, 0, 0, 0, e.loc).AddDate(0, 0, 1))
	e.runJobs()
	m.c.login(ownerEmail, ownerPassword) // months later: a new session

	q := fmt.Sprintf("?from=%s&to=%s&season_id=%s", e.season.OpensOn, e.season.ClosesOn, e.season.ID)
	reg := decode[api.AgreementsRegister](t, m.c.must(m.c.get("getAgreementsRegister", e.path("/reports/agreements-register"+q)), "register"))
	dq := fmt.Sprintf("?from=%s&to=%s&group_by=day", day, day)
	booked := decode[api.RevenueReport](t, m.c.must(m.c.get("getRevenueReport", e.path("/reports/revenue"+dq+"&basis=booked")), "booked revenue"))
	collected := decode[api.RevenueReport](t, m.c.must(m.c.get("getRevenueReport", e.path("/reports/revenue"+dq+"&basis=collected")), "collected revenue"))

	tot := reg.Totals
	if tot.Bookings != len(sales) || tot.SpotDays != spotDays || tot.Revenue != total || tot.Discount <= 0 || tot.Agreements < 2 { // the demo has agreements of its own
		t.Fatalf("register totals %+v, want %d bookings, %d spot-days, revenue %d", tot, len(sales), spotDays, total)
	}
	if booked.Totals.Gross != tot.Revenue || collected.Totals.Gross-collected.Totals.Refunds != tot.Revenue {
		t.Fatalf("register revenue %d, booked %d, collected %d - %d", tot.Revenue, booked.Totals.Gross, collected.Totals.Gross, collected.Totals.Refunds)
	}
	var sum api.AgreementsRegisterCategoryTotal
	for _, c := range reg.ByCategory {
		sum.Bookings += c.Bookings
		sum.SpotDays += c.SpotDays
		sum.Revenue += c.Revenue
		sum.Discount += c.Discount
	}
	if sum.Bookings != tot.Bookings || sum.SpotDays != tot.SpotDays || sum.Revenue != tot.Revenue || sum.Discount != tot.Discount {
		t.Fatalf("register categories %+v do not add up to %+v", reg.ByCategory, tot)
	}
	for _, f := range []string{"xlsx", "html"} {
		m.c.must(m.c.get("getAgreementsRegister", e.path("/reports/agreements-register"+q+"&format="+f)), "register "+f)
	}
}

func TestAcceptance_M8CashDeskAndAgreements(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		m := newM8(t, e)
		split := m.fullWeek()
		m.checkAllocations("after the full week")
		m.noOverbooking(split)
		m.swapAndUndo(split)
		m.agreementsAtSeasonEnd()
		m.checkAllocations("at season end")
		m.c.failIfNotImplemented()
	})
}

// TestConcurrency_AcceptanceM8Swaps: on the full week, 160 parallel
// operations race on the same spots: 60 swaps spread over six pairs of
// bookings (the split one in the first), 30 moves from a date onto taken
// spots, 30 desk bookings and 40 crossed swaps on one contended pair.
// Whatever the server accepts, no spot-day is ever allocated twice and
// every booking keeps each day exactly once (INV-01, INV-101, INV-102); a
// transaction that loses a race (unique key, or a deadlock between two
// inserts of the same keys on PostgreSQL) is 409, never 500.
func TestConcurrency_AcceptanceM8Swaps(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		m := newM8(t, e)
		split := m.fullWeek()
		const pairs = 6
		sides := []api.BookingDetail{split}
		for k := 2; len(sides) < 2*pairs; k++ {
			sides = append(sides, m.weekBookingOn(m.spots[k].ID))
		}
		const spread, contended = 120, 40
		const n = spread + contended
		var wg sync.WaitGroup
		results := make([]result, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				from := aug(10 + i%7)
				p := (i / 4) % pairs
				if i >= spread {
					p = 0 // the crossed swaps all queue on the first pair
				}
				a, b := sides[2*p], sides[2*p+1]
				switch i % 4 {
				case 0, 1:
					results[i] = m.c.post("swapBookings(burst)", e.path("/bookings/swap"), api.BookingSwapRequest{
						A: api.BookingSwapSide{BookingId: a.Id, LineId: a.Lines[0].Id},
						B: api.BookingSwapSide{BookingId: b.Id, LineId: b.Lines[0].Id}, From: from,
					})
				case 2:
					if i >= spread {
						// Crossed: the same pair, sides reversed.
						results[i] = m.c.post("swapBookings(burst)", e.path("/bookings/swap"), api.BookingSwapRequest{
							A: api.BookingSwapSide{BookingId: b.Id, LineId: b.Lines[0].Id},
							B: api.BookingSwapSide{BookingId: a.Id, LineId: a.Lines[0].Id}, From: from,
						})
						return
					}
					// Onto the spot of the other pair member: taken, refused.
					results[i] = m.c.post("moveBooking(burst)", e.path("/bookings/"+b.Id+"/move"),
						api.BookingMoveRequest{LineId: b.Lines[0].Id, SpotId: m.spots[2+(i%len(m.spots[2:]))].ID, From: &from})
				default:
					if i >= spread {
						results[i] = m.c.post("swapBookings(burst)", e.path("/bookings/swap"), api.BookingSwapRequest{
							A: api.BookingSwapSide{BookingId: a.Id, LineId: a.Lines[0].Id},
							B: api.BookingSwapSide{BookingId: b.Id, LineId: b.Lines[0].Id}, From: from,
						})
						return
					}
					_, results[i] = m.book(m.spots[i%len(m.spots)].ID, from, from, "")
				}
			}()
		}
		wg.Wait()
		accepted := 0
		for i, r := range results {
			switch {
			case r.ok():
				accepted++
			case r.Status >= 500 || r.Status < 0:
				t.Fatalf("operation %d: %s", i, r)
			}
		}
		t.Logf("burst: %d of %d accepted", accepted, n)
		m.checkAllocations("after the burst")
	})
}
