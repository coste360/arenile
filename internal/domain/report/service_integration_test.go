//go:build integration

package report_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/report"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var (
	t0   = time.Date(2027, 1, 10, 9, 0, 0, 0, time.UTC)
	rome = func() *time.Location { l, _ := time.LoadLocation("Europe/Rome"); return l }()
)

func dt(s string) calendar.Date { return calendar.MustParse(s) }

// ---- fixture ----------------------------------------------------------------------
//
// lido seeds a realistic season straight into the tables (no services, so
// the test is fast enough for a 300-spot season) and records every fact in
// an oracle computed independently of the report code: the expected values
// are plain sums over what was inserted.

type lido struct {
	t        testing.TB
	ctx      context.Context
	db       bun.IDB
	org      *store.Organization
	est      *store.Establishment
	es       store.EstablishmentScope
	zones    []*store.Zone
	spots    []*store.Spot
	pack     *store.Pack
	extra    *store.Extra
	custs    []*store.Customer
	defVAT   int64
	rows     rowsBuf
	oracle   []fact
	closures map[string]calendar.Slots // date -> closed slots (FULL or PM)
	seasons  []store.Season
}

// fact is one recorded event of the oracle.
type fact struct {
	kind     string // capacity | occupied | blocked | payment | booking | release
	date     calendar.Date
	units    int64
	amount   money.Cents
	status   string
	sale     calendar.Date // booking: created_at local date
	booked   *calendar.Date
	paid     money.Cents
	resold   bool
	isPass   bool
	signedIn bool
}

// rowsBuf accumulates rows to insert in bulk, in FK order.
type rowsBuf struct {
	bookings    []*store.Booking
	lines       []*store.BookingLine
	lineExtras  []*store.BookingLineExtra
	blocks      []*store.SpotBlock
	allocations []*store.SpotAllocation
	releases    []*store.SeasonRelease
	payments    []*store.Payment
}

