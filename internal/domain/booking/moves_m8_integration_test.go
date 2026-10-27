//go:build integration

package booking_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// Moves of M8 (spec 30.6, 30.7, 30.9, ADR 0028, INV-101, INV-102) on the
// real inventory: split stays, swap, group move, undo, duplicate.

var dd = bookingtest.D

// segs renders the segments of the first line of d as "A1@07-10..07-12+A2@07-13..07-16".
func segs(f *bookingtest.Fixture, d *booking.Detail) string {
	labels := map[string]string{}
	for _, s := range append(append([]*store.Spot{}, f.Spots...), f.FreeSpots...) {
		labels[s.ID] = s.Label
	}
	ls := booking.LineSegments(d)[d.Lines[0].ID]
	var out []string
	for _, s := range ls {
		out = append(out, fmt.Sprintf("%s@%s..%s", labels[s.SpotID], s.StartsOn.String()[5:], s.EndsOn.String()[5:]))
	}
	return strings.Join(out, "+")
}

// checkCover asserts INV-102 on d: every line with a spot has exactly one
// allocation per half-day of the stay.
func checkCover(t *testing.T, d *booking.Detail) {
	t.Helper()
	per := map[string]int{}
	for _, a := range d.SpotAllocations {
		per[*a.BookingLineID+a.Date.String()+string(a.Slot)]++
	}
	for _, l := range d.Lines {
		if l.SpotID == nil {
			continue
		}
		for _, day := range calendar.Range(d.Booking.StartsOn, d.Booking.EndsOn) {
			for _, sl := range d.Booking.Slots.Expand() {
				if n := per[l.ID+day.String()+string(sl)]; n != 1 {
					t.Fatalf("line %s on %s %s: %d allocations", l.ID, day, sl, n)
				}
			}
		}
	}
}

