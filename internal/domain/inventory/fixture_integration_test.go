//go:build integration

package inventory_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
)

// fixture is an establishment with a season, a closure, a seat zone (row1,
// spots A1..A5, online quota 40%), a zone-mode zone (free, spots Z1..Z3 and
// its implicit pool of 3), a quantity zone (pool area, pool of 4 with an
// override to 1 on 2027-08-15) and a tracked component pool (sunbeds, 10).
type fixture struct {
	db       *store.DB
	clock    *clock.Fake
	svc      *inventory.Service
	est      *store.Establishment
	estB     *store.Establishment
	es       store.EstablishmentScope
	owner    *auth.Principal
	cashier  *auth.Principal
	viewer   *auth.Principal
	ownerB   *auth.Principal
	mapID    string
	seat     *store.Zone
	zoneMode *store.Zone
	qtyZone  *store.Zone
	spots    map[string]*store.Spot // by label
	zonePool *store.Pool
	qtyPool  *store.Pool
	sunbeds  *store.Pool
	pack     *store.Pack
	customer *store.Customer
}

var t0 = time.Date(2027, 7, 1, 8, 0, 0, 0, time.UTC)

func d(s string) calendar.Date { return calendar.MustParse(s) }

func ptr[T any](v T) *T { return &v }

// tb is the subset of testing.TB also implemented by *rapid.T.
type tb interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}

