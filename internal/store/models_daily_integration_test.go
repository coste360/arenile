//go:build integration

package store_test

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMigration00010_ModelsRoundtrip inserts and reloads the daily sale
// models on both dialects and checks the constraints the ticketing service
// relies on: unique number per establishment and day, one ticket per
// booking, voided iff voided_at, provisional spot only while unassigned,
// and the daily counter restarting at 1 on a new day.
func TestMigration00010_ModelsRoundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-daily", "Lido Daily", t0)
		if err != nil {
			t.Fatal(err)
		}
		es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
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
			if _, err := db.NewInsert().Model(m).Exec(ctx); !store.IsUniqueViolation(err) {
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
		day := d("2027-07-03")

		m := &store.Map{ID: store.NewID(), EstablishmentID: est.ID, Name: store.I18nText{"it": "Spiaggia"}, Width: 100, Height: 60, GridSize: 1, SeaSide: "top", Version: 1, CreatedAt: t0, UpdatedAt: t0}
		insert(m)
		zone := &store.Zone{ID: store.NewID(), EstablishmentID: est.ID, MapID: &m.ID, Code: "row1", Name: store.I18nText{"it": "Prima fila"}, AssignmentMode: "seat", OnlineVisible: true, PriceCategory: "row1", Version: 1, CreatedAt: t0, UpdatedAt: t0}
		insert(zone)
		gate := &store.Zone{ID: store.NewID(), EstablishmentID: est.ID, Code: "free", Name: store.I18nText{"it": "Spiaggia libera"}, AssignmentMode: "quantity", OnlineVisible: true, Version: 1, CreatedAt: t0, UpdatedAt: t0}
		insert(gate)
		spot := &store.Spot{
			ID: store.NewID(), EstablishmentID: est.ID, MapID: m.ID, ZoneID: zone.ID, Label: "A1", RowLabel: "A", Number: 1,
			Kind: "umbrella", Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{},
			Attributes: []string{}, Active: true, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(spot)
		pack := &store.Pack{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "adult", Name: store.I18nText{"it": "Adulto"}, Description: store.I18nText{},
			Composition: map[string]int{"entry": 1}, AllowedFormulas: []string{"day"}, Channels: []string{"desk", "online"},
			MaxPerBooking: 10, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(pack)
		cust := &store.Customer{ID: store.NewID(), EstablishmentID: est.ID, FirstName: "", LastName: "Cliente al banco", Locale: "it", Tags: []string{}, Version: 1, CreatedAt: t0, UpdatedAt: t0}
		insert(cust)
		booking := func(code string) *store.Booking {
			b := &store.Booking{
				ID: store.NewID(), EstablishmentID: est.ID, Code: code, CustomerID: cust.ID, Channel: store.ChannelDesk,
				Status: "confirmed", StartsOn: day, EndsOn: day, Slots: calendar.SlotsFull, Formula: "day",
				PriceSnapshot: store.JSON(`{}`), TotalAmount: 2500, PolicySnapshot: store.JSON(`{"rules":{}}`), Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
			insert(b)
			return b
		}

		var n1, n2 int64
		if err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			var err error
			if n1, err = store.NextTicketNumber(ctx, tx, es, day, t0); err != nil {
				return err
			}
			n2, err = store.NextTicketNumber(ctx, tx, es, day, t0)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if n1 != 1 || n2 != 2 {
			t.Fatalf("numbers = %d, %d; want 1, 2", n1, n2)
		}
		next, err := store.NextTicketNumber(ctx, db, es, day.AddDays(1), t0)
		if err != nil || next != 1 {
			t.Fatalf("next day number = %d, %v; want 1", next, err)
		}

		b1 := booking("AR-T1")
		tk := &store.Ticket{
			ID: store.NewID(), EstablishmentID: est.ID, BookingID: b1.ID, TicketDate: day, TicketNumber: n1, Kind: store.TicketKindSale,
			Status: store.TicketValid, ZoneID: zone.ID, NeedsAssignment: true, ProvisionalSpotID: &spot.ID, HolderName: "Rossi",
			TotalAmount: 2500, PaymentMethod: store.TicketPaymentCash, GateToken: store.NewID(), Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(tk)
		var got store.Ticket
		if err := db.NewSelect().Model(&got).Where("id = ?", tk.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if got.TicketNumber != 1 || got.TicketDate != day || !got.NeedsAssignment || got.TotalAmount != 2500 || got.HolderName != "Rossi" {
			t.Fatalf("ticket roundtrip = %+v", got)
		}
		dupNumber := *tk
		dupNumber.ID, dupNumber.BookingID = store.NewID(), booking("AR-T2").ID
		expectConflict(&dupNumber)
		dupBooking := *tk
		dupBooking.ID, dupBooking.TicketNumber = store.NewID(), n2
		expectConflict(&dupBooking)

		b3 := booking("AR-T3")
		voidNoDate := *tk
		voidNoDate.ID, voidNoDate.BookingID, voidNoDate.TicketNumber, voidNoDate.Status = store.NewID(), b3.ID, n2, store.TicketVoided
		expectError("voided without voided_at", &voidNoDate)
		provAssigned := *tk
		provAssigned.ID, provAssigned.BookingID, provAssigned.TicketNumber, provAssigned.NeedsAssignment = store.NewID(), b3.ID, n2, false
		expectError("provisional spot on an assigned ticket", &provAssigned)
		entryUnassigned := *tk
		entryUnassigned.ID, entryUnassigned.BookingID, entryUnassigned.TicketNumber = store.NewID(), b3.ID, n2
		entryUnassigned.Kind, entryUnassigned.ProvisionalSpotID = store.TicketKindEntry, nil
		expectError("entry ticket needing assignment", &entryUnassigned)

		et := &store.EntryTicketType{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "adult", Name: store.I18nText{"it": "Adulto"}, Description: store.I18nText{},
			PackID: pack.ID, ZoneID: gate.ID, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(et)
		samePack := *et
		samePack.ID, samePack.Code = store.NewID(), "adult2"
		expectConflict(&samePack)

		insert(&store.DailySaleSettings{EstablishmentID: est.ID, WalkInCustomerID: &cust.ID, PrintFormat: store.PrintThermal80, AutoPrint: true, ZoneFirstEnabled: true, FooterText: store.I18nText{}, Version: 1, CreatedAt: t0, UpdatedAt: t0})
		expectError("unknown print format", &store.DailySaleSettings{EstablishmentID: store.NewID(), PrintFormat: "a3", FooterText: store.I18nText{}, Version: 1, CreatedAt: t0, UpdatedAt: t0})
	})
}

// TestConcurrency_TicketNumber takes 100 numbers in parallel transactions
// for the same establishment and day: they must be exactly 1..100.
func TestConcurrency_TicketNumber(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-num", "Lido Num", t0)
		if err != nil {
			t.Fatal(err)
		}
		es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
		if err != nil {
			t.Fatal(err)
		}
		day := calendar.MustParse("2027-07-03")
		const workers = 100
		var mu sync.Mutex
		var got []int64
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var n int64
				err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
					var err error
					n, err = store.NextTicketNumber(ctx, tx, es, day, t0)
					return err
				})
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				got = append(got, n)
				mu.Unlock()
			}()
		}
		wg.Wait()
		sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
		if len(got) != workers {
			t.Fatalf("got %d numbers, want %d", len(got), workers)
		}
		for i, n := range got {
			if n != int64(i+1) {
				t.Fatalf("numbers not 1..%d without gaps or duplicates: %v", workers, got)
			}
		}
	})
}
