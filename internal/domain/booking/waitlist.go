package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Bookings born from a waitlist offer (M5B, spec 10.5): the offer reserved
// the inventory with a hold; accepting it converts that hold into a held
// booking of the entry's customer (ConvertHold, same transaction), which
// then continues in the M4 checkout (payment, confirmation by webhook or
// manual driver only, INV-19).

// HoldBookingInput creates a held booking from a waitlist offer hold.
type HoldBookingInput struct {
	HoldID     string
	CustomerID string
	PackID     string
	// Formula of the stay (the checkout derives it like the public flow).
	Formula string
	// HoldTTL is the hold of the booking (the checkout countdown).
	HoldTTL time.Duration
}

// CreateFromHold creates, in tx, a held booking of an existing customer
// on the resources and interval of an open hold (channel of the hold, so
// the online quota stays where the offer put it, ADR 0011): one line per
// held spot, one line per unit of a held zone-mode zone, one line with the
// quantity of a held quantity zone (pool items are the pack components and
// are allocated again from the pack composition). Price and policy are the
// online ones; violations are refused, never forced.
func (s *Service) CreateFromHold(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, in HoldBookingInput) (*Detail, error) {
	if in.HoldID == "" || in.CustomerID == "" || in.PackID == "" {
		return nil, configerr.Invalid("hold_id", "Hold, cliente e pack obbligatori.")
	}
	h, err := store.GetHold(ctx, tx, es, in.HoldID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("booking: hold: %w", err)
	}
	cust, err := store.GetCustomer(ctx, tx, es, in.CustomerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, configerr.Invalid("customer_id", "Cliente inesistente.")
		}
		return nil, fmt.Errorf("booking: customer: %w", err)
	}
	if cust.AnonymizedAt != nil {
		return nil, configerr.Invalid("customer_id", "Cliente anonimizzato.")
	}
	lines, err := holdBookingLines(ctx, tx, es, h, in.PackID)
	if err != nil {
		return nil, err
	}
	ci := CreateInput{
		Channel: h.Channel, StartsOn: h.StartsOn, EndsOn: h.EndsOn, Slots: h.Slots,
		Formula: in.Formula, Lines: lines, HoldID: h.ID, Status: StatusHeld, HoldTTL: in.HoldTTL,
	}
	if err := validateCreate(&ci); err != nil {
		return nil, err
	}
	return s.createWith(ctx, tx, actor, nil, es, cust, ci)
}

// holdBookingLines maps the hold items to booking lines of pack packID.
func holdBookingLines(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, h *store.Hold, packID string) ([]LineInput, error) {
	var zoneIDs []string
	for _, it := range h.Items {
		if it.ZoneID != "" && it.SpotID == "" {
			zoneIDs = append(zoneIDs, it.ZoneID)
		}
	}
	modes := map[string]string{}
	if len(zoneIDs) > 0 {
		zones, err := store.InventoryZones(ctx, tx, es, zoneIDs)
		if err != nil {
			return nil, fmt.Errorf("booking: zones: %w", err)
		}
		for _, z := range zones {
			modes[z.ID] = z.AssignmentMode
		}
	}
	var lines []LineInput
	for _, it := range h.Items {
		switch {
		case it.SpotID != "":
			lines = append(lines, LineInput{PackID: packID, SpotID: it.SpotID, ZoneID: it.ZoneID, Quantity: 1})
		case it.ZoneID != "" && modes[it.ZoneID] == modeQuantity:
			lines = append(lines, LineInput{PackID: packID, ZoneID: it.ZoneID, Quantity: max(it.Quantity, 1)})
		case it.ZoneID != "":
			for range max(it.Quantity, 1) {
				lines = append(lines, LineInput{PackID: packID, ZoneID: it.ZoneID, Quantity: 1})
			}
		}
	}
	if len(lines) == 0 {
		return nil, configerr.Invalid("hold_id", "L'hold non contiene postazioni o zone.")
	}
	return lines, nil
}
