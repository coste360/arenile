package inventory

import (
	"context"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
)

// Season calendar of a spot (spec 30.5): for every day of the season the
// state of the morning and the afternoon and the bookings involved. Read
// from spot_allocations, season_releases, spot_blocks and closures like
// the operations day; occupant labels only with bookings.read, never in SSE
// events. Owner: the operational map builder.

// Spot calendar cell states (api.SpotCalendarStatus).
const (
	CalendarFree       = "free"
	CalendarBooked     = "booked"
	CalendarSeasonPass = "season_pass"
	CalendarReleased   = "released"
	CalendarHeld       = "held"
	CalendarBlocked    = "blocked"
	CalendarClosed     = "closed"
	CalendarInactive   = "inactive"
)

// CalendarCell is one half-day of the calendar.
type CalendarCell struct {
	Status    string
	BookingID string
}

// CalendarDay is one day of the calendar.
type CalendarDay struct {
	Date   calendar.Date
	AM, PM CalendarCell
}

// CalendarBooking is a booking shown in the calendar.
type CalendarBooking struct {
	BookingID     string
	Code          string
	Status        string
	OccupantLabel string
	StartsOn      calendar.Date
	EndsOn        calendar.Date
	IsSeasonPass  bool
	BalanceDue    money.Cents
}

// SpotCalendar is api.SpotCalendar.
type SpotCalendar struct {
	SpotID   string
	Label    string
	SeasonID string
	From, To calendar.Date
	Days     []CalendarDay
	Bookings []CalendarBooking
}

// SpotCalendar returns the season calendar of a spot (seasonID empty = the
// season containing today, or the next one). Requires auth.PermBookingsRead.
func (s *Service) SpotCalendar(ctx context.Context, p *auth.Principal, establishmentID, spotID, seasonID string) (*SpotCalendar, error) {
	_, _, _, _, _ = ctx, p, establishmentID, spotID, seasonID
	return nil, configerr.ErrNotImplemented
}
