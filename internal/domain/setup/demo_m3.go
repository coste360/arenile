package setup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// DemoVATRateBP is the VAT rate of the demo establishment (22%, the Italian
// standard rate for beach services). Real establishments set their own rate
// (spec 8.5): it is never fixed in code outside the demo.
const DemoVATRateBP int64 = 2200

// Demo volumes (spec 27.10, milestone M3).
const (
	demoCustomers      = 40
	demoSeasonPasses   = 25
	demoBookings       = 70
	demoReleasedPasses = 5
	demoHorizonDays    = 30
	demoHoldTTL        = 2 * time.Hour
)

// DemoCustomers resolves or creates customers in the caller's transaction
// (implemented by customer.Service).
type DemoCustomers interface {
	Resolve(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, customerID string, in *customer.Input) (*store.Customer, error)
}

// DemoBookings creates bookings and season passes in the caller's
// transaction with allocations, audit and outbox (implemented by
// booking.Service).
type DemoBookings interface {
	CreateInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in booking.CreateInput) (*booking.Detail, error)
}

// DemoPayment is a manual payment recorded by the demo.
type DemoPayment struct {
	BookingID     string
	Amount        money.Cents
	Method        string // store.PaymentMethod*
	CashSessionID string
}

// DemoOperations are the M3 use cases the demo runs, as ports: the demo
// never writes bookings, payments or cash rows itself, so every invariant
// (allocations, audit, outbox, ledger, cash totals) holds exactly as for
// panel operations. Customers and Bookings are required; the function
// fields adapt the public use cases of the pass, payment, cash and booking
// services (whatever their method names) and may be nil: the phase is then
// skipped and listed in DemoOperationsResult.Skipped.
type DemoOperations struct {
	// Actor is the principal of every operation (the demo owner).
	Actor     *auth.Principal
	Customers DemoCustomers
	Bookings  DemoBookings

	ReleaseDays   func(ctx context.Context, p *auth.Principal, establishmentID, passID string, dates []calendar.Date) error
	OpenCash      func(ctx context.Context, p *auth.Principal, establishmentID string, openingFloat money.Cents) (sessionID string, err error)
	RecordPayment func(ctx context.Context, p *auth.Principal, establishmentID string, pay DemoPayment) error
	CloseCash     func(ctx context.Context, p *auth.Principal, establishmentID, sessionID string, counted store.MethodTotals) error
	CancelBooking func(ctx context.Context, p *auth.Principal, establishmentID, bookingID, reason string) error
	CheckIn       func(ctx context.Context, p *auth.Principal, establishmentID, bookingID string) error
}

// DemoOperationsResult reports what SeedDemoOperations did.
type DemoOperationsResult struct {
	// Applied is false when the establishment already had customers.
	Applied      bool
	VATUpdated   int
	Customers    int
	SeasonPasses int
	Bookings     int
	Releases     int
	Resales      int
	Payments     int
	CashSessions int
	Cancelled    int
	CheckedIn    int
	// Conflicts counts bookings skipped because the resource was taken.
	Conflicts int
	// Skipped lists the phases not run (missing port or not implemented).
	Skipped []string
}

// demoWorld is what the demo reads once: catalog, spots, season, dates.
type demoWorld struct {
	es          store.EstablishmentScope
	season      *store.Season
	from, to    calendar.Date // booking horizon, inside the season
	today       calendar.Date
	base        *store.Pack
	extraSunbed *store.Extra
	passSpots   []store.Spot // rows A and B
	dailySpots  []store.Spot // rows C..H
	occupied    map[string]map[calendar.Date]bool
}

type demoBooking struct {
	id     string
	total  money.Cents
	status booking.Status
	starts calendar.Date
}

