package booking

import (
	"context"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Duplicate (spec 30.9): a new booking with the same spots, customer,
// extras, agreement and group data on new dates, priced again and
// allocated through the normal create path (DB constraints, audit, outbox).
// Owner: the moves builder.

// DuplicateInput is api.BookingDuplicateRequest.
type DuplicateInput struct {
	StartsOn calendar.Date
	EndsOn   calendar.Date
	Slots    *calendar.Slots
	Formula  *string
	Status   *string // draft | held | confirmed (default confirmed)
	DryRun   bool
}

// DuplicateResult is api.BookingDuplicateResult.
type DuplicateResult struct {
	Applied   bool
	Detail    *Detail // nil with DryRun
	Total     money.Cents
	Conflicts []availability.Conflict
	// Price is the engine result (breakdown, policy) of the new dates.
	Price *pricingconfig.SimulateResult
}

// Duplicate copies booking id on new dates: same customer, lines (pack,
// spot or zone, quantity, extras), booking extras, agreement and group
// data; the price is computed again and the allocation goes through the
// normal creation (DB constraints, audit, outbox). A dry run is the quote
// of the copy with its conflicts. Season passes are renewed, not
// duplicated. Requires auth.PermBookingsWrite.
func (s *Service) Duplicate(ctx context.Context, p *auth.Principal, establishmentID, id string, in DuplicateInput) (*DuplicateResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	src, err := store.GetBooking(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	if src.IsSeasonPass {
		return nil, configerr.Invalid("booking_id", "Un abbonamento si rinnova, non si duplica.")
	}
	ci, err := s.duplicateInput(ctx, es, src, in)
	if err != nil {
		return nil, err
	}
	if in.DryRun {
		q, err := s.Quote(ctx, p, establishmentID, ci)
		if err != nil {
			return nil, err
		}
		return &DuplicateResult{Total: q.Result.Breakdown.Total, Conflicts: q.Conflicts, Price: q.Result}, nil
	}
	d, err := s.Create(ctx, p, establishmentID, ci)
	if err != nil {
		return nil, err
	}
	return &DuplicateResult{Applied: true, Detail: d, Total: d.Booking.TotalAmount, Conflicts: []availability.Conflict{}}, nil
}

// duplicateInput is the creation copying src on the dates of in.
func (s *Service) duplicateInput(ctx context.Context, es store.EstablishmentScope, src *store.Booking, in DuplicateInput) (CreateInput, error) {
	v := &configerr.ValidationError{}
	if in.StartsOn.IsZero() {
		v.Add("starts_on", "Campo obbligatorio.")
	}
	if in.EndsOn.IsZero() || in.EndsOn.Before(in.StartsOn) {
		v.Add("ends_on", "Deve essere successiva o uguale a starts_on.")
	}
	status := StatusConfirmed
	if in.Status != nil {
		switch Status(*in.Status) {
		case StatusDraft, StatusHeld, StatusConfirmed:
			status = Status(*in.Status)
		default:
			v.Add("status", "Usa draft, held o confirmed.")
		}
	}
	if err := v.OrNil(); err != nil {
		return CreateInput{}, err
	}
	rows, err := store.ListBookingLines(ctx, s.d.DB, es, src.ID)
	if err != nil {
		return CreateInput{}, err
	}
	extras, err := store.ListBookingLineExtras(ctx, s.d.DB, es, src.ID)
	if err != nil {
		return CreateInput{}, err
	}
	lines, bookingExtras := linesToInput(rows, extras)
	for i := range lines {
		if lines[i].SpotID != "" {
			lines[i].ZoneID = "" // derived from the spot
		}
	}
	slots := src.Slots
	if in.Slots != nil {
		slots = *in.Slots
	}
	formula := duplicateFormula(src, in.StartsOn, in.EndsOn, slots)
	if in.Formula != nil && *in.Formula != "" {
		formula = *in.Formula
	}
	ci := CreateInput{
		CustomerID: src.CustomerID, Channel: store.ChannelDesk, StartsOn: in.StartsOn, EndsOn: in.EndsOn,
		Slots: slots, Formula: formula, Lines: lines, Extras: bookingExtras, Status: status,
	}
	if src.AgreementID != nil {
		ci.AgreementID = *src.AgreementID
	}
	copyGroup(&ci, src)
	return ci, nil
}

// duplicateFormula deduces the formula of the copy: day or half day for one
// day, the fixed duration of the original when the length is the same,
// multi_day otherwise.
func duplicateFormula(src *store.Booking, from, to calendar.Date, slots calendar.Slots) string {
	days := from.DaysUntil(to) + 1
	if days == 1 {
		if slots != calendar.SlotsFull {
			return string(pricing.FormulaHalfDay)
		}
		return string(pricing.FormulaDay)
	}
	if f := pricing.Formula(src.Formula); f.IsFixedDuration() && f != pricing.FormulaSeason && src.StartsOn.DaysUntil(src.EndsOn)+1 == days {
		return src.Formula
	}
	return string(pricing.FormulaMultiDay)
}

// copyGroup copies the partner, group and party of src (not the guest names
// nor the partner share, which belong to the original stay).
func copyGroup(ci *CreateInput, src *store.Booking) {
	if src.PartnerID != nil {
		id := *src.PartnerID
		ci.Group.PartnerID = &id
	}
	if src.GroupName != "" {
		n := src.GroupName
		ci.Group.GroupName = &n
	}
	if src.GroupOrigin != "" {
		o := src.GroupOrigin
		ci.Group.GroupOrigin = &o
	}
	if src.PartyAdults != nil {
		a := *src.PartyAdults
		ci.Group.PartyAdults = &a
	}
	if src.PartyChildren != nil {
		c := *src.PartyChildren
		ci.Group.PartyChildren = &c
	}
}
