//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMigration00003_ModelsRoundtrip inserts and reloads the M3 models on
// both dialects and checks the constraints the availability, payment and
// cash services rely on (unique spot allocation, hold expiry, pool counters
// non-negative, one open cash session per register, payment idempotency,
// one active release per half-day).
func TestMigration00003_ModelsRoundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-m3", "Lido M3", t0)
		if err != nil {
			t.Fatal(err)
		}
		insert := func(m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
				t.Fatalf("insert %T: %v", m, err)
			}
		}
		expectConflict := func(m any) {
			t.Helper()
			_, err := db.NewInsert().Model(m).Exec(ctx)
			if !store.IsUniqueViolation(err) {
				t.Fatalf("insert %T: want unique violation, got %v", m, err)
			}
		}
		expectError := func(what string, m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); err == nil {
				t.Fatalf("insert %s: want constraint error", what)
			}
		}
		d := calendar.MustParse
		m := &store.Map{
			ID: store.NewID(), EstablishmentID: est.ID, Name: store.I18nText{"it": "Spiaggia"}, Width: 100,
			Height: 60, GridSize: 1, SeaSide: "bottom", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(m)
		zone := &store.Zone{
			ID: store.NewID(), EstablishmentID: est.ID, MapID: &m.ID, Code: "row1", Name: store.I18nText{"it": "Prima fila"},
			AssignmentMode: "seat", OnlineVisible: true, PriceCategory: "row1", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(zone)
		spot := &store.Spot{
			ID: store.NewID(), EstablishmentID: est.ID, MapID: m.ID, ZoneID: zone.ID, Label: "A1", RowLabel: "A", Number: 1,
			Kind: "umbrella", Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{},
			Attributes: []string{}, Active: true, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(spot)
		pool := &store.Pool{
			ID: store.NewID(), EstablishmentID: est.ID, ComponentCode: "sunbed", Name: store.I18nText{"it": "Lettini"},
			Capacity: 2, CapacityOverrides: []store.CapacityOverride{}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(pool)
		pack := &store.Pack{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "base", Name: store.I18nText{"it": "Base"}, Description: store.I18nText{},
			Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"day"}, Channels: []string{"desk"},
			MaxPerBooking: 1, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(pack)
		email := "mario.rossi@example.it"
		cust := &store.Customer{
			ID: store.NewID(), EstablishmentID: est.ID, FirstName: "Mario", LastName: "Rossi", Email: &email, Locale: "it",
			Tags: []string{"vip"}, SearchText: "rossi mario", Version: 1, CreatedAt: t0, UpdatedAt: t0,
			MarketingConsent: &store.MarketingConsent{GivenAt: t0, Source: "desk", TextVersion: "v1"},
		}
		insert(cust)
		upper := "Mario@Example.it"
		expectError("uppercase email", &store.Customer{
			ID: store.NewID(), EstablishmentID: est.ID, LastName: "X", Email: &upper, Locale: "it", Tags: []string{},
			Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})

		booking := func(code string) *store.Booking {
			return &store.Booking{
				ID: store.NewID(), EstablishmentID: est.ID, Code: code, CustomerID: cust.ID, Channel: store.ChannelDesk,
				Status: "confirmed", StartsOn: d("2027-07-01"), EndsOn: d("2027-07-02"), Slots: calendar.SlotsFull, Formula: "multi_day",
				PriceSnapshot: store.JSON(`{"total":7000}`), TotalAmount: 7000, PolicySnapshot: store.JSON(`{"rules":{}}`),
				Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		bk := booking("AR-AAAA1")
		insert(bk)
		expectConflict(booking("AR-AAAA1"))
		exp := t0.Add(time.Hour)
		heldWithExpiryWrongStatus := booking("AR-BBBB2")
		heldWithExpiryWrongStatus.HoldExpiresAt = &exp
		expectError("hold_expires_at on confirmed booking", heldWithExpiryWrongStatus)

		line := &store.BookingLine{
			ID: store.NewID(), EstablishmentID: est.ID, BookingID: bk.ID, PackID: pack.ID, ZoneID: zone.ID, SpotID: &spot.ID,
			AssignmentStatus: store.AssignmentAssigned, Quantity: 1, PriceCategory: "row1", LineTotal: 7000, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(line)

		alloc := func(date string, slot calendar.Slot, kind string) *store.SpotAllocation {
			return &store.SpotAllocation{
				ID: store.NewID(), EstablishmentID: est.ID, SpotID: spot.ID, Date: d(date), Slot: slot, Kind: kind,
				BookingID: &bk.ID, BookingLineID: &line.ID, CreatedAt: t0,
			}
		}
		insert(alloc("2027-07-01", calendar.AM, store.AllocationBooking))
		insert(alloc("2027-07-01", calendar.PM, store.AllocationBooking))
		// INV-01: the same spot/date/slot can never be allocated twice.
		expectConflict(alloc("2027-07-01", calendar.AM, store.AllocationBooking))
		// INV-06: holds need expires_at, other kinds must not have it.
		expectError("hold without expiry", alloc("2027-07-02", calendar.AM, store.AllocationHold))
		withExpiry := alloc("2027-07-02", calendar.AM, store.AllocationBooking)
		withExpiry.ExpiresAt = &exp
		expectError("booking with expiry", withExpiry)

		hold := &store.Hold{
			ID: store.NewID(), EstablishmentID: est.ID, Channel: store.ChannelDesk, StartsOn: d("2027-07-03"), EndsOn: d("2027-07-03"),
			Slots: calendar.SlotsAM, Items: []store.HoldItem{{SpotID: spot.ID}}, ExpiresAt: exp, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(hold)
		insert(&store.SpotAllocation{
			ID: store.NewID(), EstablishmentID: est.ID, SpotID: spot.ID, Date: d("2027-07-03"), Slot: calendar.AM,
			Kind: store.AllocationHold, HoldID: &hold.ID, ExpiresAt: &exp, CreatedAt: t0,
		})
		block := &store.SpotBlock{
			ID: store.NewID(), EstablishmentID: est.ID, SpotID: spot.ID, StartsOn: d("2027-07-04"), EndsOn: d("2027-07-04"),
			Slots: calendar.SlotsFull, Reason: "manutenzione", CreatedAt: t0, UpdatedAt: t0,
		}
		insert(block)
		insert(&store.SpotAllocation{
			ID: store.NewID(), EstablishmentID: est.ID, SpotID: spot.ID, Date: d("2027-07-04"), Slot: calendar.AM,
			Kind: store.AllocationBlock, BlockID: &block.ID, CreatedAt: t0,
		})

		insert(&store.PoolUsage{EstablishmentID: est.ID, PoolID: pool.ID, Date: d("2027-07-01"), Slot: calendar.AM, Used: 1, UpdatedAt: t0})
		expectConflict(&store.PoolUsage{EstablishmentID: est.ID, PoolID: pool.ID, Date: d("2027-07-01"), Slot: calendar.AM, UpdatedAt: t0})
		expectError("negative held", &store.PoolUsage{EstablishmentID: est.ID, PoolID: pool.ID, Date: d("2027-07-01"), Slot: calendar.PM, Held: -1, UpdatedAt: t0})
		insert(&store.PoolAllocation{
			ID: store.NewID(), EstablishmentID: est.ID, PoolID: pool.ID, Date: d("2027-07-01"), Slot: calendar.AM,
			Kind: store.AllocationBooking, Channel: store.ChannelDesk, Quantity: 1, BookingID: &bk.ID, BookingLineID: &line.ID, CreatedAt: t0,
		})
		// Capacity is enforced by the conditional UPDATE (spec 7.3): with
		// capacity 2 and used 1, adding 2 updates no row.
		res, err := db.NewUpdate().Model((*store.PoolUsage)(nil)).
			Set("held = held + ?", 2).
			Where("pool_id = ? AND date = ? AND slot = ?", pool.ID, d("2027-07-01"), calendar.AM).
			Where("used + held + ? <= ?", 2, 2).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n, _ := res.RowsAffected(); n != 0 {
			t.Fatalf("conditional pool update over capacity affected %d rows", n)
		}

		release := func() *store.SeasonRelease {
			return &store.SeasonRelease{
				ID: store.NewID(), EstablishmentID: est.ID, BookingID: bk.ID, BookingLineID: line.ID, SpotID: spot.ID,
				Date: d("2027-07-10"), Slot: calendar.AM, CreditAmount: 0, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		r1 := release()
		insert(r1)
		expectConflict(release())
		if _, err := db.NewUpdate().Model((*store.SeasonRelease)(nil)).Set("revoked_at = ?", t0).Where("id = ?", r1.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		insert(release()) // a revoked release no longer blocks a new one

		registers := map[string]string{}
		for i, code := range []string{store.DefaultCashRegister, "bar"} {
			r := &store.CashRegister{
				ID: store.NewID(), EstablishmentID: est.ID, Code: code, Name: code, Active: true, IsDefault: i == 0,
				SortOrder: i, Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
			insert(r)
			registers[code] = r.ID
		}
		session := func(register string) *store.CashSession {
			rid := registers[register]
			return &store.CashSession{
				ID: store.NewID(), EstablishmentID: est.ID, Register: register, RegisterID: &rid, OpenedAt: t0, OpeningFloat: 10000,
				ExpectedTotals: store.MethodTotals{"cash": 10000}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		cs := session(store.DefaultCashRegister)
		insert(cs)
		expectConflict(session(store.DefaultCashRegister))
		insert(session("bar"))
		insert(&store.CashMovement{
			ID: store.NewID(), EstablishmentID: est.ID, CashSessionID: cs.ID, Kind: store.CashMovementOut, Amount: 500,
			Reason: "resto", CreatedAt: t0,
		})

		key := "idem-key-0001"
		payment := func() *store.Payment {
			return &store.Payment{
				ID: store.NewID(), EstablishmentID: est.ID, BookingID: &bk.ID, CustomerID: &cust.ID, Provider: "manual",
				Method: store.PaymentMethodCash, Direction: store.PaymentDirectionIn, Amount: 7000, Status: store.PaymentSucceeded,
				IdempotencyKey: &key, CashSessionID: &cs.ID, BusinessDate: d("2027-07-01"),
				VATBreakdown: []store.VATShare{{RateBP: 2200, Gross: 7000, Taxable: 5738, Tax: 1262}}, Metadata: map[string]any{},
				CreatedAt: t0, UpdatedAt: t0,
			}
		}
		pm := payment()
		insert(pm)
		expectConflict(payment())
		insert(&store.FiscalDocument{
			ID: store.NewID(), EstablishmentID: est.ID, PaymentID: pm.ID, Driver: "none", Status: store.FiscalIssued,
			BusinessDate: d("2027-07-01"), Payload: store.JSON(`{}`), CreatedAt: t0, UpdatedAt: t0,
		})
		insert(&store.CustomerCredit{
			ID: store.NewID(), EstablishmentID: est.ID, CustomerID: cust.ID, Amount: 1500, Source: store.CreditSourceCancellation,
			BookingID: &bk.ID, CreatedAt: t0,
		})

		// Closing the session: counted totals and difference are stored together.
		diff := money.Cents(-500)
		if _, err := db.NewUpdate().Model((*store.CashSession)(nil)).
			Set("closed_at = ?", t0).Set("counted_totals = ?", `{"cash":16500}`).Set("difference = ?", diff).
			Where("id = ?", cs.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		insert(session(store.DefaultCashRegister)) // a new session may open once the previous one is closed

		var gotBooking store.Booking
		if err := db.NewSelect().Model(&gotBooking).Where("bk.id = ?", bk.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if gotBooking.Slots != calendar.SlotsFull || gotBooking.TotalAmount != 7000 || gotBooking.BalanceDue() != 7000 ||
			gotBooking.StartsOn.String() != "2027-07-01" || gotBooking.IsSeasonPass {
			t.Fatalf("booking %+v", gotBooking)
		}
		var gotCust store.Customer
		if err := db.NewSelect().Model(&gotCust).Where("cu.id = ?", cust.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if gotCust.MarketingConsent == nil || gotCust.MarketingConsent.TextVersion != "v1" || len(gotCust.Tags) != 1 {
			t.Fatalf("customer %+v", gotCust)
		}
		var gotPayment store.Payment
		if err := db.NewSelect().Model(&gotPayment).Where("pm.id = ?", pm.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if len(gotPayment.VATBreakdown) != 1 || gotPayment.VATBreakdown[0].Tax != 1262 || gotPayment.Signed() != 7000 {
			t.Fatalf("payment %+v", gotPayment)
		}
		var gotSession store.CashSession
		if err := db.NewSelect().Model(&gotSession).Where("cs.id = ?", cs.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if gotSession.Difference == nil || *gotSession.Difference != -500 || gotSession.CountedTotals.Sum() != 16500 {
			t.Fatalf("cash session %+v", gotSession)
		}
		var gotHold store.Hold
		if err := db.NewSelect().Model(&gotHold).Where("ho.id = ?", hold.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if len(gotHold.Items) != 1 || gotHold.Items[0].SpotID != spot.ID || gotHold.Slots != calendar.SlotsAM {
			t.Fatalf("hold %+v", gotHold)
		}
	})
}
