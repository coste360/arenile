package dailysale

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Assignment modes of a zone (zones.assignment_mode).
const (
	modeSeat     = "seat"
	modeZone     = "zone"
	modeQuantity = "quantity"
)

// maxNotes bounds the internal notes of a sale.
const maxNotes = 2000

// SaleInput is a quick walk-in sale (api.DailySaleQuoteRequest and
// api.DailySaleCreate): where (Target), when (Date, Slots), what (pack and
// extras). The payment fields are ignored by QuoteDaily.
type SaleInput struct {
	Target Target
	Date   calendar.Date
	Slots  calendar.Slots
	PackID string
	Extras []booking.ExtraInput

	Payment       SalePayment
	FirstName     string
	LastName      string
	ExpectedTotal *money.Cents
	NotesInternal string
}

// SalePayment is how the sale is paid.
type SalePayment struct {
	Method        string // store.TicketPaymentCash or store.TicketPaymentCard
	CashSessionID *string
	// Tendered is the cash received (cash only), to compute the change.
	Tendered *money.Cents
}

// Quote is the preview of a sale: price, availability and, for a
// zone-first sale, the free places left in the zone.
type Quote struct {
	Result    *pricingconfig.SimulateResult
	Conflicts []availability.Conflict
	Available bool
	// ZoneFree is set for zone-first sales.
	ZoneFree *int
}

// Result is a completed sale.
type Result struct {
	Ticket    *store.Ticket
	BookingID string
	PaymentID string // empty for a free sale
	Change    *money.Cents
}

// where is the resolved target of a sale.
type where struct {
	zone *store.Zone
	spot *store.Spot // spot-first only
	// free are the free spots of a seat zone for the slot, in map order
	// (zone-first only); freeCount is the residual capacity of the zone.
	free      []string
	freeCount int
}

func validateSale(in *SaleInput) error {
	ve := &configerr.ValidationError{}
	if err := in.Target.Validate(); err != nil {
		ve.Add("spot_id", "Indica la postazione oppure la zona (una sola delle due).")
	}
	if !in.Slots.Valid() {
		ve.Add("slots", "Usa AM, PM o FULL.")
	}
	if in.PackID == "" {
		ve.Add("pack_id", "Campo obbligatorio.")
	}
	for i, x := range in.Extras {
		if x.ExtraID == "" || x.Quantity < 1 {
			ve.Add(fmt.Sprintf("extras[%d]", i), "Extra o quantità non validi.")
		}
	}
	in.NotesInternal = strings.TrimSpace(in.NotesInternal)
	if len([]rune(in.NotesInternal)) > maxNotes {
		ve.Add("notes_internal", fmt.Sprintf("Massimo %d caratteri.", maxNotes))
	}
	return ve.OrNil()
}

// resolve loads the zone (and spot) of the target and, for a zone-first
// sale, the free places of the zone read through db.
func (s *Service) resolve(ctx context.Context, db bun.IDB, p *auth.Principal, es store.EstablishmentScope, in SaleInput) (*where, error) {
	w := &where{}
	zoneID := in.Target.ZoneID
	if in.Target.SpotID != "" {
		spots, err := store.BookingSpotsByIDs(ctx, db, es, []string{in.Target.SpotID})
		if err != nil {
			return nil, err
		}
		if len(spots) == 0 {
			return nil, configerr.Invalid("spot_id", "Postazione inesistente.")
		}
		w.spot = &spots[0]
		zoneID = w.spot.ZoneID
	}
	zones, err := store.InventoryZones(ctx, db, es, []string{zoneID})
	if err != nil {
		return nil, err
	}
	if len(zones) == 0 {
		return nil, configerr.Invalid("zone_id", "Zona inesistente.")
	}
	w.zone = &zones[0]
	if w.spot != nil {
		return w, nil
	}
	if w.zone.AssignmentMode == modeQuantity {
		return nil, configerr.Invalid("zone_id", "Le zone a quantità si vendono con i biglietti d'ingresso.")
	}
	res, err := s.d.AvailabilityIn(db).GetAvailability(ctx, p, es.ID(), inventory.AvailabilityQuery{
		From: in.Date, To: in.Date, Slots: in.Slots, Channel: store.ChannelDesk, ZoneID: w.zone.ID, PackID: in.PackID,
	})
	if err != nil {
		return nil, err
	}
	for _, z := range res.Zones {
		if z.ZoneID != w.zone.ID {
			continue
		}
		w.freeCount = max(z.Available, 0)
		for _, sp := range z.Spots {
			if sp.Available {
				w.free = append(w.free, sp.SpotID)
			}
		}
		if w.zone.AssignmentMode == modeSeat {
			w.freeCount = len(w.free)
		}
	}
	return w, nil
}

