// Package bookingtest builds a booking service on a real database for
// integration tests (internal/domain/booking, internal/http/admin): a lido
// with a 2027 season, a seat zone, a zone-mode zone, packs, an extra and a
// price list, plus small DB-backed adapters for the ports that belong to
// other packages (inventory, customers, payments). The inventory adapter
// relies on the same constraint as the real one (unique spot_id, date, slot)
// so concurrency tests exercise the database, not a mock.
package bookingtest

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// D parses a date (panics on malformed input: test data only).
var D = calendar.MustParse

// Prices of the fixture price list (cents per day).
const (
	BasePerDay   = 2000
	VIPPerDay    = 5000
	SunbedPerDay = 800
)

// Fixture is a ready lido. Now is 2027-06-01 10:00 Europe/Rome.
type Fixture struct {
	T       testing.TB
	Ctx     context.Context
	DB      *store.DB
	Clock   *clock.Fake
	Svc     *booking.Service
	Pricing *pricingconfig.Service
	Inv     *Inventory
	Pay     *Payments
	Ledger  *booking.Ledger

	Est, Other    *store.Establishment
	Season        *store.Season
	SeatZone      *store.Zone // seat mode, category "front"
	FreeZone      *store.Zone // zone mode, category "back"
	Spots         []*store.Spot
	FreeSpots     []*store.Spot
	Base, VIP     *store.Pack // VIP requires approval
	Sunbed        *store.Extra
	Owner         *auth.Principal
	Manager       *auth.Principal
	Cashier       *auth.Principal
	Staff         *auth.Principal // beach_staff
	Viewer        *auth.Principal
	Outsider      *auth.Principal // owner of the other organization
	OtherSpot     *store.Spot     // a spot of the other establishment
	OrgID, OrgBID string
}

// User returns a user principal with role on the whole organization.
func User(orgID string, role auth.Role) *auth.Principal {
	return &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: orgID, Role: role}}}
}

