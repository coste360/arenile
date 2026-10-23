//go:build integration

package pass_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// t0 is before the season: every day of the season is in the future.
var t0 = time.Date(2027, 4, 1, 8, 0, 0, 0, time.UTC)

var d = calendar.MustParse

const passPrice = money.Cents(120000)

// creditPolicy is the policy snapshot of the passes created by fakeBookings:
// 50% of the resale share goes to the holder (spec 9).
const creditPolicy = `{"policy_id":"p1","rules":{"season_release_credit":{"enabled":true,"percent":50}}}`

// fakeBookings stands in for booking.Service.CreateInTx (owned by the
// bookings builder): it writes the booking, its line and the season
// allocations through the same tables and constraints, so that the pass
// flows run against the real schema.
type fakeBookings struct{ seq atomic.Int32 }

func (f *fakeBookings) CreateInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in booking.CreateInput) (*booking.Detail, error) {
	if !p.Can(es, auth.PermBookingsWrite) {
		return nil, auth.ErrForbidden
	}
	if _, err := store.GetCustomer(ctx, tx, es, in.CustomerID); err != nil {
		return nil, customer.ErrNotFound
	}
	if in.Formula != "season" || !in.IsSeasonPass || in.AllocationKind != inventory.KindSeason || len(in.Lines) != 1 {
		return nil, fmt.Errorf("unexpected booking request %+v", in)
	}
	ln := in.Lines[0]
	days := halfDays(in.StartsOn, in.EndsOn, in.Slots)
	if err := freeOrConflict(ctx, tx, ln.SpotID, days); err != nil {
		return nil, err
	}
	now := t0
	b := &store.Booking{
		ID: store.NewID(), EstablishmentID: es.ID(), Code: fmt.Sprintf("AR-P%04d", f.seq.Add(1)), CustomerID: in.CustomerID,
		Channel: in.Channel, Status: "confirmed", StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots, Formula: in.Formula,
		SeasonID: &in.SeasonID, IsSeasonPass: true, PriceSnapshot: store.JSON(`{"total":120000}`), TotalAmount: passPrice,
		PolicySnapshot: store.JSON(creditPolicy), NotesInternal: in.NotesInternal, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if in.RenewalOfBookingID != "" {
		b.RenewalOfBookingID = &in.RenewalOfBookingID
	}
	if _, err := tx.NewInsert().Model(b).Exec(ctx); err != nil {
		return nil, err
	}
	line := &store.BookingLine{
		ID: store.NewID(), EstablishmentID: es.ID(), BookingID: b.ID, PackID: ln.PackID, ZoneID: ln.ZoneID, SpotID: &ln.SpotID,
		AssignmentStatus: store.AssignmentAssigned, Quantity: 1, PriceCategory: "row1", LineTotal: passPrice, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := tx.NewInsert().Model(line).Exec(ctx); err != nil {
		return nil, err
	}
	if err := insertAllocations(ctx, tx, es, ln.SpotID, b.ID, line.ID, store.AllocationSeason, days); err != nil {
		return nil, err
	}
	return &booking.Detail{Booking: *b, Lines: []store.BookingLine{*line}}, nil
}

// fakeInventory stands in for inventory.Service (owned by the availability
// builder) with the documented semantics: release deletes the season rows,
// restore recreates them and fails with a ConflictError on occupied half-days.
type fakeInventory struct{}

func (fakeInventory) RemoveSeasonDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, days inventory.SeasonDays) error {
	for _, x := range days.Days {
		_, err := tx.NewDelete().Model((*store.SpotAllocation)(nil)).
			Where("establishment_id = ? AND booking_line_id = ? AND kind = ? AND date = ? AND slot = ?",
				es.ID(), days.BookingLineID, store.AllocationSeason, x.Date, x.Slot).Exec(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (fakeInventory) RestoreSeasonDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, days inventory.SeasonDays) error {
	if err := freeOrConflict(ctx, tx, days.SpotID, days.Days); err != nil {
		return err
	}
	return insertAllocations(ctx, tx, es, days.SpotID, days.BookingID, days.BookingLineID, store.AllocationSeason, days.Days)
}

func halfDays(from, to calendar.Date, slots calendar.Slots) []availability.DateSlot {
	var out []availability.DateSlot
	for _, day := range calendar.Range(from, to) {
		for _, s := range slots.Expand() {
			out = append(out, availability.DateSlot{Date: day, Slot: s})
		}
	}
	return out
}

func freeOrConflict(ctx context.Context, tx bun.IDB, spotID string, days []availability.DateSlot) error {
	var taken []store.SpotAllocation
	if err := tx.NewSelect().Model(&taken).Where("spot_id = ?", spotID).Scan(ctx); err != nil {
		return err
	}
	busy := map[string]bool{}
	for _, a := range taken {
		busy[a.Date.String()+string(a.Slot)] = true
	}
	var cs []availability.Conflict
	for _, x := range days {
		if busy[x.Date.String()+string(x.Slot)] {
			cs = append(cs, availability.Conflict{SpotID: spotID, Date: x.Date, Slot: x.Slot, Reason: availability.ReasonSpotUnavailable})
		}
	}
	if len(cs) > 0 {
		return &availability.ConflictError{Err: availability.ErrSpotUnavailable, Conflicts: cs}
	}
	return nil
}

func insertAllocations(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, spotID, bookingID, lineID, kind string, days []availability.DateSlot) error {
	if len(days) == 0 {
		return nil
	}
	rows := make([]store.SpotAllocation, len(days))
	for i, x := range days {
		rows[i] = store.SpotAllocation{
			ID: store.NewID(), EstablishmentID: es.ID(), SpotID: spotID, Date: x.Date, Slot: x.Slot, Kind: kind,
			BookingID: &bookingID, BookingLineID: &lineID, CreatedAt: t0,
		}
	}
	_, err := tx.NewInsert().Model(&rows).Exec(ctx)
	return err
}

type env struct {
	ctx         context.Context
	db          *store.DB
	clock       *clock.Fake
	svc         *pass.Service
	resale      *pass.Resale
	customers   *customer.Service
	estA, estB  *store.Establishment
	esA         store.EstablishmentScope
	owner       *auth.Principal
	cashier     *auth.Principal
	staff       *auth.Principal
	viewer      *auth.Principal
	ownerB      *auth.Principal
	season      *store.Season
	zone        *store.Zone
	spot, spot2 *store.Spot
	spotB       *store.Spot
	pack        *store.Pack
	holder      *store.Customer
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", t0)
	if err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", t0)
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(t0)
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	customers := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	user := func(org, est string, role auth.Role) *auth.Principal {
		id := store.NewID()
		u := &store.User{ID: id, Email: id + "@lido.it", PasswordHash: "x", Name: string(role), Locale: "it", Active: true, CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertUser(ctx, db, u); err != nil {
			t.Fatal(err)
		}
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: id, Grants: []auth.Grant{{OrganizationID: org, EstablishmentID: est, Role: role}}}
	}
	e := &env{
		ctx: ctx, db: db, clock: c, estA: estA, estB: estB, customers: customers,
		svc: pass.NewService(pass.Deps{
			DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: &fakeBookings{}, Inventory: fakeInventory{}, Credits: customers,
		}),
		resale:  pass.NewResale(pass.ResaleDeps{Clock: c, Credits: customers}),
		owner:   user(orgA.ID, "", auth.RoleOwner),
		cashier: user(orgA.ID, estA.ID, auth.RoleCashier),
		staff:   user(orgA.ID, estA.ID, auth.RoleBeachStaff),
		viewer:  user(orgA.ID, estA.ID, auth.RoleViewer),
		ownerB:  user(orgB.ID, "", auth.RoleOwner),
	}
	if e.esA, err = store.SystemEstablishmentScope(ctx, db, estA.ID); err != nil {
		t.Fatal(err)
	}
	e.season = &store.Season{ID: store.NewID(), EstablishmentID: estA.ID, Year: 2027, OpensOn: d("2027-06-01"), ClosesOn: d("2027-06-30"), Status: "active", Version: 1, CreatedAt: t0, UpdatedAt: t0}
	e.insert(t, e.season)
	var m *store.Map
	m, e.zone = e.mapZone(t, estA.ID)
	e.spot = e.newSpot(t, estA.ID, m.ID, e.zone.ID, "A1", true)
	e.spot2 = e.newSpot(t, estA.ID, m.ID, e.zone.ID, "A2", false)
	mB, zB := e.mapZone(t, estB.ID)
	e.spotB = e.newSpot(t, estB.ID, mB.ID, zB.ID, "B1", true)
	e.pack = &store.Pack{
		ID: store.NewID(), EstablishmentID: estA.ID, Code: "base", Name: store.I18nText{"it": "Base"}, Description: store.I18nText{},
		Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"season"}, Channels: []string{"desk"}, MaxPerBooking: 1, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	e.insert(t, e.pack)
	if e.holder, err = customers.Create(ctx, e.cashier, "", estA.ID, customer.Input{FirstName: "Mario", LastName: "Rossi"}, false); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) insert(t *testing.T, m any) {
	t.Helper()
	if _, err := e.db.NewInsert().Model(m).Exec(e.ctx); err != nil {
		t.Fatalf("insert %T: %v", m, err)
	}
}

func (e *env) mapZone(t *testing.T, est string) (*store.Map, *store.Zone) {
	t.Helper()
	m := &store.Map{ID: store.NewID(), EstablishmentID: est, Name: store.I18nText{"it": "Spiaggia"}, Width: 10, Height: 10, GridSize: 1, SeaSide: "bottom", Version: 1, CreatedAt: t0, UpdatedAt: t0}
	e.insert(t, m)
	z := &store.Zone{ID: store.NewID(), EstablishmentID: est, MapID: &m.ID, Code: "row1", Name: store.I18nText{"it": "Prima fila"}, AssignmentMode: "seat", PriceCategory: "row1", Version: 1, CreatedAt: t0, UpdatedAt: t0}
	e.insert(t, z)
	return m, z
}

func (e *env) newSpot(t *testing.T, est, mapID, zoneID, label string, active bool) *store.Spot {
	t.Helper()
	s := &store.Spot{
		ID: store.NewID(), EstablishmentID: est, MapID: mapID, ZoneID: zoneID, Label: label, RowLabel: "A", Number: 1, Kind: "umbrella",
		Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{}, Attributes: []string{}, Active: active, CreatedAt: t0, UpdatedAt: t0,
	}
	e.insert(t, s)
	return s
}

func (e *env) count(t *testing.T, table, where string, args ...any) int {
	t.Helper()
	n, err := e.db.NewSelect().TableExpr(table).Where(where, args...).Count(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func (e *env) createPass(t *testing.T, in pass.CreateInput) *pass.Detail {
	t.Helper()
	if in.CustomerID == "" {
		in.CustomerID = e.holder.ID
	}
	if in.SeasonID == "" {
		in.SeasonID = e.season.ID
	}
	if in.SpotID == "" {
		in.SpotID = e.spot.ID
	}
	if in.PackID == "" {
		in.PackID = e.pack.ID
	}
	dt, err := e.svc.Create(e.ctx, e.cashier, "10.0.0.1", e.estA.ID, in)
	if err != nil {
		t.Fatalf("create pass: %v", err)
	}
	return dt
}

func mustField(t *testing.T, err error, field string) {
	t.Helper()
	var ve *configerr.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want validation error on %q, got %v", field, err)
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return
		}
	}
	t.Fatalf("no error on %q: %+v", field, ve.Fields)
}

func TestPassService_CreateListAndUpdate(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		dt := e.createPass(t, pass.CreateInput{NotesInternal: "ombrellone preferito"})
		v := dt.Pass
		if !v.Booking.IsSeasonPass || v.Booking.Formula != "season" || v.Booking.StartsOn.String() != "2027-06-01" ||
			v.Booking.EndsOn.String() != "2027-06-30" || v.Booking.Slots != calendar.SlotsFull || v.SpotLabel != "A1" ||
			v.PackID != e.pack.ID || v.CustomerDisplayName != "Rossi Mario" || v.ReleasedDays != 0 || len(dt.Releases) != 0 {
			t.Fatalf("pass %+v", v)
		}
		if n := e.count(t, "spot_allocations", "booking_id = ? AND kind = ?", v.Booking.ID, store.AllocationSeason); n != 60 {
			t.Fatalf("season allocations %d, want 30 days x 2 slots", n)
		}
		if n := e.count(t, "audit_log", "action = ? AND entity_id = ?", "season_pass.created", v.Booking.ID); n != 1 {
			t.Fatalf("audit %d", n)
		}

		// The spot is taken for the whole season: a second pass conflicts
		// and lists the half-days (409 spot_unavailable).
		_, err := e.svc.Create(e.ctx, e.cashier, "", e.estA.ID, pass.CreateInput{
			CustomerID: e.holder.ID, SeasonID: e.season.ID, SpotID: e.spot.ID, PackID: e.pack.ID,
			StartsOn: ptr(d("2027-06-29")), Slots: calendar.SlotsAM,
		})
		var ce *availability.ConflictError
		if !errors.As(err, &ce) || len(ce.Conflicts) != 2 || ce.Conflicts[0].Date.String() != "2027-06-29" {
			t.Fatalf("conflict: %v", err)
		}

		// Validation and tenancy.
		_, err = e.svc.Create(e.ctx, e.cashier, "", e.estA.ID, pass.CreateInput{CustomerID: e.holder.ID, SeasonID: store.NewID(), SpotID: e.spotB.ID, PackID: e.pack.ID})
		mustField(t, err, "season_id")
		mustField(t, err, "spot_id")
		_, err = e.svc.Create(e.ctx, e.cashier, "", e.estA.ID, pass.CreateInput{CustomerID: e.holder.ID, SeasonID: e.season.ID, SpotID: e.spot2.ID, PackID: e.pack.ID})
		mustField(t, err, "spot_id") // inactive spot
		_, err = e.svc.Create(e.ctx, e.cashier, "", e.estA.ID, pass.CreateInput{CustomerID: e.holder.ID, SeasonID: e.season.ID, SpotID: e.spot.ID, PackID: e.pack.ID, EndsOn: ptr(d("2027-07-10"))})
		mustField(t, err, "ends_on")
		if _, err := e.svc.Create(e.ctx, e.viewer, "", e.estA.ID, pass.CreateInput{}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer create: %v", err)
		}
		if _, err := e.svc.Get(e.ctx, e.ownerB, e.estA.ID, v.Booking.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("cross-tenant get: %v", err)
		}
		if _, err := e.svc.Get(e.ctx, e.ownerB, e.estB.ID, v.Booking.ID); !errors.Is(err, pass.ErrNotFound) {
			t.Fatalf("pass of A through B: %v", err)
		}

		// Listing with filters.
		other, err := e.customers.Create(e.ctx, e.cashier, "", e.estA.ID, customer.Input{LastName: "Bianchi"}, false)
		if err != nil {
			t.Fatal(err)
		}
		e.spot2.Active = true
		if _, err := db.NewUpdate().Model(e.spot2).Column("active").WherePK().Exec(e.ctx); err != nil {
			t.Fatal(err)
		}
		e.createPass(t, pass.CreateInput{CustomerID: other.ID, SpotID: e.spot2.ID, Slots: calendar.SlotsPM})
		list := func(f pass.ListFilter) int {
			t.Helper()
			page, err := e.svc.List(e.ctx, e.viewer, e.estA.ID, f, "", 50)
			if err != nil {
				t.Fatal(err)
			}
			return len(page.Items)
		}
		if list(pass.ListFilter{}) != 2 || list(pass.ListFilter{CustomerID: other.ID}) != 1 || list(pass.ListFilter{SpotID: e.spot.ID}) != 1 ||
			list(pass.ListFilter{SeasonID: e.season.ID, Status: "confirmed"}) != 2 || list(pass.ListFilter{Status: "cancelled"}) != 0 {
			t.Fatal("list filters")
		}
		page, err := e.svc.List(e.ctx, e.viewer, e.estA.ID, pass.ListFilter{}, "", 1)
		if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
			t.Fatalf("page 1: %v", err)
		}
		page, err = e.svc.List(e.ctx, e.viewer, e.estA.ID, pass.ListFilter{}, page.NextCursor, 1)
		if err != nil || len(page.Items) != 1 || page.NextCursor != "" {
			t.Fatalf("page 2: %v", err)
		}
		// The customer page lists the holder's passes.
		cd, err := e.customers.Get(e.ctx, e.viewer, e.estA.ID, e.holder.ID)
		if err != nil || len(cd.SeasonPasses) != 1 || cd.SeasonPasses[0].SpotLabel != "A1" {
			t.Fatalf("customer passes: %v", err)
		}

		// Notes update with optimistic concurrency and booking.modified.
		v1 := int64(1)
		up, err := e.svc.Update(e.ctx, e.cashier, "", e.estA.ID, v.Booking.ID, &v1, pass.UpdateInput{NotesCustomer: ptr("sdraio extra")})
		if err != nil || up.Pass.Booking.Version != 2 || up.Pass.Booking.NotesCustomer != "sdraio extra" || up.Pass.Booking.NotesInternal != "ombrellone preferito" {
			t.Fatalf("update: %v %+v", err, up)
		}
		var vc *configerr.VersionConflictError
		if _, err := e.svc.Update(e.ctx, e.cashier, "", e.estA.ID, v.Booking.ID, &v1, pass.UpdateInput{NotesInternal: ptr("x")}); !errors.As(err, &vc) || vc.Current != 2 {
			t.Fatalf("stale update: %v", err)
		}
		if n := e.count(t, "outbox_events", "type = ?", event.BookingModified); n != 1 {
			t.Fatalf("booking.modified events %d", n)
		}
	})
}