func must(t tb, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newFixture(t testing.TB, db *store.DB) *fixture {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(t0)
	orgA, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-inv", "Lido Inv", t0)
	must(t, err)
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-inv-b", "Lido B", t0)
	must(t, err)
	es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
	must(t, err)
	user := func(name, org string, role auth.Role) *auth.Principal {
		u := &store.User{ID: store.NewID(), Email: name + "@example.it", PasswordHash: "x", Name: name, Locale: "it", Active: true, CreatedAt: t0, UpdatedAt: t0}
		must(t, store.InsertUser(ctx, db, u))
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: u.ID, Grants: []auth.Grant{{OrganizationID: org, Role: role}}}
	}
	f := &fixture{
		db: db, clock: c, est: est, estB: estB, es: es, spots: map[string]*store.Spot{},
		svc:     inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)}),
		owner:   user("owner", orgA.ID, auth.RoleOwner),
		cashier: user("cashier", orgA.ID, auth.RoleCashier),
		viewer:  user("viewer", orgA.ID, auth.RoleViewer),
		ownerB:  user("ownerb", orgB.ID, auth.RoleOwner),
	}
	must(t, store.InsertSeason(ctx, db, es, &store.Season{ID: store.NewID(), Year: 2027, OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-30"), Status: "active", Version: 1, CreatedAt: t0, UpdatedAt: t0}))
	must(t, store.InsertClosureDay(ctx, db, es, &store.ClosureDay{ID: store.NewID(), Date: d("2027-07-20"), Slots: []calendar.Slot{calendar.PM}, Reason: "temporale", Version: 1, CreatedAt: t0, UpdatedAt: t0}))
	m := &store.Map{ID: store.NewID(), Name: store.I18nText{"it": "Spiaggia"}, Width: 100, Height: 60, GridSize: 1, SeaSide: "bottom", Version: 1, CreatedAt: t0, UpdatedAt: t0}
	must(t, store.InsertMap(ctx, db, es, m))
	f.mapID = m.ID
	zone := func(code, mode string, quota *store.OnlineQuota, sort int) *store.Zone {
		z := &store.Zone{
			ID: store.NewID(), MapID: &m.ID, Code: code, Name: store.I18nText{"it": code}, AssignmentMode: mode, OnlineVisible: true,
			OnlineQuota: quota, PriceCategory: code, SortOrder: sort, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		must(t, store.InsertZone(ctx, db, es, z))
		return z
	}
	f.seat = zone("row1", "seat", &store.OnlineQuota{Type: "percent", Value: 40}, 1)
	f.zoneMode = zone("free", "zone", nil, 2)
	f.qtyZone = zone("area", "quantity", &store.OnlineQuota{Type: "count", Value: 2}, 3)
	var spots []store.Spot
	addSpot := func(label, row string, n int, z *store.Zone) {
		spots = append(spots, store.Spot{
			ID: store.NewID(), MapID: m.ID, ZoneID: z.ID, Label: label, RowLabel: row, Number: n, Kind: "umbrella",
			X: float64(n), Y: 1, Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{},
			Attributes: []string{}, Active: true, CreatedAt: t0, UpdatedAt: t0,
		})
	}
	for i := 1; i <= 5; i++ {
		addSpot(fmt.Sprintf("A%d", i), "A", i, f.seat)
	}
	for i := 1; i <= 3; i++ {
		addSpot(fmt.Sprintf("Z%d", i), "Z", i, f.zoneMode)
	}
	must(t, store.InsertSpots(ctx, db, es, spots))
	for i := range spots {
		f.spots[spots[i].Label] = &spots[i]
	}
	pool := func(code string, zoneID *string, capacity int, system bool, overrides []store.CapacityOverride) *store.Pool {
		p := &store.Pool{
			ID: store.NewID(), ZoneID: zoneID, ComponentCode: code, Name: store.I18nText{"it": code}, Capacity: capacity,
			CapacityOverrides: overrides, SystemManaged: system, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		must(t, store.InsertPool(ctx, db, es, p))
		return p
	}
	f.zonePool = pool("spot", &f.zoneMode.ID, 3, true, nil)
	f.qtyPool = pool("pool_area", &f.qtyZone.ID, 4, false, []store.CapacityOverride{{From: d("2027-08-15"), To: d("2027-08-15"), Capacity: 1}})
	f.sunbeds = pool("sunbed", nil, 10, false, nil)
	must(t, store.InsertComponent(ctx, db, es, &store.Component{ID: store.NewID(), EstablishmentID: est.ID, Code: "sunbed", Name: store.I18nText{"it": "Lettino"}, Tracked: true, PoolID: &f.sunbeds.ID, Version: 1, CreatedAt: t0, UpdatedAt: t0}))
	f.pack = &store.Pack{
		ID: store.NewID(), EstablishmentID: est.ID, Code: "base", Name: store.I18nText{"it": "Base"}, Description: store.I18nText{}, Composition: map[string]int{"umbrella": 1, "sunbed": 2},
		AllowedFormulas: []string{"day"}, Channels: []string{"desk", "online"}, MaxPerBooking: 5, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	must(t, store.InsertPack(ctx, db, es, f.pack))
	f.customer = &store.Customer{ID: store.NewID(), EstablishmentID: est.ID, FirstName: "Mario", LastName: "Rossi", Locale: "it", Tags: []string{"vip"}, SearchText: "rossi mario", Version: 1, CreatedAt: t0, UpdatedAt: t0}
	_, err = db.NewInsert().Model(f.customer).Exec(ctx)
	must(t, err)
	return f
}

// bookingSpec describes a booking row with its lines, inserted directly
// (the booking service is another package's concern).
type bookingSpec struct {
	status   string
	channel  string
	from, to string
	slots    calendar.Slots
	lines    []lineSpec
	season   bool
	expires  *time.Time
	byDesk   bool // a panel operation on a booking of any channel (INV-105)
}

type lineSpec struct {
	zone   *store.Zone
	spot   *store.Spot
	status string
	qty    int
}

type testBooking struct {
	id      string
	lineIDs []string
}

func (f *fixture) insertBooking(t testing.TB, db bun.IDB, b bookingSpec) testBooking {
	t.Helper()
	ctx := context.Background()
	if b.status == "" {
		b.status = "confirmed"
	}
	if b.channel == "" {
		b.channel = store.ChannelDesk
	}
	if b.slots == "" {
		b.slots = calendar.SlotsFull
	}
	formula := "day"
	if b.season {
		formula = "season"
	}
	bk := &store.Booking{
		ID: store.NewID(), EstablishmentID: f.est.ID, Code: "T-" + store.NewID(), CustomerID: f.customer.ID, Channel: b.channel,
		Status: b.status, StartsOn: d(b.from), EndsOn: d(b.to), Slots: b.slots, Formula: formula, IsSeasonPass: b.season,
		PriceSnapshot: store.JSON(`{}`), TotalAmount: 5000, PaidAmount: 1000, PolicySnapshot: store.JSON(`{"rules":{}}`),
		HoldExpiresAt: b.expires, Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	_, err := db.NewInsert().Model(bk).Exec(ctx)
	must(t, err)
	out := testBooking{id: bk.ID}
	for i, l := range b.lines {
		st := l.status
		if st == "" {
			switch {
			case l.spot != nil:
				st = store.AssignmentAssigned
			case l.zone.AssignmentMode == "zone":
				st = store.AssignmentPending
			default:
				st = store.AssignmentNotApplicable
			}
		}
		bl := &store.BookingLine{
			ID: store.NewID(), EstablishmentID: f.est.ID, BookingID: bk.ID, PackID: f.pack.ID, ZoneID: l.zone.ID,
			AssignmentStatus: st, Quantity: max(l.qty, 1), PriceCategory: l.zone.Code, LineTotal: 5000, SortOrder: i, CreatedAt: t0, UpdatedAt: t0,
		}
		if l.spot != nil && st == store.AssignmentAssigned {
			bl.SpotID = &l.spot.ID
		}
		_, err := db.NewInsert().Model(bl).Exec(ctx)
		must(t, err)
		out.lineIDs = append(out.lineIDs, bl.ID)
	}
	return out
}

// book inserts a booking and allocates it in one transaction, like the
// booking service does.
func (f *fixture) book(t testing.TB, b bookingSpec, components map[string]int) (testBooking, error) {
	t.Helper()
	var tb testBooking
	err := store.RunInTx(context.Background(), f.db, func(ctx context.Context, tx bun.Tx) error {
		tb = f.insertBooking(t, tx, b)
		_, err := f.svc.Allocate(ctx, tx, f.es, f.allocRequest(b, tb, components))
		return err
	})
	return tb, err
}

func (f *fixture) allocRequest(b bookingSpec, tb testBooking, components map[string]int) inventory.AllocateRequest {
	kind := inventory.KindBooking
	switch {
	case b.expires != nil:
		kind = inventory.KindHold
	case b.season:
		kind = inventory.KindSeason
	}
	if b.channel == "" {
		b.channel = store.ChannelDesk
	}
	if b.slots == "" {
		b.slots = calendar.SlotsFull
	}
	req := inventory.AllocateRequest{BookingID: tb.id, Channel: b.channel, StartsOn: d(b.from), EndsOn: d(b.to), Slots: b.slots, Kind: kind, ExpiresAt: b.expires, ByDesk: b.byDesk}
	for i, l := range b.lines {
		lr := inventory.LineRequest{LineID: tb.lineIDs[i], ZoneID: l.zone.ID, Quantity: l.qty, Components: components}
		if l.spot != nil {
			lr.SpotID = l.spot.ID
		}
		req.Lines = append(req.Lines, lr)
	}
	return req
}

func (f *fixture) count(t testing.TB, table, where string, args ...any) int {
	t.Helper()
	n, err := f.db.NewSelect().TableExpr(table).Where(where, args...).Count(context.Background())
	must(t, err)
	return int(n)
}

// usage returns used, held, online_used of a counter (zeros if absent).
func (f *fixture) usage(t tb, poolID, date string, slot calendar.Slot) (int, int, int) {
	t.Helper()
	u, err := store.GetPoolUsage(context.Background(), f.db, f.es, store.PoolKey{PoolID: poolID, Date: d(date), Slot: slot})
	must(t, err)
	return u.Used, u.Held, u.OnlineUsed
}

// checkInvariants verifies INV-01..03 and INV-06 on the whole database.
func (f *fixture) checkInvariants(t testing.TB) {
	t.Helper()
	ctx := context.Background()
	// INV-01 (also guaranteed by the index): never two rows per half-day.
	var dup int
	must(t, f.db.NewSelect().TableExpr("(SELECT spot_id, date, slot, COUNT(*) AS n FROM spot_allocations GROUP BY spot_id, date, slot HAVING COUNT(*) > 1) AS x").ColumnExpr("COUNT(*)").Scan(ctx, &dup))
	if dup != 0 {
		t.Fatalf("INV-01: %d duplicated half-days", dup)
	}
	// INV-02 / INV-03: counters match the detail and stay within capacity.
	var usages []store.PoolUsage
	must(t, f.db.NewSelect().Model(&usages).Scan(ctx))
	pools, err := store.InventoryPools(ctx, f.db, f.es, nil)
	must(t, err)
	byID := map[string]store.Pool{}
	for _, p := range pools {
		byID[p.ID] = p
	}
	for _, u := range usages {
		var sums struct {
			Used   int `bun:"used"`
			Held   int `bun:"held"`
			Online int `bun:"online"`
		}
		must(t, f.db.NewSelect().TableExpr("pool_allocations").
			ColumnExpr("COALESCE(SUM(CASE WHEN kind = 'booking' THEN quantity ELSE 0 END), 0) AS used").
			ColumnExpr("COALESCE(SUM(CASE WHEN kind = 'hold' THEN quantity ELSE 0 END), 0) AS held").
			ColumnExpr("COALESCE(SUM(CASE WHEN channel <> 'desk' THEN quantity ELSE 0 END), 0) AS online").
			Where("pool_id = ? AND date = ? AND slot = ?", u.PoolID, u.Date, u.Slot).Scan(ctx, &sums))
		if sums.Used != u.Used || sums.Held != u.Held || sums.Online != u.OnlineUsed {
			t.Fatalf("INV-03 %s %s %s: counter %d/%d/%d detail %d/%d/%d", u.PoolID, u.Date, u.Slot, u.Used, u.Held, u.OnlineUsed, sums.Used, sums.Held, sums.Online)
		}
		p := byID[u.PoolID]
		capacity := p.Capacity
		for _, o := range p.CapacityOverrides {
			if !u.Date.Before(o.From) && !u.Date.After(o.To) {
				capacity = o.Capacity
				break
			}
		}
		if u.Used < 0 || u.Held < 0 || u.Used+u.Held > capacity {
			t.Fatalf("INV-02 %s %s %s: used %d held %d capacity %d", u.PoolID, u.Date, u.Slot, u.Used, u.Held, capacity)
		}
	}
	// INV-06
	if n := f.count(t, "spot_allocations", "(kind = 'hold') <> (expires_at IS NOT NULL)"); n != 0 {
		t.Fatalf("INV-06: %d rows", n)
	}
}
