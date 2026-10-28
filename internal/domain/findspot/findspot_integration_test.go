//go:build integration

package findspot_test

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/findspot"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var d = bookingtest.D

// occupy books spot from..to for another customer.
func occupy(t *testing.T, f *bookingtest.Fixture, spot *store.Spot, from, to string) {
	t.Helper()
	f.MustCreate(f.Input(spot, from, to))
}

// TestBookFindSpotProposal_SegmentsAllocated (INV-102): with every seat
// spot busy on part of the stay, the search proposes a split stay; booking
// it allocates one spot per half-day along the segments, priced as quoted;
// booking it again is refused by the database.
func TestBookFindSpotProposal_SegmentsAllocated(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		svc := findspot.NewService(findspot.Deps{DB: db, Clock: f.Clock, Quoter: f.Svc, Booker: f.Svc})
		// A1 busy 12-13, A2 busy 10-11, A3 and A4 busy all stay.
		occupy(t, f, f.Spots[0], "2027-07-12", "2027-07-13")
		occupy(t, f, f.Spots[1], "2027-07-10", "2027-07-11")
		occupy(t, f, f.Spots[2], "2027-07-10", "2027-07-13")
		occupy(t, f, f.Spots[3], "2027-07-10", "2027-07-13")
		pack := f.Base.ID
		q := findspot.Query{StartsOn: d("2027-07-10"), EndsOn: d("2027-07-13"), Slots: "FULL", ZoneIDs: []string{f.SeatZone.ID}, PackID: &pack}

		res, err := svc.Search(f.Ctx, f.Cashier, f.Est.ID, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Proposals) != 0 {
			t.Fatalf("no change allowed: %d proposals", len(res.Proposals))
		}
		q.MaxSpotChanges = 1
		res, err = svc.Search(f.Ctx, f.Cashier, f.Est.ID, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Proposals) != 1 {
			t.Fatalf("%d proposals, want 1", len(res.Proposals))
		}
		p := res.Proposals[0]
		if p.Proposal.SpotChanges != 1 || p.Total == nil || *p.Total != 4*bookingtest.BasePerDay || p.Rank != 1 || p.ID == "" {
			t.Fatalf("proposal %+v", p)
		}
		if res.Spots[f.Spots[0].ID].Label != "A1" || res.Spots[f.Spots[1].ID].Label != "A2" {
			t.Fatalf("spot info %+v", res.Spots)
		}

		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-13")
		in.Lines = findspot.ProposalLines(p.Proposal, f.Base.ID)
		wrong := money.Cents(1)
		if _, err := svc.Book(f.Ctx, f.Cashier, f.Est.ID, in, &wrong); !errors.Is(err, ticketing.ErrPriceChanged) {
			t.Fatalf("price changed: %v", err)
		}
		det, err := svc.Book(f.Ctx, f.Cashier, f.Est.ID, in, p.Total)
		if err != nil {
			t.Fatal(err)
		}
		ls := booking.LineSegments(det)[det.Lines[0].ID]
		if len(ls) != 2 || ls[0].SpotID != f.Spots[0].ID || ls[1].SpotID != f.Spots[1].ID ||
			!ls[0].EndsOn.Equal(d("2027-07-11")) || !ls[1].StartsOn.Equal(d("2027-07-12")) || len(det.SpotAllocations) != 8 {
			t.Fatalf("segments %+v, %d allocations", ls, len(det.SpotAllocations))
		}
		// The same proposal again: the half-days are taken.
		in2 := f.Input(f.Spots[0], "2027-07-10", "2027-07-13")
		in2.Lines = findspot.ProposalLines(p.Proposal, f.Base.ID)
		var ce *availability.ConflictError
		if _, err := svc.Book(f.Ctx, f.Cashier, f.Est.ID, in2, nil); !errors.As(err, &ce) {
			t.Fatalf("second booking: %v", err)
		}
		// And the search finds nothing left.
		res, err = svc.Search(f.Ctx, f.Cashier, f.Est.ID, q)
		if err != nil || len(res.Proposals) != 0 {
			t.Fatalf("after booking: %d proposals, %v", len(res.Proposals), err)
		}
	})
}

// TestSearch_GroupAdjacentAndPermissions: a group of two gets adjacent
// spots first; a principal of another organization cannot search.
func TestSearch_GroupAdjacentAndPermissions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		svc := findspot.NewService(findspot.Deps{DB: db, Clock: f.Clock, Booker: f.Svc})
		occupy(t, f, f.Spots[1], "2027-07-10", "2027-07-10")
		q := findspot.Query{StartsOn: d("2027-07-10"), EndsOn: d("2027-07-11"), Slots: "FULL", Count: 2, ZoneIDs: []string{f.SeatZone.ID}}
		res, err := svc.Search(f.Ctx, f.Cashier, f.Est.ID, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Proposals) == 0 || !res.Proposals[0].Proposal.Adjacent {
			t.Fatalf("proposals %+v", res.Proposals)
		}
		first := res.Proposals[0].Proposal.SpotIDs()
		if first[0].String() != f.Spots[2].ID || first[1].String() != f.Spots[3].ID {
			t.Fatalf("first group %v", first)
		}
		if res.Proposals[0].Total != nil {
			t.Fatalf("priced without a pack")
		}
		if _, err := svc.Search(f.Ctx, f.Outsider, f.Est.ID, q); err == nil {
			t.Fatalf("outsider searched")
		}
		q.Count = 21
		if _, err := svc.Search(f.Ctx, f.Cashier, f.Est.ID, q); err == nil {
			t.Fatalf("count 21 accepted")
		}
	})
}