func newLido(t testing.TB, db bun.IDB, slug string, spots, rowsN int) *lido {
	t.Helper()
	ctx := context.Background()
	org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org "+slug, slug, "Lido "+slug, t0)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := establishment.ParseSettings(est.Settings, est.Currency)
	l := &lido{t: t, ctx: ctx, db: db, org: org, est: est, es: es, defVAT: settings.Pricing.DefaultVATRateBP, closures: map[string]calendar.Slots{}}
	m := &store.Map{ID: store.NewID(), EstablishmentID: est.ID, Name: store.I18nText{"it": "Spiaggia"}, Width: 100, Height: 60, GridSize: 1, SeaSide: "top", Version: 1, CreatedAt: t0, UpdatedAt: t0}
	l.ins(m)
	for i, name := range []string{"Prima fila", "Centrale", "Palme"} {
		z := &store.Zone{
			ID: store.NewID(), EstablishmentID: est.ID, MapID: &m.ID, Code: fmt.Sprintf("Z%d", i+1), Name: store.I18nText{"it": name},
			AssignmentMode: "seat", PriceCategory: "std", SortOrder: i, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		l.ins(z)
		l.zones = append(l.zones, z)
	}
	perRow := spots / rowsN
	for i := range spots {
		row := min(i/perRow, rowsN-1)
		z := l.zones[min(row*len(l.zones)/rowsN, len(l.zones)-1)]
		sp := &store.Spot{
			ID: store.NewID(), EstablishmentID: est.ID, MapID: m.ID, ZoneID: z.ID, Label: fmt.Sprintf("%c%d", 'A'+row, i%perRow+1),
			RowLabel: string(rune('A' + row)), Number: i%perRow + 1, Kind: "umbrella", Shape: store.Shape{Type: "circle", W: 1, H: 1},
			Capacity: map[string]int{}, MaxExtra: map[string]int{}, Attributes: []string{}, Active: true, CreatedAt: t0, UpdatedAt: t0,
		}
		l.spots = append(l.spots, sp)
	}
	l.insAll(l.spots)
	// An inactive spot never counts, even with an allocation.
	off := &store.Spot{
		ID: store.NewID(), EstablishmentID: est.ID, MapID: m.ID, ZoneID: l.zones[0].ID, Label: "OFF", RowLabel: "A", Kind: "umbrella",
		Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{}, Attributes: []string{}, Active: false, CreatedAt: t0, UpdatedAt: t0,
	}
	l.ins(off)
	vat10 := int64(1000)
	l.pack = &store.Pack{
		ID: store.NewID(), EstablishmentID: est.ID, Code: "STD", Name: store.I18nText{"it": "Ombrellone e 2 lettini"}, Description: store.I18nText{},
		Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"day"}, Channels: []string{"desk", "online"}, MaxPerBooking: 1, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	l.extra = &store.Extra{
		ID: store.NewID(), EstablishmentID: est.ID, Code: "BAR", Name: store.I18nText{"it": "Consumazione bar"}, Description: store.I18nText{},
		PricingUnit: "per_booking", MaxQuantity: 5, AttachTo: "line", Channels: []string{"desk"}, Active: true, VATRateBP: &vat10, Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	l.ins(l.pack)
	l.ins(l.extra)
	for i := range 300 {
		c := &store.Customer{
			ID: store.NewID(), EstablishmentID: est.ID, FirstName: fmt.Sprintf("Nome%d", i), LastName: fmt.Sprintf("Cognome%d", i),
			Email: strp(fmt.Sprintf("cliente%d@example.test", i)), Locale: "it", Tags: []string{}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		if i == 0 {
			c.AnonymizedAt = &t0
		}
		l.custs = append(l.custs, c)
	}
	l.insAll(l.custs)
	return l
}

func strp(s string) *string { return &s }

func (l *lido) ins(m any) {
	l.t.Helper()
	if _, err := l.db.NewInsert().Model(m).Exec(l.ctx); err != nil {
		l.t.Fatalf("insert %T: %v", m, err)
	}
}

func insChunks[T any](l *lido, rows []*T) {
	l.t.Helper()
	for i := 0; i < len(rows); i += 400 {
		chunk := rows[i:min(i+400, len(rows))]
		if _, err := l.db.NewInsert().Model(&chunk).Exec(l.ctx); err != nil {
			l.t.Fatalf("insert %T: %v", chunk[0], err)
		}
	}
}

func (l *lido) insAll(rows any) {
	l.t.Helper()
	switch r := rows.(type) {
	case []*store.Spot:
		insChunks(l, r)
	case []*store.Customer:
		insChunks(l, r)
	default:
		l.t.Fatalf("insAll %T", rows)
	}
}

func (l *lido) flush() {
	l.t.Helper()
	insChunks(l, l.rows.bookings)
	insChunks(l, l.rows.lines)
	insChunks(l, l.rows.lineExtras)
	insChunks(l, l.rows.blocks)
	insChunks(l, l.rows.allocations)
	insChunks(l, l.rows.releases)
	insChunks(l, l.rows.payments)
	l.rows = rowsBuf{}
}

func (l *lido) season(year int, opens, closes string) {
	s := store.Season{ID: store.NewID(), EstablishmentID: l.est.ID, Year: year, OpensOn: dt(opens), ClosesOn: dt(closes), Status: "active", Version: 1, CreatedAt: t0, UpdatedAt: t0}
	l.ins(&s)
	l.seasons = append(l.seasons, s)
}

func (l *lido) closure(day string, slots calendar.Slots) {
	cd := &store.ClosureDay{ID: store.NewID(), EstablishmentID: l.est.ID, Date: dt(day), Slots: slots.Expand(), Reason: "Maltempo", Version: 1, CreatedAt: t0, UpdatedAt: t0}
	l.ins(cd)
	l.closures[day] = slots
}

// snapshot is a price snapshot with the VAT split of a pack at the default
// rate plus an extra at 10%.
func (l *lido) snapshot(pack, extra money.Cents) store.JSON {
	pt, ptax := money.VATSplit(pack, l.defVAT)
	doc := fmt.Sprintf(`{"engine_version":1,"currency":"EUR","total":%d,"vat":[{"rate":%s,"taxable":%d,"tax":%d}`,
		pack+extra, strconv.FormatFloat(float64(l.defVAT)/100, 'f', -1, 64), pt, ptax)
	if extra > 0 {
		et, etax := money.VATSplit(extra, 1000)
		doc += fmt.Sprintf(`,{"rate":10,"taxable":%d,"tax":%d}`, et, etax)
	}
	return store.JSON(doc + "]}")
}

type bookingSpec struct {
	spot      *store.Spot
	customer  *store.Customer
	channel   string
	status    string
	formula   string
	starts    calendar.Date
	ends      calendar.Date
	slots     calendar.Slots
	pack      money.Cents
	extra     money.Cents
	createdAt time.Time
	paidPart  int // percent paid at confirmation
	method    string
	allocKind string // booking | season | "" (none)
	allocDays []calendar.Date
	isPass    bool
}

// booking inserts a booking with its line, allocations and payments, and
// records the oracle facts.
func (l *lido) booking(s bookingSpec) *store.BookingLine {
	total := s.pack + s.extra
	b := &store.Booking{
		ID: store.NewID(), EstablishmentID: l.est.ID, Code: store.NewID()[24:], CustomerID: s.customer.ID, Channel: s.channel, Status: s.status,
		StartsOn: s.starts, EndsOn: s.ends, Slots: s.slots, Formula: s.formula, IsSeasonPass: s.isPass,
		PriceSnapshot: l.snapshot(s.pack, s.extra), TotalAmount: total, PolicySnapshot: store.JSON("{}"), Version: 1,
		CreatedAt: s.createdAt, UpdatedAt: s.createdAt,
	}
	confirmed := slices.Contains(store.ReportBookedStatuses, s.status) || s.status == "cancelled"
	if confirmed {
		c := s.createdAt.Add(10 * time.Minute)
		b.ConfirmedAt = &c
	}
	line := &store.BookingLine{
		ID: store.NewID(), EstablishmentID: l.est.ID, BookingID: b.ID, PackID: l.pack.ID, ZoneID: s.spot.ZoneID, SpotID: &s.spot.ID,
		AssignmentStatus: store.AssignmentAssigned, Quantity: 1, PriceCategory: "std", LineTotal: total, CreatedAt: s.createdAt, UpdatedAt: s.createdAt,
	}
	l.rows.bookings = append(l.rows.bookings, b)
	l.rows.lines = append(l.rows.lines, line)
	if s.extra > 0 {
		l.rows.lineExtras = append(l.rows.lineExtras, &store.BookingLineExtra{
			ID: store.NewID(), EstablishmentID: l.est.ID, BookingID: b.ID, BookingLineID: &line.ID, ExtraID: l.extra.ID, Quantity: 1, Total: s.extra,
			CreatedAt: s.createdAt, UpdatedAt: s.createdAt,
		})
	}
	if s.allocKind != "" {
		for _, day := range s.allocDays {
			for _, slot := range s.slots.Expand() {
				if l.closed(day, slot) {
					continue
				}
				l.rows.allocations = append(l.rows.allocations, &store.SpotAllocation{
					ID: store.NewID(), EstablishmentID: l.est.ID, SpotID: s.spot.ID, Date: day, Slot: slot, Kind: s.allocKind,
					BookingID: &b.ID, BookingLineID: &line.ID, CreatedAt: s.createdAt,
				})
				if l.open(day) {
					l.oracle = append(l.oracle, fact{kind: "occupied", date: day, units: 1})
				}
			}
		}
	}
	f := fact{kind: "booking", date: s.starts, amount: total, status: s.status, sale: calendar.In(s.createdAt, rome), isPass: s.isPass}
	if confirmed {
		bd := calendar.In(*b.ConfirmedAt, rome)
		f.booked = &bd
		paid := total * money.Cents(s.paidPart) / 100
		if paid > 0 {
			l.pay(b, paid, s.method, store.PaymentDirectionIn, *b.ConfirmedAt)
		}
		if s.status == "cancelled" && paid > 0 {
			refund := paid * 70 / 100
			l.pay(b, refund, s.method, store.PaymentDirectionRefund, b.ConfirmedAt.Add(48*time.Hour))
			paid -= refund
		}
		b.PaidAmount = paid
	}
	f.paid = b.PaidAmount
	l.oracle = append(l.oracle, f)
	return line
}

func (l *lido) pay(b *store.Booking, amount money.Cents, method, dir string, at time.Time) {
	pm := &store.Payment{
		ID: store.NewID(), EstablishmentID: l.est.ID, BookingID: &b.ID, CustomerID: &b.CustomerID, Provider: "manual", Method: method,
		Direction: dir, Amount: amount, Status: store.PaymentSucceeded, VATBreakdown: payment.SplitBySnapshot(b.PriceSnapshot, amount, l.defVAT),
		BusinessDate: calendar.In(at, rome), Metadata: map[string]any{}, CreatedAt: at, UpdatedAt: at,
	}
	l.rows.payments = append(l.rows.payments, pm)
	l.oracle = append(l.oracle, fact{kind: "payment", date: pm.BusinessDate, amount: amount, signedIn: dir == store.PaymentDirectionIn})
}

func (l *lido) open(day calendar.Date) bool {
	for _, s := range l.seasons {
		if !day.Before(s.OpensOn) && !day.After(s.ClosesOn) {
			return true
		}
	}
	return false
}

func (l *lido) closed(day calendar.Date, slot calendar.Slot) bool {
	c, ok := l.closures[day.String()]
	return ok && (c == calendar.SlotsFull || calendar.Slots(slot) == c)
}

// generate fills the season [from, to] of year: season passes on the first
// passPct% spots (with releases, half of them resold), daily bookings on
// the others, a few blocks, the capacity facts.
func (l *lido) generate(seed uint64, from, to string, passPct int) {
	r := rand.New(rand.NewPCG(seed, 7))
	days := calendar.Range(dt(from), dt(to))
	nPass := len(l.spots) * passPct / 100
	pick := func(xs []string) string { return xs[r.IntN(len(xs))] }
	for _, day := range days {
		for _, slot := range []calendar.Slot{calendar.AM, calendar.PM} {
			if l.open(day) && !l.closed(day, slot) {
				l.oracle = append(l.oracle, fact{kind: "capacity", date: day, units: int64(len(l.spots))})
			}
		}
	}
	// Season passes, confirmed before the season and paid by bank transfer.
	for i := range nPass {
		sp := l.spots[i]
		created := time.Date(dt(from).Year(), 3, 1+i%28, 10, 0, 0, 0, time.UTC)
		var allocDays, released []calendar.Date
		for _, day := range days {
			if r.IntN(100) < 5 {
				released = append(released, day)
			} else {
				allocDays = append(allocDays, day)
			}
		}
		line := l.booking(bookingSpec{
			spot: sp, customer: l.custs[1+i%(len(l.custs)-1)], channel: store.ChannelDesk, status: "confirmed", formula: "season",
			starts: dt(from), ends: dt(to), slots: calendar.SlotsFull, pack: 120000, createdAt: created, paidPart: 100,
			method: store.PaymentMethodBankTransfer, allocKind: store.AllocationSeason, allocDays: allocDays, isPass: true,
		})
		for _, day := range released {
			for _, slot := range []calendar.Slot{calendar.AM, calendar.PM} {
				if l.closed(day, slot) {
					continue
				}
				rel := &store.SeasonRelease{
					ID: store.NewID(), EstablishmentID: l.est.ID, BookingID: line.BookingID, BookingLineID: line.ID, SpotID: sp.ID, Date: day, Slot: slot,
					CreditAmount: 400, CreatedAt: created, UpdatedAt: created,
				}
				resold := r.IntN(2) == 0
				if resold {
					rl := l.booking(bookingSpec{
						spot: sp, customer: l.custs[1+r.IntN(len(l.custs)-1)], channel: store.ChannelOnline, status: "confirmed", formula: "half_day",
						starts: day, ends: day, slots: calendar.Slots(slot), pack: 1500, createdAt: day.Time().Add(-20 * time.Hour), paidPart: 100,
						method: store.PaymentMethodCard, allocKind: store.AllocationBooking, allocDays: []calendar.Date{day},
					})
					rel.ResoldBookingLineID = &rl.ID
				}
				l.rows.releases = append(l.rows.releases, rel)
				l.oracle = append(l.oracle, fact{kind: "release", date: day, units: 1, amount: 400, resold: resold})
			}
		}
	}
	// Daily bookings on the other spots.
	statuses := []string{"confirmed", "confirmed", "confirmed", "completed", "completed", "completed", "completed", "cancelled", "no_show", "pending_payment"}
	for _, day := range days {
		if l.closures[day.String()] == calendar.SlotsFull {
			continue
		}
		for i := nPass; i < len(l.spots); i++ {
			sp := l.spots[i]
			if i == nPass && day.Day()%10 == 3 { // blocked for maintenance
				blk := &store.SpotBlock{ID: store.NewID(), EstablishmentID: l.est.ID, SpotID: sp.ID, StartsOn: day, EndsOn: day, Slots: calendar.SlotsFull, Reason: "Manutenzione", CreatedAt: t0, UpdatedAt: t0}
				l.rows.blocks = append(l.rows.blocks, blk)
				for _, slot := range []calendar.Slot{calendar.AM, calendar.PM} {
					if l.closed(day, slot) {
						continue
					}
					l.rows.allocations = append(l.rows.allocations, &store.SpotAllocation{
						ID: store.NewID(), EstablishmentID: l.est.ID, SpotID: sp.ID, Date: day, Slot: slot, Kind: store.AllocationBlock, BlockID: &blk.ID, CreatedAt: t0,
					})
					l.oracle = append(l.oracle, fact{kind: "blocked", date: day, units: 1})
				}
				continue
			}
			if r.IntN(100) >= 55 {
				continue
			}
			slots := calendar.SlotsFull
			switch r.IntN(7) {
			case 0:
				slots = calendar.SlotsAM
			case 1:
				slots = calendar.SlotsPM
			}
			if l.closures[day.String()] == calendar.SlotsPM {
				slots = calendar.SlotsAM
			}
			price := money.Cents(2500)
			formula := "day"
			if slots != calendar.SlotsFull {
				price, formula = 1500, "half_day"
			}
			var extra money.Cents
			if r.IntN(10) < 3 {
				extra = money.Cents(500 + r.IntN(7)*111)
			}
			status := statuses[r.IntN(len(statuses))]
			lead := r.IntN(45)
			created := time.Date(day.Year(), day.Month(), day.Day(), 6+r.IntN(18), r.IntN(60), 0, 0, time.UTC).AddDate(0, 0, -lead)
			alloc := store.AllocationBooking
			if status == "cancelled" || status == "pending_payment" {
				alloc = "" // cancelled bookings release their units; pending ones hold them (not counted)
			}
			paidPart := 100
			if r.IntN(10) < 3 {
				paidPart = 30
			}
			l.booking(bookingSpec{
				spot: sp, customer: l.custs[r.IntN(len(l.custs))], channel: pick([]string{"desk", "desk", "desk", "online", "online", "api"}), status: status,
				formula: formula, starts: day, ends: day, slots: slots, pack: price, extra: extra, createdAt: created, paidPart: paidPart,
				method: pick([]string{store.PaymentMethodCash, store.PaymentMethodCard}), allocKind: alloc, allocDays: []calendar.Date{day},
			})
		}
	}
	// A counter sale without a booking (channel counter).
	at := time.Date(dt(from).Year(), dt(from).Month(), dt(from).Day(), 11, 0, 0, 0, time.UTC)
	l.rows.payments = append(l.rows.payments, &store.Payment{
		ID: store.NewID(), EstablishmentID: l.est.ID, Provider: "manual", Method: store.PaymentMethodCash, Direction: store.PaymentDirectionIn,
		Amount: 350, Status: store.PaymentSucceeded, VATBreakdown: payment.FlatSplit(350, 1000), BusinessDate: calendar.In(at, rome),
		Metadata: map[string]any{}, CreatedAt: at, UpdatedAt: at,
	})
	l.oracle = append(l.oracle, fact{kind: "payment", date: calendar.In(at, rome), amount: 350, signedIn: true})
	l.flush()
}

// expected is the oracle summary of a period.
type expected struct {
	capacity, occupied, released, resold int64
	collected, booked, value, open       money.Cents
	bookings, cancelled, noShows, passes int
	openN                                int
}

func (l *lido) expect(p report.Period) expected {
	var e expected
	for _, f := range l.oracle {
		in := p.Contains(f.date)
		switch f.kind {
		case "capacity":
			if in {
				e.capacity += f.units
			}
		case "blocked":
			if in {
				e.capacity -= f.units
			}
		case "occupied":
			if in {
				e.occupied += f.units
			}
		case "release":
			if in {
				e.released++
				if f.resold {
					e.resold++
				}
			}
		case "payment":
			if in {
				if f.signedIn {
					e.collected += f.amount
				} else {
					e.collected -= f.amount
				}
			}
		case "booking":
			booked := slices.Contains(store.ReportBookedStatuses, f.status)
			if f.booked != nil && booked && p.Contains(*f.booked) {
				e.booked += f.amount
				if f.isPass {
					e.passes++
				}
			}
			if p.Contains(f.sale) {
				e.bookings++
				e.value += f.amount
			}
			if in {
				switch f.status {
				case "cancelled":
					e.cancelled++
				case "no_show":
					e.noShows++
				}
				if booked && f.amount > f.paid {
					e.open += f.amount - f.paid
					e.openN++
				}
			}
		}
	}
	return e
}

func owner(l *lido) *auth.Principal {
	return &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: l.org.ID, Role: auth.RoleOwner}}}
}

