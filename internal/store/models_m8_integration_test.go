//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMigration00013_ModelsRoundtrip inserts and reloads the M8 models on
// both dialects and checks the constraints the M8 services rely on: the
// agreement quota cap (INV-103), one spot per booking line and half-day
// (INV-102), positive partner payments with a void reason, the subject of a
// requirement verification and the zero values of the new columns of
// existing tables (code that does not know them keeps working).
func TestMigration00013_ModelsRoundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-m8", "Lido M8", t0)
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

		// Existing tables with the M8 columns at their zero values.
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
		spot := func(label string) *store.Spot {
			return &store.Spot{
				ID: store.NewID(), EstablishmentID: est.ID, MapID: m.ID, ZoneID: zone.ID, Label: label, RowLabel: "A", Number: 1,
				Kind: "umbrella", Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{},
				Attributes: []string{}, Active: true, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		a1, a2 := spot("A1"), spot("A2")
		a2.OnlineExcluded = true
		insert(a1)
		insert(a2)
		var got store.Spot
		if err := db.NewSelect().Model(&got).Where("id = ?", a1.ID).Scan(ctx); err != nil || got.OnlineExcluded {
			t.Fatalf("spot online_excluded default: %v %+v", err, got.OnlineExcluded)
		}
		extra := &store.Extra{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "lettino", Name: store.I18nText{"it": "Lettino"}, Description: store.I18nText{},
			PricingUnit: "per_day", MaxQuantity: 4, AttachTo: "line", Channels: []string{"desk"}, Active: true, Version: 1,
			ShowOnMap: true, MapLabel: "L", CreatedAt: t0, UpdatedAt: t0,
		}
		insert(extra)
		pack := &store.Pack{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "base", Name: store.I18nText{"it": "Base"}, Description: store.I18nText{},
			Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"day"}, Channels: []string{"desk"},
			MaxPerBooking: 1, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(pack)

		// Partners and customer categories.
		hotel := &store.Partner{
			ID: store.NewID(), EstablishmentID: est.ID, Name: "Hotel Miramare", Kind: store.PartnerKindAccommodation,
			Active: true, SearchText: "hotel miramare", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(hotel)
		expectError("unknown partner kind", &store.Partner{
			ID: store.NewID(), EstablishmentID: est.ID, Name: "X", Kind: "hotel", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		residents := &store.CustomerCategory{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "residenti", Name: "Residenti", Requirement: "documento di residenza",
			RegisterCategory: store.RegisterResidents, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(residents)
		expectConflict(&store.CustomerCategory{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "residenti", Name: "Doppia", RegisterCategory: store.RegisterOther,
			Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		cust := &store.Customer{
			ID: store.NewID(), EstablishmentID: est.ID, LastName: "Rossi", Locale: "it", Tags: []string{},
			SearchText: "rossi", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(cust)
		insert(&store.CustomerCategoryMember{EstablishmentID: est.ID, CustomerID: cust.ID, CategoryID: residents.ID, CreatedAt: t0})
		expectConflict(&store.CustomerCategoryMember{EstablishmentID: est.ID, CustomerID: cust.ID, CategoryID: residents.ID, CreatedAt: t0})

		// Agreements: a legacy row (no M8 fields) and a partner agreement.
		legacy := &store.Agreement{
			ID: store.NewID(), EstablishmentID: est.ID, Name: "Vecchia", Action: store.JSON(`{"type":"percent_off","percent":"10"}`),
			Conditions: store.JSON(`{}`), Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(legacy)
		target, cat := store.AgreementTargetPartner, store.RegisterAccommodation
		ag := &store.Agreement{
			ID: store.NewID(), EstablishmentID: est.ID, Name: "Ospiti Miramare", Action: store.JSON(`{"type":"percent_off","percent":"15"}`),
			Conditions: store.JSON(`{}`), Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
			Target: &target, PartnerID: &hotel.ID, Quota: store.JSON(`{"reserved_per_day":2,"max_per_day":3,"release_days_before":3}`),
			Channels: store.JSON(`["desk","online_link"]`), Requirement: "conferma dell'hotel", RegisterCategory: &cat,
		}
		insert(ag)
		bad := "hotel_guests"
		expectError("unknown agreement target", &store.Agreement{
			ID: store.NewID(), EstablishmentID: est.ID, Name: "X", Action: store.JSON(`{}`), Conditions: store.JSON(`{}`),
			Version: 1, CreatedAt: t0, UpdatedAt: t0, Target: &bad,
		})
		insert(&store.AgreementVersion{
			ID: store.NewID(), EstablishmentID: est.ID, AgreementID: ag.ID, Version: 1, Snapshot: store.JSON(`{"name":"Ospiti Miramare"}`), ChangedAt: t0,
		})
		expectConflict(&store.AgreementVersion{
			ID: store.NewID(), EstablishmentID: est.ID, AgreementID: ag.ID, Version: 1, Snapshot: store.JSON(`{}`), ChangedAt: t0,
		})

		// INV-103: used never exceeds max_units.
		maxUnits := 3
		insert(&store.AgreementQuotaUsage{
			EstablishmentID: est.ID, AgreementID: ag.ID, Date: d("2027-08-15"), Slot: calendar.AM, Reserved: 2, Used: 3,
			MaxUnits: &maxUnits, ReleaseOn: d("2027-08-12"), UpdatedAt: t0,
		})
		expectError("used above max_units", &store.AgreementQuotaUsage{
			EstablishmentID: est.ID, AgreementID: ag.ID, Date: d("2027-08-15"), Slot: calendar.PM, Reserved: 2, Used: 4,
			MaxUnits: &maxUnits, ReleaseOn: d("2027-08-12"), UpdatedAt: t0,
		})
		res, err := db.NewUpdate().Model((*store.AgreementQuotaUsage)(nil)).
			Set("used = used + ?", 1).
			Where("agreement_id = ? AND date = ? AND slot = ?", ag.ID, d("2027-08-15"), calendar.AM).
			Where("max_units IS NULL OR used + ? <= max_units", 1).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n, _ := res.RowsAffected(); n != 0 {
			t.Fatalf("conditional update above the cap affected %d rows", n)
		}

		// Booking with the M8 fields.
		adults, children := 2, 1
		share, shareValue := store.PartnerSharePercent, int64(5000)
		bk := &store.Booking{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "AR-M8AA1", CustomerID: cust.ID, Channel: store.ChannelDesk,
			Status: "confirmed", StartsOn: d("2027-08-10"), EndsOn: d("2027-08-16"), Slots: calendar.SlotsFull, Formula: "week",
			PriceSnapshot: store.JSON(`{"total":21000}`), TotalAmount: 21000, PolicySnapshot: store.JSON(`{"rules":{}}`),
			AgreementID: &ag.ID, PartnerID: &hotel.ID, GroupName: "Famiglia Rossi", PartyAdults: &adults, PartyChildren: &children,
			GuestNames: []string{"Anna"}, PartnerShareType: &share, PartnerShareValue: &shareValue, PartnerAmount: 10500,
			Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(bk)
		var gotBk store.Booking
		if err := db.NewSelect().Model(&gotBk).Where("id = ?", bk.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if gotBk.PartnerAmount != 10500 || gotBk.GroupName != "Famiglia Rossi" || len(gotBk.GuestNames) != 1 || *gotBk.PartyAdults != 2 {
			t.Fatalf("booking roundtrip %+v", gotBk)
		}
		line := &store.BookingLine{
			ID: store.NewID(), EstablishmentID: est.ID, BookingID: bk.ID, PackID: pack.ID, ZoneID: zone.ID, SpotID: &a1.ID,
			AssignmentStatus: store.AssignmentAssigned, Quantity: 1, PriceCategory: "row1", LineTotal: 21000, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(line)
		alloc := func(spotID, date string, slot calendar.Slot) *store.SpotAllocation {
			return &store.SpotAllocation{
				ID: store.NewID(), EstablishmentID: est.ID, SpotID: spotID, Date: d(date), Slot: slot, Kind: store.AllocationBooking,
				BookingID: &bk.ID, BookingLineID: &line.ID, CreatedAt: t0,
			}
		}
		// Split stay: A1 on the 10th, A2 on the 11th.
		insert(alloc(a1.ID, "2027-08-10", calendar.AM))
		insert(alloc(a2.ID, "2027-08-11", calendar.AM))
		// INV-102: the same line never holds two spots on one half-day.
		expectConflict(alloc(a2.ID, "2027-08-10", calendar.AM))
		insert(&store.AgreementQuotaAllocation{
			ID: store.NewID(), EstablishmentID: est.ID, AgreementID: ag.ID, BookingID: bk.ID, BookingLineID: line.ID,
			Date: d("2027-08-15"), Slot: calendar.AM, Quantity: 1, CreatedAt: t0,
		})

		// Partner payments: positive amount, void reason with voided_at.
		insert(&store.PartnerPayment{
			ID: store.NewID(), EstablishmentID: est.ID, PartnerID: hotel.ID, Amount: 5000, ReceivedOn: d("2027-08-31"),
			Method: "bank_transfer", CreatedAt: t0, UpdatedAt: t0,
		})
		expectError("zero payment", &store.PartnerPayment{
			ID: store.NewID(), EstablishmentID: est.ID, PartnerID: hotel.ID, Amount: 0, ReceivedOn: d("2027-08-31"),
			Method: "cash", CreatedAt: t0, UpdatedAt: t0,
		})
		voided := t0.Add(time.Hour)
		expectError("void without reason", &store.PartnerPayment{
			ID: store.NewID(), EstablishmentID: est.ID, PartnerID: hotel.ID, Amount: 100, ReceivedOn: d("2027-08-31"),
			Method: "cash", VoidedAt: &voided, CreatedAt: t0, UpdatedAt: t0,
		})

		// Requirement verifications: the subject names the owner.
		insert(&store.RequirementVerification{
			ID: store.NewID(), EstablishmentID: est.ID, Subject: "booking", BookingID: &bk.ID, AgreementID: &ag.ID,
			Requirement: "conferma dell'hotel", VerifiedAt: t0,
		})
		insert(&store.RequirementVerification{
			ID: store.NewID(), EstablishmentID: est.ID, Subject: "customer", CustomerID: &cust.ID, CustomerCategoryID: &residents.ID,
			Requirement: "documento di residenza", VerifiedAt: t0,
		})
		expectError("booking subject without booking", &store.RequirementVerification{
			ID: store.NewID(), EstablishmentID: est.ID, Subject: "booking", CustomerID: &cust.ID, VerifiedAt: t0,
		})

		// Move undo log.
		insert(&store.MoveUndoEntry{
			ID: store.NewID(), EstablishmentID: est.ID, Kind: store.MoveUndoSwap, Payload: store.JSON(`{"lines":[]}`),
			BookingIDs: []string{bk.ID}, CreatedAt: t0, ExpiresAt: t0.Add(store.MoveUndoWindow),
		})
		expectError("undo entry expiring before creation", &store.MoveUndoEntry{
			ID: store.NewID(), EstablishmentID: est.ID, Kind: store.MoveUndoMove, Payload: store.JSON(`{}`),
			BookingIDs: []string{}, CreatedAt: t0, ExpiresAt: t0,
		})
	})
}