// TestSeasonRelease_RemovesAndRestoresAllocations verifies INV-10: an active
// release has no season allocation of its line on that half-day; a resold
// release has a booking allocation of the resale line; revoking recreates
// the season allocation; credits follow the policy snapshot.
func TestSeasonRelease_RemovesAndRestoresAllocations(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		e.clock.Set(time.Date(2027, 6, 5, 8, 0, 0, 0, time.UTC))
		dt := e.createPass(t, pass.CreateInput{StartsOn: ptr(d("2027-06-05"))})
		pid, lineID := dt.Pass.Booking.ID, dt.Pass.LineID

		rel, err := e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{Dates: []calendar.Date{d("2027-06-11"), d("2027-06-10")}, Note: "vacanza"})
		if err != nil {
			t.Fatal(err)
		}
		if len(rel) != 4 || rel[0].Date.String() != "2027-06-10" || rel[0].Slot != calendar.AM || rel[0].Note != "vacanza" {
			t.Fatalf("releases %+v", rel)
		}
		if n := e.count(t, "spot_allocations", "booking_line_id = ? AND date IN (?)", lineID, bun.List([]calendar.Date{d("2027-06-10"), d("2027-06-11")})); n != 0 {
			t.Fatalf("season rows left on released days: %d", n)
		}
		if n := e.count(t, "spot_allocations", "booking_line_id = ?", lineID); n != 26*2-4 {
			t.Fatalf("other season rows: %d", n)
		}
		var payload []byte
		if err := db.NewSelect().TableExpr("outbox_events").Column("payload").Where("type = ?", event.SeasonDayReleased).Scan(e.ctx, &payload); err != nil {
			t.Fatal(err)
		}
		var ac event.AllocationChange
		if err := json.Unmarshal(payload, &ac); err != nil || ac.BookingID != pid || len(ac.ReleaseIDs) != 4 || ac.SpotIDs[0] != e.spot.ID ||
			len(ac.Dates) != 2 || ac.Reason != "day_released" {
			t.Fatalf("event payload %s (%v)", payload, err)
		}

		// Already released, past and out of range days, permissions, tenancy.
		_, err = e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{Dates: []calendar.Date{d("2027-06-11")}, Slots: calendar.SlotsPM})
		if !errors.Is(err, pass.ErrAlreadyReleased) {
			t.Fatalf("release twice: %v", err)
		}
		_, err = e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{Dates: []calendar.Date{d("2027-06-04")}})
		mustField(t, err, "dates[0]")
		if _, err := e.svc.Release(e.ctx, e.viewer, "", e.estA.ID, pid, pass.ReleaseInput{Dates: []calendar.Date{d("2027-06-12")}}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer release: %v", err)
		}
		if _, err := e.svc.Release(e.ctx, e.ownerB, "", e.estB.ID, pid, pass.ReleaseInput{Dates: []calendar.Date{d("2027-06-12")}}); !errors.Is(err, pass.ErrNotFound) {
			t.Fatalf("release through B: %v", err)
		}

		// The released half-day is resold: a booking allocation of another
		// line occupies it and the booking service calls MarkResold.
		buyer, err := e.customers.Create(e.ctx, e.cashier, "", e.estA.ID, customer.Input{LastName: "Verdi"}, false)
		if err != nil {
			t.Fatal(err)
		}
		resaleLine := e.resaleBooking(t, buyer.ID, "2027-06-10", calendar.SlotsAM, 3000)
		err = store.RunInTx(e.ctx, db, func(ctx context.Context, tx bun.Tx) error {
			return e.resale.MarkResold(ctx, tx, e.esA, resaleLine, e.spot.ID, []calendar.Date{d("2027-06-10")}, calendar.SlotsAM)
		})
		if err != nil {
			t.Fatal(err)
		}
		rl, err := e.svc.ListReleases(e.ctx, e.viewer, e.estA.ID, pid)
		if err != nil {
			t.Fatal(err)
		}
		all := rl.Releases
		resold := all[0]
		if rl.ResoldBookingIDs[resaleLine] == "" {
			t.Fatalf("resale booking ids %v", rl.ResoldBookingIDs)
		}
		if resold.ResoldBookingLineID == nil || *resold.ResoldBookingLineID != resaleLine || resold.CreditAmount != 1500 || all[1].ResoldBookingLineID != nil {
			t.Fatalf("resold release %+v", resold)
		}
		if bal := e.balance(t); bal != 1500 {
			t.Fatalf("holder credit %v, want 50%% of 3000", bal)
		}
		if err := e.svc.Revoke(e.ctx, e.staff, "", e.estA.ID, pid, resold.ID); !errors.Is(err, pass.ErrReleaseResold) {
			t.Fatalf("revoke resold: %v", err)
		}

		// Revoke a release not resold: the season allocation comes back.
		pm10 := all[1]
		if err := e.svc.Revoke(e.ctx, e.staff, "", e.estA.ID, pid, pm10.ID); err != nil {
			t.Fatal(err)
		}
		if n := e.count(t, "spot_allocations", "booking_line_id = ? AND date = ? AND slot = ? AND kind = ?", lineID, pm10.Date, pm10.Slot, store.AllocationSeason); n != 1 {
			t.Fatalf("restored rows %d", n)
		}
		if err := e.svc.Revoke(e.ctx, e.staff, "", e.estA.ID, pid, pm10.ID); !errors.Is(err, pass.ErrReleaseRevoked) {
			t.Fatalf("revoke twice: %v", err)
		}
		if n := e.count(t, "outbox_events", "type = ?", event.SeasonDayReleaseRevoked); n != 1 {
			t.Fatalf("revoked events %d", n)
		}
		// The day can be released again after a revocation.
		if _, err := e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{Dates: []calendar.Date{d("2027-06-10")}, Slots: calendar.SlotsPM}); err != nil {
			t.Fatalf("release after revoke: %v", err)
		}

		// A released half-day taken by a block cannot be restored.
		e.block(t, "2027-06-11", calendar.AM)
		am11 := findRelease(t, all, "2027-06-11", calendar.AM)
		var ce *availability.ConflictError
		if err := e.svc.Revoke(e.ctx, e.staff, "", e.estA.ID, pid, am11.ID); !errors.As(err, &ce) {
			t.Fatalf("revoke on blocked day: %v", err)
		}
		if got, _ := store.GetSeasonRelease(e.ctx, db, e.esA, pid, am11.ID); got.RevokedAt != nil {
			t.Fatal("failed revocation must roll back")
		}

		// The resale is cancelled: the release is resellable again and the
		// credit is reversed.
		err = store.RunInTx(e.ctx, db, func(ctx context.Context, tx bun.Tx) error {
			if _, err := tx.NewDelete().Model((*store.SpotAllocation)(nil)).Where("booking_line_id = ?", resaleLine).Exec(ctx); err != nil {
				return err
			}
			return e.resale.UnmarkResold(ctx, tx, e.esA, resaleLine)
		})
		if err != nil {
			t.Fatal(err)
		}
		if bal := e.balance(t); bal != 0 {
			t.Fatalf("credit after unmark %v", bal)
		}
		if n := e.count(t, "customer_credits", "customer_id = ? AND source = ?", e.holder.ID, store.CreditSourceSeasonRelease); n != 2 {
			t.Fatalf("ledger rows %d, want grant + reversal", n)
		}
		e.assertINV10(t)

		// A cancelled pass cannot release days.
		if _, err := db.NewUpdate().Model((*store.Booking)(nil)).Set("status = 'cancelled'").Where("id = ?", pid).Exec(e.ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{Dates: []calendar.Date{d("2027-06-20")}}); !errors.Is(err, pass.ErrNotActive) {
			t.Fatalf("release on cancelled pass: %v", err)
		}
	})
}

