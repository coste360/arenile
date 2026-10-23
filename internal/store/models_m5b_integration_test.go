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

// TestMigration00008_ModelsRoundtrip inserts and reloads the M5B models on
// both dialects and checks the constraints the renewal, waitlist and credit
// services rely on: one proposal per campaign and pass and one open proposal
// per pass, one pending offer per entry, credit sign per source, one expiry
// per grant, the non-negative credit balance and the backfilled ledger.
func TestMigration00008_ModelsRoundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-m5b", "Lido M5B", t0)
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

		s26 := &store.Season{ID: store.NewID(), EstablishmentID: est.ID, Year: 2026, OpensOn: d("2026-05-15"), ClosesOn: d("2026-09-20"), Status: "active", Version: 1, CreatedAt: t0, UpdatedAt: t0}
		s27 := &store.Season{ID: store.NewID(), EstablishmentID: est.ID, Year: 2027, OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-19"), Status: "draft", Version: 1, CreatedAt: t0, UpdatedAt: t0}
		insert(s26)
		insert(s27)
		ro := &store.SeasonRollover{
			ID: store.NewID(), EstablishmentID: est.ID, SourceSeasonID: s26.ID, TargetSeasonID: s27.ID, TargetYear: 2027,
			Options: store.JSON(`{"include_rate_periods":true}`), Result: store.JSON(`{"rate_periods":3}`), CreatedAt: t0,
		}
		insert(ro)
		dup := *ro
		dup.ID = store.NewID()
		expectConflict(&dup)

		m := &store.Map{ID: store.NewID(), EstablishmentID: est.ID, Name: store.I18nText{"it": "Spiaggia"}, Width: 100, Height: 60, GridSize: 1, SeaSide: "top", Version: 1, CreatedAt: t0, UpdatedAt: t0}
		insert(m)
		zone := &store.Zone{ID: store.NewID(), EstablishmentID: est.ID, MapID: &m.ID, Code: "row1", Name: store.I18nText{"it": "Prima fila"}, AssignmentMode: "seat", OnlineVisible: true, PriceCategory: "row1", Version: 1, CreatedAt: t0, UpdatedAt: t0}
		insert(zone)
		spot := &store.Spot{
			ID: store.NewID(), EstablishmentID: est.ID, MapID: m.ID, ZoneID: zone.ID, Label: "A1", RowLabel: "A", Number: 1,
			Kind: "umbrella", Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{},
			Attributes: []string{}, Active: true, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(spot)
		pack := &store.Pack{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "base", Name: store.I18nText{"it": "Base"}, Description: store.I18nText{},
			Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"season"}, Channels: []string{"desk"},
			MaxPerBooking: 1, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(pack)
		cust := &store.Customer{ID: store.NewID(), EstablishmentID: est.ID, FirstName: "Mario", LastName: "Rossi", Locale: "it", Tags: []string{}, Version: 1, CreatedAt: t0, UpdatedAt: t0}
		insert(cust)
		booking := func(code, status string, season *store.Season) *store.Booking {
			return &store.Booking{
				ID: store.NewID(), EstablishmentID: est.ID, Code: code, CustomerID: cust.ID, Channel: store.ChannelDesk,
				Status: status, StartsOn: season.OpensOn, EndsOn: season.ClosesOn, Slots: calendar.SlotsFull, Formula: "season",
				SeasonID: &season.ID, IsSeasonPass: true, PriceSnapshot: store.JSON(`{}`), TotalAmount: 150000,
				PolicySnapshot: store.JSON(`{"rules":{}}`), Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		src := booking("AR-PASS1", "confirmed", s26)
		insert(src)
		deadline := t0.Add(30 * 24 * time.Hour)
		held := booking("AR-RENEW1", "held", s27)
		held.RenewalOfBookingID = &src.ID
		held.HoldExpiresAt = &deadline
		insert(held)
		held2 := booking("AR-RENEW2", "held", s27)
		held2.HoldExpiresAt = &deadline
		insert(held2)

		camp := &store.RenewalCampaign{
			ID: store.NewID(), EstablishmentID: est.ID, Name: "Rinnovi 2027", Status: store.RenewalCampaignOpen,
			SourceSeasonID: s26.ID, TargetSeasonID: s27.ID, DeadlineAt: deadline, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(camp)
		closedNoDate := *camp
		closedNoDate.ID, closedNoDate.Status, closedNoDate.TargetSeasonID = store.NewID(), store.RenewalCampaignClosed, s26.ID
		expectError("closed campaign without closed_at", &closedNoDate)

		prop := &store.RenewalProposal{
			ID: store.NewID(), EstablishmentID: est.ID, CampaignID: camp.ID, SourceBookingID: src.ID, BookingID: held.ID,
			CustomerID: cust.ID, SpotID: spot.ID, PackID: pack.ID, Status: store.RenewalProposed, PreviousAmount: 145000,
			TotalAmount: 150000, DepositDue: 45000, DeadlineAt: deadline, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(prop)
		var gotProp store.RenewalProposal
		if err := db.NewSelect().Model(&gotProp).Where("id = ?", prop.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if gotProp.TotalAmount != 150000 || !gotProp.DeadlineAt.Equal(deadline) || gotProp.Status != store.RenewalProposed {
			t.Fatalf("proposal roundtrip = %+v", gotProp)
		}
		// One open proposal per source pass (also across campaigns).
		again := *prop
		again.ID, again.BookingID = store.NewID(), held2.ID
		expectConflict(&again)
		// responded_at is set exactly when the proposal is no longer proposed.
		answered := *prop
		answered.ID, answered.Status = store.NewID(), store.RenewalDeclined
		expectError("declined without responded_at", &answered)

		hold := &store.Hold{
			ID: store.NewID(), EstablishmentID: est.ID, Channel: store.ChannelOnline, StartsOn: d("2027-07-03"), EndsOn: d("2027-07-03"),
			Slots: calendar.SlotsFull, Items: []store.HoldItem{{SpotID: spot.ID}}, ExpiresAt: deadline, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(hold)
		hold2 := *hold
		hold2.ID = store.NewID()
		insert(&hold2)
		insert(&store.WaitlistSettings{EstablishmentID: est.ID, Enabled: true, PublicJoin: true, AutoOffer: true, OfferHoldMinutes: 120, Version: 1, CreatedAt: t0, UpdatedAt: t0})
		expectError("offer hold below 5 minutes", &store.WaitlistSettings{EstablishmentID: store.NewID(), OfferHoldMinutes: 1, Version: 1, CreatedAt: t0, UpdatedAt: t0})
		tok := "hash-of-token"
		entry := &store.WaitlistEntry{
			ID: store.NewID(), EstablishmentID: est.ID, CustomerID: cust.ID, Channel: store.ChannelOnline, StartsOn: d("2027-07-03"),
			EndsOn: d("2027-07-05"), Slots: calendar.SlotsFull, PackID: pack.ID, ZoneID: &zone.ID, Quantity: 1,
			Status: store.WaitlistOffered, Locale: "it", AccessTokenHash: &tok, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(entry)
		sameToken := *entry
		sameToken.ID = store.NewID()
		expectConflict(&sameToken)
		offer := &store.WaitlistOffer{
			ID: store.NewID(), EstablishmentID: est.ID, EntryID: entry.ID, Status: store.WaitlistOfferPending, HoldID: hold.ID,
			SpotID: &spot.ID, Cause: store.WaitlistCauseCancellation, ExpiresAt: deadline, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(offer)
		second := *offer
		second.ID, second.HoldID = store.NewID(), hold2.ID
		expectConflict(&second) // one pending offer per entry

		// Credit ledger: sign per source, expiry references its grant once.
		grant := &store.CustomerCredit{ID: store.NewID(), EstablishmentID: est.ID, CustomerID: cust.ID, Amount: 2000, Source: store.CreditSourceGoodwill, CreatedAt: t0}
		insert(grant)
		expectError("negative goodwill", &store.CustomerCredit{ID: store.NewID(), EstablishmentID: est.ID, CustomerID: cust.ID, Amount: -1, Source: store.CreditSourceGoodwill, CreatedAt: t0})
		expectError("positive redemption", &store.CustomerCredit{ID: store.NewID(), EstablishmentID: est.ID, CustomerID: cust.ID, Amount: 1, Source: store.CreditSourceRedemption, CreatedAt: t0})
		expectError("expiry without reference", &store.CustomerCredit{ID: store.NewID(), EstablishmentID: est.ID, CustomerID: cust.ID, Amount: -2000, Source: store.CreditSourceExpiry, CreatedAt: t0})
		insert(&store.CustomerCredit{ID: store.NewID(), EstablishmentID: est.ID, CustomerID: cust.ID, Amount: -2000, Source: store.CreditSourceExpiry, ReferenceCreditID: &grant.ID, CreatedAt: t0})
		expectConflict(&store.CustomerCredit{ID: store.NewID(), EstablishmentID: est.ID, CustomerID: cust.ID, Amount: -1, Source: store.CreditSourceExpiry, ReferenceCreditID: &grant.ID, CreatedAt: t0})
		insert(&store.CustomerCredit{ID: store.NewID(), EstablishmentID: est.ID, CustomerID: cust.ID, Amount: -500, Source: store.CreditSourceAdjustment, CreatedAt: t0})

		insert(&store.CustomerCreditBalance{EstablishmentID: est.ID, CustomerID: cust.ID, Balance: 0, UpdatedAt: t0})
		res, err := db.NewUpdate().Model((*store.CustomerCreditBalance)(nil)).Set("balance = balance - 1").
			Where("customer_id = ?", cust.ID).Where("balance - 1 >= 0").Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n, _ := res.RowsAffected(); n != 0 {
			t.Fatalf("conditional debit below zero affected %d rows", n)
		}
		if _, err := db.NewUpdate().Model((*store.CustomerCreditBalance)(nil)).Set("balance = -1").Where("customer_id = ?", cust.ID).Exec(ctx); err == nil {
			t.Fatal("negative balance accepted")
		}
	})
}
