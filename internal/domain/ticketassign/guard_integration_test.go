//go:build integration

package ticketassign_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/ticketassign"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestTicketAssign_GenericBookingChangesRefused: the booking of a ticket is
// changed only through the ticket endpoints (INV-83, INV-84): the generic
// Move, Update and Cancel answer ErrTicketOwned, and the pending-lines
// auto-assignment of spec 7.5 leaves ticket lines alone, so the ticket row
// never drifts from its booking.
func TestTicketAssign_GenericBookingChangesRefused(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		tk := e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: f.FreeZone.ID}, true, "")
		lines, err := store.ListBookingLines(ctx, db, e.es, tk.BookingID)
		if err != nil || len(lines) != 1 {
			t.Fatalf("lines: %v %d", err, len(lines))
		}
		move := booking.MoveInput{LineID: lines[0].ID, SpotID: f.FreeSpots[0].ID}
		if _, err := e.bk.Move(ctx, f.Cashier, est, tk.BookingID, move); !errors.Is(err, booking.ErrTicketOwned) {
			t.Fatalf("generic move: %v", err)
		}
		if _, err := e.bk.Cancel(ctx, f.Cashier, est, tk.BookingID, booking.CancelInput{Reason: "x", Settlement: booking.SettleNone}); !errors.Is(err, booking.ErrTicketOwned) {
			t.Fatalf("generic cancel: %v", err)
		}
		to := bookingtest.D("2027-07-11")
		if _, err := e.bk.Update(ctx, f.Cashier, est, tk.BookingID, nil, booking.PatchInput{StartsOn: &to, EndsOn: &to}); !errors.Is(err, booking.ErrTicketOwned) {
			t.Fatalf("generic modify: %v", err)
		}
		// The generic auto-assignment (pending lines of zone-mode zones)
		// skips the ticket: nothing assigned, the ticket still to assign.
		res, err := e.bk.AutoAssign(ctx, f.Cashier, est, booking.AutoAssignInput{From: bookingtest.D(day), To: bookingtest.D(day)})
		if err != nil || len(res.Assigned) != 0 || len(res.Unassigned) != 0 {
			t.Fatalf("booking auto-assign touched the ticket: %v %+v", err, res)
		}
		if cur := e.ticket(tk.ID); !cur.NeedsAssignment || cur.Version != 1 {
			t.Fatalf("ticket changed: %+v", cur)
		}
		if got := e.spotsOf(tk.BookingID); len(got) != 0 {
			t.Fatalf("allocations after the refused changes: %v", got)
		}
		// The ticket's own actions still work: assign, then void (which is
		// the only cancellation of a ticket booking).
		if _, err := e.svc.Assign(ctx, f.Cashier, est, tk.ID, ticketassign.Input{SpotID: f.FreeSpots[0].ID}); err != nil {
			t.Fatal(err)
		}
		v, err := e.tickets.Void(ctx, f.Cashier, est, tk.ID, ticketing.VoidInput{Reason: "cliente andato via", Refund: ticketing.VoidNone})
		if err != nil || v.State != ticketing.StateVoided {
			t.Fatalf("void: %v %+v", err, v)
		}
		d, _ := e.bk.Get(ctx, f.Viewer, est, tk.BookingID)
		if d.Booking.Status != string(booking.StatusCancelled) || len(d.SpotAllocations) != 0 {
			t.Fatalf("booking after void: %s, %d allocations", d.Booking.Status, len(d.SpotAllocations))
		}
	})
}

// TestTicketAssign_KeepsTicketPrice: an assignment or a move never changes
// the total of a ticket booking, whatever the price of the new spot (the
// guest paid at the desk); the engine price stays in the audit diff.
func TestTicketAssign_KeepsTicketPrice(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		tk := e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: f.FreeZone.ID}, true, "")
		before := tk.TotalAmount
		if _, err := e.svc.Assign(ctx, f.Cashier, est, tk.ID, ticketassign.Input{SpotID: f.FreeSpots[0].ID}); err != nil {
			t.Fatal(err)
		}
		// A move to another zone (another price category) keeps the price too.
		if _, err := e.svc.Move(ctx, f.Cashier, est, tk.ID, ticketassign.Input{SpotID: f.Spots[0].ID}); err != nil {
			t.Fatal(err)
		}
		d, err := e.bk.Get(ctx, f.Viewer, est, tk.BookingID)
		if err != nil {
			t.Fatal(err)
		}
		if d.Booking.TotalAmount != before {
			t.Fatalf("total %d, ticket %d", d.Booking.TotalAmount, before)
		}
		if n := f.Count((*store.AuditEntry)(nil), "entity_id = ? AND action LIKE ? AND CAST(diff AS TEXT) LIKE ?", tk.BookingID, "%move%", "%price_kept%"); n != 2 {
			t.Fatalf("audit entries with price_kept: %d", n)
		}
	})
}

