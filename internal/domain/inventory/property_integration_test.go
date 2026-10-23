//go:build integration

package inventory_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/uptrace/bun"
	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// Model-based property test of the allocation engine: random sequences of
// bookings (spots and the quantity pool, desk and online, stable and held),
// releases, confirmations and clock moves are applied both to the database
// and to an in-memory model. After every step:
//   - the database accepted an operation iff the model says the resources
//     were free (expired holds count as free: INV-04, INV-05);
//   - the spot rows equal the model (INV-01) and the pool counters equal the
//     model and the detail rows (INV-02, INV-03).

type modelKey struct {
	spot string
	date string
	slot calendar.Slot
}

type modelRow struct {
	booking string
	expires *time.Time
}

type modelPool struct {
	booking string
	date    string
	slot    calendar.Slot
	qty     int
	online  bool
	expires *time.Time
}

type model struct {
	spots    map[modelKey]modelRow
	pool     []modelPool
	expiries map[string]*time.Time // booking -> hold expiry (nil once confirmed)
	bookings []string
}

var propDates = []string{"2027-07-01", "2027-07-02", "2027-07-03"}

func (m *model) valid(r *time.Time, now time.Time) bool { return r == nil || r.After(now) }

func halfDays(from, to int, slots calendar.Slots) []modelKey {
	var out []modelKey
	for i := from; i <= to; i++ {
		for _, s := range slots.Expand() {
			out = append(out, modelKey{date: propDates[i], slot: s})
		}
	}
	return out
}

func TestPoolUsage_MatchesAllocations(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := storetest.SQLite(t)
		f := newFixture(t, db)
		m := &model{spots: map[modelKey]modelRow{}, expiries: map[string]*time.Time{}}
		capacity := f.qtyPool.Capacity
		steps := rapid.IntRange(1, 25).Draw(rt, "steps")
		for step := 0; step < steps; step++ {
			now := f.clock.Now()
			switch rapid.IntRange(0, 9).Draw(rt, "op") {
			case 0, 1, 2, 3, 4, 5: // book
				from := rapid.IntRange(0, 2).Draw(rt, "from")
				to := rapid.IntRange(from, 2).Draw(rt, "to")
				slots := rapid.SampledFrom([]calendar.Slots{calendar.SlotsAM, calendar.SlotsPM, calendar.SlotsFull}).Draw(rt, "slots")
				online := rapid.Bool().Draw(rt, "online")
				held := rapid.Bool().Draw(rt, "held")
				useSpot := rapid.Bool().Draw(rt, "spot")
				label := rapid.SampledFrom([]string{"A1", "A2"}).Draw(rt, "label")
				qty := rapid.IntRange(1, 3).Draw(rt, "qty")
				spec := bookingSpec{from: propDates[from], to: propDates[to], slots: slots, channel: store.ChannelDesk}
				if online {
					spec.channel = store.ChannelAPI // quota-bound but the quantity zone quota is 2: keep it desk-like for spots
				}
				var exp *time.Time
				if held {
					e := now.Add(time.Duration(rapid.IntRange(1, 20).Draw(rt, "ttl")) * time.Minute)
					exp = &e
					spec.status, spec.expires = "held", exp
				}
				keys := halfDays(from, to, slots)
				want := true
				if useSpot {
					spot := f.spots[label]
					spec.lines = []lineSpec{{zone: f.seat, spot: spot}}
					spec.channel = store.ChannelDesk // seat quota is covered by its own tests
					for _, k := range keys {
						k.spot = spot.ID
						if r, ok := m.spots[k]; ok && m.valid(r.expires, now) {
							want = false
						}
					}
				} else {
					spec.lines = []lineSpec{{zone: f.qtyZone, qty: qty}}
					for _, k := range keys {
						used, onl := 0, 0
						for _, p := range m.pool {
							if p.date == k.date && p.slot == k.slot && m.valid(p.expires, now) {
								used += p.qty
								if p.online {
									onl += p.qty
								}
							}
						}
						if used+qty > capacity || (online && onl+qty > 2) {
							want = false
						}
					}
				}
				tb, err := f.book(t, spec, nil)
				if want != (err == nil) {
					rt.Fatalf("step %d: book %+v: want ok=%v, got %v", step, spec, want, err)
				}
				if err != nil {
					var ce *availability.ConflictError
					if !errors.As(err, &ce) {
						rt.Fatalf("unexpected error %v", err)
					}
					break
				}
				m.bookings = append(m.bookings, tb.id)
				m.expiries[tb.id] = exp
				if useSpot {
					spotID := f.spots[label].ID
					// the database reclaims the expired holds of the spot over the range
					for k, r := range m.spots {
						if k.spot == spotID && !m.valid(r.expires, now) && between(k.date, from, to) {
							delete(m.spots, k)
						}
					}
					for _, k := range keys {
						k.spot = spotID
						m.spots[k] = modelRow{booking: tb.id, expires: exp}
					}
				} else {
					m.pool = slices.DeleteFunc(m.pool, func(p modelPool) bool { return !m.valid(p.expires, now) && between(p.date, from, to) })
					for _, k := range keys {
						m.pool = append(m.pool, modelPool{booking: tb.id, date: k.date, slot: k.slot, qty: qty, online: online, expires: exp})
					}
				}
			case 6: // release a booking
				if len(m.bookings) == 0 {
					continue
				}
				id := rapid.SampledFrom(m.bookings).Draw(rt, "release")
				must(t, store.RunInTx(context.Background(), db, func(ctx context.Context, tx bun.Tx) error {
					_, err := f.svc.Release(ctx, tx, f.es, inventoryRelease(id))
					return err
				}))
				for k, r := range m.spots {
					if r.booking == id {
						delete(m.spots, k)
					}
				}
				m.pool = slices.DeleteFunc(m.pool, func(p modelPool) bool { return p.booking == id })
			case 7: // confirm a held booking
				if len(m.bookings) == 0 {
					continue
				}
				id := rapid.SampledFrom(m.bookings).Draw(rt, "confirm")
				err := store.RunInTx(context.Background(), db, func(ctx context.Context, tx bun.Tx) error { return f.svc.Confirm(ctx, tx, f.es, id) })
				exp := m.expiries[id]
				if exp != nil && !exp.After(now) {
					if !errors.Is(err, availability.ErrHoldExpired) {
						rt.Fatalf("confirm of expired hold: %v", err)
					}
					break
				}
				if err != nil {
					rt.Fatalf("confirm: %v", err)
				}
				// bookings.hold_expires_at is the booking service's concern;
				// clear it here so a later confirm does not see it.
				_, err = db.NewUpdate().TableExpr("bookings").Set("hold_expires_at = NULL").Set("status = 'confirmed'").Where("id = ?", id).Exec(context.Background())
				must(t, err)
				m.expiries[id] = nil
				for k, r := range m.spots {
					if r.booking == id {
						r.expires = nil
						m.spots[k] = r
					}
				}
				for i := range m.pool {
					if m.pool[i].booking == id {
						m.pool[i].expires = nil
					}
				}
			default: // time passes
				f.clock.Advance(time.Duration(rapid.IntRange(1, 15).Draw(rt, "advance")) * time.Minute)
			}
			compareModel(rt, f, m)
			f.checkInvariants(t)
		}
	})
}