// SeedDemoOperations adds the M3 demo data to the configured demo
// establishment (run after SeedDemo): VAT 22% on the catalog, 40 customers,
// 25 season passes, about 70 bookings over the next 30 season days in mixed
// statuses, released pass days with resales, a closed cash session with
// payments and an open one, a few cancellations and check-ins. It is
// idempotent: when the establishment already has customers only the VAT
// default is (re)checked. Every write goes through the M3 use cases (ops).
func (s *Service) SeedDemoOperations(ctx context.Context, establishmentID string, ops DemoOperations) (*DemoOperationsResult, error) {
	if ops.Actor == nil || ops.Customers == nil || ops.Bookings == nil {
		return nil, fmt.Errorf("%w: setup: demo operations need an actor, customers and bookings", configerr.ErrValidation)
	}
	res := &DemoOperationsResult{}
	w, existing, err := s.demoPrepare(ctx, establishmentID, res)
	if err != nil {
		return nil, fmt.Errorf("setup: demo operations: %w", err)
	}
	if existing {
		return res, nil
	}
	customers, err := s.demoCustomers(ctx, w, ops, res)
	if err != nil || len(customers) == 0 {
		return res, err
	}
	res.Applied = true
	passes, err := s.demoPasses(ctx, w, ops, customers, res)
	if err != nil {
		return res, err
	}
	bookings, err := s.demoBookings(ctx, w, ops, customers, res)
	if err != nil {
		return res, err
	}
	if err := s.demoReleases(ctx, w, ops, customers, passes, res); err != nil {
		return res, err
	}
	if err := s.demoCash(ctx, w, ops, bookings, res); err != nil {
		return res, err
	}
	if err := s.demoLifecycle(ctx, w, ops, bookings, res); err != nil {
		return res, err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		// The seeded operations are history, not news: their outbox events
		// are marked dispatched so that the first `serve` does not email
		// dozens of fictitious customers or replay them to sinks (they stay
		// in the outbox table for inspection).
		if _, err := tx.NewUpdate().Model((*store.OutboxEvent)(nil)).
			Set("dispatched_at = ?", s.d.Clock.Now()).
			Where("establishment_id = ?", w.es.ID()).Where("dispatched_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("mark demo events dispatched: %w", err)
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: w.es.ID(), Actor: audit.SystemActor, Action: "seed.demo_operations",
			EntityType: "establishment", EntityID: w.es.ID(),
			Diff: map[string]any{
				"customers": res.Customers, "season_passes": res.SeasonPasses, "bookings": res.Bookings,
				"releases": res.Releases, "payments": res.Payments, "cash_sessions": res.CashSessions,
			},
		})
	})
	if err != nil {
		return res, fmt.Errorf("setup: demo operations: %w", err)
	}
	return res, nil
}

// demoPrepare sets the demo VAT and loads the world. existing is true when
// the establishment already has customers (nothing else to do).
func (s *Service) demoPrepare(ctx context.Context, establishmentID string, res *DemoOperationsResult) (*demoWorld, bool, error) {
	w := &demoWorld{occupied: map[string]map[calendar.Date]bool{}}
	existing := false
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := store.SystemEstablishmentScope(ctx, tx, establishmentID)
		if err != nil {
			return err
		}
		w.es = es
		if err := s.demoVAT(ctx, tx, es, res); err != nil {
			return err
		}
		// The same precedent as SeedDemo: a count on the scoped table
		// decides idempotency.
		n, err := tx.NewSelect().Model((*store.Customer)(nil)).Where("cu.establishment_id = ?", es.ID()).Count(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			existing = true
			return nil
		}
		e, err := store.GetEstablishment(ctx, tx, es)
		if err != nil {
			return err
		}
		w.today = s.today(e)
		w.season, err = store.GetSeasonByYear(ctx, tx, es, DefaultSeasonYear(w.today))
		if err != nil {
			return fmt.Errorf("demo season (run SeedDemo first): %w", err)
		}
		w.from = w.today
		if w.from.Before(w.season.OpensOn) {
			w.from = w.season.OpensOn
		}
		w.to = w.from.AddDays(demoHorizonDays - 1)
		if w.to.After(w.season.ClosesOn) {
			w.to = w.season.ClosesOn
		}
		return s.demoCatalog(ctx, tx, w)
	})
	return w, existing, err
}

