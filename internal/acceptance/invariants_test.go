//go:build integration

package acceptance

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// snapshot is everything the invariants read, loaded once at the end of
// the day (test-side reads; the application wrote every row).
type snapshot struct {
	bookings   map[string]store.Booking
	lines      []store.BookingLine
	spotAllocs []store.SpotAllocation
	poolAllocs []store.PoolAllocation
	usage      []store.PoolUsage
	pools      map[string]store.Pool
	payments   []store.Payment
	sessions   []store.CashSession
	movements  []store.CashMovement
	releases   []store.SeasonRelease
	holds      []store.Hold
	closures   map[string]bool // date|slot
	audit      []store.AuditEntry
	outbox     []store.OutboxEvent
}

func loadSnapshot(t *testing.T, e *env) *snapshot {
	t.Helper()
	ctx := context.Background()
	s := &snapshot{bookings: map[string]store.Booking{}, pools: map[string]store.Pool{}, closures: map[string]bool{}}
	var bookings []store.Booking
	var pools []store.Pool
	var closures []store.ClosureDay
	for _, q := range []struct {
		dst   any
		where string
	}{
		{&bookings, "bk.establishment_id = ?"},
		{&s.lines, "bl.establishment_id = ?"},
		{&s.spotAllocs, "sa.establishment_id = ?"},
		{&s.poolAllocs, "pla.establishment_id = ?"},
		{&s.usage, "pu.establishment_id = ?"},
		{&pools, "pl.establishment_id = ?"},
		{&s.payments, "pm.establishment_id = ?"},
		{&s.sessions, "cs.establishment_id = ?"},
		{&s.movements, "cm.establishment_id = ?"},
		{&s.releases, "sr.establishment_id = ?"},
		{&s.holds, "ho.establishment_id = ?"},
		{&closures, "cd.establishment_id = ?"},
		{&s.audit, "establishment_id = ?"},
		{&s.outbox, "establishment_id = ?"},
	} {
		if err := e.db.NewSelect().Model(q.dst).Where(q.where, e.estID).Scan(ctx); err != nil {
			t.Fatalf("load %T: %v", q.dst, err)
		}
	}
	for _, b := range bookings {
		s.bookings[b.ID] = b
	}
	for _, p := range pools {
		s.pools[p.ID] = p
	}
	for _, c := range closures {
		for _, sl := range c.Slots {
			s.closures[c.Date.String()+"|"+string(sl)] = true
		}
	}
	return s
}