func between(date string, from, to int) bool {
	return date >= propDates[from] && date <= propDates[to]
}

func compareModel(rt *rapid.T, f *fixture, m *model) {
	ctx := context.Background()
	var rows []store.SpotAllocation
	must(rt, f.db.NewSelect().Model(&rows).Where("sa.kind <> 'block'").Scan(ctx))
	if len(rows) != len(m.spots) {
		rt.Fatalf("spot rows %d, model %d", len(rows), len(m.spots))
	}
	for _, r := range rows {
		mr, ok := m.spots[modelKey{spot: r.SpotID, date: r.Date.String(), slot: r.Slot}]
		if !ok || r.BookingID == nil || *r.BookingID != mr.booking {
			rt.Fatalf("row %s %s %s not in model", r.SpotID, r.Date, r.Slot)
		}
	}
	for _, ds := range propDates {
		for _, sl := range []calendar.Slot{calendar.AM, calendar.PM} {
			wantUsed, wantHeld, wantOnline := 0, 0, 0
			for _, p := range m.pool {
				if p.date == ds && p.slot == sl {
					if p.expires == nil {
						wantUsed += p.qty
					} else {
						wantHeld += p.qty
					}
					if p.online {
						wantOnline += p.qty
					}
				}
			}
			u, h, o := f.usage(rt, f.qtyPool.ID, ds, sl)
			if u != wantUsed || h != wantHeld || o != wantOnline {
				rt.Fatalf("pool %s %s: db %d/%d/%d model %d/%d/%d", ds, sl, u, h, o, wantUsed, wantHeld, wantOnline)
			}
		}
	}
}

func inventoryRelease(id string) inventory.ReleaseRequest {
	return inventory.ReleaseRequest{BookingID: id}
}