// New builds the fixture on db.
func New(t testing.TB, db *store.DB) *Fixture {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 6, 1, 8, 0, 0, 0, time.UTC))
	f := &Fixture{T: t, Ctx: ctx, DB: db, Clock: c}
	now := c.Now()
	org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-delfino", "Lido Delfino", now)
	if err != nil {
		t.Fatal(err)
	}
	est.Settings = store.JSON(`{"pricing":{"half_day_factor":"0.5","rounding":"none","default_vat_rate_bp":2200}}`)
	if _, err := db.NewUpdate().Model(est).WherePK().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	orgB, other, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-altro", "Lido Altro", now)
	if err != nil {
		t.Fatal(err)
	}
	f.Est, f.Other, f.OrgID, f.OrgBID = est, other, org.ID, orgB.ID
	f.Owner = User(org.ID, auth.RoleOwner)
	f.Manager = User(org.ID, auth.RoleManager)
	f.Cashier = User(org.ID, auth.RoleCashier)
	f.Staff = User(org.ID, auth.RoleBeachStaff)
	f.Viewer = User(org.ID, auth.RoleViewer)
	f.Outsider = User(orgB.ID, auth.RoleOwner)
	for _, p := range []*auth.Principal{f.Owner, f.Manager, f.Cashier, f.Staff, f.Viewer, f.Outsider} {
		// bookings.created_by_user_id references users.
		f.Insert(&store.User{ID: p.UserID, Email: p.UserID + "@example.org", Name: "Test", Locale: "it", Active: true, CreatedAt: now, UpdatedAt: now})
	}

	f.Season = &store.Season{
		ID: store.NewID(), EstablishmentID: est.ID, Year: 2027, OpensOn: D("2027-05-15"), ClosesOn: D("2027-09-15"),
		Status: "active", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.Insert(f.Season)
	mapID := f.addMap(est.ID, now)
	f.SeatZone = f.addZone(est.ID, mapID, "A", "seat", "front", now)
	f.FreeZone = f.addZone(est.ID, mapID, "B", "zone", "back", now)
	for i := 1; i <= 4; i++ {
		f.Spots = append(f.Spots, f.addSpot(est.ID, mapID, f.SeatZone.ID, fmt.Sprintf("A%d", i), i, now))
	}
	for i := 1; i <= 2; i++ {
		f.FreeSpots = append(f.FreeSpots, f.addSpot(est.ID, mapID, f.FreeZone.ID, fmt.Sprintf("B%d", i), i, now))
	}
	otherMap := f.addMap(other.ID, now)
	otherZone := f.addZone(other.ID, otherMap, "X", "seat", "front", now)
	f.OtherSpot = f.addSpot(other.ID, otherMap, otherZone.ID, "X1", 1, now)

	f.Base = f.addPack(est.ID, "base", false, now)
	f.VIP = f.addPack(est.ID, "vip", true, now)
	f.Sunbed = &store.Extra{
		ID: store.NewID(), EstablishmentID: est.ID, Code: "sunbed", Name: store.I18nText{"it": "Lettino"}, Description: store.I18nText{},
		PricingUnit: "per_day", MaxQuantity: 3, AttachTo: "line", Channels: []string{"desk", "online"}, Active: true,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.Insert(f.Sunbed)

	deps := pricingconfig.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)}
	f.Pricing = pricingconfig.NewService(deps)
	for _, in := range []pricingconfig.RuleInput{
		{Name: "base", TargetType: pricing.TargetPack, TargetIDs: []string{f.Base.ID}, Action: pricing.Action{Type: pricing.ActionSet, Amount: BasePerDay}, Stage: pricing.StageBase, Active: true},
		{Name: "vip", TargetType: pricing.TargetPack, TargetIDs: []string{f.VIP.ID}, Action: pricing.Action{Type: pricing.ActionSet, Amount: VIPPerDay}, Stage: pricing.StageBase, Active: true},
		{Name: "sunbed", TargetType: pricing.TargetExtra, Action: pricing.Action{Type: pricing.ActionSet, Amount: SunbedPerDay}, Stage: pricing.StageBase, Active: true},
	} {
		in.SeasonID = f.Season.ID
		if _, err := f.Pricing.CreateRule(ctx, f.Owner, est.ID, in); err != nil {
			t.Fatalf("create rule %s: %v", in.Name, err)
		}
	}
	f.Inv = &Inventory{Clock: c}
	f.Ledger = booking.NewLedger(booking.LedgerDeps{Clock: c})
	f.Pay = &Payments{Ledger: f.Ledger, Clock: c}
	f.Svc = booking.NewService(booking.Deps{
		DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c),
		Inventory: f.Inv, Pricer: f.Pricing, Customers: &Customers{Clock: c}, Payments: f.Pay,
	})
	return f
}

// Insert inserts a model or fails the test.
func (f *Fixture) Insert(m any) {
	f.T.Helper()
	if _, err := f.DB.NewInsert().Model(m).Exec(f.Ctx); err != nil {
		f.T.Fatalf("insert %T: %v", m, err)
	}
}

func (f *Fixture) addMap(estID string, now time.Time) string {
	m := &store.Map{ID: store.NewID(), EstablishmentID: estID, Name: store.I18nText{"it": "Spiaggia"}, Width: 1000, Height: 600, GridSize: 10, SeaSide: "bottom", Version: 1, CreatedAt: now, UpdatedAt: now}
	f.Insert(m)
	return m.ID
}

