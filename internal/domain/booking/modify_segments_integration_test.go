//go:build integration

package booking_test

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestModify_KeepsSegments (INV-106): a PATCH that does not replace the
// lines keeps the segments of a split stay and of a scheduled move (same
// allocations, same per-day total); when the dates change the segments are
// fitted to the new stay instead of being collapsed onto one spot.
func TestModify_KeepsSegments(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		vipCategory(t, f, f.Spots[1])
		splitTotal := money.Cents(2*bookingtest.BasePerDay + 2*bookingtest.VIPPerDay)

		// A split stay from find spot: A1 10-11, A2 (vip) 12-13.
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-13")
		in.Lines[0].SpotID = ""
		in.Lines[0].Segments = []booking.SegmentInput{
			{SpotID: f.Spots[0].ID, StartsOn: dd("2027-07-10"), EndsOn: dd("2027-07-11")},
			{SpotID: f.Spots[1].ID, StartsOn: dd("2027-07-12"), EndsOn: dd("2027-07-13")},
		}
		split := mustCreate(t, f, r.svc, in)
		// A1 is taken on 12-13 by someone else: collapsing the split stay
		// onto A1 would be refused (or would double-book).
		mustCreate(t, f, r.svc, f.Input(f.Spots[0], "2027-07-12", "2027-07-13"))

		note := "ombrellone vicino alla doccia"
		patches := map[string]booking.PatchInput{
			"notes":    {NotesInternal: &note},
			"customer": {NotesCustomer: &note},
			"extras":   {ExtrasSet: true, Extras: []booking.ExtraInput{{ExtraID: f.Sunbed.ID, Quantity: 1}}},
			"noextras": {ExtrasSet: true, Extras: []booking.ExtraInput{}},
		}
		for _, name := range []string{"notes", "customer", "extras", "noextras"} {
			res, err := r.svc.Update(f.Ctx, f.Cashier, f.Est.ID, split.Booking.ID, nil, patches[name])
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if got := segs(f, res.Detail); got != "A1@07-10..07-11+A2@07-12..07-13" {
				t.Fatalf("%s: segments %s", name, got)
			}
			checkCover(t, res.Detail)
			want := splitTotal
			if name == "extras" {
				want += 4 * bookingtest.SunbedPerDay
			}
			if res.NewTotal != want {
				t.Fatalf("%s: total %d, want %d", name, res.NewTotal, want)
			}
		}

		// The stay grows by one day at the end: the last spot extends; it
		// shrinks at the start: the first segment is clipped.
		end := dd("2027-07-14")
		res, err := r.svc.Update(f.Ctx, f.Cashier, f.Est.ID, split.Booking.ID, nil, booking.PatchInput{EndsOn: &end})
		if err != nil {
			t.Fatal(err)
		}
		if got := segs(f, res.Detail); got != "A1@07-10..07-11+A2@07-12..07-14" || res.NewTotal != splitTotal+bookingtest.VIPPerDay {
			t.Fatalf("extended: %s total %d", got, res.NewTotal)
		}
		checkCover(t, res.Detail)
		start := dd("2027-07-11")
		if res, err = r.svc.Update(f.Ctx, f.Cashier, f.Est.ID, split.Booking.ID, nil, booking.PatchInput{StartsOn: &start}); err != nil {
			t.Fatal(err)
		}
		if got := segs(f, res.Detail); got != "A1@07-11..07-11+A2@07-12..07-14" {
			t.Fatalf("shrunk: %s", got)
		}
		checkCover(t, res.Detail)
		// Moved entirely after the split: the spot of the line (the first
		// segment's) for the whole new stay.
		s2, e2 := dd("2027-07-20"), dd("2027-07-21")
		if res, err = r.svc.Update(f.Ctx, f.Cashier, f.Est.ID, split.Booking.ID, nil, booking.PatchInput{StartsOn: &s2, EndsOn: &e2}); err != nil {
			t.Fatal(err)
		}
		if got := segs(f, res.Detail); got != "A1@07-20..07-21" || res.NewTotal != 2*bookingtest.BasePerDay {
			t.Fatalf("relocated: %s total %d", got, res.NewTotal)
		}
		checkCover(t, res.Detail)

		// A scheduled move: A3 for 10-14, from 13 on A2 (vip). The days
		// before 13 never move to A2, whatever is edited later.
		d := mustCreate(t, f, r.svc, f.Input(f.Spots[2], "2027-07-10", "2027-07-14"))
		from := dd("2027-07-13")
		mv, err := r.svc.Move(f.Ctx, f.Cashier, f.Est.ID, d.Booking.ID, booking.MoveInput{LineID: d.Lines[0].ID, SpotID: f.Spots[1].ID, From: &from})
		if err != nil {
			t.Fatal(err)
		}
		movedTotal := money.Cents(3*bookingtest.BasePerDay + 2*bookingtest.VIPPerDay)
		if got := segs(f, mv.Detail); got != "A3@07-10..07-12+A2@07-13..07-14" || mv.NewTotal != movedTotal {
			t.Fatalf("move: %s total %d", got, mv.NewTotal)
		}
		// A2 is busy on 10-12 (the other booking cannot take it either).
		mustCreate(t, f, r.svc, f.Input(f.Spots[1], "2027-07-10", "2027-07-12"))
		res, err = r.svc.Update(f.Ctx, f.Cashier, f.Est.ID, d.Booking.ID, nil, booking.PatchInput{NotesInternal: &note})
		if err != nil {
			t.Fatal(err)
		}
		if got := segs(f, res.Detail); got != "A3@07-10..07-12+A2@07-13..07-14" || res.NewTotal != movedTotal {
			t.Fatalf("notes after a scheduled move: %s total %d", got, res.NewTotal)
		}
		checkCover(t, res.Detail)
		if res.Detail.Booking.NotesInternal != note {
			t.Fatalf("note not saved")
		}

		// Replacing the lines explicitly onto one busy spot is still a
		// conflict, never a silent double booking.
		_, err = r.svc.Update(f.Ctx, f.Cashier, f.Est.ID, d.Booking.ID, nil, booking.PatchInput{
			Lines: []booking.LineInput{{PackID: f.Base.ID, SpotID: f.Spots[1].ID}},
		})
		var ce *availability.ConflictError
		if !errors.As(err, &ce) {
			t.Fatalf("lines replaced onto a busy spot: %v", err)
		}
	})
}
