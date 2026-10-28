package inventory

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/store"
)

// Operational map additions of M8 (spec 30.4, ADR 0028): arrival and
// departure flags, the days the booking line spends on the spot (split
// stays), the setup from the extras with show_on_map ("2L 1S"), the
// agreement or partner badge, the group and the party size; and, for the
// range view, the partially free spots ("N giorni su M"). Admin API only:
// none of this goes into SSE events (the map refetches on an event).

// SetupItem is one element of the spot setup (an extra with show_on_map).
type SetupItem struct {
	ExtraID  string
	MapLabel string
	Quantity int
}

// AgreementRef is the agreement badge of a booking.
type AgreementRef struct {
	ID          string
	Name        string
	Code        string
	PartnerID   string
	PartnerName string
	Requirement string
}

// PartnerRef is the partner badge of a booking.
type PartnerRef struct {
	ID   string
	Name string
	Kind string
}

// OperationalMarks are the M8 fields of an OperationalBooking (embedded).
type OperationalMarks struct {
	// IsFirstDay / IsLastDay: the day is the first (arrival) or the last
	// (departure) of the booking.
	IsFirstDay bool
	IsLastDay  bool
	// SegmentStartsOn / SegmentEndsOn: the run of consecutive days of the
	// line on this spot, set only when the line occupies more than one
	// spot (split stay, scheduled move, swap).
	SegmentStartsOn *calendar.Date
	SegmentEndsOn   *calendar.Date
	Setup           []SetupItem
	SetupLabel      string
	Agreement       *AgreementRef
	Partner         *PartnerRef
	GroupName       string
	// PartySize is adults + children, nil when neither is known.
	PartySize *int
}

// applyMarks fills the OperationalMarks of the bookings of day (four
// queries for the whole day, none per spot).
func (s *Service) applyMarks(ctx context.Context, es store.EstablishmentScope, day *OperationsDay) error {
	var bookingIDs, lineIDs []string
	for i := range day.Spots {
		b := day.Spots[i].Booking
		if b == nil {
			continue
		}
		bookingIDs = append(bookingIDs, b.BookingID)
		if b.LineID != "" {
			lineIDs = append(lineIDs, b.LineID)
		}
		b.IsFirstDay = day.Date.Equal(b.StartsOn)
		b.IsLastDay = day.Date.Equal(b.EndsOn)
	}
	if len(bookingIDs) == 0 {
		return nil
	}
	bookingIDs = slices.Compact(slices.Sorted(slices.Values(bookingIDs)))
	lineIDs = slices.Compact(slices.Sorted(slices.Values(lineIDs)))

	badges, err := store.BookingBadges(ctx, s.d.DB, es, bookingIDs)
	if err != nil {
		return err
	}
	byBooking := make(map[string]*store.BookingBadge, len(badges))
	for i := range badges {
		byBooking[badges[i].ID] = &badges[i]
	}
	extras, err := store.SetupExtrasByBooking(ctx, s.d.DB, es, bookingIDs)
	if err != nil {
		return err
	}
	setup, err := s.setupByLine(ctx, es, extras)
	if err != nil {
		return err
	}
	split, err := store.SplitLineSpotDays(ctx, s.d.DB, es, lineIDs)
	if err != nil {
		return err
	}
	for i := range day.Spots {
		sp := &day.Spots[i]
		b := sp.Booking
		if b == nil {
			continue
		}
		if bg := byBooking[b.BookingID]; bg != nil {
			applyBadge(&b.OperationalMarks, bg)
		}
		b.Setup = setup[setupKey{b.BookingID, b.LineID}]
		b.SetupLabel = SetupLabel(b.Setup)
		if from, to, ok := spotSegment(split, b.LineID, sp.SpotID, day.Date); ok {
			b.SegmentStartsOn, b.SegmentEndsOn = &from, &to
		}
	}
	return nil
}

func applyBadge(m *OperationalMarks, bg *store.BookingBadge) {
	m.GroupName = bg.GroupName
	if bg.PartyAdults != nil || bg.PartyChildren != nil {
		n := deref(bg.PartyAdults) + deref(bg.PartyChildren)
		m.PartySize = &n
	}
	if bg.AgreementID != nil && bg.AgreementName != nil {
		m.Agreement = &AgreementRef{
			ID: *bg.AgreementID, Name: *bg.AgreementName, Code: derefStr(bg.AgreementCode),
			PartnerID: derefStr(bg.AgreementPartnerID), PartnerName: derefStr(bg.AgreementPartnerName),
			Requirement: derefStr(bg.AgreementRequirement),
		}
	}
	if bg.PartnerID != nil && bg.PartnerName != nil {
		m.Partner = &PartnerRef{ID: *bg.PartnerID, Name: *bg.PartnerName, Kind: derefStr(bg.PartnerKind)}
	}
}