func (f *Fixture) addZone(estID, mapID, code, mode, category string, now time.Time) *store.Zone {
	z := &store.Zone{
		ID: store.NewID(), EstablishmentID: estID, MapID: &mapID, Code: code, Name: store.I18nText{"it": code},
		AssignmentMode: mode, OnlineVisible: true, PriceCategory: category, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.Insert(z)
	return z
}

func (f *Fixture) addSpot(estID, mapID, zoneID, label string, n int, now time.Time) *store.Spot {
	s := &store.Spot{
		ID: store.NewID(), EstablishmentID: estID, MapID: mapID, ZoneID: zoneID, Label: label, RowLabel: "1", Number: n,
		Kind: "umbrella", X: float64(10 * n), Y: 10, Shape: store.Shape{Type: "circle", W: 20, H: 20}, Capacity: map[string]int{}, MaxExtra: map[string]int{},
		Attributes: []string{}, Active: true, CreatedAt: now, UpdatedAt: now,
	}
	f.Insert(s)
	return s
}

func (f *Fixture) addPack(estID, code string, approval bool, now time.Time) *store.Pack {
	p := &store.Pack{
		ID: store.NewID(), EstablishmentID: estID, Code: code, Name: store.I18nText{"it": code}, Description: store.I18nText{},
		Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"day", "multi_day", "week", "half_day"},
		Channels: []string{"desk", "online"}, MaxPerBooking: 4, RequiresApproval: approval, Active: true,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.Insert(p)
	return p
}

// Input is a desk booking of pack on spot from..to (FULL, multi_day or day)
// for a new inline customer.
func (f *Fixture) Input(spot *store.Spot, from, to string) booking.CreateInput {
	formula := "multi_day"
	if from == to {
		formula = "day"
	}
	return booking.CreateInput{
		Customer: &customer.Input{FirstName: "Mario", LastName: "Rossi", Phone: "+393331234567"},
		StartsOn: D(from), EndsOn: D(to), Slots: calendar.SlotsFull, Formula: formula,
		Lines: []booking.LineInput{{PackID: f.Base.ID, SpotID: spot.ID}},
	}
}

// MustCreate creates a booking as the owner or fails.
func (f *Fixture) MustCreate(in booking.CreateInput) *booking.Detail {
	f.T.Helper()
	d, err := f.Svc.Create(f.Ctx, f.Owner, f.Est.ID, in)
	if err != nil {
		f.T.Fatalf("create booking: %v", err)
	}
	return d
}

// Pay records a payment of amount on the booking through the ledger.
func (f *Fixture) PayBooking(bookingID string, amount money.Cents) {
	f.T.Helper()
	es, err := store.SystemEstablishmentScope(f.Ctx, f.DB, f.Est.ID)
	if err != nil {
		f.T.Fatal(err)
	}
	if _, err := f.Ledger.ApplyPayment(f.Ctx, f.DB, es, bookingID, amount); err != nil {
		f.T.Fatal(err)
	}
}

// Count counts the rows of model in the fixture establishment matching where.
func (f *Fixture) Count(model any, where string, args ...any) int {
	f.T.Helper()
	q := f.DB.NewSelect().Model(model).Where("establishment_id = ?", f.Est.ID)
	if where != "" {
		q = q.Where(where, args...)
	}
	n, err := q.Count(f.Ctx)
	if err != nil {
		f.T.Fatal(err)
	}
	return int(n)
}

// ---- adapters -------------------------------------------------------------------

// Customers implements booking.Customers on the customers table.
type Customers struct{ Clock clock.Clock }

// Resolve returns the existing customer or inserts in.
func (c *Customers) Resolve(ctx context.Context, tx bun.IDB, _ *auth.Principal, es store.EstablishmentScope, id string, in *customer.Input) (*store.Customer, error) {
	if id != "" {
		cu, err := store.GetCustomer(ctx, tx, es, id)
		if errors.Is(err, store.ErrNotFound) {
			return nil, customer.ErrNotFound
		}
		return cu, err
	}
	now := c.Clock.Now()
	cu := &store.Customer{
		ID: store.NewID(), FirstName: in.FirstName, LastName: in.LastName, Locale: "it", Tags: []string{},
		SearchText: "rossi mario", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if in.Phone != "" {
		ph := in.Phone
		cu.Phone = &ph
	}
	if err := store.InsertCustomer(ctx, tx, es, cu); err != nil {
		return nil, err
	}
	return cu, nil
}

// Payments implements booking.Payments: refunds go through the ledger (the
// payment rows themselves belong to the payment service).
type Payments struct {
	Ledger *booking.Ledger
	Clock  clock.Clock
	mu     sync.Mutex
	// Refunds lists the refund requests recorded.
	Refunds []payment.RefundRequest
}

// RecordRefund applies the refund to paid_amount and remembers it.
func (p *Payments) RecordRefund(ctx context.Context, tx bun.IDB, _ *auth.Principal, es store.EstablishmentScope, req payment.RefundRequest) (*store.Payment, error) {
	if _, err := p.Ledger.ApplyPayment(ctx, tx, es, req.BookingID, -req.Amount); err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.Refunds = append(p.Refunds, req)
	p.mu.Unlock()
	id := req.BookingID
	return &store.Payment{ID: store.NewID(), BookingID: &id, Amount: req.Amount, Direction: store.PaymentDirectionRefund, Method: req.Method, Provider: "manual", Status: "succeeded", CreatedAt: p.Clock.Now()}, nil
}

// RefundCount returns how many refunds were recorded.
func (p *Payments) RefundCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.Refunds)
}

