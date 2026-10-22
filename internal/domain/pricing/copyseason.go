package pricing

import (
	"fmt"
	"slices"
	"sort"

	"github.com/coste360/arenile/internal/calendar"
)

// CopySeasonInput is the price list of a season to duplicate.
type CopySeasonInput struct {
	FromYear    int                  `json:"from_year"`
	ToYear      int                  `json:"to_year"`
	Rules       []Rule               `json:"rules"`
	RatePeriods []RatePeriod         `json:"rate_periods"`
	FixedPrices []FixedDurationPrice `json:"fixed_prices"`
	// NewID returns the ID of the copy of an object (kind is "rule",
	// "rate_period" or "fixed_price"). Nil keeps the old IDs; the caller then
	// assigns IDs when persisting. Keeping ID generation outside keeps the
	// function pure.
	NewID func(kind, oldID string) string `json:"-"`
}

// CopySeasonResult is the duplicated price list.
type CopySeasonResult struct {
	Rules       []Rule               `json:"rules"`
	RatePeriods []RatePeriod         `json:"rate_periods"`
	FixedPrices []FixedDurationPrice `json:"fixed_prices"`
	// Warnings lists situations the manager should review (overlapping
	// periods or adjacent ranges that are no longer adjacent).
	Warnings []string `json:"warnings"`
}

// CopySeason duplicates rules, rate periods and fixed prices from FromYear to
// ToYear, shifting every date range endpoint with ShiftDate. Rules keep their
// priority, conditions and Seq; only date_ranges change.
func CopySeason(in CopySeasonInput) (CopySeasonResult, error) {
	if in.FromYear <= 0 || in.ToYear <= 0 || in.FromYear == in.ToYear {
		return CopySeasonResult{}, invalidInput("copy season: invalid years %d -> %d", in.FromYear, in.ToYear)
	}
	newID := in.NewID
	if newID == nil {
		newID = func(_, old string) string { return old }
	}
	years := in.ToYear - in.FromYear
	out := CopySeasonResult{
		Rules:       make([]Rule, 0, len(in.Rules)),
		RatePeriods: make([]RatePeriod, 0, len(in.RatePeriods)),
		FixedPrices: make([]FixedDurationPrice, 0, len(in.FixedPrices)),
		Warnings:    []string{},
	}
	for _, r := range in.Rules {
		c := r
		c.ID = newID("rule", r.ID)
		c.TargetIDs = slices.Clone(r.TargetIDs)
		c.Conditions = cloneConditions(r.Conditions)
		c.Conditions.DateRanges = shiftRanges(r.Conditions.DateRanges, years)
		out.Rules = append(out.Rules, c)
	}
	for _, p := range in.RatePeriods {
		out.RatePeriods = append(out.RatePeriods, RatePeriod{
			ID: newID("rate_period", p.ID), Code: p.Code, Ranges: shiftRanges(p.Ranges, years),
		})
	}
	for _, fp := range in.FixedPrices {
		c := fp
		c.ID = newID("fixed_price", fp.ID)
		if fp.RatePeriodCode != nil {
			code := *fp.RatePeriodCode
			c.RatePeriodCode = &code
		}
		out.FixedPrices = append(out.FixedPrices, c)
	}
	out.Warnings = append(out.Warnings, periodWarnings(in.RatePeriods, out.RatePeriods)...)
	return out, nil
}

func cloneConditions(c Conditions) Conditions {
	out := c
	out.RatePeriods = slices.Clone(c.RatePeriods)
	out.Weekdays = slices.Clone(c.Weekdays)
	out.Slots = slices.Clone(c.Slots)
	out.Formulas = slices.Clone(c.Formulas)
	out.PriceCategories = slices.Clone(c.PriceCategories)
	out.ZoneIDs = slices.Clone(c.ZoneIDs)
	out.SpotAttributes = slices.Clone(c.SpotAttributes)
	out.Channels = slices.Clone(c.Channels)
	out.CustomerTags = slices.Clone(c.CustomerTags)
	out.AgreementIDs = slices.Clone(c.AgreementIDs)
	return out
}

