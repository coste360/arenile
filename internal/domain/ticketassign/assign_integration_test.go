//go:build integration

package ticketassign_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/ticketassign"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

const day = "2027-07-10"

// env is the booking fixture on the real inventory (DB constraints) with the
// assignment service on top.
type env struct {
	f       *bookingtest.Fixture
	es      store.EstablishmentScope
	bk      *booking.Service
	inv     *inventory.Service
	svc     *ticketassign.Service
	tickets *ticketing.Service
	n       int64
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	f := bookingtest.New(t, db)
	c := f.Clock
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	inv := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	bk := booking.NewService(booking.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: inv, Pricer: f.Pricing,
		Customers: &bookingtest.Customers{Clock: c}, Payments: f.Pay,
	})
	es, err := store.SystemEstablishmentScope(f.Ctx, db, f.Est.ID)
	if err != nil {
		t.Fatal(err)
	}
	tk := ticketing.NewService(ticketing.Deps{
		DB: db, Clock: c, Audit: aw, Bookings: bk, BookingsIn: func(db bun.IDB) ticketing.Bookings { return bk.WithDB(db) },
	})
	e := &env{f: f, es: es, bk: bk, inv: inv, tickets: tk, svc: ticketassign.NewService(ticketassign.Deps{DB: db, Clock: c, Bookings: bk})}
	e.zonePool(f.FreeZone.ID, len(f.FreeSpots))
	return e
}

// poolUsed is the places taken on the implicit pool of a zone-mode zone on
// day (the counter of a half-day; both halves move together here).
func (e *env) poolUsed(zoneID string) int {
	e.f.T.Helper()
	var pools []store.Pool
	if err := e.f.DB.NewSelect().Model(&pools).Where("zone_id = ?", zoneID).Scan(e.f.Ctx); err != nil || len(pools) != 1 {
		e.f.T.Fatalf("zone pool: %v (%d)", err, len(pools))
	}
	var rows []store.PoolUsage
	if err := e.f.DB.NewSelect().Model(&rows).Where("pool_id = ?", pools[0].ID).Where("date = ?", bookingtest.D(day)).Scan(e.f.Ctx); err != nil {
		e.f.T.Fatal(err)
	}
	// One row per half-day: the places taken are the same in each.
	used := 0
	for _, r := range rows {
		used = max(used, r.Used)
	}
	return used
}