// ---- tests --------------------------------------------------------------------------

// TestReportService_ReconcilesWithLedger seeds two seasons and checks every
// report against the oracle and the collected revenue against the
// corrispettivi export to the cent, on SQLite and PostgreSQL.
func TestReportService_ReconcilesWithLedger(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		l := newLido(t, db, "lido-rep", 30, 5)
		l.season(2026, "2026-06-01", "2026-06-20")
		l.season(2027, "2027-06-01", "2027-06-20")
		l.closure("2027-06-10", calendar.SlotsFull)
		l.closure("2027-06-11", calendar.SlotsPM)
		l.generate(2026, "2026-06-01", "2026-06-20", 20)
		l.generate(2027, "2027-06-01", "2027-06-20", 20)
		other := newLido(t, db, "lido-other", 5, 1)
		other.season(2027, "2027-06-01", "2027-06-20")
		other.generate(99, "2027-06-01", "2027-06-20", 0)

		svc := report.NewService(report.Deps{DB: db, Clock: clock.NewFake(t0)})
		p := owner(l)
		period := report.Period{From: dt("2027-05-01"), To: dt("2027-06-30")}
		want := l.expect(period)

		sum, err := svc.Summary(ctx, p, l.est.ID, period)
		if err != nil {
			t.Fatal(err)
		}
		if sum.CapacityUnits != want.capacity || sum.OccupiedUnits != want.occupied || sum.OccupancyBP != report.Ratio(want.occupied, want.capacity) {
			t.Fatalf("occupancy %d/%d, want %d/%d", sum.OccupiedUnits, sum.CapacityUnits, want.occupied, want.capacity)
		}
		if sum.RevenueCollected != want.collected || sum.RevenueBooked != want.booked || sum.Bookings != want.bookings {
			t.Fatalf("summary money %+v, want %+v", sum, want)
		}
		if sum.Cancellations != want.cancelled || sum.NoShows != want.noShows || sum.SeasonPassesSold != 0 {
			t.Fatalf("summary counts %+v, want %+v", sum, want)
		}
		if sum.OpenBalanceTotal != want.open || sum.OpenBalanceBookings != want.openN {
			t.Fatalf("open balances %d (%d), want %d (%d)", sum.OpenBalanceTotal, sum.OpenBalanceBookings, want.open, want.openN)
		}
		if want.capacity == 0 || want.occupied == 0 || want.collected == 0 || want.open == 0 || want.cancelled == 0 {
			t.Fatalf("fixture too small: %+v", want)
		}

		// Collected revenue vs the daily corrispettivi export (same ledger rows).
		ledger, err := store.ListSucceededPayments(ctx, db, l.es, period.From, period.To)
		if err != nil {
			t.Fatal(err)
		}
		corr := fiscal.BuildReport(ledger, period.From, period.To, fiscal.GroupByDay, "EUR", l.defVAT)
		for _, by := range []report.RevenueGroupBy{report.RevenueByDay, report.RevenueByWeek, report.RevenueByMonth, report.RevenueByVATRate, report.RevenueByPaymentMethod, report.RevenueByChannel} {
			rev, err := svc.Revenue(ctx, p, l.est.ID, report.RevenueQuery{Period: period, GroupBy: by})
			if err != nil {
				t.Fatal(err)
			}
			if rev.Totals.Gross != corr.GrandTotal || rev.Totals.Gross != want.collected {
				t.Fatalf("%s: gross %d, corrispettivi %d, oracle %d", by, rev.Totals.Gross, corr.GrandTotal, want.collected)
			}
			var tax money.Cents
			for _, rt := range corr.Totals {
				tax += rt.Tax
			}
			if by != report.RevenueByChannel && rev.Totals.Tax != tax {
				t.Fatalf("%s: tax %d, corrispettivi %d", by, rev.Totals.Tax, tax)
			}
			if by == report.RevenueByVATRate {
				for _, rt := range corr.Totals {
					i := slices.IndexFunc(rev.Rows, func(r report.RevenueRow) bool { return r.Key == strconv.FormatInt(rt.RateBP, 10) })
					if i < 0 || rev.Rows[i].Gross != rt.Gross || rev.Rows[i].Taxable != rt.Taxable || rev.Rows[i].Tax != rt.Tax {
						t.Fatalf("rate %d: %+v vs %+v", rt.RateBP, rev.Rows, rt)
					}
				}
			}
			if by == report.RevenueByChannel && !slices.ContainsFunc(rev.Rows, func(r report.RevenueRow) bool { return r.Key == report.ChannelCounter && r.Gross == 350 }) {
				t.Fatalf("counter sale missing: %+v", rev.Rows)
			}
		}

		// Booked revenue: by day equals the oracle, packs + extras equal it too
		// (no booking-level discounts in the fixture).
		booked, err := svc.Revenue(ctx, p, l.est.ID, report.RevenueQuery{Period: period, GroupBy: report.RevenueByChannel, Basis: report.BasisBooked})
		if err != nil {
			t.Fatal(err)
		}
		packs, err := svc.Revenue(ctx, p, l.est.ID, report.RevenueQuery{Period: period, GroupBy: report.RevenueByPack, Basis: report.BasisBooked})
		if err != nil {
			t.Fatal(err)
		}
		extras, err := svc.Revenue(ctx, p, l.est.ID, report.RevenueQuery{Period: period, GroupBy: report.RevenueByExtra, Basis: report.BasisBooked})
		if err != nil {
			t.Fatal(err)
		}
		if booked.Totals.Gross != want.booked || packs.Totals.Gross+extras.Totals.Gross != want.booked || len(extras.Rows) != 1 || extras.Rows[0].Key != "BAR" {
			t.Fatalf("booked %d, packs %d + extras %d, want %d", booked.Totals.Gross, packs.Totals.Gross, extras.Totals.Gross, want.booked)
		}
		if _, err := svc.Revenue(ctx, p, l.est.ID, report.RevenueQuery{Period: period, GroupBy: report.RevenueByPack}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("pack on collected: %v", err)
		}

		// Occupancy groupings agree with each other.
		var occTotals []report.OccupancyRow
		for _, by := range []report.OccupancyGroupBy{report.OccupancyByDay, report.OccupancyByZone, report.OccupancyByRow, report.OccupancyBySlot} {
			o, err := svc.Occupancy(ctx, p, l.est.ID, report.OccupancyQuery{Period: period, GroupBy: by})
			if err != nil {
				t.Fatal(err)
			}
			occTotals = append(occTotals, o.Totals)
		}
		for _, tot := range occTotals[1:] {
			if tot != occTotals[0] || tot.CapacityUnits != want.capacity {
				t.Fatalf("occupancy totals differ: %+v vs %+v", tot, occTotals[0])
			}
		}
		zone, err := svc.Occupancy(ctx, p, l.est.ID, report.OccupancyQuery{Period: period, GroupBy: report.OccupancyByRow, ZoneID: l.zones[0].ID})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range zone.Rows {
			if r.ZoneID != l.zones[0].ID {
				t.Fatalf("zone filter leaked: %+v", r)
			}
		}

		bk, err := svc.Bookings(ctx, p, l.est.ID, period, report.BookingsByLeadTime)
		if err != nil {
			t.Fatal(err)
		}
		if bk.Totals.Bookings != want.bookings || bk.Totals.Value != want.value {
			t.Fatalf("bookings %d/%d, want %d/%d", bk.Totals.Bookings, bk.Totals.Value, want.bookings, want.value)
		}
		cn, err := svc.Cancellations(ctx, p, l.est.ID, period, report.CancellationsByChannel)
		if err != nil {
			t.Fatal(err)
		}
		if cn.Totals.Cancelled != want.cancelled || cn.Totals.NoShows != want.noShows || cn.Totals.Refunded == 0 || cn.Totals.LostValue == 0 {
			t.Fatalf("cancellations %+v, want %+v", cn.Totals, want)
		}
		sp, err := svc.SeasonPasses(ctx, p, l.est.ID, period, report.SeasonPassesByZone)
		if err != nil {
			t.Fatal(err)
		}
		if sp.Totals.ReleasedUnits != want.released || sp.Totals.ResoldUnits != want.resold || sp.Totals.CreditsIssued != money.Cents(400*want.released) {
			t.Fatalf("releases %+v, want %d/%d", sp.Totals, want.released, want.resold)
		}
		if sp.Totals.ResoldUnits > 0 && sp.Totals.ResaleRevenue != money.Cents(1500*sp.Totals.ResoldUnits) {
			t.Fatalf("resale revenue %d for %d units", sp.Totals.ResaleRevenue, sp.Totals.ResoldUnits)
		}
		passes, err := svc.SeasonPasses(ctx, p, l.est.ID, report.Period{From: dt("2027-03-01"), To: dt("2027-03-31")}, "")
		if err != nil {
			t.Fatal(err)
		}
		if passes.PassesSold != 6 || passes.PassesValue != 6*120000 {
			t.Fatalf("passes sold %d (%d)", passes.PassesSold, passes.PassesValue)
		}

		top, err := svc.TopCustomers(ctx, p, l.est.ID, period, report.SortByRevenue, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(top.Rows) != 5 || top.Rows[0].Revenue < top.Rows[4].Revenue || top.Rows[0].DisplayName == "" || top.Rows[0].SpotDays == 0 {
			t.Fatalf("top = %+v", top.Rows)
		}
		ob, err := svc.OpenBalances(ctx, p, l.est.ID, period)
		if err != nil {
			t.Fatal(err)
		}
		if ob.TotalBalance != want.open || len(ob.Rows) != want.openN {
			t.Fatalf("open balances %d", ob.TotalBalance)
		}

		// Year comparison: both spans measured like the summary.
		cur := report.Period{From: dt("2027-06-01"), To: dt("2027-06-20")}
		cmp, err := svc.Comparison(ctx, p, l.est.ID, report.ComparisonQuery{Period: cur, Granularity: report.GranularityWeek})
		if err != nil {
			t.Fatal(err)
		}
		wc, wp := l.expect(cur), l.expect(report.PreviousSpan(cur, 0))
		got := map[string]report.ComparisonMetric{}
		for _, m := range cmp.Metrics {
			got[m.Metric] = m
		}
		if got[report.MetricRevenueCollected].Current != int64(wc.collected) || got[report.MetricRevenueCollected].Previous != int64(wp.collected) ||
			got[report.MetricOccupancyBP].Current != report.Ratio(wc.occupied, wc.capacity) || got[report.MetricOccupancyBP].Previous != report.Ratio(wp.occupied, wp.capacity) ||
			got[report.MetricBookings].Previous != int64(wp.bookings) || got[report.MetricReleasedUnits].Previous != wp.released {
			t.Fatalf("comparison %+v, want cur %+v prev %+v", cmp.Metrics, wc, wp)
		}
		var seriesRev money.Cents
		for _, pt := range cmp.Series {
			seriesRev += pt.PreviousRevenue
		}
		if seriesRev != wp.collected || cmp.Previous.From.String() != "2026-06-01" {
			t.Fatalf("series previous revenue %d, want %d", seriesRev, wp.collected)
		}
		wk, err := svc.Comparison(ctx, p, l.est.ID, report.ComparisonQuery{Period: cur, Align: report.AlignWeekday, Granularity: report.GranularityDay})
		if err != nil {
			t.Fatal(err)
		}
		if wk.Previous.From.String() != "2026-06-02" || wk.Align != report.AlignWeekday || len(wk.Series) != 20 {
			t.Fatalf("weekday comparison %s %s %d", wk.Previous.From, wk.Align, len(wk.Series))
		}

		// Scoping and permissions.
		otherOwner := owner(other)
		if _, err := svc.Summary(ctx, otherOwner, l.est.ID, period); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("cross tenant: %v", err)
		}
		viewer := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: l.org.ID, EstablishmentID: l.est.ID, Role: auth.RoleViewer}}}
		if _, err := svc.OpenBalances(ctx, viewer, l.est.ID, period); err != nil {
			t.Fatalf("viewer: %v", err)
		}
		cashier := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: l.org.ID, EstablishmentID: l.est.ID, Role: auth.RoleCashier}}}
		if _, err := svc.Summary(ctx, cashier, l.est.ID, period); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier: %v", err)
		}
		if _, err := svc.Summary(ctx, p, l.est.ID, report.Period{}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("zero period: %v", err)
		}
		os, err := svc.Summary(ctx, otherOwner, other.est.ID, period)
		if err != nil {
			t.Fatal(err)
		}
		if ow := other.expect(period); os.RevenueCollected != ow.collected || os.OccupiedUnits != ow.occupied {
			t.Fatalf("other establishment mixed data: %+v vs %+v", os, ow)
		}
	})
}

