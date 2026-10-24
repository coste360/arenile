//go:build integration

package dailysale_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/app"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/dailysale"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// day is the sale day of the tests (the fixture clock is 2027-06-01; the
// clock is moved to the morning of day by newEnv).
const day = "2027-07-10"

// Prices of the entry ticket types (cents per day).
const (
	adultPerDay = 1000
	childPerDay = 500
)

// env is the bookingtest lido on the real services, wired as internal/app
// and internal/http/dailysale_wiring.go do: inventory (DB constraints),
// customers, cash, fiscal, payments (manual driver), bookings, ticketing and
// the daily sale.
type env struct {
	f       *bookingtest.Fixture
	es      store.EstablishmentScope
	inv     *inventory.Service
	bk      *booking.Service
	pay     *payment.Service
	cash    *cash.Service
	fiscal  *fiscal.Service
	tickets *ticketing.Service
	sales   *dailysale.Service
	// Entry ticket types on the quantity zone Gate (capacity GateCapacity).
	gate         *store.Zone
	adult, child *store.EntryTicketType
}

// gateCapacity is the daily capacity of the entry zone.
const gateCapacity = 3

func newEnv(t testing.TB, db *store.DB) *env {
	t.Helper()
	f := bookingtest.New(t, db)
	c := f.Clock
	c.Set(time.Date(2027, 7, 10, 7, 0, 0, 0, time.UTC)) // 09:00 in Rome on day
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	reg := app.NewRegistry(c, ext.Extensions{})
	inv := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	cust := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	cs := cash.NewService(cash.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	fs := fiscal.NewService(fiscal.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Drivers: reg})
	pay := payment.NewService(payment.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: booking.NewLedger(booking.LedgerDeps{Clock: c}),
		Cash: cs, Fiscal: fs, Credits: cust, Providers: reg,
	})
	bk := booking.NewService(booking.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: inv, Pricer: f.Pricing, Customers: cust, Payments: pay,
		Releases: pass.NewResale(pass.ResaleDeps{Clock: c, Credits: cust}),
	})
	pay.SetConfirmer(bk)
	tk := ticketing.NewService(ticketing.Deps{
		DB: db, Clock: c, Audit: aw, Bookings: bk,
		BookingsIn: func(db bun.IDB) ticketing.Bookings { return bk.WithDB(db) },
	})
	sales := dailysale.NewService(dailysale.Deps{
		DB: db, Clock: c, Audit: aw, Bookings: bk, Payments: dailysale.PaymentRecorder{Svc: pay}, Tickets: tk, Customers: cust,
		AvailabilityIn: func(db bun.IDB) dailysale.Availability { return inv.WithDB(db) },
	})
	es, err := store.SystemEstablishmentScope(f.Ctx, db, f.Est.ID)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{f: f, es: es, inv: inv, bk: bk, pay: pay, cash: cs, fiscal: fs, tickets: tk, sales: sales}
	e.zonePool(f.FreeZone.ID, len(f.FreeSpots), true)
	e.entryTypes()
	return e
}

