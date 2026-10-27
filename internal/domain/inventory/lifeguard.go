package inventory

import (
	"context"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
)

// Lifeguard list (spec 30.10): per row and spot the occupant's last name,
// the setup (extras with show_on_map), arrival/departure and balance due,
// built from the operations day. Never emails or phones. Owner: the
// operational map builder.

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

// LifeguardList returns the rows for a date and slot (mapID optional).
// Requires auth.PermBookingsRead.
func (s *Service) LifeguardList(ctx context.Context, p *auth.Principal, establishmentID string, date calendar.Date, slots calendar.Slots, mapID string) ([]LifeguardRow, error) {
	_, _, _, _, _, _ = ctx, p, establishmentID, date, slots, mapID
	return nil, configerr.ErrNotImplemented
}