// ListByBooking returns no payments (the payment service owns them).
func (p *Payments) ListByBooking(context.Context, bun.IDB, store.EstablishmentScope, string) ([]store.Payment, error) {
	return []store.Payment{}, nil
}

// Inventory implements booking.Inventory with spot_allocations only (pools
// are out of scope here): rows are inserted in a savepoint so that a unique
// violation is turned into *availability.ConflictError listing the taken
// half-days; expired holds are reclaimed first (INV-05).
type Inventory struct{ Clock clock.Clock }

func (i *Inventory) rows(es store.EstablishmentScope, req inventory.AllocateRequest, now time.Time) []store.SpotAllocation {
	var out []store.SpotAllocation
	start := req.StartsOn
	for _, l := range req.Lines {
		if l.SpotID == "" {
			continue
		}
		for _, d := range calendar.Range(start, req.EndsOn) {
			for _, s := range req.Slots.Expand() {
				r := store.SpotAllocation{ID: store.NewID(), EstablishmentID: es.ID(), SpotID: l.SpotID, Date: d, Slot: s, Kind: string(req.Kind), CreatedAt: now}
				if req.BookingID != "" && l.LineID != "" {
					b, ln := req.BookingID, l.LineID
					r.BookingID, r.BookingLineID = &b, &ln
				}
				if req.Kind == inventory.KindHold {
					r.ExpiresAt = req.ExpiresAt
				}
				out = append(out, r)
			}
		}
	}
	return out
}

func (i *Inventory) conflicts(ctx context.Context, db bun.IDB, es store.EstablishmentScope, req inventory.AllocateRequest, now time.Time, ignoreBooking string) ([]availability.Conflict, error) {
	var out []availability.Conflict
	for _, l := range req.Lines {
		if l.SpotID == "" {
			continue
		}
		var taken []store.SpotAllocation
		q := db.NewSelect().Model(&taken).Where("spot_id = ?", l.SpotID).
			Where("date >= ?", req.StartsOn).Where("date <= ?", req.EndsOn).
			Where("slot IN (?)", bun.List(req.Slots.Expand())).
			Where("(expires_at IS NULL OR expires_at > ?)", now)
		if ignoreBooking != "" {
			q = q.Where("(booking_id IS NULL OR booking_id <> ?)", ignoreBooking)
		}
		if err := q.Scan(ctx); err != nil {
			return nil, err
		}
		for _, t := range taken {
			out = append(out, availability.Conflict{SpotID: t.SpotID, Date: t.Date, Slot: t.Slot, Reason: availability.ReasonSpotUnavailable})
		}
	}
	return out, nil
}

