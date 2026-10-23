package report

import (
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Functional core of the KPI summary and of the year comparison: both are
// built from the per-day facts of a span, so the two spans of a comparison
// are measured with exactly the same definitions as the summary and the
// detailed reports.

// dayFacts are the metrics of one calendar day.
type dayFacts struct {
	capacity, occupied int64
	collected, booked  money.Cents
	bookings           int
	bookingsValue      money.Cents
	cancellations      int
	noShows            int
	passes             int
	released, resold   int64
}

func (f *dayFacts) add(o *dayFacts) {
	f.capacity += o.capacity
	f.occupied += o.occupied
	f.collected += o.collected
	f.booked += o.booked
	f.bookings += o.bookings
	f.bookingsValue += o.bookingsValue
	f.cancellations += o.cancellations
	f.noShows += o.noShows
	f.passes += o.passes
	f.released += o.released
	f.resold += o.resold
}

// spanFacts are the facts of every day of a period (index = days from
// period.From).
type spanFacts struct {
	period Period
	days   []dayFacts
}

// factsInput is what buildFacts needs, loaded by the shell.
type factsInput struct {
	period    Period
	occupancy *Occupancy // grouped by day
	payments  []store.ReportPayment
	bookings  []store.ReportBooking
	releases  []store.ReportRelease
	loc       *time.Location
}

// buildFacts computes the per-day facts with the definitions of the
// detailed reports: occupancy by day, collected revenue by business date
// (refunds negative), booked revenue and season passes by confirmation
// date, bookings by sale date, cancellations and no-shows by first day,
// releases by release date.
func buildFacts(in factsInput) *spanFacts {
	p := in.period
	sf := &spanFacts{period: p, days: make([]dayFacts, p.Days())}
	index := make(map[string]int, len(sf.days))
	for i := range sf.days {
		index[p.From.AddDays(i).String()] = i
	}
	day := func(key string) *dayFacts {
		if i, ok := index[key]; ok {
			return &sf.days[i]
		}
		return nil
	}
	if in.occupancy != nil {
		for _, r := range in.occupancy.Rows {
			if f := day(r.Key); f != nil {
				f.capacity += r.CapacityUnits
				f.occupied += r.OccupiedUnits
			}
		}
	}
	for i := range in.payments {
		pm := &in.payments[i]
		if f := day(pm.BusinessDate.String()); f != nil {
			if pm.Direction == store.PaymentDirectionRefund {
				f.collected -= pm.Amount
			} else {
				f.collected += pm.Amount
			}
		}
	}
	for i := range in.bookings {
		b := &in.bookings[i]
		if d, ok := bookedOn(b, p, in.loc); ok {
			f := day(d.String())
			f.booked += b.TotalAmount
			if b.IsSeasonPass {
				f.passes++
			}
		}
		if d, ok := soldOn(b, p, in.loc); ok {
			f := day(d.String())
			f.bookings++
			f.bookingsValue += b.TotalAmount
		}
		if f := day(b.StartsOn.String()); f != nil {
			switch b.Status {
			case statusCancelled:
				f.cancellations++
			case statusNoShow:
				f.noShows++
			}
		}
	}
	for _, rl := range in.releases {
		if f := day(rl.Date.String()); f != nil {
			f.released++
			if rl.ResoldLineID != nil {
				f.resold++
			}
		}
	}
	return sf
}

// total sums the facts of days [from, to) (indexes clamped to the span).
func (sf *spanFacts) total(from, to int) dayFacts {
	var t dayFacts
	for i := max(from, 0); i < min(to, len(sf.days)); i++ {
		t.add(&sf.days[i])
	}
	return t
}

func (sf *spanFacts) all() dayFacts { return sf.total(0, len(sf.days)) }

func averageValue(f dayFacts) money.Cents {
	if f.bookings == 0 {
		return 0
	}
	return f.bookingsValue / money.Cents(f.bookings)
}

// buildSummary is the KPI set of a span plus its open balances.
func buildSummary(sf *spanFacts, open *OpenBalances, now time.Time) *Summary {
	t := sf.all()
	s := &Summary{
		Period: sf.period, OccupiedUnits: t.occupied, CapacityUnits: t.capacity, OccupancyBP: Ratio(t.occupied, t.capacity),
		RevenueCollected: t.collected, RevenueBooked: t.booked, Bookings: t.bookings, AverageBookingValue: averageValue(t),
		Cancellations: t.cancellations, NoShows: t.noShows, SeasonPassesSold: t.passes, GeneratedAt: now,
	}
	if open != nil {
		s.OpenBalanceTotal = open.TotalBalance
		s.OpenBalanceBookings = len(open.Rows)
	}
	return s
}

// metric builds one comparison metric.
func metric(key, unit string, cur, prev int64) ComparisonMetric {
	return ComparisonMetric{Metric: key, Unit: unit, Current: cur, Previous: prev, Delta: cur - prev, DeltaBP: DeltaBP(cur, prev)}
}

// buildComparison compares the facts of the current and previous spans.
// The series buckets the current span by granularity; previous day j is
// aligned with current day j (the two spans start on the aligned days), and
// when the previous span is one day longer (29 February) its extra day is
// added to the last bucket, so the series always adds up to the metrics.
func buildComparison(cur, prev *spanFacts, granularity Granularity, align Align) *Comparison {
	c, p := cur.all(), prev.all()
	out := &Comparison{
		Current: cur.period, Previous: prev.period, Granularity: granularity, Align: align,
		Metrics: []ComparisonMetric{
			metric(MetricOccupancyBP, UnitBasisPoints, Ratio(c.occupied, c.capacity), Ratio(p.occupied, p.capacity)),
			metric(MetricRevenueCollected, UnitCents, int64(c.collected), int64(p.collected)),
			metric(MetricRevenueBooked, UnitCents, int64(c.booked), int64(p.booked)),
			metric(MetricBookings, UnitCount, int64(c.bookings), int64(p.bookings)),
			metric(MetricAverageValue, UnitCents, int64(averageValue(c)), int64(averageValue(p))),
			metric(MetricCancellations, UnitCount, int64(c.cancellations), int64(p.cancellations)),
			metric(MetricNoShows, UnitCount, int64(c.noShows), int64(p.noShows)),
			metric(MetricSeasonPassesSold, UnitCount, int64(c.passes), int64(p.passes)),
			metric(MetricReleasedUnits, UnitCount, c.released, p.released),
			metric(MetricResoldUnits, UnitCount, c.resold, p.resold),
		},
		Series: []ComparisonPoint{},
	}
	n := len(cur.days)
	start := 0
	for start < n {
		d := cur.period.From.AddDays(start)
		end := start + 1
		for end < n && bucketKey(cur.period.From.AddDays(end), granularity) == bucketKey(d, granularity) {
			end++
		}
		prevEnd := end
		if end == n {
			prevEnd = len(prev.days) // the extra previous day, if any
		}
		ct, pt := cur.total(start, end), prev.total(start, prevEnd)
		key, label := bucketKey(d, granularity), bucketLabel(d, granularity)
		pf := prev.period.From.AddDays(min(start, len(prev.days)-1))
		out.Series = append(out.Series, ComparisonPoint{
			Key: key, Label: label, CurrentFrom: d, PreviousFrom: pf,
			CurrentRevenue: ct.collected, PreviousRevenue: pt.collected,
			CurrentOccupancyBP: Ratio(ct.occupied, ct.capacity), PreviousOccupancyBP: Ratio(pt.occupied, pt.capacity),
			CurrentBookings: ct.bookings, PreviousBookings: pt.bookings,
		})
		start = end
	}
	return out
}

// bucketKey is the series key of d: the date, the ISO week or the month.
func bucketKey(d calendar.Date, g Granularity) string {
	switch g {
	case GranularityDay:
		return d.String()
	case GranularityMonth:
		return MonthKey(d)
	default:
		return WeekKey(d)
	}
}

func bucketLabel(d calendar.Date, g Granularity) string {
	switch g {
	case GranularityDay:
		return dayLabel(d)
	case GranularityMonth:
		return monthLabel(d)
	default:
		return weekLabel(d)
	}
}