// createInput is the booking of the sale on spotID (empty: the zone).
func createInput(in SaleInput, customerID, zoneID, spotID string) booking.CreateInput {
	line := booking.LineInput{PackID: in.PackID, Quantity: 1}
	if spotID != "" {
		line.SpotID = spotID
	} else {
		line.ZoneID = zoneID
	}
	return booking.CreateInput{
		CustomerID: customerID, Channel: store.ChannelDesk, StartsOn: in.Date, EndsOn: in.Date, Slots: in.Slots,
		Formula: FormulaFor(in.Slots), Lines: []booking.LineInput{line}, Extras: in.Extras,
		NotesInternal: in.NotesInternal, Status: booking.StatusConfirmed,
	}
}

// QuoteDaily prices a quick sale and reports whether it can be sold
// (auth.PermBookingsWrite). A zone-first quote on a seat zone is priced on
// the spot the sale would hold provisionally (the last free one).
func (s *Service) QuoteDaily(ctx context.Context, p *auth.Principal, establishmentID string, in SaleInput) (*Quote, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	if err := validateSale(&in); err != nil {
		return nil, err
	}
	if in.Date.IsZero() {
		return nil, configerr.Invalid("date", "Campo obbligatorio.")
	}
	w, err := s.resolve(ctx, s.d.DB, p, es, in)
	if err != nil {
		return nil, err
	}
	spotID := in.Target.SpotID
	out := &Quote{}
	if in.Target.ZoneFirst() {
		free := w.freeCount
		out.ZoneFree = &free
		if w.zone.AssignmentMode == modeSeat {
			switch {
			case len(w.free) > 0:
				spotID = w.free[len(w.free)-1]
			default:
				// Price on any spot of the zone: the sale is refused anyway.
				spots, err := store.ListZoneSpots(ctx, s.d.DB, es, []string{w.zone.ID})
				if err != nil {
					return nil, err
				}
				if len(spots) == 0 {
					return nil, configerr.Invalid("zone_id", "La zona non ha postazioni.")
				}
				spotID = spots[len(spots)-1].ID
			}
		}
	}
	q, err := s.d.Bookings.Quote(ctx, p, establishmentID, createInput(in, "", w.zone.ID, spotID))
	if err != nil {
		return nil, err
	}
	out.Result, out.Conflicts = q.Result, q.Conflicts
	out.Available = len(q.Conflicts) == 0
	if in.Target.ZoneFirst() {
		out.Available = w.freeCount > 0
		if w.zone.AssignmentMode == modeSeat {
			out.Conflicts = []availability.Conflict{}
		}
	}
	return out, nil
}