// bigZone adds a zone-mode zone with n spots and a pool of n places.
func (e *env) bigZone(code string, n int) (*store.Zone, []*store.Spot) {
	f := e.f
	f.T.Helper()
	now := f.Clock.Now()
	mapID := f.Spots[0].MapID
	zone := &store.Zone{
		ID: store.NewID(), EstablishmentID: f.Est.ID, MapID: &mapID, Code: code, Name: store.I18nText{"it": code},
		AssignmentMode: "zone", OnlineVisible: true, PriceCategory: "back", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.Insert(zone)
	spots := make([]*store.Spot, 0, n)
	for i := 1; i <= n; i++ {
		sp := &store.Spot{
			ID: store.NewID(), EstablishmentID: f.Est.ID, MapID: mapID, ZoneID: zone.ID, Label: fmt.Sprintf("%s%d", code, i), RowLabel: "2", Number: i,
			Kind: "umbrella", X: float64(10 * i), Y: 50, Shape: store.Shape{Type: "circle", W: 20, H: 20}, Capacity: map[string]int{}, MaxExtra: map[string]int{},
			Attributes: []string{}, Active: true, CreatedAt: now, UpdatedAt: now,
		}
		f.Insert(sp)
		spots = append(spots, sp)
	}
	e.zonePool(zone.ID, n)
	return zone, spots
}

// TestConcurrency_VoidVsAssign: 100 tickets, each voided and assigned at the
// same time. Both paths lock the booking row first and the ticket row
// second, so there is no deadlock: every pair ends with the ticket either
// voided (allocations released) or assigned (two half-day allocations on
// its spot), and no caller gets an unexpected error.
func TestConcurrency_VoidVsAssign(t *testing.T) {
	const parallel = 100
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		zone, spots := e.bigZone("V", parallel)
		tickets := make([]*store.Ticket, parallel)
		for i := range tickets {
			tickets[i] = e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: zone.ID}, true, "")
		}
		expected := []error{ticketing.ErrVoided, ticketing.ErrAssignmentConflict, availability.ErrSpotUnavailable, configerr.ErrVersionConflict, booking.ErrNotModifiable}
		var (
			wg    sync.WaitGroup
			mu    sync.Mutex
			other []error
		)
		record := func(err error) {
			if err == nil {
				return
			}
			for _, want := range expected {
				if errors.Is(err, want) {
					return
				}
			}
			mu.Lock()
			other = append(other, err)
			mu.Unlock()
		}
		start := make(chan struct{})
		for i := range parallel {
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				_, err := e.tickets.Void(ctx, f.Cashier, est, tickets[i].ID, ticketing.VoidInput{Reason: "x", Refund: ticketing.VoidNone})
				record(err)
			}()
			go func() {
				defer wg.Done()
				<-start
				_, err := e.svc.Assign(ctx, f.Cashier, est, tickets[i].ID, ticketassign.Input{SpotID: spots[i].ID})
				record(err)
			}()
		}
		close(start)
		wg.Wait()
		if len(other) > 0 {
			t.Fatalf("unexpected errors (%d), first: %v", len(other), other[0])
		}
		voided := 0
		for i, tk := range tickets {
			cur := e.ticket(tk.ID)
			allocs := e.spotsOf(tk.BookingID)
			switch {
			case cur.Status == store.TicketVoided:
				voided++
				if len(allocs) != 0 {
					t.Fatalf("voided ticket %d keeps allocations: %v", i, allocs)
				}
			case !cur.NeedsAssignment:
				if allocs[spots[i].ID] != 2 || len(allocs) != 1 {
					t.Fatalf("assigned ticket %d allocations: %v", i, allocs)
				}
			default:
				t.Fatalf("ticket %d neither voided nor assigned: %+v", i, cur)
			}
		}
		// The zone pool holds exactly the assigned tickets (their spots count
		// in the implicit pool); the voided ones released their place.
		if used := e.poolUsed(zone.ID); used != parallel-voided {
			t.Fatalf("zone pool used: %d, want %d (voided %d)", used, parallel-voided, voided)
		}
	})
}