func get(t *testing.T, f *bookingtest.Fixture, svc *booking.Service, id string) *booking.Detail {
	t.Helper()
	d, err := svc.Get(f.Ctx, f.Owner, f.Est.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// vipCategory gives spot the price category "vip" priced VIPPerDay for the
// base pack (ADR 0028: each day is priced with the spot of that day).
func vipCategory(t *testing.T, f *bookingtest.Fixture, spot *store.Spot) {
	t.Helper()
	cat := "vip"
	if _, err := f.DB.NewUpdate().Model((*store.Spot)(nil)).Set("price_category_override = ?", cat).Where("id = ?", spot.ID).Exec(f.Ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Pricing.CreateRule(f.Ctx, f.Owner, f.Est.ID, pricingconfig.RuleInput{
		SeasonID: f.Season.ID, Name: "vip-category", TargetType: pricing.TargetPack, TargetIDs: []string{f.Base.ID},
		Conditions: pricing.Conditions{PriceCategories: []string{cat}}, Action: pricing.Action{Type: pricing.ActionSet, Amount: bookingtest.VIPPerDay},
		Priority: 10, Stage: pricing.StageBase, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCreate_SplitStaySegments(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		vipCategory(t, f, f.Spots[1])
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-13")
		in.Lines[0].SpotID = ""
		in.Lines[0].Segments = []booking.SegmentInput{
			{SpotID: f.Spots[0].ID, StartsOn: dd("2027-07-10"), EndsOn: dd("2027-07-11")},
			{SpotID: f.Spots[1].ID, StartsOn: dd("2027-07-12"), EndsOn: dd("2027-07-13")},
		}
		d, err := r.svc.Create(f.Ctx, f.Cashier, f.Est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if got := segs(f, d); got != "A1@07-10..07-11+A2@07-12..07-13" {
			t.Fatalf("segments %s", got)
		}
		checkCover(t, d)
		if want := money.Cents(2*bookingtest.BasePerDay + 2*bookingtest.VIPPerDay); d.Booking.TotalAmount != want {
			t.Fatalf("total %d, want %d (per-day category)", d.Booking.TotalAmount, want)
		}
		if *d.Lines[0].SpotID != f.Spots[0].ID {
			t.Fatalf("line spot is not the first segment's")
		}

		// Segments that leave a gap are refused before any write.
		bad := f.Input(f.Spots[2], "2027-07-10", "2027-07-13")
		bad.Lines[0].Segments = []booking.SegmentInput{
			{SpotID: f.Spots[2].ID, StartsOn: dd("2027-07-10"), EndsOn: dd("2027-07-10")},
			{SpotID: f.Spots[3].ID, StartsOn: dd("2027-07-12"), EndsOn: dd("2027-07-13")},
		}
		if _, err := r.svc.Create(f.Ctx, f.Cashier, f.Est.ID, bad); err == nil || !strings.Contains(err.Error(), "segments") {
			t.Fatalf("gap accepted: %v", err)
		}
		// The expected total protects the proposal price.
		exp := money.Cents(1)
		in2 := f.Input(f.Spots[2], "2027-07-20", "2027-07-20")
		in2.ExpectedTotal = &exp
		if _, err := r.svc.Create(f.Ctx, f.Cashier, f.Est.ID, in2); !errors.Is(err, booking.ErrTotalChanged) {
			t.Fatalf("expected total: %v", err)
		}
	})
}

// TestMove_FromDatePricedPerDay: a scheduled move ("from 13 on A2") keeps
// the days before on A1 and prices each day with its spot.
func TestMove_FromDatePricedPerDay(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		vipCategory(t, f, f.Spots[1])
		d := mustCreate(t, f, r.svc, f.Input(f.Spots[0], "2027-07-10", "2027-07-14"))
		from := dd("2027-07-13")
		res, err := r.svc.Move(f.Ctx, f.Cashier, f.Est.ID, d.Booking.ID, booking.MoveInput{LineID: d.Lines[0].ID, SpotID: f.Spots[1].ID, From: &from})
		if err != nil {
			t.Fatal(err)
		}
		if got := segs(f, res.Detail); got != "A1@07-10..07-12+A2@07-13..07-14" {
			t.Fatalf("segments %s", got)
		}
		if want := money.Cents(3*bookingtest.BasePerDay + 2*bookingtest.VIPPerDay); res.NewTotal != want {
			t.Fatalf("new total %d, want %d", res.NewTotal, want)
		}
		if res.Undo == nil || res.Undo.Kind != store.MoveUndoMove {
			t.Fatalf("no undo info: %+v", res.Undo)
		}
		checkCover(t, res.Detail)
		// Undo: back on A1 for the whole stay, at the single-spot price.
		ds, err := r.svc.UndoMove(f.Ctx, f.Cashier, f.Est.ID, res.Undo.MoveID)
		if err != nil {
			t.Fatal(err)
		}
		if got := segs(f, ds[0]); got != "A1@07-10..07-14" || ds[0].Booking.TotalAmount != 5*bookingtest.BasePerDay || *ds[0].Lines[0].SpotID != f.Spots[0].ID {
			t.Fatalf("undo: %s, total %d", got, ds[0].Booking.TotalAmount)
		}
		checkCover(t, ds[0])
	})
}

func mustCreate(t *testing.T, f *bookingtest.Fixture, svc *booking.Service, in booking.CreateInput) *booking.Detail {
	t.Helper()
	d, err := svc.Create(f.Ctx, f.Cashier, f.Est.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestSwap_CommonIntervalFromDate (INV-101): the two lines exchange their
// spots on the common interval from the chosen date; outside it each stays
// where it was; audit and event for both, one undo entry.
func TestSwap_CommonIntervalFromDate(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		x := mustCreate(t, f, r.svc, f.Input(f.Spots[0], "2027-07-10", "2027-07-16"))
		y := mustCreate(t, f, r.svc, f.Input(f.Spots[1], "2027-07-12", "2027-07-20"))
		audits := f.Count((*store.AuditEntry)(nil), "action = 'booking.swapped'")
		events := f.Count((*store.OutboxEvent)(nil), "type = 'booking.modified'")
		in := booking.SwapInput{
			A:    booking.SwapSide{BookingID: x.Booking.ID, LineID: x.Lines[0].ID},
			B:    booking.SwapSide{BookingID: y.Booking.ID, LineID: y.Lines[0].ID},
			From: dd("2027-07-13"), DryRun: true,
		}
		preview, err := r.svc.Swap(f.Ctx, f.Cashier, f.Est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if preview.Applied || preview.Undo != nil || segs(f, get(t, f, r.svc, x.Booking.ID)) != "A1@07-10..07-16" {
			t.Fatalf("dry run wrote: %+v", preview)
		}
		if !preview.From.Equal(dd("2027-07-13")) || !preview.To.Equal(dd("2027-07-16")) || preview.A.Detail == nil {
			t.Fatalf("preview interval %s..%s", preview.From, preview.To)
		}
		in.DryRun = false
		res, err := r.svc.Swap(f.Ctx, f.Cashier, f.Est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if got := segs(f, res.A.Detail); got != "A1@07-10..07-12+A2@07-13..07-16" {
			t.Fatalf("A segments %s", got)
		}
		if got := segs(f, res.B.Detail); got != "A2@07-12..07-12+A1@07-13..07-16+A2@07-17..07-20" {
			t.Fatalf("B segments %s", got)
		}
		checkCover(t, res.A.Detail)
		checkCover(t, res.B.Detail)
		if res.A.FromSpotID != f.Spots[0].ID || res.A.ToSpotID != f.Spots[1].ID || res.Undo == nil || len(res.Undo.BookingIDs) != 2 {
			t.Fatalf("outcome %+v undo %+v", res.A, res.Undo)
		}
		if res.A.Detail.Booking.Version != x.Booking.Version+1 || res.B.Detail.Booking.Version != y.Booking.Version+1 {
			t.Fatalf("versions not bumped")
		}
		if n := f.Count((*store.AuditEntry)(nil), "action = 'booking.swapped'"); n != audits+2 {
			t.Fatalf("swap audits %d", n-audits)
		}
		if n := f.Count((*store.OutboxEvent)(nil), "type = 'booking.modified'"); n != events+2 {
			t.Fatalf("modified events %d", n-events)
		}

		// No common interval from the date.
		in.From = dd("2027-07-18")
		if _, err := r.svc.Swap(f.Ctx, f.Cashier, f.Est.ID, in); !errors.Is(err, booking.ErrSwapConflict) {
			t.Fatalf("no common interval: %v", err)
		}
		// Stale expected version.
		stale := x.Booking.Version
		in.From, in.A.ExpectedVersion = dd("2027-07-13"), &stale
		if _, err := r.svc.Swap(f.Ctx, f.Cashier, f.Est.ID, in); err == nil || !strings.Contains(err.Error(), "version") {
			t.Fatalf("stale version: %v", err)
		}
	})
}

// TestGroupMove_AllOrNothing: two bookings move together from a date; a
// group move hitting one occupied half-day changes nothing.
func TestGroupMove_AllOrNothing(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		x := mustCreate(t, f, r.svc, f.Input(f.Spots[0], "2027-07-10", "2027-07-12"))
		y := mustCreate(t, f, r.svc, f.Input(f.Spots[1], "2027-07-10", "2027-07-12"))
		from := dd("2027-07-11")
		res, err := r.svc.GroupMove(f.Ctx, f.Cashier, f.Est.ID, booking.GroupMoveInput{From: &from, Moves: []booking.GroupMoveItem{
			{BookingID: x.Booking.ID, LineID: x.Lines[0].ID, SpotID: f.Spots[1].ID},
			{BookingID: y.Booking.ID, LineID: y.Lines[0].ID, SpotID: f.Spots[2].ID},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Results) != 2 || !res.Applied || res.Undo == nil || res.Undo.Kind != store.MoveUndoGroup {
			t.Fatalf("result %+v", res)
		}
		if got := segs(f, get(t, f, r.svc, x.Booking.ID)); got != "A1@07-10..07-10+A2@07-11..07-12" {
			t.Fatalf("x %s", got)
		}
		if got := segs(f, get(t, f, r.svc, y.Booking.ID)); got != "A2@07-10..07-10+A3@07-11..07-12" {
			t.Fatalf("y %s", got)
		}
		blocker := mustCreate(t, f, r.svc, f.Input(f.Spots[3], "2027-07-12", "2027-07-12"))
		_ = blocker
		allocs := f.Count((*store.SpotAllocation)(nil), "")
		_, err = r.svc.GroupMove(f.Ctx, f.Cashier, f.Est.ID, booking.GroupMoveInput{From: &from, Moves: []booking.GroupMoveItem{
			{BookingID: x.Booking.ID, LineID: x.Lines[0].ID, SpotID: f.Spots[0].ID},
			{BookingID: y.Booking.ID, LineID: y.Lines[0].ID, SpotID: f.Spots[3].ID},
		}})
		var ce *availability.ConflictError
		if !errors.As(err, &ce) {
			t.Fatalf("conflict expected: %v", err)
		}
		if got := segs(f, get(t, f, r.svc, x.Booking.ID)); got != "A1@07-10..07-10+A2@07-11..07-12" || f.Count((*store.SpotAllocation)(nil), "") != allocs {
			t.Fatalf("partial group move: x %s", got)
		}
	})
}

// TestUndoMove: the last move is undone by its author within 5 minutes
// when the original spots are still free; never twice, never by another
// user, never after the window, never onto a spot taken meanwhile.
func TestUndoMove(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		x := mustCreate(t, f, r.svc, f.Input(f.Spots[0], "2027-07-10", "2027-07-12"))
		y := mustCreate(t, f, r.svc, f.Input(f.Spots[1], "2027-07-10", "2027-07-12"))
		swap := booking.SwapInput{
			A:    booking.SwapSide{BookingID: x.Booking.ID, LineID: x.Lines[0].ID},
			B:    booking.SwapSide{BookingID: y.Booking.ID, LineID: y.Lines[0].ID},
			From: dd("2027-07-10"),
		}
		res, err := r.svc.Swap(f.Ctx, f.Cashier, f.Est.ID, swap)
		if err != nil {
			t.Fatal(err)
		}
		last, err := r.svc.LastMove(f.Ctx, f.Cashier, f.Est.ID)
		if err != nil || last == nil || last.MoveID != res.Undo.MoveID {
			t.Fatalf("last move %+v %v", last, err)
		}
		if d := get(t, f, r.svc, x.Booking.ID); d.LastMove == nil || d.LastMove.MoveID != res.Undo.MoveID {
			t.Fatalf("detail last move %+v", d.LastMove)
		}
		if _, err := r.svc.UndoMove(f.Ctx, f.Manager, f.Est.ID, res.Undo.MoveID); !errors.Is(err, booking.ErrNotFound) {
			t.Fatalf("other user: %v", err)
		}
		ds, err := r.svc.UndoMove(f.Ctx, f.Cashier, f.Est.ID, res.Undo.MoveID)
		if err != nil {
			t.Fatal(err)
		}
		if len(ds) != 2 || segs(f, get(t, f, r.svc, x.Booking.ID)) != "A1@07-10..07-12" || segs(f, get(t, f, r.svc, y.Booking.ID)) != "A2@07-10..07-12" {
			t.Fatalf("undo did not restore")
		}
		if n := f.Count((*store.AuditEntry)(nil), "action = 'booking.move_undone'"); n != 2 {
			t.Fatalf("undo audits %d", n)
		}
		if _, err := r.svc.UndoMove(f.Ctx, f.Cashier, f.Est.ID, res.Undo.MoveID); !errors.Is(err, booking.ErrMoveUndoConflict) {
			t.Fatalf("second undo: %v", err)
		}
		if last, _ := r.svc.LastMove(f.Ctx, f.Cashier, f.Est.ID); last != nil {
			t.Fatalf("undone move still last")
		}

		// A move whose original spot is taken meanwhile cannot be undone.
		mv, err := r.svc.Move(f.Ctx, f.Cashier, f.Est.ID, x.Booking.ID, booking.MoveInput{LineID: x.Lines[0].ID, SpotID: f.Spots[2].ID})
		if err != nil {
			t.Fatal(err)
		}
		mustCreate(t, f, r.svc, f.Input(f.Spots[0], "2027-07-11", "2027-07-11"))
		if _, err := r.svc.UndoMove(f.Ctx, f.Cashier, f.Est.ID, mv.Undo.MoveID); !errors.Is(err, booking.ErrMoveUndoConflict) {
			t.Fatalf("taken spot: %v", err)
		}
		if got := segs(f, get(t, f, r.svc, x.Booking.ID)); got != "A3@07-10..07-12" {
			t.Fatalf("failed undo changed the booking: %s", got)
		}

		// After the window: 410.
		mv2, err := r.svc.Move(f.Ctx, f.Cashier, f.Est.ID, y.Booking.ID, booking.MoveInput{LineID: y.Lines[0].ID, SpotID: f.Spots[3].ID})
		if err != nil {
			t.Fatal(err)
		}
		f.Clock.Advance(store.MoveUndoWindow + time.Second)
		if _, err := r.svc.UndoMove(f.Ctx, f.Cashier, f.Est.ID, mv2.Undo.MoveID); !errors.Is(err, booking.ErrMoveUndoExpired) {
			t.Fatalf("expired: %v", err)
		}
		// A booking changed after the move cannot be undone.
		mv3, err := r.svc.Move(f.Ctx, f.Cashier, f.Est.ID, y.Booking.ID, booking.MoveInput{LineID: y.Lines[0].ID, SpotID: f.Spots[1].ID})
		if err != nil {
			t.Fatal(err)
		}
		notes := "nota"
		if _, err := r.svc.Update(f.Ctx, f.Cashier, f.Est.ID, y.Booking.ID, nil, booking.PatchInput{NotesInternal: &notes}); err != nil {
			t.Fatal(err)
		}
		if _, err := r.svc.UndoMove(f.Ctx, f.Cashier, f.Est.ID, mv3.Undo.MoveID); !errors.Is(err, booking.ErrMoveUndoConflict) {
			t.Fatalf("changed booking: %v", err)
		}
	})
}

// TestDuplicate: same spot, customer and extras on new dates, re-priced;
// the dry run shows the conflicts of the new dates.
func TestDuplicate(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-11")
		in.Lines[0].Extras = []booking.ExtraInput{{ExtraID: f.Sunbed.ID, Quantity: 1}}
		src := mustCreate(t, f, r.svc, in)
		mustCreate(t, f, r.svc, f.Input(f.Spots[0], "2027-08-02", "2027-08-02"))
		pre, err := r.svc.Duplicate(f.Ctx, f.Cashier, f.Est.ID, src.Booking.ID, booking.DuplicateInput{StartsOn: dd("2027-08-01"), EndsOn: dd("2027-08-03"), DryRun: true})
		if err != nil {
			t.Fatal(err)
		}
		if pre.Applied || len(pre.Conflicts) != 2 || pre.Total != 3*(bookingtest.BasePerDay+bookingtest.SunbedPerDay) {
			t.Fatalf("preview %+v", pre)
		}
		res, err := r.svc.Duplicate(f.Ctx, f.Cashier, f.Est.ID, src.Booking.ID, booking.DuplicateInput{StartsOn: dd("2027-08-05"), EndsOn: dd("2027-08-07")})
		if err != nil {
			t.Fatal(err)
		}
		d := res.Detail
		if !res.Applied || d.Booking.CustomerID != src.Booking.CustomerID || *d.Lines[0].SpotID != f.Spots[0].ID ||
			len(d.Extras) != 1 || d.Booking.Formula != "multi_day" || d.Booking.TotalAmount != 3*(bookingtest.BasePerDay+bookingtest.SunbedPerDay) {
			t.Fatalf("duplicate %+v", d.Booking)
		}
		if _, err := r.svc.Duplicate(f.Ctx, f.Cashier, f.Est.ID, src.Booking.ID, booking.DuplicateInput{StartsOn: dd("2027-08-02"), EndsOn: dd("2027-08-02")}); !errors.Is(err, availability.ErrSpotUnavailable) {
			t.Fatalf("occupied: %v", err)
		}
		if _, err := r.svc.Duplicate(f.Ctx, f.Viewer, f.Est.ID, src.Booking.ID, booking.DuplicateInput{StartsOn: dd("2027-08-20"), EndsOn: dd("2027-08-20")}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer: %v", err)
		}
	})
}

// TestConcurrency_SwapBookings (INV-101): 100 parallel swaps and group
// moves over four weeks, each with three bookings on the same three spots
// (one week alone would only measure the queue on its three booking
// locks): no operation deadlocks, every swap applies, and afterwards every
// line covers its stay exactly once, each spot has one allocation per
// half-day (no double allocation) and the undo log has one entry per
// applied operation.
func TestConcurrency_SwapBookings(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		const weeks = 4
		groups := make([][]*booking.Detail, weeks)
		for w := range weeks {
			from := dd("2027-07-05").AddDays(7 * w)
			for i := range 3 {
				groups[w] = append(groups[w], mustCreate(t, f, r.svc, f.Input(f.Spots[i], from.String(), from.AddDays(4).String())))
			}
		}
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			applied int
			other   []error
		)
		start := make(chan struct{})
		for i := range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				bs, j := groups[i%weeks], i/weeks
				first := bs[0].Booking.StartsOn
				group := j%5 == 4
				var err error
				if group {
					// Group move: rotate the three lines from the third day.
					from := first.AddDays(2)
					_, err = r.svc.GroupMove(f.Ctx, f.Cashier, f.Est.ID, booking.GroupMoveInput{From: &from, Moves: rotate(r.svc, f, bs, from)})
				} else {
					a, b := bs[j%3], bs[(j+1)%3]
					_, err = r.svc.Swap(f.Ctx, f.Cashier, f.Est.ID, booking.SwapInput{
						A: booking.SwapSide{BookingID: a.Booking.ID, LineID: a.Lines[0].ID}, B: booking.SwapSide{BookingID: b.Booking.ID, LineID: b.Lines[0].ID},
						From: first.AddDays(j % 4),
					})
				}
				mu.Lock()
				defer mu.Unlock()
				var ce *availability.ConflictError
				switch {
				case err == nil:
					applied++
				case group && (errors.Is(err, configerr.ErrValidation) || errors.As(err, &ce)):
					// A group move built on a stale read may name a spot
					// twice or a spot taken meanwhile: refused, nothing written.
				default:
					other = append(other, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if len(other) > 0 {
			t.Fatalf("unexpected errors (%d), first: %v", len(other), other[0])
		}
		used := map[string]int{}
		for _, bs := range groups {
			for _, b := range bs {
				d := get(t, f, r.svc, b.Booking.ID)
				checkCover(t, d)
				for _, a := range d.SpotAllocations {
					used[a.SpotID+a.Date.String()+string(a.Slot)]++
				}
			}
		}
		for k, n := range used {
			if n != 1 {
				t.Fatalf("half-day %s allocated %d times", k, n)
			}
		}
		if len(used) != weeks*3*5*2 {
			t.Fatalf("%d half-days allocated, want %d", len(used), weeks*3*5*2)
		}
		if n := f.Count((*store.MoveUndoEntry)(nil), ""); n != applied {
			t.Fatalf("%d undo entries for %d applied operations", n, applied)
		}
	})
}