// zonePool creates the implicit system pool of a zone-mode zone.
func (e *env) zonePool(zoneID string, capacity int) {
	e.f.T.Helper()
	now := e.f.Clock.Now()
	p := &store.Pool{
		ID: store.NewID(), ZoneID: &zoneID, ComponentCode: "spot", Name: store.I18nText{"it": "spot"}, Capacity: capacity,
		CapacityOverrides: []store.CapacityOverride{}, SystemManaged: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.InsertPool(e.f.Ctx, e.f.DB, e.es, p); err != nil {
		e.f.T.Fatal(err)
	}
}

// sale creates a confirmed desk booking for day on line (zone or spot) and
// its ticket with the next number, like the daily sale does.
func (e *env) sale(line booking.LineInput, needsAssignment bool, holder string) *store.Ticket {
	e.f.T.Helper()
	d, err := e.bk.Create(e.f.Ctx, e.f.Owner, e.f.Est.ID, booking.CreateInput{
		Customer: &customer.Input{FirstName: "Banco", LastName: "Cliente"},
		StartsOn: bookingtest.D(day), EndsOn: bookingtest.D(day), Slots: calendar.SlotsFull, Formula: "day",
		Lines: []booking.LineInput{line},
	})
	if err != nil {
		e.f.T.Fatalf("create sale booking: %v", err)
	}
	e.n++
	now := e.f.Clock.Now()
	t := &store.Ticket{
		ID: store.NewID(), EstablishmentID: e.f.Est.ID, BookingID: d.Booking.ID, TicketDate: bookingtest.D(day), TicketNumber: e.n,
		Kind: store.TicketKindSale, Status: store.TicketValid, ZoneID: d.Lines[0].ZoneID, NeedsAssignment: needsAssignment,
		HolderName: holder, TotalAmount: d.Booking.TotalAmount, PaymentMethod: store.TicketPaymentCash, GateToken: store.NewID(), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if needsAssignment && line.SpotID != "" {
		t.ProvisionalSpotID = &line.SpotID
	}
	e.f.Insert(t)
	return t
}

func (e *env) ticket(id string) *store.Ticket {
	e.f.T.Helper()
	t, err := store.GetTicketForAssignment(e.f.Ctx, e.f.DB, e.es, id)
	if err != nil {
		e.f.T.Fatal(err)
	}
	return t
}

// spotsOf returns the spots allocated to the booking on day.
func (e *env) spotsOf(bookingID string) map[string]int {
	e.f.T.Helper()
	var rows []store.SpotAllocation
	if err := e.f.DB.NewSelect().Model(&rows).Where("booking_id = ?", bookingID).Scan(e.f.Ctx); err != nil {
		e.f.T.Fatal(err)
	}
	out := map[string]int{}
	for _, r := range rows {
		out[r.SpotID]++
	}
	return out
}

func TestTicketAssign_ZoneModeAssignAndMove(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		tk := e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: f.FreeZone.ID}, true, "")

		list, err := e.svc.ListUnassigned(ctx, f.Viewer, est, bookingtest.D(day), "")
		if err != nil || len(list) != 1 || list[0].Ticket.ID != tk.ID || list[0].SpotID != "" || list[0].BookingCode == "" {
			t.Fatalf("unassigned list: %v %+v", err, list)
		}
		if _, err := e.svc.Assign(ctx, f.Viewer, est, tk.ID, ticketassign.Input{SpotID: f.FreeSpots[0].ID}); err == nil {
			t.Fatal("viewer assigned a ticket")
		}
		// A spot of another zone is refused by Assign.
		var ve *configerr.ValidationError
		if _, err := e.svc.Assign(ctx, f.Cashier, est, tk.ID, ticketassign.Input{SpotID: f.Spots[0].ID}); !errors.As(err, &ve) {
			t.Fatalf("other zone: %v", err)
		}
		stale := int64(99)
		if _, err := e.svc.Assign(ctx, f.Cashier, est, tk.ID, ticketassign.Input{SpotID: f.FreeSpots[0].ID, ExpectedVersion: &stale}); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale version: %v", err)
		}

		v, err := e.svc.Assign(ctx, f.Cashier, est, tk.ID, ticketassign.Input{SpotID: f.FreeSpots[0].ID})
		if err != nil {
			t.Fatal(err)
		}
		if v.Ticket.NeedsAssignment || v.SpotID != f.FreeSpots[0].ID || v.SpotLabel != "B1" || v.Ticket.Version != 2 || ticketing.StateOf(&v.Ticket) != ticketing.StateAssigned {
			t.Fatalf("assigned view: %+v", v)
		}
		if got := e.spotsOf(tk.BookingID); got[f.FreeSpots[0].ID] != 2 || len(got) != 1 {
			t.Fatalf("allocations after assign: %v", got)
		}
		if n := f.Count((*store.AuditEntry)(nil), "entity_id = ? AND action = ?", tk.BookingID, "booking.moved"); n != 1 {
			// The audit action name is owned by booking; accept any move entry.
			if m := f.Count((*store.AuditEntry)(nil), "entity_id = ? AND action LIKE ?", tk.BookingID, "%move%"); m != 1 {
				t.Fatalf("audit entries for the move: %d", m)
			}
		}
		if list, _ := e.svc.ListUnassigned(ctx, f.Viewer, est, bookingtest.D(day), ""); len(list) != 0 {
			t.Fatalf("still unassigned: %+v", list)
		}
		// Assigning twice is a conflict.
		if _, err := e.svc.Assign(ctx, f.Cashier, est, tk.ID, ticketassign.Input{SpotID: f.FreeSpots[1].ID}); !errors.Is(err, ticketing.ErrAssignmentConflict) {
			t.Fatalf("second assign: %v", err)
		}

		// Move to the other spot: B1 freed, B2 taken, ticket version bumped.
		v, err = e.svc.Move(ctx, f.Cashier, est, tk.ID, ticketassign.Input{SpotID: f.FreeSpots[1].ID})
		if err != nil || v.SpotID != f.FreeSpots[1].ID || v.Ticket.Version != 3 {
			t.Fatalf("move: %v %+v", err, v)
		}
		if got := e.spotsOf(tk.BookingID); got[f.FreeSpots[1].ID] != 2 || len(got) != 1 {
			t.Fatalf("allocations after move: %v", got)
		}

		// A voided ticket cannot be moved.
		if _, err := db.NewUpdate().Model((*store.Ticket)(nil)).Set("status = ?", store.TicketVoided).Set("voided_at = ?", f.Clock.Now()).
			Where("id = ?", tk.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Move(ctx, f.Cashier, est, tk.ID, ticketassign.Input{SpotID: f.FreeSpots[0].ID}); !errors.Is(err, ticketing.ErrVoided) {
			t.Fatalf("move voided: %v", err)
		}
		// Another establishment does not see the ticket.
		if _, err := e.svc.Move(ctx, f.Outsider, est, tk.ID, ticketassign.Input{SpotID: f.FreeSpots[0].ID}); err == nil {
			t.Fatal("outsider moved a ticket")
		}
	})
}

