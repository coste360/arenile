package setup

import (
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
)

// Default season dates: 1 May - 30 September.
func defaultSeasonDates(year int) (opens, closes calendar.Date) {
	return calendar.New(year, time.May, 1), calendar.New(year, time.September, 30)
}

// periodOf classifies a day of the season:
//   - low:  May and from 16 September (and any day before May / after September)
//   - mid:  June and 1-15 September
//   - high: July
//   - peak: August (Ferragosto)
func periodOf(d calendar.Date) string {
	switch d.Month() {
	case time.June:
		return PeriodMid
	case time.July:
		return PeriodHigh
	case time.August:
		return PeriodPeak
	case time.September:
		if d.Day() <= 15 {
			return PeriodMid
		}
	}
	return PeriodLow
}

// periodRanges splits [opens, closes] into the ranges of each rate period.
// Every day of the season belongs to exactly one period, so base rules
// conditioned on the periods cover the whole season (no coverage gaps).
// Periods without days are omitted.
func periodRanges(opens, closes calendar.Date) map[string][]store.DateRange {
	out := map[string][]store.DateRange{}
	var (
		cur  string
		from calendar.Date
	)
	days := calendar.Range(opens, closes)
	for i, d := range days {
		p := periodOf(d)
		if i == 0 {
			cur, from = p, d
			continue
		}
		if p != cur {
			out[cur] = append(out[cur], store.DateRange{From: from, To: days[i-1]})
			cur, from = p, d
		}
	}
	if len(days) > 0 {
		out[cur] = append(out[cur], store.DateRange{From: from, To: days[len(days)-1]})
	}
	return out
}
