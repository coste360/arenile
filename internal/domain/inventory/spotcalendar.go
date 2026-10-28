package inventory

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Season calendar of a spot (spec 30.5): for every day of the season the
// state of the morning and the afternoon and the bookings involved. Read
// from spot_allocations, season_releases, spot_blocks and closures like
// the operations day (a fixed number of queries for the whole season, none
// per day); occupant labels only with bookings.read, never in SSE events.

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

// MaxCalendarDays bounds the calendar of one season.
const MaxCalendarDays = 366

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
	BookingID string
	Code      string
	Status    string
	// OccupantLabel is set only for principals with bookings.read.
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
// season containing today, or the next one, or the last one). Requires
// bookings.read or operations.run; occupant labels need bookings.read.
func (s *Service) SpotCalendar(ctx context.Context, p *auth.Principal, establishmentID, spotID, seasonID string) (*SpotCalendar, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if errors.Is(err, auth.ErrForbidden) {
		es, err = s.authorize(ctx, p, establishmentID, auth.PermOperationsRun)
	}
	if err != nil {
		return nil, err
	}
	out, err := s.spotCalendar(ctx, es, spotID, seasonID, p.Can(es, auth.PermBookingsRead))
	return out, wrap("spot calendar", err)
}

func (s *Service) spotCalendar(ctx context.Context, es store.EstablishmentScope, spotID, seasonID string, withLabels bool) (*SpotCalendar, error) {
	now := s.d.Clock.Now()
	spots, err := store.InventorySpots(ctx, s.d.DB, es, store.InventorySpotFilter{SpotIDs: []string{spotID}})
	if err != nil {
		return nil, err
	}
	if len(spots) == 0 {
		return nil, store.ErrNotFound
	}
	sp := &spots[0]
	season, err := s.calendarSeason(ctx, es, seasonID, now)
	if err != nil {
		return nil, err
	}
	from, to := season.OpensOn, season.ClosesOn
	if n := from.DaysUntil(to) + 1; n <= 0 || n > MaxCalendarDays {
		return nil, configerr.Invalid("season_id", "Stagione non valida per il calendario.")
	}
	cal, err := s.loadCalendar(ctx, es, from, to)
	if err != nil {
		return nil, err
	}
	allocs, err := store.SpotAllocationsInRange(ctx, s.d.DB, es, []string{sp.ID}, from, to, true, now)
	if err != nil {
		return nil, err
	}
	releases, err := store.SpotSeasonReleases(ctx, s.d.DB, es, sp.ID, from, to)
	if err != nil {
		return nil, err
	}
	var bookingIDs []string
	byHalfDay := make(map[availability.DateSlot]*store.SpotAllocation, len(allocs))
	for i := range allocs {
		a := &allocs[i]
		byHalfDay[availability.DateSlot{Date: a.Date, Slot: a.Slot}] = a
		if a.BookingID != nil && !slices.Contains(bookingIDs, *a.BookingID) {
			bookingIDs = append(bookingIDs, *a.BookingID)
		}
	}
	released := map[availability.DateSlot]bool{}
	for _, r := range releases {
		if r.ResoldBookingLineID == nil {
			released[availability.DateSlot{Date: r.Date, Slot: r.Slot}] = true
		}
	}
	summaries, err := store.BookingSummariesByID(ctx, s.d.DB, es, bookingIDs)
	if err != nil {
		return nil, err
	}
	out := &SpotCalendar{
		SpotID: sp.ID, Label: sp.Label, SeasonID: season.ID, From: from, To: to,
		Days: make([]CalendarDay, 0, from.DaysUntil(to)+1), Bookings: make([]CalendarBooking, 0, len(summaries)),
	}
	cell := func(ds availability.DateSlot) CalendarCell {
		a := byHalfDay[ds]
		switch {
		case !sp.Active:
			return CalendarCell{Status: CalendarInactive}
		case a != nil:
			c := CalendarCell{Status: calendarStatus(a)}
			if a.BookingID != nil {
				c.BookingID = *a.BookingID
			}
			return c
		case cal.closed[ds]:
			return CalendarCell{Status: CalendarClosed}
		case released[ds]:
			return CalendarCell{Status: CalendarReleased}
		}
		return CalendarCell{Status: CalendarFree}
	}
	for _, d := range calendar.Range(from, to) {
		out.Days = append(out.Days, CalendarDay{
			Date: d, AM: cell(availability.DateSlot{Date: d, Slot: calendar.AM}), PM: cell(availability.DateSlot{Date: d, Slot: calendar.PM}),
		})
	}
	byID := make(map[string]*store.BookingSummary, len(summaries))
	for i := range summaries {
		byID[summaries[i].ID] = &summaries[i]
	}
	for _, id := range bookingIDs { // allocation order: date, slot
		b := byID[id]
		if b == nil {
			continue
		}
		cb := CalendarBooking{
			BookingID: b.ID, Code: b.Code, Status: b.Status, StartsOn: b.StartsOn, EndsOn: b.EndsOn,
			IsSeasonPass: b.IsSeasonPass, BalanceDue: b.BalanceDue(),
		}
		if withLabels {
			cb.OccupantLabel = OccupantLabel(b.FirstName, b.LastName, b.AnonymizedAt != nil)
		}
		out.Bookings = append(out.Bookings, cb)
	}
	return out, nil
}

// calendarSeason resolves the season of the calendar.
func (s *Service) calendarSeason(ctx context.Context, es store.EstablishmentScope, seasonID string, now time.Time) (*store.Season, error) {
	if seasonID != "" {
		return store.GetSeason(ctx, s.d.DB, es, seasonID)
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return store.CalendarSeason(ctx, s.d.DB, es, calendar.In(now, loc))
}

// calendarStatus maps an allocation row to its calendar state.
func calendarStatus(a *store.SpotAllocation) string {
	switch a.Kind {
	case store.AllocationBlock:
		return CalendarBlocked
	case store.AllocationHold:
		return CalendarHeld
	case store.AllocationSeason:
		return CalendarSeasonPass
	}
	return CalendarBooked
}