// demoVAT sets 22% as the establishment default and on the packs and extras
// without their own rate (they showed 0% before).
func (s *Service) demoVAT(ctx context.Context, tx bun.Tx, es store.EstablishmentScope, res *DemoOperationsResult) error {
	now := s.d.Clock.Now()
	e, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return err
	}
	settings, err := establishment.ParseSettings(e.Settings, e.Currency)
	if err != nil {
		return err
	}
	if settings.Pricing.DefaultVATRateBP == 0 {
		settings.Pricing.DefaultVATRateBP = DemoVATRateBP
		raw, err := store.MarshalJSONValue(settings)
		if err != nil {
			return err
		}
		version := e.Version
		e.Settings, e.Version, e.UpdatedAt = raw, version+1, now
		if ok, err := store.UpdateEstablishmentIfVersion(ctx, tx, es, e, version); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("%w: establishment changed during the demo seed", configerr.ErrConflict)
		}
		res.VATUpdated++
	}
	vat := DemoVATRateBP
	packs, err := store.ListAllPacks(ctx, tx, es)
	if err != nil {
		return err
	}
	for i := range packs {
		p := &packs[i]
		if p.VATRateBP != nil && *p.VATRateBP != 0 {
			continue
		}
		old := p.Version
		p.VATRateBP, p.Version, p.UpdatedAt = &vat, old+1, now
		if _, err := store.UpdatePack(ctx, tx, es, p, old); err != nil {
			return err
		}
		res.VATUpdated++
	}
	extras, err := store.ListAllExtras(ctx, tx, es)
	if err != nil {
		return err
	}
	for i := range extras {
		x := &extras[i]
		if x.VATRateBP != nil && *x.VATRateBP != 0 {
			continue
		}
		old := x.Version
		x.VATRateBP, x.Version, x.UpdatedAt = &vat, old+1, now
		if _, err := store.UpdateExtra(ctx, tx, es, x, old); err != nil {
			return err
		}
		res.VATUpdated++
	}
	return nil
}

// demoCatalog loads the base pack, the extra sunbed and the umbrella spots
// of the classic template (rows A-B for passes, C-H for daily bookings).
func (s *Service) demoCatalog(ctx context.Context, tx bun.Tx, w *demoWorld) error {
	packs, err := store.ListAllPacks(ctx, tx, w.es)
	if err != nil {
		return err
	}
	for i := range packs {
		if packs[i].Code == PackBase {
			w.base = &packs[i]
		}
	}
	extras, err := store.ListAllExtras(ctx, tx, w.es)
	if err != nil {
		return err
	}
	for i := range extras {
		if extras[i].Code == extraSunbed.Code {
			w.extraSunbed = &extras[i]
		}
	}
	if w.base == nil {
		return fmt.Errorf("%w: demo pack %q (run SeedDemo first)", configerr.ErrNotFound, PackBase)
	}
	maps, err := store.ListMaps(ctx, tx, w.es, "", 100)
	if err != nil {
		return err
	}
	for _, m := range maps.Items {
		spots, err := store.ListMapSpots(ctx, tx, w.es, m.ID)
		if err != nil {
			return err
		}
		for _, sp := range spots {
			if !sp.Active || sp.Kind != "umbrella" {
				continue
			}
			switch sp.RowLabel {
			case "A", "B":
				w.passSpots = append(w.passSpots, sp)
			default:
				w.dailySpots = append(w.dailySpots, sp)
			}
		}
	}
	if len(w.passSpots) < demoSeasonPasses || len(w.dailySpots) == 0 {
		return fmt.Errorf("%w: demo map without enough umbrellas (classic template expected)", configerr.ErrNotFound)
	}
	return nil
}

// demoCustomers creates the fake customers in one transaction.
func (s *Service) demoCustomers(ctx context.Context, w *demoWorld, ops DemoOperations, res *DemoOperationsResult) ([]store.Customer, error) {
	var out []store.Customer
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		for i := range demoCustomers {
			first := demoFirstNames[i%len(demoFirstNames)]
			last := demoLastNames[i%len(demoLastNames)]
			in := &customer.Input{
				FirstName: first, LastName: last,
				Email:  fmt.Sprintf("%s.%s.%02d@example.com", asciiLower(first), asciiLower(last), i+1),
				Phone:  fmt.Sprintf("+39000000%04d", i+1),
				Locale: demoLocales[i%len(demoLocales)], Tags: demoTags[i%len(demoTags)],
			}
			if i%6 == 0 {
				in.Notes = "Cliente dimostrativo: preferisce le file centrali"
			}
			c, err := ops.Customers.Resolve(ctx, tx, ops.Actor, w.es, "", in)
			if err != nil {
				return err
			}
			out = append(out, *c)
		}
		return nil
	})
	if errors.Is(err, configerr.ErrNotImplemented) {
		res.Skipped = append(res.Skipped, "customers: not implemented")
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("setup: demo customers: %w", err)
	}
	res.Customers = len(out)
	return out, nil
}