// TestReportService_SeasonPerformance seeds a 300-spot lido for a whole
// season (about 20k bookings and 60k allocations) and checks that the
// heaviest reports stay under budget and still reconcile.
func TestReportService_SeasonPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("season fixture")
	}
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		l := newLido(t, db, "lido-big", 300, 10)
		l.season(2026, "2026-06-01", "2026-09-15")
		l.season(2027, "2027-06-01", "2027-09-15")
		l.closure("2027-07-20", calendar.SlotsFull)
		l.generate(2026, "2026-06-01", "2026-09-15", 20)
		l.generate(2027, "2027-06-01", "2027-09-15", 20)
		svc := report.NewService(report.Deps{DB: db, Clock: clock.NewFake(t0)})
		p := owner(l)
		season := report.Period{From: dt("2027-06-01"), To: dt("2027-09-15")}

		const budget = 1500 * time.Millisecond // per report; measured about 0.3s on SQLite, 0.15s on PostgreSQL
		timed := func(name string, fn func() error) {
			t.Helper()
			start := time.Now()
			if err := fn(); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			el := time.Since(start)
			t.Logf("%s: %s", name, el)
			if el > budget {
				t.Fatalf("%s took %s (budget %s)", name, el, budget)
			}
		}
		var sum *report.Summary
		timed("summary", func() (err error) { sum, err = svc.Summary(ctx, p, l.est.ID, season); return err })
		timed("comparison", func() error {
			_, err := svc.Comparison(ctx, p, l.est.ID, report.ComparisonQuery{Period: season, Granularity: report.GranularityWeek})
			return err
		})
		timed("occupancy by row", func() error {
			_, err := svc.Occupancy(ctx, p, l.est.ID, report.OccupancyQuery{Period: season, GroupBy: report.OccupancyByRow})
			return err
		})
		timed("revenue booked by vat", func() error {
			_, err := svc.Revenue(ctx, p, l.est.ID, report.RevenueQuery{Period: season, GroupBy: report.RevenueByVATRate, Basis: report.BasisBooked})
			return err
		})
		timed("top customers", func() error {
			_, err := svc.TopCustomers(ctx, p, l.est.ID, season, report.SortByRevenue, 100)
			return err
		})
		want := l.expect(season)
		if sum.RevenueCollected != want.collected || sum.OccupiedUnits != want.occupied || sum.CapacityUnits != want.capacity || sum.Bookings != want.bookings {
			t.Fatalf("season summary %+v, want %+v", sum, want)
		}
		if want.bookings < 10000 {
			t.Fatalf("fixture smaller than a season: %d bookings", want.bookings)
		}
	})
}