func shiftRanges(rs []DateRange, years int) []DateRange {
	if rs == nil {
		return nil
	}
	out := make([]DateRange, len(rs))
	for i, r := range rs {
		from, to := ShiftDate(r.From, years, false), ShiftDate(r.To, years, true)
		if to.Before(from) {
			// Mixed anchoring inverted a short range: fall back to the same
			// calendar dates for both endpoints.
			from, to = sameCalendarDate(r.From, years), sameCalendarDate(r.To, years)
		}
		out[i] = DateRange{From: from, To: to}
	}
	return out
}

// Anchor kinds used by ShiftDate.
const (
	AnchorWeekday  = "weekday"
	AnchorCalendar = "calendar"
	AnchorEaster   = "easter"
)

// ShiftDate moves a range endpoint by years. The rule, applied in order:
//
//  1. Easter anchor: an endpoint from the Saturday before Easter to the
//     Tuesday after it keeps its offset from Easter (Easter is always a
//     Sunday, so the weekday is preserved too).
//  2. Calendar anchor: the same day and month (Feb 29 -> Feb 28) when the
//     endpoint is a fixed Italian national holiday (e.g. Ferragosto), or a
//     start endpoint on the 1st of a month or the day after a fixed holiday,
//     or an end endpoint on the last day of a month or the day before a fixed
//     holiday. These dates are meaningful as calendar dates, and anchoring
//     both sides of a boundary keeps adjacent periods adjacent.
//  3. Weekday anchor: otherwise the date moves by the whole number of weeks
//     closest to the calendar shift (364 days for one year), so a period that
//     starts on a Saturday still starts on a Saturday.
func ShiftDate(d calendar.Date, years int, isEnd bool) calendar.Date {
	switch anchorOf(d, isEnd) {
	case AnchorEaster:
		off := calendar.Easter(d.Year()).DaysUntil(d)
		return calendar.Easter(d.Year() + years).AddDays(off)
	case AnchorCalendar:
		return sameCalendarDate(d, years)
	}
	delta := d.DaysUntil(sameCalendarDate(d, years))
	weeks := delta / 7
	if rem := delta - weeks*7; rem > 3 || (rem < -3) {
		if rem > 0 {
			weeks++
		} else {
			weeks--
		}
	}
	return d.AddDays(weeks * 7)
}

func anchorOf(d calendar.Date, isEnd bool) string {
	off := calendar.Easter(d.Year()).DaysUntil(d)
	if off >= -1 && off <= 2 {
		return AnchorEaster
	}
	if calendar.IsItalianFixedHoliday(d) {
		return AnchorCalendar
	}
	if isEnd {
		if d.AddDays(1).Day() == 1 || calendar.IsItalianFixedHoliday(d.AddDays(1)) {
			return AnchorCalendar
		}
	} else if d.Day() == 1 || calendar.IsItalianFixedHoliday(d.AddDays(-1)) {
		return AnchorCalendar
	}
	return AnchorWeekday
}

func sameCalendarDate(d calendar.Date, years int) calendar.Date {
	return calendar.MonthDayOf(d).In(d.Year() + years)
}

func periodWarnings(src, dst []RatePeriod) []string {
	type span struct {
		code string
		from calendar.Date
		to   calendar.Date
	}
	collect := func(ps []RatePeriod) []span {
		var s []span
		for _, p := range ps {
			for _, r := range p.Ranges {
				s = append(s, span{p.Code, r.From, r.To})
			}
		}
		return s
	}
	srcSpans, dstSpans := collect(src), collect(dst)
	var warnings []string
	// adjacency: index pairs adjacent in source must stay adjacent.
	for i, a := range srcSpans {
		for j, b := range srcSpans {
			if i != j && a.to.AddDays(1).Equal(b.from) && !dstSpans[i].to.AddDays(1).Equal(dstSpans[j].from) {
				warnings = append(warnings, fmt.Sprintf("periods %q and %q are no longer adjacent (%s / %s)",
					a.code, b.code, dstSpans[i].to, dstSpans[j].from))
			}
		}
	}
	sorted := slices.Clone(dstSpans)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].from.Before(sorted[j].from) })
	for i := 1; i < len(sorted); i++ {
		if !sorted[i].from.After(sorted[i-1].to) {
			warnings = append(warnings, fmt.Sprintf("periods %q and %q overlap after the copy", sorted[i-1].code, sorted[i].code))
		}
	}
	return warnings
}