// rotate builds a group move putting each booking line on the spot the next
// one has on day (read without locks: a stale read is refused).
func rotate(svc *booking.Service, f *bookingtest.Fixture, bs []*booking.Detail, day calendar.Date) []booking.GroupMoveItem {
	var spots []string
	for _, b := range bs {
		d, err := svc.Get(f.Ctx, f.Owner, f.Est.ID, b.Booking.ID)
		if err != nil {
			return nil
		}
		spot := ""
		for _, a := range d.SpotAllocations {
			if a.Date.Equal(day) {
				spot = a.SpotID
			}
		}
		spots = append(spots, spot)
	}
	var out []booking.GroupMoveItem
	for i, b := range bs {
		out = append(out, booking.GroupMoveItem{BookingID: b.Booking.ID, LineID: b.Lines[0].ID, SpotID: spots[(i+1)%len(bs)]})
	}
	return out
}

// TestConcurrency_FindSpotBook (INV-102): 100 parallel bookings of the same
// split proposal: exactly one wins, the others get a conflict and leave
// nothing behind.
func TestConcurrency_FindSpotBook(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		var (
			wg            sync.WaitGroup
			mu            sync.Mutex
			ok, conflicts int
			other         []error
		)
		start := make(chan struct{})
		for range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				in := f.Input(f.Spots[0], "2027-07-10", "2027-07-13")
				in.Lines[0].Segments = []booking.SegmentInput{
					{SpotID: f.Spots[0].ID, StartsOn: dd("2027-07-10"), EndsOn: dd("2027-07-11")},
					{SpotID: f.Spots[1].ID, StartsOn: dd("2027-07-12"), EndsOn: dd("2027-07-13")},
				}
				_, err := r.svc.Create(f.Ctx, f.Cashier, f.Est.ID, in)
				mu.Lock()
				defer mu.Unlock()
				var ce *availability.ConflictError
				switch {
				case err == nil:
					ok++
				case errors.As(err, &ce):
					conflicts++
				default:
					other = append(other, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if len(other) > 0 {
			t.Fatalf("unexpected errors (%d), first: %v", len(other), other[0])
		}
		if ok != 1 || conflicts != parallel-1 {
			t.Fatalf("ok = %d, conflicts = %d", ok, conflicts)
		}
		if n := f.Count((*store.SpotAllocation)(nil), ""); n != 8 {
			t.Fatalf("%d allocations, want 8", n)
		}
		if n := f.Count((*store.Booking)(nil), ""); n != 1 {
			t.Fatalf("%d bookings", n)
		}
	})
}