// create runs one booking creation in its own transaction. Conflicts and
// policy refusals are counted and skipped (ok=false, err=nil); a not
// implemented service stops the phase (errNotReady).
func (s *Service) demoCreate(ctx context.Context, w *demoWorld, ops DemoOperations, in booking.CreateInput, res *DemoOperationsResult) (*booking.Detail, error) {
	var d *booking.Detail
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		d, err = ops.Bookings.CreateInTx(ctx, tx, ops.Actor, w.es, in)
		return err
	})
	var ce *availability.ConflictError
	switch {
	case err == nil:
		return d, nil
	case errors.Is(err, configerr.ErrNotImplemented):
		return nil, errNotReady
	case errors.As(err, &ce), errors.Is(err, availability.ErrSpotUnavailable), errors.Is(err, availability.ErrPoolExhausted),
		errors.Is(err, availability.ErrQuotaExceeded), errors.Is(err, configerr.ErrValidation), errors.Is(err, configerr.ErrConflict):
		res.Conflicts++
		s.d.Log.DebugContext(ctx, "demo booking skipped", "error", err.Error())
		return nil, nil
	default:
		return nil, err
	}
}

var errNotReady = errors.New("setup: demo: use case not implemented")

// demoPasses creates the season passes on rows A and B (whole season,
// full day, base pack, season allocations).
func (s *Service) demoPasses(ctx context.Context, w *demoWorld, ops DemoOperations, customers []store.Customer, res *DemoOperationsResult) ([]demoBooking, error) {
	var out []demoBooking
	for i := range demoSeasonPasses {
		sp := w.passSpots[i]
		d, err := s.demoCreate(ctx, w, ops, booking.CreateInput{
			CustomerID: customers[i%len(customers)].ID, Channel: store.ChannelDesk,
			StartsOn: w.season.OpensOn, EndsOn: w.season.ClosesOn, Slots: calendar.SlotsFull,
			Formula: string(pricing.FormulaSeason), Status: booking.StatusConfirmed, Force: true,
			Lines:    []booking.LineInput{{PackID: w.base.ID, SpotID: sp.ID, Quantity: 1}},
			SeasonID: w.season.ID, IsSeasonPass: true, AllocationKind: inventory.KindSeason,
			NotesInternal: "Abbonamento dimostrativo",
		}, res)
		if errors.Is(err, errNotReady) {
			res.Skipped = append(res.Skipped, "season passes: not implemented")
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("setup: demo season pass: %w", err)
		}
		if d == nil {
			continue
		}
		out = append(out, demoBooking{id: d.Booking.ID, total: d.Booking.TotalAmount, status: booking.Status(d.Booking.Status), starts: d.Booking.StartsOn})
		w.occupy(sp.ID, w.season.OpensOn, w.season.ClosesOn)
	}
	res.SeasonPasses = len(out)
	return out, nil
}

// demoBookings spreads desk bookings over the horizon: one to five days,
// mostly full day, a few half days and extras; 80% confirmed, 10% held
// (expiring holds, the M4 checkout state), 10% draft.
func (s *Service) demoBookings(ctx context.Context, w *demoWorld, ops DemoOperations, customers []store.Customer, res *DemoOperationsResult) ([]demoBooking, error) {
	horizon := w.from.DaysUntil(w.to) + 1
	var out []demoBooking
	for i := range demoBookings {
		length := 1 + i%5
		start := w.from.AddDays((i * 7) % horizon)
		end := start.AddDays(length - 1)
		if end.After(w.to) {
			end = w.to
		}
		sp, ok := w.freeSpot(i, start, end)
		if !ok {
			continue
		}
		in := booking.CreateInput{
			CustomerID: customers[(i+demoSeasonPasses)%len(customers)].ID, Channel: store.ChannelDesk,
			StartsOn: start, EndsOn: end, Slots: calendar.SlotsFull, Formula: string(pricing.FormulaDay),
			Status: booking.StatusConfirmed, Force: true,
			Lines: []booking.LineInput{{PackID: w.base.ID, SpotID: sp.ID, Quantity: 1}},
		}
		if start.Before(end) {
			in.Formula = string(pricing.FormulaMultiDay)
		} else {
			switch i % 7 {
			case 3:
				in.Slots, in.Formula = calendar.SlotsAM, string(pricing.FormulaHalfDay)
			case 4:
				in.Slots, in.Formula = calendar.SlotsPM, string(pricing.FormulaHalfDay)
			}
		}
		if i%4 == 0 && w.extraSunbed != nil {
			in.Lines[0].Extras = []booking.ExtraInput{{ExtraID: w.extraSunbed.ID, Quantity: 1}}
		}
		switch i % 10 {
		case 5:
			in.Status, in.HoldTTL = booking.StatusHeld, demoHoldTTL
		case 9:
			in.Status = booking.StatusDraft
		}
		d, err := s.demoCreate(ctx, w, ops, in, res)
		if errors.Is(err, errNotReady) {
			res.Skipped = append(res.Skipped, "bookings: not implemented")
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("setup: demo booking: %w", err)
		}
		if d == nil {
			continue
		}
		if in.Status != booking.StatusDraft {
			w.occupy(sp.ID, start, end)
		}
		out = append(out, demoBooking{id: d.Booking.ID, total: d.Booking.TotalAmount, status: booking.Status(d.Booking.Status), starts: d.Booking.StartsOn})
	}
	res.Bookings = len(out)
	return out, nil
}

