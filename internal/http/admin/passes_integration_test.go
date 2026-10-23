//go:build integration

package admin_test

import (
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var passT0 = time.Date(2027, 4, 1, 8, 0, 0, 0, time.UTC)

// passFakeBookings writes the booking, line and season allocations of a
// pass like booking.Service.CreateInTx (owned by the bookings builder).
type passFakeBookings struct{}

func (passFakeBookings) CreateInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in booking.CreateInput) (*booking.Detail, error) {
	ln := in.Lines[0]
	b := &store.Booking{
		ID: store.NewID(), EstablishmentID: es.ID(), Code: "AR-PASS1", CustomerID: in.CustomerID, Channel: in.Channel, Status: "confirmed",
		StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots, Formula: in.Formula, SeasonID: &in.SeasonID, IsSeasonPass: true,
		PriceSnapshot: store.JSON(`{}`), TotalAmount: 90000, PolicySnapshot: store.JSON(`{"rules":{}}`), Version: 1, CreatedAt: passT0, UpdatedAt: passT0,
	}
	if _, err := tx.NewInsert().Model(b).Exec(ctx); err != nil {
		return nil, err
	}
	line := &store.BookingLine{
		ID: store.NewID(), EstablishmentID: es.ID(), BookingID: b.ID, PackID: ln.PackID, ZoneID: ln.ZoneID, SpotID: &ln.SpotID,
		AssignmentStatus: store.AssignmentAssigned, Quantity: 1, PriceCategory: "row1", LineTotal: 90000, CreatedAt: passT0, UpdatedAt: passT0,
	}
	if _, err := tx.NewInsert().Model(line).Exec(ctx); err != nil {
		return nil, err
	}
	var days []availability.DateSlot
	for _, day := range calendar.Range(in.StartsOn, in.EndsOn) {
		for _, s := range in.Slots.Expand() {
			days = append(days, availability.DateSlot{Date: day, Slot: s})
		}
	}
	if err := (passFakeInventory{}).RestoreSeasonDays(ctx, tx, es, inventory.SeasonDays{BookingID: b.ID, BookingLineID: line.ID, SpotID: ln.SpotID, Days: days}); err != nil {
		return nil, err
	}
	return &booking.Detail{Booking: *b}, nil
}

type passFakeInventory struct{}