// TestConcurrency_SeasonReleaseSameDay: 100 parallel releases of the same
// half-day of a pass produce exactly one active release (the partial unique
// index on active releases is the arbiter, not an application check).
func TestConcurrency_SeasonReleaseSameDay(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		dt := e.createPass(t, pass.CreateInput{})
		pid := dt.Pass.Booking.ID
		const workers = 100
		var ok, conflicts atomic.Int32
		var wg sync.WaitGroup
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{Dates: []calendar.Date{d("2027-06-15")}, Slots: calendar.SlotsAM})
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, pass.ErrAlreadyReleased):
					conflicts.Add(1)
				default:
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if ok.Load() != 1 || conflicts.Load() != workers-1 {
			t.Fatalf("releases ok=%d conflicts=%d", ok.Load(), conflicts.Load())
		}
		if n := e.count(t, "season_releases", "booking_id = ? AND revoked_at IS NULL", pid); n != 1 {
			t.Fatalf("active releases %d", n)
		}
		e.assertINV10(t)
	})
}

// resaleBooking inserts a booking of buyer on the pass spot for one day
// (allocation kind booking) and returns its line id.
func (e *env) resaleBooking(t *testing.T, buyerID, day string, slots calendar.Slots, total money.Cents) string {
	t.Helper()
	b := &store.Booking{
		ID: store.NewID(), EstablishmentID: e.estA.ID, Code: "AR-R" + day[8:], CustomerID: buyerID, Channel: store.ChannelDesk, Status: "confirmed",
		StartsOn: d(day), EndsOn: d(day), Slots: slots, Formula: "half_day", PriceSnapshot: store.JSON(`{}`), TotalAmount: total,
		PolicySnapshot: store.JSON(`{"rules":{}}`), Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	e.insert(t, b)
	line := &store.BookingLine{
		ID: store.NewID(), EstablishmentID: e.estA.ID, BookingID: b.ID, PackID: e.pack.ID, ZoneID: e.zone.ID, SpotID: &e.spot.ID,
		AssignmentStatus: store.AssignmentAssigned, Quantity: 1, PriceCategory: "row1", LineTotal: total, CreatedAt: t0, UpdatedAt: t0,
	}
	e.insert(t, line)
	if err := insertAllocations(e.ctx, e.db, e.esA, e.spot.ID, b.ID, line.ID, store.AllocationBooking, halfDays(d(day), d(day), slots)); err != nil {
		t.Fatal(err)
	}
	return line.ID
}

func (e *env) block(t *testing.T, day string, slot calendar.Slot) {
	t.Helper()
	b := &store.SpotBlock{ID: store.NewID(), EstablishmentID: e.estA.ID, SpotID: e.spot.ID, StartsOn: d(day), EndsOn: d(day), Slots: calendar.Slots(slot), Reason: "manutenzione", CreatedAt: t0, UpdatedAt: t0}
	e.insert(t, b)
	e.insert(t, &store.SpotAllocation{ID: store.NewID(), EstablishmentID: e.estA.ID, SpotID: e.spot.ID, Date: d(day), Slot: slot, Kind: store.AllocationBlock, BlockID: &b.ID, CreatedAt: t0})
}

func (e *env) balance(t *testing.T) money.Cents {
	t.Helper()
	bal, err := e.customers.CreditBalance(e.ctx, e.db, e.esA, e.holder.ID, d("2027-06-01"))
	if err != nil {
		t.Fatal(err)
	}
	return bal
}

// assertINV10 checks INV-10 on the whole database.
func (e *env) assertINV10(t *testing.T) {
	t.Helper()
	if n := e.count(t, "season_releases AS sr",
		"sr.revoked_at IS NULL AND EXISTS (SELECT 1 FROM spot_allocations AS sa WHERE sa.booking_line_id = sr.booking_line_id AND sa.date = sr.date AND sa.slot = sr.slot AND sa.kind = 'season')"); n != 0 {
		t.Fatalf("INV-10: %d active releases still have their season allocation", n)
	}
	if n := e.count(t, "season_releases AS sr",
		"sr.resold_booking_line_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM spot_allocations AS sa WHERE sa.booking_line_id = sr.resold_booking_line_id AND sa.date = sr.date AND sa.slot = sr.slot AND sa.kind = 'booking')"); n != 0 {
		t.Fatalf("INV-10: %d resold releases without the resale allocation", n)
	}
}

func findRelease(t *testing.T, rs []store.SeasonRelease, day string, slot calendar.Slot) store.SeasonRelease {
	t.Helper()
	for _, r := range rs {
		if r.Date.String() == day && r.Slot == slot {
			return r
		}
	}
	t.Fatalf("no release %s %s", day, slot)
	return store.SeasonRelease{}
}

func ptr[T any](v T) *T { return &v }