// demoReleases releases three days of the first passes and resells two of
// them to other customers (spec 6, season_releases).
func (s *Service) demoReleases(ctx context.Context, w *demoWorld, ops DemoOperations, customers []store.Customer, passes []demoBooking, res *DemoOperationsResult) error {
	if ops.ReleaseDays == nil {
		res.Skipped = append(res.Skipped, "season releases: no port")
		return nil
	}
	first := w.from.AddDays(2)
	if first.After(w.to) {
		first = w.from
	}
	dates := []calendar.Date{first}
	for d := first.AddDays(1); len(dates) < 3 && !d.After(w.to); d = d.AddDays(1) {
		dates = append(dates, d)
	}
	for i := 0; i < demoReleasedPasses && i < len(passes); i++ {
		err := ops.ReleaseDays(ctx, ops.Actor, w.es.ID(), passes[i].id, dates)
		if errors.Is(err, configerr.ErrNotImplemented) {
			res.Skipped = append(res.Skipped, "season releases: not implemented")
			return nil
		}
		if err != nil {
			return fmt.Errorf("setup: demo release: %w", err)
		}
		res.Releases += len(dates)
		if i >= 2 {
			continue
		}
		// Resell the first released day of this spot (desk, full day).
		d, err := s.demoCreate(ctx, w, ops, booking.CreateInput{
			CustomerID: customers[(i+7)%len(customers)].ID, Channel: store.ChannelDesk,
			StartsOn: dates[0], EndsOn: dates[0], Slots: calendar.SlotsFull, Formula: string(pricing.FormulaDay),
			Status: booking.StatusConfirmed, Force: true,
			Lines:         []booking.LineInput{{PackID: w.base.ID, SpotID: w.passSpots[i].ID, Quantity: 1}},
			NotesInternal: "Rivendita di un giorno rilasciato dall'abbonato",
		}, res)
		if err != nil && !errors.Is(err, errNotReady) {
			return fmt.Errorf("setup: demo resale: %w", err)
		}
		if d != nil {
			res.Resales++
		}
	}
	return nil
}