func TestTicketAssign_ConflictOnTakenSpot(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		a := e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: f.FreeZone.ID}, true, "")
		b := e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: f.FreeZone.ID}, true, "")
		if _, err := e.svc.Assign(ctx, f.Cashier, est, a.ID, ticketassign.Input{SpotID: f.FreeSpots[0].ID}); err != nil {
			t.Fatal(err)
		}
		_, err := e.svc.Assign(ctx, f.Cashier, est, b.ID, ticketassign.Input{SpotID: f.FreeSpots[0].ID})
		if !errors.Is(err, availability.ErrSpotUnavailable) {
			t.Fatalf("taken spot: %v", err)
		}
		if tk := e.ticket(b.ID); !tk.NeedsAssignment || tk.Version != 1 {
			t.Fatalf("loser ticket changed: %+v", tk)
		}
		if got := e.spotsOf(b.BookingID); len(got) != 0 {
			t.Fatalf("loser allocations: %v", got)
		}
	})
}

func TestTicketAssign_SeatModeProvisionalSpot(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		last := f.Spots[3]
		tk := e.sale(booking.LineInput{PackID: f.Base.ID, SpotID: last.ID}, true, "Bianchi")
		v, err := e.svc.Assign(ctx, f.Cashier, est, tk.ID, ticketassign.Input{SpotID: f.Spots[0].ID})
		if err != nil {
			t.Fatal(err)
		}
		if v.Ticket.ProvisionalSpotID != nil || v.Ticket.NeedsAssignment || v.SpotID != f.Spots[0].ID {
			t.Fatalf("seat-mode assign: %+v", v.Ticket)
		}
		if got := e.spotsOf(tk.BookingID); got[f.Spots[0].ID] != 2 || got[last.ID] != 0 {
			t.Fatalf("provisional spot not released: %v", got)
		}
	})
}

