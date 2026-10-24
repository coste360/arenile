package dailysale

import (
	"context"
	"fmt"
	"slices"

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

// Limits of an entry sale (api.EntrySaleItem, api.EntrySaleCreate).
const (
	maxEntryItems    = 20
	maxEntryQuantity = 100
)

// EntryItem is a quantity of one entry ticket type.
type EntryItem struct {
	TypeID   string
	Quantity int
}

// EntryInput is an entry sale (api.EntrySaleQuoteRequest, api.EntrySaleCreate).
type EntryInput struct {
	Date  calendar.Date
	Slots calendar.Slots
	Items []EntryItem

	Payment       SalePayment
	FirstName     string
	LastName      string
	ExpectedTotal *money.Cents
}

// EntryQuote is the preview of an entry sale.
type EntryQuote struct {
	Result    *pricingconfig.SimulateResult
	Conflicts []availability.Conflict
	Available bool
	// Remaining is the minimum residual capacity of the zones involved.
	Remaining int
}

func validateEntry(in *EntryInput) error {
	ve := &configerr.ValidationError{}
	if in.Date.IsZero() {
		ve.Add("date", "Campo obbligatorio.")
	}
	if !in.Slots.Valid() {
		ve.Add("slots", "Usa AM, PM o FULL.")
	}
	if len(in.Items) == 0 || len(in.Items) > maxEntryItems {
		ve.Add("items", fmt.Sprintf("Da 1 a %d tipi di biglietto.", maxEntryItems))
	}
	seen := map[string]bool{}
	for i, it := range in.Items {
		if it.TypeID == "" || seen[it.TypeID] {
			ve.Add(fmt.Sprintf("items[%d].entry_ticket_type_id", i), "Tipo mancante o ripetuto.")
		}
		seen[it.TypeID] = true
		if it.Quantity < 1 || it.Quantity > maxEntryQuantity {
			ve.Add(fmt.Sprintf("items[%d].quantity", i), fmt.Sprintf("Quantità da 1 a %d.", maxEntryQuantity))
		}
	}
	return ve.OrNil()
}

// entryLines resolves the entry ticket types of in into booking lines (one
// per type, its pack on its quantity zone) and returns the zones involved.
func entryLines(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in EntryInput) ([]booking.LineInput, []string, error) {
	types, err := store.ListEntryTicketTypes(ctx, db, es)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]*store.EntryTicketType, len(types))
	for i := range types {
		byID[types[i].ID] = &types[i]
	}
	lines := make([]booking.LineInput, 0, len(in.Items))
	var zones []string
	for i, it := range in.Items {
		t := byID[it.TypeID]
		if t == nil || !t.Active {
			return nil, nil, configerr.Invalid(fmt.Sprintf("items[%d].entry_ticket_type_id", i), "Tipo di biglietto inesistente o disattivato.")
		}
		lines = append(lines, booking.LineInput{PackID: t.PackID, ZoneID: t.ZoneID, Quantity: it.Quantity})
		if !slices.Contains(zones, t.ZoneID) {
			zones = append(zones, t.ZoneID)
		}
	}
	// One ticket, one QR, one gate: a sale is a group entering together
	// through one zone (INV-84). Types of different zones are sold apart.
	if len(zones) > 1 {
		return nil, nil, configerr.Invalid("items", "I tipi scelti appartengono a zone diverse: vendili in biglietti separati.")
	}
	return lines, zones, nil
}

func entryCreateInput(in EntryInput, customerID string, lines []booking.LineInput) booking.CreateInput {
	return booking.CreateInput{
		CustomerID: customerID, Channel: store.ChannelDesk, StartsOn: in.Date, EndsOn: in.Date, Slots: in.Slots,
		Formula: FormulaFor(in.Slots), Lines: lines, Status: booking.StatusConfirmed,
	}
}

// QuoteEntry prices an entry sale and reports the capacity left
// (auth.PermBookingsWrite).
func (s *Service) QuoteEntry(ctx context.Context, p *auth.Principal, establishmentID string, in EntryInput) (*EntryQuote, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	if err := validateEntry(&in); err != nil {
		return nil, err
	}
	lines, zones, err := entryLines(ctx, s.d.DB, es, in)
	if err != nil {
		return nil, err
	}
	q, err := s.d.Bookings.Quote(ctx, p, establishmentID, entryCreateInput(in, "", lines))
	if err != nil {
		return nil, err
	}
	remaining := -1
	for _, z := range zones {
		res, err := s.d.AvailabilityIn(s.d.DB).GetAvailability(ctx, p, establishmentID, inventory.AvailabilityQuery{
			From: in.Date, To: in.Date, Slots: in.Slots, Channel: store.ChannelDesk, ZoneID: z,
		})
		if err != nil {
			return nil, err
		}
		for _, za := range res.Zones {
			if za.ZoneID == z && (remaining < 0 || za.Available < remaining) {
				remaining = za.Available
			}
		}
	}
	return &EntryQuote{
		Result: q.Result, Conflicts: q.Conflicts, Available: len(q.Conflicts) == 0, Remaining: max(remaining, 0),
	}, nil
}

// CreateEntry sells entry tickets (auth.PermBookingsWrite and
// auth.PermCashOperate): one confirmed desk booking with a line per ticket
// type on its quantity zone (the zone pool is the daily capacity shared
// with the online channel: 409 pool_exhausted), manual payment and one
// numbered ticket with the QR checked at the gate.
func (s *Service) CreateEntry(ctx context.Context, p *auth.Principal, establishmentID string, in EntryInput) (*Result, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	if err := validateEntry(&in); err != nil {
		return nil, err
	}
	if err := validatePayment(in.Payment); err != nil {
		return nil, err
	}
	if err := s.checkDate(ctx, es, in.Date); err != nil {
		return nil, err
	}
	custID, err := s.walkIn(ctx, p, es)
	if err != nil {
		return nil, err
	}
	var out *Result
	err = s.runSale(ctx, func(ctx context.Context, tx bun.Tx) error {
		lines, _, err := entryLines(ctx, tx, es, in)
		if err != nil {
			return err
		}
		d, err := s.d.Bookings.CreateInTx(ctx, tx, p, es, entryCreateInput(in, custID, lines))
		if err != nil {
			return err
		}
		out, err = s.settle(ctx, tx, p, es, in.Payment, in.ExpectedTotal, d, ticketing.IssueInput{
			Date: in.Date, Kind: store.TicketKindEntry, ZoneID: lines[0].ZoneID, HolderName: HolderName(in.FirstName, in.LastName),
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