// checkInvariants verifies, after the simulated day, the M3 invariants of
// docs/invariants.md across allocations, bookings, payments, cash and the
// corrispettivi export. Each check names its invariant.
func checkInvariants(t *testing.T, e *env, d *day) {
	t.Helper()
	s := loadSnapshot(t, e)
	now := e.clock.Now()
	var errs []string
	fail := func(format string, args ...any) {
		if len(errs) < 40 {
			errs = append(errs, fmt.Sprintf(format, args...))
		}
	}

	// INV-01: one allocation per (spot, date, slot).
	seen := map[string]string{}
	for _, a := range s.spotAllocs {
		k := a.SpotID + "|" + a.Date.String() + "|" + string(a.Slot)
		if other, dup := seen[k]; dup {
			fail("INV-01: %s allocated twice (%s, %s)", k, other, a.ID)
		}
		seen[k] = a.ID
	}

	// INV-06: expires_at only on holds; INV-05: no expired hold left once
	// the jobs ran.
	for _, a := range s.spotAllocs {
		if (a.ExpiresAt != nil) != (a.Kind == store.AllocationHold) {
			fail("INV-06: spot allocation %s kind %s expires_at %v", a.ID, a.Kind, a.ExpiresAt)
		}
		if a.Kind == store.AllocationHold && a.ExpiresAt != nil && !a.ExpiresAt.After(now) {
			fail("INV-05: expired hold allocation %s still present", a.ID)
		}
	}
	for _, a := range s.poolAllocs {
		if (a.ExpiresAt != nil) != (a.Kind == store.AllocationHold) {
			fail("INV-06: pool allocation %s kind %s expires_at %v", a.ID, a.Kind, a.ExpiresAt)
		}
		if a.Kind == store.AllocationHold && a.ExpiresAt != nil && !a.ExpiresAt.After(now) {
			fail("INV-05: expired pool hold %s still present", a.ID)
		}
	}

	// INV-02 / INV-03: pool_usage equals the detail and fits the capacity.
	type poolKey struct {
		pool string
		date calendar.Date
		slot calendar.Slot
	}
	used, held := map[poolKey]int{}, map[poolKey]int{}
	for _, a := range s.poolAllocs {
		k := poolKey{a.PoolID, a.Date, a.Slot}
		if a.Kind == store.AllocationHold {
			held[k] += a.Quantity
		} else {
			used[k] += a.Quantity
		}
	}
	usageRows := map[poolKey]bool{}
	for _, u := range s.usage {
		k := poolKey{u.PoolID, u.Date, u.Slot}
		usageRows[k] = true
		if u.Used != used[k] || u.Held != held[k] {
			fail("INV-03: pool_usage %s %s %s used/held %d/%d, allocations %d/%d", u.PoolID, u.Date, u.Slot, u.Used, u.Held, used[k], held[k])
		}
		if u.Used < 0 || u.Held < 0 || u.Used+u.Held > effectiveCapacity(s.pools[u.PoolID], u.Date) {
			fail("INV-02: pool %s %s %s used %d + held %d over capacity %d", u.PoolID, u.Date, u.Slot, u.Used, u.Held, effectiveCapacity(s.pools[u.PoolID], u.Date))
		}
	}
	for k, q := range used {
		if q > 0 && !usageRows[k] {
			fail("INV-03: pool allocations without pool_usage row %+v", k)
		}
	}

	// INV-20 and lines: resources match the booking status; a confirmed
	// line holds exactly its days on its spot (minus released days).
	spotByBooking := map[string][]store.SpotAllocation{}
	spotByLine := map[string][]store.SpotAllocation{}
	for _, a := range s.spotAllocs {
		if a.BookingID != nil {
			spotByBooking[*a.BookingID] = append(spotByBooking[*a.BookingID], a)
		}
		if a.BookingLineID != nil {
			spotByLine[*a.BookingLineID] = append(spotByLine[*a.BookingLineID], a)
		}
		owners := 0
		for _, o := range []*string{a.BookingID, a.HoldID, a.BlockID} {
			if o != nil {
				owners++
			}
		}
		if owners != 1 || (a.BookingID != nil) != (a.BookingLineID != nil) {
			fail("schema: spot allocation %s must have exactly one owner", a.ID)
		}
	}
	poolByBooking := map[string][]store.PoolAllocation{}
	for _, a := range s.poolAllocs {
		if a.BookingID != nil {
			poolByBooking[*a.BookingID] = append(poolByBooking[*a.BookingID], a)
		}
	}
	released := map[string]bool{} // line|date|slot of active releases
	for _, r := range s.releases {
		if r.RevokedAt == nil {
			released[r.BookingLineID+"|"+r.Date.String()+"|"+string(r.Slot)] = true
		}
	}
	for id, b := range s.bookings {
		allocs := spotByBooking[id]
		pallocs := poolByBooking[id]
		switch b.Status {
		case "no_show", "completed":
			// Spec 7 leaves the resources of a no-show to the policy: not asserted.
		case "draft", "expired", "cancelled", "rejected":
			if len(allocs)+len(pallocs) > 0 {
				fail("INV-20: booking %s %s still holds %d spot and %d pool allocations", b.Code, b.Status, len(allocs), len(pallocs))
			}
		case "held", "pending_payment", "awaiting_approval":
			for _, a := range allocs {
				if a.Kind != store.AllocationHold {
					fail("INV-20: booking %s %s has a %s allocation", b.Code, b.Status, a.Kind)
				}
			}
			if b.HoldExpiresAt == nil {
				fail("INV-06: booking %s %s without hold_expires_at", b.Code, b.Status)
			}
		case "confirmed", "checked_in":
			for _, a := range allocs {
				if a.Kind != store.AllocationBooking && a.Kind != store.AllocationSeason {
					fail("INV-20: booking %s %s has a %s allocation", b.Code, b.Status, a.Kind)
				}
				if a.Kind == store.AllocationSeason && !b.IsSeasonPass {
					fail("INV-20: booking %s has season allocations without being a pass", b.Code)
				}
			}
			for _, a := range pallocs {
				if a.Kind == store.AllocationHold {
					fail("INV-20: booking %s %s has a pool hold", b.Code, b.Status)
				}
			}
			if b.HoldExpiresAt != nil {
				fail("INV-06: confirmed booking %s keeps hold_expires_at", b.Code)
			}
		}
	}
	for _, l := range s.lines {
		b, ok := s.bookings[l.BookingID]
		if !ok || (b.Status != "confirmed" && b.Status != "checked_in") || l.SpotID == nil {
			continue
		}
		want := map[string]bool{}
		for dt := b.StartsOn; !dt.After(b.EndsOn); dt = dt.AddDays(1) {
			for _, sl := range b.Slots.Expand() {
				k := dt.String() + "|" + string(sl)
				if s.closures[k] || released[l.ID+"|"+k] {
					continue
				}
				want[k] = true
			}
		}
		got := map[string]bool{}
		for _, a := range spotByLine[l.ID] {
			got[a.Date.String()+"|"+string(a.Slot)] = true
			if a.SpotID != *l.SpotID {
				fail("lines: allocation %s of line %s on spot %s, line on %s", a.ID, l.ID, a.SpotID, *l.SpotID)
			}
		}
		if len(got) != len(want) {
			fail("lines: booking %s line %s has %d allocations, want %d", b.Code, l.ID, len(got), len(want))
			continue
		}
		for k := range want {
			if !got[k] {
				fail("lines: booking %s line %s misses %s", b.Code, l.ID, k)
				break
			}
		}
	}

	// INV-10: released days are free of the pass and resales are real.
	for _, r := range s.releases {
		if r.RevokedAt != nil {
			continue
		}
		for _, a := range spotByLine[r.BookingLineID] {
			if a.Date.Equal(r.Date) && a.Slot == r.Slot {
				fail("INV-10: release %s still allocated to the pass", r.ID)
			}
		}
		if r.ResoldBookingLineID != nil {
			found := false
			for _, a := range spotByLine[*r.ResoldBookingLineID] {
				if a.Date.Equal(r.Date) && a.Slot == r.Slot && a.SpotID == r.SpotID && a.Kind == store.AllocationBooking {
					found = true
				}
			}
			if !found {
				fail("INV-10: release %s resold without the resale allocation", r.ID)
			}
		}
	}

	// INV-22 / INV-24: paid_amount = payments - refunds, refunds <= paid.
	in, out := map[string]money.Cents{}, map[string]money.Cents{}
	for _, p := range s.payments {
		if p.BookingID == nil || p.Status != store.PaymentSucceeded {
			continue
		}
		if p.Direction == store.PaymentDirectionRefund {
			out[*p.BookingID] += p.Amount
		} else {
			in[*p.BookingID] += p.Amount
		}
	}
	for id, b := range s.bookings {
		if b.PaidAmount != in[id]-out[id] {
			fail("INV-22: booking %s paid_amount %d, payments %d - refunds %d", b.Code, b.PaidAmount, in[id], out[id])
		}
		if out[id] > in[id] {
			fail("INV-24: booking %s refunds %d over payments %d", b.Code, out[id], in[id])
		}
	}

	// INV-25: cash session totals = float + takings + movements; the
	// difference of a closed session = counted - expected.
	for _, cs := range s.sessions {
		exp := store.MethodTotals{store.PaymentMethodCash: cs.OpeningFloat}
		for _, p := range s.payments {
			if p.CashSessionID != nil && *p.CashSessionID == cs.ID && p.Status == store.PaymentSucceeded {
				exp[p.Method] += p.Signed()
			}
		}
		for _, m := range s.movements {
			if m.CashSessionID != cs.ID {
				continue
			}
			if m.Kind == store.CashMovementOut {
				exp[store.PaymentMethodCash] -= m.Amount
			} else {
				exp[store.PaymentMethodCash] += m.Amount
			}
		}
		got := cs.ExpectedTotals
		if cs.ClosedAt == nil {
			// An open session reports live totals through the API (the
			// stored column is the snapshot written at closing).
			r := d.c.must(d.c.get("getCashSession", e.path("/cash-sessions/"+cs.ID)), "get open cash session")
			got = store.MethodTotals{}
			for m, v := range decode[api.CashSessionDetail](t, r).Session.ExpectedTotals {
				got[m] = money.Cents(v)
			}
		}
		if !sameTotals(exp, got) {
			fail("INV-25: cash session %s expected_totals %v, float+payments+movements %v", cs.ID, got, exp)
		}
		if cs.ClosedAt != nil {
			if cs.Difference == nil || *cs.Difference != cs.CountedTotals.Sum()-cs.ExpectedTotals.Sum() {
				fail("INV-25: closed session %s difference %v, counted %d - expected %d", cs.ID, cs.Difference, cs.CountedTotals.Sum(), cs.ExpectedTotals.Sum())
			}
		}
	}

	// Scenario expectations hold in the database.
	for _, id := range d.expectCancelled {
		if s.bookings[id].Status != "cancelled" {
			fail("scenario: booking %s should be cancelled, is %s", id, s.bookings[id].Status)
		}
	}
	for _, id := range d.expectExpired {
		if s.bookings[id].Status != "expired" {
			fail("scenario: booking %s should be expired, is %s", id, s.bookings[id].Status)
		}
	}
	for _, id := range d.expectCheckedIn {
		if st := s.bookings[id].Status; st != "checked_in" {
			fail("scenario: booking %s should be checked in, is %s", id, st)
		}
	}
	var openSessions int
	for _, cs := range s.sessions {
		if cs.ClosedAt == nil {
			openSessions++
		}
	}
	if openSessions != 1 || len(s.sessions) != 2 {
		fail("scenario: %d cash sessions, %d open; want 2 and 1", len(s.sessions), openSessions)
	}

	// INV-18: every booking has its audit trail and its events; the outbox
	// is drained and carries no personal data.
	audited := map[string]bool{}
	for _, a := range s.audit {
		if a.EntityType == "booking" && a.EntityID != nil {
			audited[*a.EntityID] = true
		}
	}
	for id, b := range s.bookings {
		if !audited[id] {
			fail("INV-18: booking %s without audit entries", b.Code)
		}
	}
	bookingEvents := 0
	for _, ev := range s.outbox {
		if ev.DispatchedAt == nil {
			fail("outbox: event %s %s not dispatched", ev.ID, ev.Type)
		}
		if strings.HasPrefix(ev.Type, "booking.") {
			bookingEvents++
		}
		for _, pii := range d.personalData {
			if strings.Contains(string(ev.Payload), pii) {
				fail("outbox: personal data %q in event %s %s", pii, ev.ID, ev.Type)
				break
			}
		}
	}
	if bookingEvents < len(s.bookings) {
		fail("INV-18: %d booking events for %d bookings", bookingEvents, len(s.bookings))
	}

	// Corrispettivi export = payments - refunds of the day, by method and
	// VAT rate (refunds settled as customer credit included, method credit).
	checkCorrispettivi(t, e, d, s, fail)

	if len(errs) > 0 {
		t.Fatalf("invariants violated after the simulated day:\n  %s", strings.Join(errs, "\n  "))
	}
}

