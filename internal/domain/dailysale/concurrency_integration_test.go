//go:build integration

package dailysale_test

import (
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/dailysale"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestConcurrency_DailyTicketNumbering: 200 sales of the same day at once
// on three registers (three open cash sessions) get the numbers 1..200,
// without duplicates and without gaps (atomic counter upsert in the sale
// transaction, UNIQUE (establishment_id, ticket_date, ticket_number)).
func TestConcurrency_DailyTicketNumbering(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		const sales = 200
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		registers := []*store.CashSession{e.openSession("cassa-1"), e.openSession("cassa-2"), e.openSession("cassa-3")}
		_, spots := e.addSeatZone("N", sales)

		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			numbers []int64
			errs    []error
		)
		start := make(chan struct{})
		for i := range sales {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				in := e.spotSale(spots[i])
				sid := registers[i%len(registers)].ID
				in.Payment.CashSessionID = &sid
				if i%2 == 1 {
					in.Payment.Method = store.TicketPaymentCard
				}
				r, err := e.sales.CreateDaily(ctx, f.Cashier, est, in)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs = append(errs, err)
					return
				}
				numbers = append(numbers, r.Ticket.TicketNumber)
			}(i)
		}
		close(start)
		wg.Wait()
		if len(errs) > 0 {
			t.Fatalf("%d sales failed, first: %v", len(errs), errs[0])
		}
		sort.Slice(numbers, func(a, b int) bool { return numbers[a] < numbers[b] })
		for i, n := range numbers {
			if n != int64(i+1) {
				t.Fatalf("numbers are not 1..%d: position %d has %d", sales, i, n)
			}
		}
		// Every register took its share and the counter row agrees.
		for _, cs := range registers {
			if n := f.Count((*store.Ticket)(nil), "cash_session_id = ?", cs.ID); n == 0 {
				t.Fatalf("register %s took no ticket", cs.Register)
			}
		}
		var c store.DailyTicketCounter
		if err := db.NewSelect().Model(&c).Where("establishment_id = ?", est).Scan(ctx); err != nil || c.LastNumber != sales {
			t.Fatalf("counter %v %+v", err, c)
		}
	})
}

// TestConcurrency_ZoneFirstCapacity: 100 zone-first sales at once on a seat
// zone of 4 spots and on a zone-mode zone of 2 places: exactly 4 and 2
// succeed (one spot each, by the unique allocation key; the pool by its
// conditional update), the others are zone_full.
func TestConcurrency_ZoneFirstCapacity(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		const attempts = 100
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		e.openSession("")
		for _, tc := range []struct {
			zone     *store.Zone
			capacity int
		}{{f.SeatZone, len(f.Spots)}, {f.FreeZone, len(f.FreeSpots)}} {
			var (
				wg       sync.WaitGroup
				mu       sync.Mutex
				ok, full int
				other    []error
				spotsOf  = map[string]int{}
			)
			start := make(chan struct{})
			for range attempts {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					in := e.spotSale(f.Spots[0])
					in.Target = dailysale.Target{ZoneID: tc.zone.ID}
					r, err := e.sales.CreateDaily(ctx, f.Cashier, est, in)
					mu.Lock()
					defer mu.Unlock()
					switch {
					case err == nil:
						ok++
						if r.Ticket.ProvisionalSpotID != nil {
							spotsOf[*r.Ticket.ProvisionalSpotID]++
						}
					case errors.Is(err, ticketing.ErrZoneFull):
						full++
					default:
						other = append(other, err)
					}
				}()
			}
			close(start)
			wg.Wait()
			if len(other) > 0 {
				t.Fatalf("zone %s: unexpected errors (%d), first: %v", tc.zone.Code, len(other), other[0])
			}
			if ok != tc.capacity || full != attempts-tc.capacity {
				t.Fatalf("zone %s: %d sold, %d full, want %d sold", tc.zone.Code, ok, full, tc.capacity)
			}
			for spot, n := range spotsOf {
				if n != 1 {
					t.Fatalf("spot %s held by %d tickets", spot, n)
				}
			}
			unassigned := f.Count((*store.Ticket)(nil), "zone_id = ? AND needs_assignment = ?", tc.zone.ID, true)
			if unassigned != tc.capacity {
				t.Fatalf("zone %s: %d tickets to assign", tc.zone.Code, unassigned)
			}
		}
	})
}

// TestConcurrency_EntryCheckInOnce: 100 scans of the same entry ticket at
// once (several phones at the gate) admit it exactly once.
func TestConcurrency_EntryCheckInOnce(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		const scans = 100
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		e.openSession("")
		r, err := e.sales.CreateEntry(ctx, f.Cashier, est, dailysale.EntryInput{
			Date: bookingtest.D(day), Slots: "FULL", Items: []dailysale.EntryItem{{TypeID: e.adult.ID, Quantity: 1}},
			Payment: dailysale.SalePayment{Method: store.TicketPaymentCard},
		})
		if err != nil {
			t.Fatal(err)
		}
		var (
			wg       sync.WaitGroup
			mu       sync.Mutex
			outcomes = map[string]int{}
			errs     []error
		)
		start := make(chan struct{})
		for range scans {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				g, err := e.tickets.CheckIn(ctx, f.Staff, est, r.Ticket.GateToken, false)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs = append(errs, err)
					return
				}
				outcomes[g.Outcome]++
			}()
		}
		close(start)
		wg.Wait()
		if len(errs) > 0 {
			t.Fatalf("%d scans failed, first: %v", len(errs), errs[0])
		}
		if outcomes[ticketing.GateAdmitted] != 1 || outcomes[ticketing.GateAlreadyEntered] != scans-1 {
			t.Fatalf("outcomes %v", outcomes)
		}
	})
}