// demoCash opens a session, records payments for the first half of the
// confirmed bookings (cash and card, some deposits), closes it with a small
// shortfall, then opens the current session with a few more payments.
func (s *Service) demoCash(ctx context.Context, w *demoWorld, ops DemoOperations, bookings []demoBooking, res *DemoOperationsResult) error {
	if ops.OpenCash == nil || ops.RecordPayment == nil || ops.CloseCash == nil {
		res.Skipped = append(res.Skipped, "cash and payments: no port")
		return nil
	}
	var confirmed []demoBooking
	for _, b := range bookings {
		if b.status == booking.StatusConfirmed && b.total > 0 {
			confirmed = append(confirmed, b)
		}
	}
	half := len(confirmed) / 2
	const closedFloat, openFloat money.Cents = 10000, 15000
	closedID, err := ops.OpenCash(ctx, ops.Actor, w.es.ID(), closedFloat)
	if errors.Is(err, configerr.ErrNotImplemented) {
		res.Skipped = append(res.Skipped, "cash and payments: not implemented")
		return nil
	}
	if err != nil {
		return fmt.Errorf("setup: demo open cash: %w", err)
	}
	res.CashSessions++
	counted := store.MethodTotals{store.PaymentMethodCash: closedFloat}
	for i, b := range confirmed[:half] {
		pay := DemoPayment{BookingID: b.id, Amount: b.total, Method: store.PaymentMethodCash, CashSessionID: closedID}
		if i%3 == 1 {
			pay.Method = store.PaymentMethodCard
		}
		if i%4 == 2 { // deposit only: balance due at the desk
			pay.Amount = max(b.total*3/10, 1)
		}
		if err := ops.RecordPayment(ctx, ops.Actor, w.es.ID(), pay); err != nil {
			return fmt.Errorf("setup: demo payment: %w", err)
		}
		counted[pay.Method] += pay.Amount
		res.Payments++
	}
	counted[store.PaymentMethodCash] -= 200 // a 2 euro shortfall shows the difference
	if err := ops.CloseCash(ctx, ops.Actor, w.es.ID(), closedID, counted); err != nil {
		return fmt.Errorf("setup: demo close cash: %w", err)
	}
	openID, err := ops.OpenCash(ctx, ops.Actor, w.es.ID(), openFloat)
	if err != nil {
		return fmt.Errorf("setup: demo open cash: %w", err)
	}
	res.CashSessions++
	for i, b := range confirmed[half:] {
		if i >= 6 {
			break
		}
		method := store.PaymentMethodCash
		if i%2 == 1 {
			method = store.PaymentMethodCard
		}
		if err := ops.RecordPayment(ctx, ops.Actor, w.es.ID(), DemoPayment{BookingID: b.id, Amount: b.total, Method: method, CashSessionID: openID}); err != nil {
			return fmt.Errorf("setup: demo payment: %w", err)
		}
		res.Payments++
	}
	return nil
}

// demoLifecycle cancels a few confirmed bookings (the policy decides the
// refund) and checks in the bookings starting today.
func (s *Service) demoLifecycle(ctx context.Context, w *demoWorld, ops DemoOperations, bookings []demoBooking, res *DemoOperationsResult) error {
	for i, b := range bookings {
		if b.status != booking.StatusConfirmed {
			continue
		}
		switch {
		case i%13 == 7 && ops.CancelBooking != nil:
			err := ops.CancelBooking(ctx, ops.Actor, w.es.ID(), b.id, "Cambio programma (demo)")
			if errors.Is(err, configerr.ErrNotImplemented) {
				ops.CancelBooking = nil
				res.Skipped = append(res.Skipped, "cancellations: not implemented")
				continue
			}
			if err != nil {
				return fmt.Errorf("setup: demo cancel: %w", err)
			}
			res.Cancelled++
		case b.starts.Equal(w.today) && ops.CheckIn != nil:
			err := ops.CheckIn(ctx, ops.Actor, w.es.ID(), b.id)
			if errors.Is(err, configerr.ErrNotImplemented) {
				ops.CheckIn = nil
				res.Skipped = append(res.Skipped, "check-in: not implemented")
				continue
			}
			if err != nil {
				return fmt.Errorf("setup: demo check-in: %w", err)
			}
			res.CheckedIn++
		}
	}
	if ops.CancelBooking == nil && res.Cancelled == 0 {
		res.Skipped = append(res.Skipped, "cancellations: no port")
	}
	return nil
}

// freeSpot picks a daily spot free on [start, end], starting from a
// position derived from i so that bookings spread over the rows.
func (w *demoWorld) freeSpot(i int, start, end calendar.Date) (store.Spot, bool) {
	n := len(w.dailySpots)
	for k := range n {
		sp := w.dailySpots[(i*11+k)%n]
		if w.free(sp.ID, start, end) {
			return sp, true
		}
	}
	return store.Spot{}, false
}

func (w *demoWorld) free(spotID string, start, end calendar.Date) bool {
	for d := start; !d.After(end); d = d.AddDays(1) {
		if w.occupied[spotID][d] {
			return false
		}
	}
	return true
}

func (w *demoWorld) occupy(spotID string, start, end calendar.Date) {
	if w.occupied[spotID] == nil {
		w.occupied[spotID] = map[calendar.Date]bool{}
	}
	for d := start; !d.After(end); d = d.AddDays(1) {
		w.occupied[spotID][d] = true
	}
}

// asciiLower lowercases s keeping only ASCII letters (email local parts).
func asciiLower(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			out = append(out, c+'a'-'A')
		case c >= 'a' && c <= 'z':
			out = append(out, c)
		}
	}
	return string(out)
}