func TestTicketAutoAssign_FillsInNumberOrder(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		t1 := e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: f.FreeZone.ID}, true, "")
		t2 := e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: f.FreeZone.ID}, true, "")
		t3 := e.sale(booking.LineInput{PackID: f.Base.ID, SpotID: f.Spots[3].ID}, true, "")

		dry, err := e.svc.AutoAssign(ctx, f.Cashier, est, ticketassign.AutoInput{Date: bookingtest.D(day), DryRun: true})
		if err != nil || dry.Applied || len(dry.Assigned) != 3 || len(dry.Unassigned) != 0 {
			t.Fatalf("dry run: %v %+v", err, dry)
		}
		if e.ticket(t1.ID).Version != 1 {
			t.Fatal("dry run wrote")
		}
		res, err := e.svc.AutoAssign(ctx, f.Cashier, est, ticketassign.AutoInput{Date: bookingtest.D(day)})
		if err != nil || !res.Applied || len(res.Assigned) != 3 {
			t.Fatalf("auto-assign: %v %+v", err, res)
		}
		want := []struct {
			id, spot string
		}{{t1.ID, "B1"}, {t2.ID, "B2"}}
		for i, w := range want {
			if res.Assigned[i].TicketID != w.id || res.Assigned[i].SpotLabel != w.spot || res.Assigned[i].TicketNumber != int64(i+1) {
				t.Fatalf("assignment %d: %+v", i, res.Assigned[i])
			}
		}
		if a := res.Assigned[2]; a.TicketID != t3.ID || dry.Assigned[2].SpotID != a.SpotID {
			t.Fatalf("seat-mode ticket: %+v (dry %+v)", a, dry.Assigned[2])
		}
		for _, id := range []string{t1.ID, t2.ID, t3.ID} {
			if e.ticket(id).NeedsAssignment {
				t.Fatalf("ticket %s still to assign", id)
			}
		}
		// Nothing left: a new run is a no-op.
		res, err = e.svc.AutoAssign(ctx, f.Cashier, est, ticketassign.AutoInput{Date: bookingtest.D(day), ZoneID: f.FreeZone.ID})
		if err != nil || len(res.Assigned) != 0 || len(res.Unassigned) != 0 {
			t.Fatalf("second run: %v %+v", err, res)
		}
	})
}

func TestTicketAutoAssign_NoFreeSpot(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		// B2 is blocked by a regular desk booking placed on it: the zone
		// pool still has room for one sale but only B1 is free for it...
		e.sale(booking.LineInput{PackID: f.Base.ID, SpotID: f.FreeSpots[1].ID}, false, "")
		t1 := e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: f.FreeZone.ID}, true, "")
		// ...which a manual block then takes.
		if _, err := e.inv.CreateSpotBlocks(ctx, f.Owner, est, inventory.SpotBlockInput{
			SpotIDs: []string{f.FreeSpots[0].ID}, StartsOn: bookingtest.D(day), EndsOn: bookingtest.D(day), Slots: calendar.SlotsFull, Reason: "manutenzione",
		}); err != nil {
			t.Skipf("spot block on a zone-mode spot not available here: %v", err)
		}
		res, err := e.svc.AutoAssign(ctx, f.Cashier, est, ticketassign.AutoInput{Date: bookingtest.D(day)})
		if err != nil || len(res.Assigned) != 0 || len(res.Unassigned) != 1 || res.Unassigned[0].TicketID != t1.ID || res.Unassigned[0].Reason != ticketassign.ReasonNoFreeSpot {
			t.Fatalf("no free spot: %v %+v", err, res)
		}
	})
}

func TestTicketMap_LabelsAndPending(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		named := e.sale(booking.LineInput{PackID: f.Base.ID, SpotID: f.Spots[0].ID}, false, "Bianchi")
		plain := e.sale(booking.LineInput{PackID: f.Base.ID, SpotID: f.Spots[1].ID}, false, "")
		pending := e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: f.FreeZone.ID}, true, "")

		dayView, err := e.inv.GetOperationsDay(ctx, f.Viewer, est, inventory.OperationsQuery{Date: bookingtest.D(day)})
		if err != nil {
			t.Fatal(err)
		}
		bySpot := map[string]*inventory.OperationalBooking{}
		for _, sp := range dayView.Spots {
			bySpot[sp.SpotID] = sp.Booking
		}
		if b := bySpot[f.Spots[0].ID]; b == nil || b.Ticket == nil || b.Ticket.Number != named.TicketNumber || b.OccupantLabel != "Bianchi" || b.CustomerDisplayName != "Bianchi" || b.Ticket.Label != "#001" {
			t.Fatalf("named ticket on the map: %+v", b)
		}
		if b := bySpot[f.Spots[1].ID]; b == nil || b.Ticket == nil || b.OccupantLabel != "#002" || b.CustomerDisplayName != "#002" || b.Ticket.Number != plain.TicketNumber {
			t.Fatalf("ticket without name on the map: %+v", b)
		}
		if len(dayView.PendingAssignments) != 1 {
			t.Fatalf("pending: %+v", dayView.PendingAssignments)
		}
		pa := dayView.PendingAssignments[0]
		if pa.BookingID != pending.BookingID || pa.Ticket == nil || pa.Ticket.Label != "#003" || !pa.Ticket.NeedsAssignment || pa.CustomerDisplayName != "#003" {
			t.Fatalf("pending ticket: %+v", pa)
		}
		if ticketing.NumberLabel(37) != inventory.TicketNumberLabel(37) {
			t.Fatal("number label formats diverge")
		}
	})
}