// Allocate inserts the spot rows of req; zone lines without a spot stay pending.
func (i *Inventory) Allocate(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req inventory.AllocateRequest) (*inventory.Allocation, error) {
	now := i.Clock.Now()
	if (req.Kind == inventory.KindHold) != (req.ExpiresAt != nil) {
		return nil, fmt.Errorf("bookingtest: expires_at iff hold")
	}
	rows := i.rows(es, req, now)
	out := &inventory.Allocation{}
	for _, l := range req.Lines {
		la := inventory.LineAllocation{LineID: l.LineID, SpotID: l.SpotID, AssignmentStatus: store.AssignmentAssigned}
		if l.SpotID == "" {
			la.AssignmentStatus = store.AssignmentPending
		} else if !contains(out.SpotIDs, l.SpotID) {
			out.SpotIDs = append(out.SpotIDs, l.SpotID)
		}
		out.Lines = append(out.Lines, la)
	}
	out.Dates = append(out.Dates, calendar.Range(req.StartsOn, req.EndsOn)...)
	if len(rows) == 0 {
		return out, nil
	}
	err := tx.RunInTx(ctx, nil, func(ctx context.Context, sp bun.Tx) error {
		if _, err := sp.NewDelete().Model((*store.SpotAllocation)(nil)).Where("establishment_id = ?", es.ID()).
			Where("kind = 'hold'").Where("expires_at <= ?", now).Exec(ctx); err != nil {
			return err
		}
		_, err := sp.NewInsert().Model(&rows).Exec(ctx)
		return err
	})
	if err != nil {
		if store.IsUniqueViolation(err) {
			cs, cerr := i.conflicts(ctx, tx, es, req, now, "")
			if cerr != nil {
				return nil, cerr
			}
			return nil, &availability.ConflictError{Err: availability.ErrSpotUnavailable, Conflicts: cs}
		}
		return nil, err
	}
	out.SpotRows = len(rows)
	return out, nil
}

// Check lists the conflicts of req without writing.
func (i *Inventory) Check(ctx context.Context, db bun.IDB, es store.EstablishmentScope, req inventory.AllocateRequest) ([]availability.Conflict, error) {
	return i.conflicts(ctx, db, es, req, i.Clock.Now(), req.BookingID)
}

// Release deletes the rows of the booking (lines, from a date).
func (i *Inventory) Release(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req inventory.ReleaseRequest) (*inventory.Allocation, error) {
	var gone []store.SpotAllocation
	q := tx.NewDelete().Model(&gone).Where("establishment_id = ?", es.ID()).Where("booking_id = ?", req.BookingID)
	if len(req.LineIDs) > 0 {
		q = q.Where("booking_line_id IN (?)", bun.List(req.LineIDs))
	}
	if req.From != nil {
		q = q.Where("date >= ?", *req.From)
	}
	if _, err := q.Returning("spot_id, date").Exec(ctx); err != nil {
		return nil, err
	}
	out := &inventory.Allocation{SpotRows: len(gone)}
	for _, g := range gone {
		if !contains(out.SpotIDs, g.SpotID) {
			out.SpotIDs = append(out.SpotIDs, g.SpotID)
		}
	}
	return out, nil
}

// Confirm turns the hold rows of the booking into booking rows.
func (i *Inventory) Confirm(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) error {
	now := i.Clock.Now()
	n, err := tx.NewSelect().Model((*store.SpotAllocation)(nil)).Where("establishment_id = ?", es.ID()).
		Where("booking_id = ?", bookingID).Where("kind = 'hold'").Where("expires_at <= ?", now).Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return availability.ErrHoldExpired
	}
	_, err = tx.NewUpdate().Model((*store.SpotAllocation)(nil)).Set("kind = 'booking'").Set("expires_at = NULL").
		Where("establishment_id = ?", es.ID()).Where("booking_id = ?", bookingID).Where("kind = 'hold'").Exec(ctx)
	return err
}

// ExtendHold moves the expiry of the hold rows.
func (i *Inventory) ExtendHold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, expiresAt time.Time) error {
	_, err := tx.NewUpdate().Model((*store.SpotAllocation)(nil)).Set("expires_at = ?", expiresAt).
		Where("establishment_id = ?", es.ID()).Where("booking_id = ?", bookingID).Where("kind = 'hold'").Exec(ctx)
	return err
}

// ConvertHold is not supported by the test adapter.
func (i *Inventory) ConvertHold(context.Context, bun.IDB, store.EstablishmentScope, inventory.ConvertHoldRequest) (*inventory.Allocation, error) {
	return nil, configerr.ErrNotImplemented
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

var (
	_ booking.Inventory = (*Inventory)(nil)
	_ booking.Customers = (*Customers)(nil)
	_ booking.Payments  = (*Payments)(nil)
)