// TestMove_PastDaysNeverChange (spec 30.7): an explicit from in the past is
// refused by move, swap and group move (422), and a move without from of a
// stay in progress starts today, so the days already past keep their spot.
func TestMove_PastDaysNeverChange(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		a := mustCreate(t, f, r.svc, f.Input(f.Spots[0], "2027-07-10", "2027-07-14"))
		b := mustCreate(t, f, r.svc, f.Input(f.Spots[1], "2027-07-10", "2027-07-14"))
		f.Clock.Set(time.Date(2027, 7, 12, 10, 0, 0, 0, time.UTC))
		past := dd("2027-07-11")
		isFrom422 := func(err error) bool {
			var ve *configerr.ValidationError
			return errors.As(err, &ve) && len(ve.Fields) > 0 && ve.Fields[0].Field == "from"
		}
		if _, err := r.svc.Move(f.Ctx, f.Cashier, f.Est.ID, a.Booking.ID, booking.MoveInput{LineID: a.Lines[0].ID, SpotID: f.Spots[2].ID, From: &past}); !isFrom422(err) {
			t.Fatalf("move from a past date: %v", err)
		}
		if _, err := r.svc.Swap(f.Ctx, f.Cashier, f.Est.ID, booking.SwapInput{
			A: booking.SwapSide{BookingID: a.Booking.ID, LineID: a.Lines[0].ID},
			B: booking.SwapSide{BookingID: b.Booking.ID, LineID: b.Lines[0].ID}, From: past,
		}); !isFrom422(err) {
			t.Fatalf("swap from a past date: %v", err)
		}
		if _, err := r.svc.GroupMove(f.Ctx, f.Cashier, f.Est.ID, booking.GroupMoveInput{
			Moves: []booking.GroupMoveItem{{BookingID: a.Booking.ID, LineID: a.Lines[0].ID, SpotID: f.Spots[2].ID}}, From: &past,
		}); !isFrom422(err) {
			t.Fatalf("group move from a past date: %v", err)
		}
		// Without from: the stay in progress moves from today.
		res, err := r.svc.Move(f.Ctx, f.Cashier, f.Est.ID, a.Booking.ID, booking.MoveInput{LineID: a.Lines[0].ID, SpotID: f.Spots[2].ID})
		if err != nil {
			t.Fatal(err)
		}
		if got := segs(f, res.Detail); got != "A1@07-10..07-11+A3@07-12..07-14" {
			t.Fatalf("move without from of a stay in progress: %s", got)
		}
		checkCover(t, res.Detail)
	})
}