func (passFakeInventory) RemoveSeasonDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, days inventory.SeasonDays) error {
	for _, x := range days.Days {
		if _, err := tx.NewDelete().Model((*store.SpotAllocation)(nil)).
			Where("booking_line_id = ? AND date = ? AND slot = ?", days.BookingLineID, x.Date, x.Slot).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (passFakeInventory) RestoreSeasonDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, days inventory.SeasonDays) error {
	rows := make([]store.SpotAllocation, len(days.Days))
	for i, x := range days.Days {
		rows[i] = store.SpotAllocation{
			ID: store.NewID(), EstablishmentID: es.ID(), SpotID: days.SpotID, Date: x.Date, Slot: x.Slot, Kind: store.AllocationSeason,
			BookingID: &days.BookingID, BookingLineID: &days.BookingLineID, CreatedAt: passT0,
		}
	}
	_, err := tx.NewInsert().Model(&rows).Exec(ctx)
	return err
}

func TestSeasonPassesHTTP_Flow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", passT0)
		if err != nil {
			t.Fatal(err)
		}
		_, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", passT0)
		if err != nil {
			t.Fatal(err)
		}
		c := clock.NewFake(passT0)
		aw, ob := audit.NewWriter(c), event.NewOutbox(c)
		customers := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
		h := &admin.Handlers{
			Customers: customers, Log: slog.New(slog.DiscardHandler),
			Passes: pass.NewService(pass.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: passFakeBookings{}, Inventory: passFakeInventory{}, Credits: customers}),
		}
		uid := store.NewID()
		if err := store.InsertUser(ctx, db, &store.User{ID: uid, Email: "staff@lido.it", PasswordHash: "x", Name: "S", Locale: "it", Active: true, CreatedAt: passT0, UpdatedAt: passT0}); err != nil {
			t.Fatal(err)
		}
		cashier := &auth.Principal{Kind: auth.PrincipalUser, UserID: uid, Grants: []auth.Grant{{OrganizationID: orgA.ID, EstablishmentID: estA.ID, Role: auth.RoleCashier}}}
		viewer := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: orgA.ID, EstablishmentID: estA.ID, Role: auth.RoleViewer}}}

		ins := func(m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
				t.Fatalf("insert %T: %v", m, err)
			}
		}
		season := &store.Season{ID: store.NewID(), EstablishmentID: estA.ID, Year: 2027, OpensOn: calendar.MustParse("2027-06-01"), ClosesOn: calendar.MustParse("2027-06-10"), Status: "active", Version: 1, CreatedAt: passT0, UpdatedAt: passT0}
		ins(season)
		m := &store.Map{ID: store.NewID(), EstablishmentID: estA.ID, Name: store.I18nText{"it": "S"}, Width: 10, Height: 10, GridSize: 1, SeaSide: "bottom", Version: 1, CreatedAt: passT0, UpdatedAt: passT0}
		ins(m)
		z := &store.Zone{ID: store.NewID(), EstablishmentID: estA.ID, MapID: &m.ID, Code: "row1", Name: store.I18nText{"it": "R"}, AssignmentMode: "seat", PriceCategory: "row1", Version: 1, CreatedAt: passT0, UpdatedAt: passT0}
		ins(z)
		spot := &store.Spot{
			ID: store.NewID(), EstablishmentID: estA.ID, MapID: m.ID, ZoneID: z.ID, Label: "A1", RowLabel: "A", Number: 1, Kind: "umbrella",
			Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{}, Attributes: []string{}, Active: true, CreatedAt: passT0, UpdatedAt: passT0,
		}
		ins(spot)
		pk := &store.Pack{
			ID: store.NewID(), EstablishmentID: estA.ID, Code: "base", Name: store.I18nText{"it": "Base"}, Description: store.I18nText{},
			Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"season"}, Channels: []string{"desk"}, MaxPerBooking: 1, Active: true, Version: 1, CreatedAt: passT0, UpdatedAt: passT0,
		}
		ins(pk)
		holder, err := customers.Create(ctx, cashier, "", estA.ID, customer.Input{LastName: "Rossi", FirstName: "Mario"}, false)
		if err != nil {
			t.Fatal(err)
		}
		a := estA.ID

		status, hdr, body := callTarget(t, cashier, http.MethodPost, "/", api.SeasonPassCreate{CustomerId: holder.ID, SeasonId: season.ID, SpotId: spot.ID, PackId: pk.ID},
			func(w http.ResponseWriter, r *http.Request) {
				h.CreateSeasonPass(w, r, a, api.CreateSeasonPassParams{})
			})
		if status != 201 || hdr.Get("ETag") != `"1"` {
			t.Fatalf("create: %d %v", status, body)
		}
		p := body["pass"].(map[string]any)
		pid := p["id"].(string)
		if p["spot_label"] != "A1" || p["customer_display_name"] != "Rossi Mario" || p["total_amount"].(float64) != 90000 ||
			p["balance_due"].(float64) != 90000 || p["season_id"] != season.ID || p["slots"] != "FULL" {
			t.Fatalf("pass %v", p)
		}
		status, _, body = callTarget(t, viewer, http.MethodPost, "/", api.SeasonPassCreate{CustomerId: holder.ID, SeasonId: season.ID, SpotId: spot.ID, PackId: pk.ID},
			func(w http.ResponseWriter, r *http.Request) {
				h.CreateSeasonPass(w, r, a, api.CreateSeasonPassParams{})
			})
		if status != 403 {
			t.Fatalf("viewer create: %d %v", status, body)
		}

		status, _, body = callTarget(t, viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.ListSeasonPasses(w, r, a, api.ListSeasonPassesParams{SpotId: &spot.ID})
		})
		if status != 200 || len(body["items"].([]any)) != 1 {
			t.Fatalf("list: %d %v", status, body)
		}

		pm := api.Slots("PM")
		status, _, body = callTarget(t, cashier, http.MethodPost, "/", api.SeasonReleaseCreate{Dates: []api.Date{calendar.MustParse("2027-06-05")}, Slots: &pm, Note: strp("al mare altrove")},
			func(w http.ResponseWriter, r *http.Request) {
				h.CreateSeasonPassReleases(w, r, a, pid, api.CreateSeasonPassReleasesParams{})
			})
		items, _ := body["items"].([]any)
		if status != 201 || len(items) != 1 || items[0].(map[string]any)["status"] != "released" || items[0].(map[string]any)["slot"] != "PM" {
			t.Fatalf("release: %d %v", status, body)
		}
		relID := items[0].(map[string]any)["id"].(string)
		status, _, body = callTarget(t, cashier, http.MethodPost, "/", api.SeasonReleaseCreate{Dates: []api.Date{calendar.MustParse("2027-06-05")}},
			func(w http.ResponseWriter, r *http.Request) {
				h.CreateSeasonPassReleases(w, r, a, pid, api.CreateSeasonPassReleasesParams{})
			})
		if status != 409 || body["type"] != "conflict" {
			t.Fatalf("release twice: %d %v", status, body)
		}
		status, _, body = callTarget(t, cashier, http.MethodPost, "/", api.SeasonReleaseCreate{Dates: []api.Date{calendar.MustParse("2027-07-05")}},
			func(w http.ResponseWriter, r *http.Request) {
				h.CreateSeasonPassReleases(w, r, a, pid, api.CreateSeasonPassReleasesParams{})
			})
		if status != 400 || body["type"] != "validation_failed" {
			t.Fatalf("release outside: %d %v", status, body)
		}

		status, hdr, body = callTarget(t, viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) { h.GetSeasonPass(w, r, a, pid) })
		if status != 200 || hdr.Get("ETag") != `"1"` || body["pass"].(map[string]any)["released_days"].(float64) != 1 || len(body["releases"].([]any)) != 1 {
			t.Fatalf("get: %d %v", status, body)
		}
		status, _, _ = callTarget(t, viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) { h.GetSeasonPass(w, r, estB.ID, pid) })
		if status != 404 {
			t.Fatalf("cross-tenant get: %d", status)
		}

		status, _, _ = callTarget(t, cashier, http.MethodDelete, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.RevokeSeasonPassRelease(w, r, a, pid, relID, api.RevokeSeasonPassReleaseParams{})
		})
		if status != 204 {
			t.Fatalf("revoke: %d", status)
		}
		status, _, body = callTarget(t, viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) { h.ListSeasonPassReleases(w, r, a, pid) })
		if status != 200 || body["items"].([]any)[0].(map[string]any)["status"] != "revoked" {
			t.Fatalf("releases after revoke: %d %v", status, body)
		}

		status, hdr, body = callTarget(t, cashier, http.MethodPatch, "/", api.SeasonPassUpdate{NotesInternal: strp("paga a fine mese")},
			func(w http.ResponseWriter, r *http.Request) {
				h.UpdateSeasonPass(w, r, a, pid, api.UpdateSeasonPassParams{IfMatch: strp(`"1"`)})
			})
		if status != 200 || hdr.Get("ETag") != `"2"` {
			t.Fatalf("update: %d %v", status, body)
		}
		status, _, body = callTarget(t, cashier, http.MethodPatch, "/", api.SeasonPassUpdate{NotesInternal: strp("x")},
			func(w http.ResponseWriter, r *http.Request) {
				h.UpdateSeasonPass(w, r, a, pid, api.UpdateSeasonPassParams{IfMatch: strp(`"1"`)})
			})
		if status != 412 {
			t.Fatalf("stale update: %d %v", status, body)
		}
	})
}