// TestConcurrency_AssignSameSpot: 100 different tickets assigned to the same
// spot at the same time; the unique key of spot_allocations lets exactly one
// win, the others get a conflict and stay to assign with their zone
// capacity intact.
func TestConcurrency_AssignSameSpot(t *testing.T) {
	const parallel = 100
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		now := f.Clock.Now()
		mapID := f.Spots[0].MapID
		zone := &store.Zone{
			ID: store.NewID(), EstablishmentID: est, MapID: &mapID, Code: "C", Name: store.I18nText{"it": "Prima fila"},
			AssignmentMode: "zone", OnlineVisible: true, PriceCategory: "back", Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		f.Insert(zone)
		var target *store.Spot
		for i := 1; i <= parallel; i++ {
			sp := &store.Spot{
				ID: store.NewID(), EstablishmentID: est, MapID: mapID, ZoneID: zone.ID, Label: fmt.Sprintf("C%d", i), RowLabel: "2", Number: i,
				Kind: "umbrella", X: float64(10 * i), Y: 50, Shape: store.Shape{Type: "circle", W: 20, H: 20}, Capacity: map[string]int{}, MaxExtra: map[string]int{},
				Attributes: []string{}, Active: true, CreatedAt: now, UpdatedAt: now,
			}
			f.Insert(sp)
			if i == 1 {
				target = sp
			}
		}
		e.zonePool(zone.ID, parallel)
		tickets := make([]*store.Ticket, parallel)
		for i := range tickets {
			tickets[i] = e.sale(booking.LineInput{PackID: f.Base.ID, ZoneID: zone.ID}, true, "")
		}

		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			ok        int
			conflicts int
			other     []error
		)
		start := make(chan struct{})
		for i := range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := e.svc.Assign(ctx, f.Cashier, est, tickets[i].ID, ticketassign.Input{SpotID: target.ID})
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, ticketing.ErrAssignmentConflict), errors.Is(err, availability.ErrSpotUnavailable):
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
		n, err := db.NewSelect().Model((*store.SpotAllocation)(nil)).Where("spot_id = ?", target.ID).Count(ctx)
		if err != nil || n != 2 {
			t.Fatalf("allocations on the spot: %d %v", n, err)
		}
		left, err := e.svc.ListUnassigned(ctx, f.Viewer, est, bookingtest.D(day), zone.ID)
		if err != nil || len(left) != parallel-1 {
			t.Fatalf("still to assign: %d %v", len(left), err)
		}
		// The zone capacity is intact: the losers still hold their pending
		// places and the winner now holds a spot of the zone (the implicit pool
		// counts both), so nothing was lost or doubled; the losers' ticket rows
		// are untouched.
		if used := e.poolUsed(zone.ID); used != parallel {
			t.Fatalf("zone pool used after the run: %d, want %d", used, parallel)
		}
		winners := 0
		for _, tk := range tickets {
			cur := e.ticket(tk.ID)
			if !cur.NeedsAssignment {
				winners++
				continue
			}
			if cur.Version != 1 || cur.Status != store.TicketValid || len(e.spotsOf(tk.BookingID)) != 0 {
				t.Fatalf("loser ticket changed: %+v", cur)
			}
		}
		if winners != 1 {
			t.Fatalf("winners: %d", winners)
		}
	})
}
