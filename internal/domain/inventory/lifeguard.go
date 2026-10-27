package inventory

import (
	"context"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Lifeguard list (spec 30.10): per row and spot the occupant's last name,
// the setup (extras with show_on_map), arrival/departure and balance due,
// built from the operations day. Never emails or phones.

// LifeguardRow is api.LifeguardRow.
type LifeguardRow struct {
	RowLabel      string
	SpotID        string
	SpotLabel     string
	OccupantLabel string
	SetupLabel    string
	GroupName     string
	Arriving      bool
	Departing     bool
	BalanceDue    bool
}

// Lifeguard is api.LifeguardList plus the establishment name of the
// printable header.
type Lifeguard struct {
	EstablishmentName string
	Locale            string
	Date              calendar.Date
	Slot              calendar.Slots
	Rows              []LifeguardRow
	Totals            OperationsTotals
}

// LifeguardList returns the occupied spots of a date and slot (mapID
// optional), in map order (row, number, label). Requires
// auth.PermBookingsRead.
func (s *Service) LifeguardList(ctx context.Context, p *auth.Principal, establishmentID string, date calendar.Date, slots calendar.Slots, mapID string) (*Lifeguard, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if slots == "" {
		slots = calendar.SlotsFull
	}
	if date.IsZero() {
		return nil, configerr.Invalid("date", "Data obbligatoria.")
	}
	if !slots.Valid() {
		return nil, configerr.Invalid("slot", "Fascia non valida.")
	}
	out, err := s.lifeguard(ctx, es, OperationsQuery{Date: date, Slot: slots, MapID: mapID})
	return out, wrap("lifeguard list", err)
}

func (s *Service) lifeguard(ctx context.Context, es store.EstablishmentScope, q OperationsQuery) (*Lifeguard, error) {
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	day, err := s.operationsDay(ctx, es, q)
	if err != nil {
		return nil, err
	}
	spots, err := store.InventorySpots(ctx, s.d.DB, es, store.InventorySpotFilter{MapID: q.MapID})
	if err != nil {
		return nil, err
	}
	rowOf := make(map[string]string, len(spots))
	for _, sp := range spots {
		rowOf[sp.ID] = sp.RowLabel
	}
	out := &Lifeguard{
		EstablishmentName: est.Name, Locale: est.DefaultLocale, Date: day.Date, Slot: day.Slot,
		Rows: []LifeguardRow{}, Totals: day.Totals,
	}
	for _, sp := range day.Spots { // map order, like InventorySpots
		b := sp.Booking
		if b == nil {
			continue
		}
		out.Rows = append(out.Rows, LifeguardRow{
			RowLabel: rowOf[sp.SpotID], SpotID: sp.SpotID, SpotLabel: sp.Label, OccupantLabel: b.OccupantLabel,
			SetupLabel: b.SetupLabel, GroupName: b.GroupName, Arriving: b.IsFirstDay, Departing: b.IsLastDay,
			BalanceDue: b.BalanceDue > 0,
		})
	}
	return out, nil
}