// zonePool creates the pool of a zone (the implicit system pool of a
// zone-mode zone, or the daily capacity of a quantity zone).
func (e *env) zonePool(zoneID string, capacity int, system bool) {
	e.f.T.Helper()
	now := e.f.Clock.Now()
	p := &store.Pool{
		ID: store.NewID(), ZoneID: &zoneID, ComponentCode: "spot", Name: store.I18nText{"it": "pool"}, Capacity: capacity,
		CapacityOverrides: []store.CapacityOverride{}, SystemManaged: system, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if !system {
		p.ComponentCode = "entry"
	}
	if err := store.InsertPool(e.f.Ctx, e.f.DB, e.es, p); err != nil {
		e.f.T.Fatal(err)
	}
}

// entryTypes creates the quantity zone Gate and the types Adulto and
// Bambino, each a pack with its price rule.
func (e *env) entryTypes() {
	f := e.f
	f.T.Helper()
	now := f.Clock.Now()
	e.gate = &store.Zone{
		ID: store.NewID(), EstablishmentID: f.Est.ID, Code: "gate", Name: store.I18nText{"it": "Spiaggia libera"},
		AssignmentMode: "quantity", OnlineVisible: true, PriceCategory: "gate", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.Insert(e.gate)
	e.zonePool(e.gate.ID, gateCapacity, false)
	mk := func(code string, price money.Cents) *store.EntryTicketType {
		pk := &store.Pack{
			ID: store.NewID(), EstablishmentID: f.Est.ID, Code: code, Name: store.I18nText{"it": code}, Description: store.I18nText{},
			Composition: map[string]int{}, AllowedFormulas: []string{"day", "half_day"}, Channels: []string{"desk", "online"},
			MaxPerBooking: 100, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		f.Insert(pk)
		if _, err := f.Pricing.CreateRule(f.Ctx, f.Owner, f.Est.ID, pricingconfig.RuleInput{
			Name: code, TargetType: pricing.TargetPack, TargetIDs: []string{pk.ID}, SeasonID: f.Season.ID,
			Action: pricing.Action{Type: pricing.ActionSet, Amount: price}, Stage: pricing.StageBase, Active: true,
		}); err != nil {
			f.T.Fatalf("rule %s: %v", code, err)
		}
		et, err := e.tickets.CreateEntryType(f.Ctx, f.Owner, f.Est.ID, ticketing.EntryTypeInput{
			Code: code, Name: store.I18nText{"it": code}, PackID: pk.ID, ZoneID: e.gate.ID, Active: true,
		})
		if err != nil {
			f.T.Fatalf("entry type %s: %v", code, err)
		}
		return et
	}
	e.adult = mk("adulto", adultPerDay)
	e.child = mk("bambino", childPerDay)
}

// openSession opens a cash session on register as the cashier.
func (e *env) openSession(register string) *store.CashSession {
	e.f.T.Helper()
	in := cash.OpenInput{OpeningFloat: 10000}
	if register != "" {
		// Registers other than the default must exist (ADR 0019 D).
		code := register
		r, err := e.cash.CreateRegister(e.f.Ctx, e.f.Owner, "", e.f.Est.ID, cash.RegisterInput{Code: &code, Name: register})
		if err != nil {
			e.f.T.Fatalf("register %s: %v", register, err)
		}
		in.RegisterID = r.ID
	}
	cs, err := e.cash.Open(e.f.Ctx, e.f.Cashier, "", e.f.Est.ID, in)
	if err != nil {
		e.f.T.Fatalf("open %s: %v", register, err)
	}
	return cs
}

// spotSale is a full-day spot-first sale of the base pack paid in cash.
func (e *env) spotSale(spot *store.Spot) dailysale.SaleInput {
	return dailysale.SaleInput{
		Target: dailysale.Target{SpotID: spot.ID}, Date: bookingtest.D(day), Slots: "FULL", PackID: e.f.Base.ID,
		Payment: dailysale.SalePayment{Method: store.TicketPaymentCash},
	}
}

// addSeatZone adds a seat zone with n spots to the fixture map.
func (e *env) addSeatZone(code string, n int) (*store.Zone, []*store.Spot) {
	f := e.f
	f.T.Helper()
	now := f.Clock.Now()
	mapID := *f.SeatZone.MapID
	z := &store.Zone{
		ID: store.NewID(), EstablishmentID: f.Est.ID, MapID: &mapID, Code: code, Name: store.I18nText{"it": code},
		AssignmentMode: "seat", OnlineVisible: true, PriceCategory: "front", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.Insert(z)
	spots := make([]*store.Spot, 0, n)
	for i := 1; i <= n; i++ {
		s := &store.Spot{
			ID: store.NewID(), EstablishmentID: f.Est.ID, MapID: mapID, ZoneID: z.ID, Label: fmt.Sprintf("%s%d", code, i), RowLabel: code, Number: i,
			Kind: "umbrella", X: float64(i), Y: 50, Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{},
			Attributes: []string{}, Active: true, CreatedAt: now, UpdatedAt: now,
		}
		f.Insert(s)
		spots = append(spots, s)
	}
	return z, spots
}