func checkCorrispettivi(t *testing.T, e *env, d *day, s *snapshot, fail func(string, ...any)) {
	t.Helper()
	from, to := e.today, e.today
	var want money.Cents
	for _, p := range s.payments {
		if p.Status != store.PaymentSucceeded {
			continue
		}
		if p.Test {
			// The online payments of the day go through the Stripe
			// configuration in mode test (online_test.go): no real money,
			// so they are test rows and stay out of the corrispettivi
			// (INV-74), whatever the booking.
			continue
		}
		if p.BusinessDate.Before(from) || p.BusinessDate.After(to) {
			fail("corrispettivi: payment %s business date %s outside the simulated day", p.ID, p.BusinessDate)
			continue
		}
		want += p.Signed()
	}
	q := url.Values{"from": {from.String()}, "to": {to.String()}, "group_by": {"day"}, "format": {"json"}}
	r := d.c.must(d.c.get("getCorrispettivi", e.path("/fiscal/corrispettivi?"+q.Encode())), "corrispettivi")
	rep := decode[api.CorrispettiviReport](t, r)
	if rep.GrandTotal != int64(want) {
		fail("corrispettivi: grand total %d, payments - refunds %d", rep.GrandTotal, want)
	}
	var rows, totals int64
	for _, row := range rep.Rows {
		rows += row.Gross
		if row.Taxable+row.Tax != row.Gross {
			fail("INV-16: corrispettivi row %s %s %d: taxable %d + tax %d != gross %d", row.Period, row.Method, row.VatRateBp, row.Taxable, row.Tax, row.Gross)
		}
	}
	for _, tot := range rep.Totals {
		totals += tot.Gross
		if tot.Taxable+tot.Tax != tot.Gross {
			fail("INV-16: corrispettivi total %d: taxable %d + tax %d != gross %d", tot.VatRateBp, tot.Taxable, tot.Tax, tot.Gross)
		}
	}
	if rows != rep.GrandTotal || totals != rep.GrandTotal {
		fail("corrispettivi: rows %d, totals %d, grand total %d", rows, totals, rep.GrandTotal)
	}
}

// effectiveCapacity is the pool capacity on a date (capacity_overrides).
func effectiveCapacity(p store.Pool, d calendar.Date) int {
	c := p.Capacity
	for _, o := range p.CapacityOverrides {
		if !d.Before(o.From) && !d.After(o.To) {
			c = o.Capacity
		}
	}
	return c
}

// sameTotals compares method totals ignoring zero entries.
func sameTotals(a, b store.MethodTotals) bool {
	for k, v := range a {
		if v != b[k] {
			return false
		}
	}
	for k, v := range b {
		if v != a[k] {
			return false
		}
	}
	return true
}
