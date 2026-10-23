package report

import (
	"cmp"
	"slices"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Functional core of the bookings, cancellations and season passes reports.
// The inputs are the slim booking rows of store.ReportBookings (sold
// statuses only) and the releases of store.ReportReleases.

const (
	statusCancelled = "cancelled"
	statusNoShow    = "no_show"
)

// seededRows keeps rows in insertion order, creating them on first use.
type seededRows[R any] struct {
	rows  map[string]*R
	order []string
	mk    func(key string) *R
}

func newSeededRows[R any](mk func(key string) *R) *seededRows[R] {
	return &seededRows[R]{rows: map[string]*R{}, mk: mk}
}

func (s *seededRows[R]) get(key string) *R {
	if r, ok := s.rows[key]; ok {
		return r
	}
	r := s.mk(key)
	s.rows[key] = r
	s.order = append(s.order, key)
	return r
}

// sortKeys orders the keys with less (after seeding).
func (s *seededRows[R]) sortKeys(less func(a, b string) int) { slices.SortStableFunc(s.order, less) }

func (s *seededRows[R]) list() []R {
	out := make([]R, 0, len(s.order))
	for _, k := range s.order {
		out = append(out, *s.rows[k])
	}
	return out
}

// positionIn orders keys by their index in order (unknown keys last).
func positionIn(order []string) func(a, b string) int {
	pos := func(k string) int {
		if i := slices.Index(order, k); i >= 0 {
			return i
		}
		return len(order)
	}
	return func(a, b string) int { return cmp.Or(cmp.Compare(pos(a), pos(b)), cmp.Compare(a, b)) }
}

// leadDays is starts_on minus the sale date, never negative.
func leadDays(b *store.ReportBooking, sale calendar.Date) int {
	return max(sale.DaysUntil(b.StartsOn), 0)
}

// buildBookings aggregates the bookings sold in p (created_at in loc) by
// groupBy. Lead-time groups always list every bucket and day groups every
// day of the period.
func buildBookings(p Period, bookings []store.ReportBooking, groupBy BookingsGroupBy, loc *time.Location) *Bookings {
	type acc struct {
		row  BookingsRow
		lead int
	}
	rows := newSeededRows(func(k string) *acc {
		r := &acc{row: BookingsRow{Key: k, Label: k}}
		switch groupBy {
		case BookingsByChannel:
			r.row.Label = labelOr(channelLabels, k)
		case BookingsByLeadTime:
			r.row.Label = labelOr(leadLabels, LeadTimeBucket(k))
		case BookingsByFormula:
			r.row.Label = labelOr(formulaLabels, k)
		default:
			if d, err := calendar.Parse(k); err == nil {
				r.row.Label = dayLabel(d)
			}
		}
		return r
	})
	switch groupBy {
	case BookingsByLeadTime:
		for _, b := range LeadTimeBuckets {
			rows.get(string(b))
		}
	case BookingsByDay:
		for _, d := range calendar.Range(p.From, p.To) {
			rows.get(d.String())
		}
	}
	tot := acc{row: BookingsRow{Key: "total", Label: "Totale"}}
	for i := range bookings {
		b := &bookings[i]
		sale, ok := soldOn(b, p, loc)
		if !ok {
			continue
		}
		lead := leadDays(b, sale)
		var key string
		switch groupBy {
		case BookingsByChannel:
			key = b.Channel
		case BookingsByLeadTime:
			key = string(LeadTimeBucketOf(lead))
		case BookingsByFormula:
			key = b.Formula
		default:
			key = sale.String()
		}
		for _, r := range []*acc{rows.get(key), &tot} {
			r.row.Bookings++
			r.row.Value += b.TotalAmount
			r.lead += lead
		}
	}
	switch groupBy {
	case BookingsByChannel:
		rows.sortKeys(positionIn(channelOrder))
	case BookingsByFormula:
		rows.sortKeys(positionIn(formulaOrder))
	}
	finish := func(r *acc) BookingsRow {
		if r.row.Bookings > 0 {
			r.row.AverageValue = r.row.Value / money.Cents(r.row.Bookings)
			r.row.AverageLeadDays = r.lead / r.row.Bookings
		}
		r.row.ShareBP = Ratio(int64(r.row.Bookings), int64(tot.row.Bookings))
		return r.row
	}
	out := &Bookings{Period: p, GroupBy: groupBy, Rows: make([]BookingsRow, 0, len(rows.order))}
	for _, k := range rows.order {
		out.Rows = append(out.Rows, finish(rows.rows[k]))
	}
	out.Totals = finish(&tot)
	return out
}

// buildCancellations aggregates the bookings starting in p by groupBy: all
// sold bookings are the denominator; LostValue is the value of the
// cancelled ones minus what was retained (net paid amount), Refunded the
// succeeded refunds of the cancelled ones.
func buildCancellations(p Period, bookings []store.ReportBooking, groupBy CancellationsGroupBy) *Cancellations {
	rows := newSeededRows(func(k string) *CancellationsRow {
		r := &CancellationsRow{Key: k, Label: k}
		switch groupBy {
		case CancellationsByChannel:
			r.Label = labelOr(channelLabels, k)
		case CancellationsByMonth:
			if d, err := calendar.Parse(k + "-01"); err == nil {
				r.Label = monthLabel(d)
			}
		default:
			if d, err := calendar.Parse(k); err == nil {
				r.Label = dayLabel(d)
			}
		}
		return r
	})
	switch groupBy {
	case CancellationsByDay:
		for _, d := range calendar.Range(p.From, p.To) {
			rows.get(d.String())
		}
	case CancellationsByMonth:
		for _, d := range calendar.Range(p.From, p.To) {
			rows.get(MonthKey(d))
		}
	}
	tot := CancellationsRow{Key: "total", Label: "Totale"}
	for i := range bookings {
		b := &bookings[i]
		if !p.Contains(b.StartsOn) {
			continue
		}
		var key string
		switch groupBy {
		case CancellationsByChannel:
			key = b.Channel
		case CancellationsByMonth:
			key = MonthKey(b.StartsOn)
		default:
			key = b.StartsOn.String()
		}
		for _, r := range []*CancellationsRow{rows.get(key), &tot} {
			r.Bookings++
			switch b.Status {
			case statusCancelled:
				r.Cancelled++
				r.LostValue += max(b.TotalAmount-b.PaidAmount, 0)
				r.Refunded += b.Refunded
			case statusNoShow:
				r.NoShows++
			}
		}
	}
	if groupBy == CancellationsByChannel {
		rows.sortKeys(positionIn(channelOrder))
	}
	finish := func(r CancellationsRow) CancellationsRow {
		r.CancelledBP = Ratio(int64(r.Cancelled), int64(r.Bookings))
		r.NoShowBP = Ratio(int64(r.NoShows), int64(r.Bookings))
		return r
	}
	out := &Cancellations{Period: p, GroupBy: groupBy, Rows: make([]CancellationsRow, 0, len(rows.order))}
	for _, r := range rows.list() {
		out.Rows = append(out.Rows, finish(r))
	}
	out.Totals = finish(tot)
	return out
}

// passesSold counts the season passes in a booked status confirmed in p.
func passesSold(p Period, bookings []store.ReportBooking, loc *time.Location) (sold int, value money.Cents, renewals int) {
	for i := range bookings {
		b := &bookings[i]
		if !b.IsSeasonPass {
			continue
		}
		if _, ok := bookedOn(b, p, loc); !ok {
			continue
		}
		sold++
		value += b.TotalAmount
		if b.RenewalOf != nil {
			renewals++
		}
	}
	return sold, value, renewals
}

// resaleShares prorates the total of every resold booking line over its
// allocated units: each unit gets total/units, and the remainder cents go
// one each to the earliest units of the line in releases (ordered by date
// and slot), so the shares of a fully released line add up to its total.
func resaleShares(releases []store.ReportRelease) []money.Cents {
	out := make([]money.Cents, len(releases))
	seen := map[string]int64{}
	for i, r := range releases {
		if r.ResoldLineID == nil || r.LineUnits <= 0 {
			continue
		}
		n := seen[*r.ResoldLineID]
		seen[*r.ResoldLineID] = n + 1
		base, rem := int64(r.LineTotal)/r.LineUnits, int64(r.LineTotal)%r.LineUnits
		out[i] = money.Cents(base)
		if n < rem {
			out[i]++
		}
	}
	return out
}

// buildSeasonPasses makes the season passes report: passes sold in p and
// the releases dated in p grouped by groupBy.
func buildSeasonPasses(p Period, bookings []store.ReportBooking, releases []store.ReportRelease, zones []store.ReportZone, groupBy SeasonPassesGroupBy, loc *time.Location) *SeasonPasses {
	zoneByID := map[string]store.ReportZone{}
	for _, z := range zones {
		zoneByID[z.ID] = z
	}
	rows := newSeededRows(func(k string) *SeasonPassesRow {
		r := &SeasonPassesRow{Key: k, Label: k}
		switch groupBy {
		case SeasonPassesByZone:
			z := zoneByID[k]
			r.Label = i18nName(z.Name, z.Code)
		case SeasonPassesByMonth:
			if d, err := calendar.Parse(k + "-01"); err == nil {
				r.Label = monthLabel(d)
			}
		default:
			if d, err := calendar.Parse(k); err == nil {
				r.Label = dayLabel(d)
			}
		}
		return r
	})
	switch groupBy {
	case SeasonPassesByDay:
		for _, d := range calendar.Range(p.From, p.To) {
			rows.get(d.String())
		}
	case SeasonPassesByMonth:
		for _, d := range calendar.Range(p.From, p.To) {
			rows.get(MonthKey(d))
		}
	}
	shares := resaleShares(releases)
	tot := SeasonPassesRow{Key: "total", Label: "Totale"}
	for i, rl := range releases {
		if !p.Contains(rl.Date) {
			continue
		}
		var key string
		switch groupBy {
		case SeasonPassesByZone:
			key = rl.ZoneID
		case SeasonPassesByMonth:
			key = MonthKey(rl.Date)
		default:
			key = rl.Date.String()
		}
		for _, r := range []*SeasonPassesRow{rows.get(key), &tot} {
			r.ReleasedUnits++
			r.CreditsIssued += rl.CreditAmount
			if rl.ResoldLineID != nil {
				r.ResoldUnits++
				r.ResaleRevenue += shares[i]
			}
		}
	}
	if groupBy == SeasonPassesByZone {
		rows.sortKeys(func(a, b string) int {
			za, zb := zoneByID[a], zoneByID[b]
			return cmp.Or(cmp.Compare(za.SortOrder, zb.SortOrder), cmp.Compare(za.Code, zb.Code), cmp.Compare(a, b))
		})
	}
	out := &SeasonPasses{Period: p, GroupBy: groupBy, Rows: make([]SeasonPassesRow, 0, len(rows.order))}
	for _, r := range rows.list() {
		r.ResaleBP = Ratio(r.ResoldUnits, r.ReleasedUnits)
		out.Rows = append(out.Rows, r)
	}
	tot.ResaleBP = Ratio(tot.ResoldUnits, tot.ReleasedUnits)
	out.Totals = tot
	out.PassesSold, out.PassesValue, out.Renewals = passesSold(p, bookings, loc)
	return out
}