// setupKey identifies the setup of a booking line.
type setupKey struct{ bookingID, lineID string }

// setupByLine folds the setup extras per booking line: line extras on
// their line, booking-level extras on the first line of the booking (as
// the allocation does with their components).
func (s *Service) setupByLine(ctx context.Context, es store.EstablishmentScope, extras []store.SetupExtra) (map[setupKey][]SetupItem, error) {
	out := map[setupKey][]SetupItem{}
	if len(extras) == 0 {
		return out, nil
	}
	var bookingLevel []string
	for _, x := range extras {
		if x.BookingLineID == nil {
			bookingLevel = append(bookingLevel, x.BookingID)
		}
	}
	first := map[string]string{}
	if len(bookingLevel) > 0 {
		lines, err := store.LinePacksByBooking(ctx, s.d.DB, es, slices.Compact(slices.Sorted(slices.Values(bookingLevel))))
		if err != nil {
			return nil, err
		}
		for _, l := range lines { // ordered by booking, sort_order
			if _, ok := first[l.BookingID]; !ok {
				first[l.BookingID] = l.LineID
			}
		}
	}
	for _, x := range extras {
		line := first[x.BookingID]
		if x.BookingLineID != nil {
			line = *x.BookingLineID
		}
		label := strings.TrimSpace(x.MapLabel)
		if label == "" {
			label = x.Code
		}
		k := setupKey{x.BookingID, line}
		items := out[k]
		if j := slices.IndexFunc(items, func(it SetupItem) bool { return it.ExtraID == x.ExtraID }); j >= 0 {
			items[j].Quantity += x.Quantity
		} else {
			items = append(items, SetupItem{ExtraID: x.ExtraID, MapLabel: label, Quantity: x.Quantity})
		}
		out[k] = items
	}
	for k := range out {
		slices.SortFunc(out[k], func(a, b SetupItem) int {
			return cmp.Or(cmp.Compare(a.MapLabel, b.MapLabel), cmp.Compare(a.ExtraID, b.ExtraID))
		})
	}
	return out, nil
}

// SetupLabel is the compact setup of a spot (spec 30.4): quantity and map
// label of each item, space separated ("2L 1S"); empty when no item. Pure.
func SetupLabel(items []SetupItem) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		if it.Quantity <= 0 || it.MapLabel == "" {
			continue
		}
		parts = append(parts, strconv.Itoa(it.Quantity)+it.MapLabel)
	}
	return strings.Join(parts, " ")
}

// spotSegment returns the run of consecutive dates around date that the
// line spends on spotID (rows sorted by line and date). ok is false when
// the line is not split. Pure.
func spotSegment(rows []store.LineSpotDay, lineID, spotID string, date calendar.Date) (from, to calendar.Date, ok bool) {
	if lineID == "" {
		return from, to, false
	}
	var dates []calendar.Date
	for _, r := range rows {
		if r.BookingLineID == lineID && r.SpotID == spotID {
			dates = append(dates, r.Date)
		}
	}
	i := slices.IndexFunc(dates, func(d calendar.Date) bool { return d.Equal(date) })
	if i < 0 {
		return from, to, false
	}
	lo, hi := i, i
	for lo > 0 && dates[lo-1].AddDays(1).Equal(dates[lo]) {
		lo--
	}
	for hi < len(dates)-1 && dates[hi].AddDays(1).Equal(dates[hi+1]) {
		hi++
	}
	return dates[lo], dates[hi], true
}

// freeDays counts the dates of the range on which a spot is free for every
// requested open half-day (spec 30.4 range view, "libero in parte": N days
// out of M). total is the number of open dates (closures excluded). Pure.
func freeDays(open []availability.DateSlot, isFree func(availability.DateSlot) bool) (free []calendar.Date, total int) {
	byDate := map[calendar.Date]bool{}
	var order []calendar.Date
	for _, ds := range open {
		ok, seen := byDate[ds.Date]
		if !seen {
			order = append(order, ds.Date)
			ok = true
		}
		byDate[ds.Date] = ok && isFree(ds)
	}
	free = []calendar.Date{}
	for _, d := range order {
		if byDate[d] {
			free = append(free, d)
		}
	}
	return free, len(order)
}

func deref(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