// CreateDaily sells a day (auth.PermBookingsWrite and auth.PermCashOperate)
// in one transaction: confirmed desk booking through the booking service
// (audit + outbox), manual payment (cash needs an open cash session),
// numbered ticket. Spot-first occupies the spot (409 spot_unavailable);
// zone-first consumes the zone capacity at once (409 zone_full): the
// implicit pool of a zone-mode zone, or a provisional allocation of the last
// free spot of a seat zone (tried from the last spot backwards: each
// attempt is a savepoint, the unique allocation key decides). The ticket of
// a zone-first sale is "da assegnare".
func (s *Service) CreateDaily(ctx context.Context, p *auth.Principal, establishmentID string, in SaleInput) (*Result, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	if err := validateSale(&in); err != nil {
		return nil, err
	}
	if err := validatePayment(in.Payment); err != nil {
		return nil, err
	}
	if err := s.checkDate(ctx, es, in.Date); err != nil {
		return nil, err
	}
	if in.Target.ZoneFirst() {
		st, err := s.Settings(ctx, p, establishmentID)
		if err != nil {
			return nil, err
		}
		if !st.ZoneFirstEnabled {
			return nil, configerr.Invalid("zone_id", "La vendita per zona è disattivata nelle impostazioni.")
		}
	}
	custID, err := s.walkIn(ctx, p, es)
	if err != nil {
		return nil, err
	}
	var out *Result
	err = s.runSale(ctx, func(ctx context.Context, tx bun.Tx) error {
		w, err := s.resolve(ctx, tx, p, es, in)
		if err != nil {
			return err
		}
		var (
			d           *booking.Detail
			provisional *string
		)
		switch {
		case !in.Target.ZoneFirst():
			d, err = s.d.Bookings.CreateInTx(ctx, tx, p, es, createInput(in, custID, w.zone.ID, w.spot.ID))
		case w.zone.AssignmentMode == modeSeat:
			d, provisional, err = s.createOnFreeSpot(ctx, tx, p, es, in, custID, w)
		default:
			d, err = s.d.Bookings.CreateInTx(ctx, tx, p, es, createInput(in, custID, w.zone.ID, ""))
			if errors.Is(err, availability.ErrPoolExhausted) || errors.Is(err, availability.ErrQuotaExceeded) {
				err = ticketing.ErrZoneFull
			}
		}
		if err != nil {
			return err
		}
		out, err = s.settle(ctx, tx, p, es, in.Payment, in.ExpectedTotal, d, ticketing.IssueInput{
			Date: in.Date, Kind: store.TicketKindSale, ZoneID: w.zone.ID, NeedsAssignment: in.Target.ZoneFirst(),
			ProvisionalSpotID: provisional, HolderName: HolderName(in.FirstName, in.LastName),
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// createOnFreeSpot books the last free spot of a seat zone, trying the
// next one backwards when a concurrent sale took it (savepoint per attempt).
func (s *Service) createOnFreeSpot(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in SaleInput, custID string, w *where) (*booking.Detail, *string, error) {
	for _, spotID := range slices.Backward(w.free) {
		var d *booking.Detail
		err := tx.RunInTx(ctx, nil, func(ctx context.Context, sp bun.Tx) error {
			var err error
			d, err = s.d.Bookings.CreateInTx(ctx, sp, p, es, createInput(in, custID, w.zone.ID, spotID))
			return err
		})
		var ce *availability.ConflictError
		if errors.As(err, &ce) || errors.Is(err, availability.ErrSpotUnavailable) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		id := spotID
		return d, &id, nil
	}
	return nil, nil, ticketing.ErrZoneFull
}

func validatePayment(pm SalePayment) error {
	ve := &configerr.ValidationError{}
	switch pm.Method {
	case store.TicketPaymentCash, store.TicketPaymentCard:
	default:
		ve.Add("payment.method", "Usa cash o card.")
	}
	if pm.Tendered != nil && (*pm.Tendered < 0 || pm.Method != store.TicketPaymentCash) {
		ve.Add("payment.tendered_amount", "Solo per il pagamento in contante, importo non negativo.")
	}
	return ve.OrNil()
}

// settle checks the expected total, records the payment and issues the
// ticket of the booking d, all in tx.
func (s *Service) settle(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, pm SalePayment, expected *money.Cents, d *booking.Detail, ti ticketing.IssueInput) (*Result, error) {
	total := d.Booking.TotalAmount
	if expected != nil && *expected != total {
		return nil, ticketing.ErrPriceChanged
	}
	out := &Result{BookingID: d.Booking.ID}
	if pm.Tendered != nil {
		if *pm.Tendered < total {
			return nil, configerr.Invalid("payment.tendered_amount", "Il contante ricevuto non copre il totale.")
		}
		change := *pm.Tendered - total
		out.Change = &change
	}
	ti.BookingID, ti.TotalAmount, ti.PaymentMethod = d.Booking.ID, total, pm.Method
	if total > 0 {
		paid, err := s.d.Payments.RecordManualInTx(ctx, tx, p, es, PaymentInput{
			BookingID: d.Booking.ID, Amount: total, Method: pm.Method, CashSessionID: pm.CashSessionID,
			Note: "Vendita giornaliera",
		})
		if err != nil {
			return nil, err
		}
		out.PaymentID = paid.ID
		ti.PaymentID, ti.CashSessionID = &paid.ID, paid.CashSessionID
	}
	t, err := s.d.Tickets.Issue(ctx, tx, p, es, ti)
	if err != nil {
		return nil, err
	}
	out.Ticket = t
	return out, nil
}

// maxSaleAttempts bounds the retries of a sale transaction that lost the
// booking code race (booking.ErrCodeExhausted: on PostgreSQL many long
// concurrent sales wait on each other's uncommitted code).
const maxSaleAttempts = 5

// runSale runs fn in a transaction, again from scratch when the booking
// code allocation reported contention. A rolled back attempt writes
// nothing and gives its ticket number back.
func (s *Service) runSale(ctx context.Context, fn func(ctx context.Context, tx bun.Tx) error) error {
	var err error
	for range maxSaleAttempts {
		err = store.RunInTx(ctx, s.d.DB, fn)
		if !errors.Is(err, booking.ErrCodeExhausted) {
			return err
		}
	}
	return err
}
