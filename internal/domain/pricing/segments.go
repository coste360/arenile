package pricing

import (
	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Split stays (spec 30.6, ADR 0028 decision 1): a line whose stay is split
// over several spots carries its segments; each day is priced with the
// spot occupied that day (price category, zone, attributes). When every
// segment has the category of the line the price is the one of a single
// spot. The engine stays pure: the segments are part of the line input.

// LineSegment is the part of a line's stay spent on one spot, From to To
// inclusive.
type LineSegment struct {
	SpotID string        `json:"spot_id"`
	From   calendar.Date `json:"from"`
	To     calendar.Date `json:"to"`
}

// segmentCtxs returns the line context of every day covered by a segment
// of l (nil without segments). Segments must lie inside the stay and must
// not overlap; days not covered keep the line's own context.
func (e *engine) segmentCtxs(l LineInput, b *bookingCtx) (map[calendar.Date]*lineCtx, error) {
	if len(l.Segments) == 0 {
		return nil, nil
	}
	first, last := b.dates[0], b.dates[len(b.dates)-1]
	byspot := map[string]*lineCtx{}
	out := map[calendar.Date]*lineCtx{}
	for _, sg := range l.Segments {
		if sg.SpotID == "" || sg.From.IsZero() || sg.To.IsZero() || sg.To.Before(sg.From) {
			return nil, invalidInput("invalid segment %q %s..%s", sg.SpotID, sg.From, sg.To)
		}
		if sg.From.Before(first) || sg.To.After(last) {
			return nil, invalidInput("segment %s..%s outside the stay", sg.From, sg.To)
		}
		lc, ok := byspot[sg.SpotID]
		if !ok {
			var err error
			if lc, err = e.newLineCtx(LineInput{PackID: l.PackID, SpotID: sg.SpotID}); err != nil {
				return nil, err
			}
			byspot[sg.SpotID] = lc
		}
		for _, d := range calendar.Range(sg.From, sg.To) {
			if _, dup := out[d]; dup {
				return nil, invalidInput("segments overlap on %s", d)
			}
			out[d] = lc
		}
	}
	return out, nil
}

// fixedAmount is the fixed-duration price of a line (before the half-day
// factor) and its rate period. A split stay over several price categories
// is prorated by the days spent in each category (ADR 0028); ok is false
// when one of the categories has no fixed price (the caller prices by day).
func (e *engine) fixedAmount(lc *lineCtx, perDay map[calendar.Date]*lineCtx, dates []calendar.Date, f Formula, period string) (money.Cents, string, bool) {
	days := map[string]int64{}
	var order []string
	for _, d := range dates {
		cat := lc.category
		if x, ok := perDay[d]; ok {
			cat = x.category
		}
		if _, seen := days[cat]; !seen {
			order = append(order, cat)
		}
		days[cat]++
	}
	if len(order) <= 1 {
		cat := lc.category
		if len(order) == 1 {
			cat = order[0]
		}
		fp, ok := e.lookupFixed(lc.pack.ID, f, cat, period)
		if !ok {
			return 0, "", false
		}
		rp := ""
		if fp.RatePeriodCode != nil {
			rp = *fp.RatePeriodCode
		}
		return fp.Amount, rp, true
	}
	var (
		total money.Cents
		rp    string
	)
	for i, cat := range order {
		fp, ok := e.lookupFixed(lc.pack.ID, f, cat, period)
		if !ok {
			return 0, "", false
		}
		if i == 0 && fp.RatePeriodCode != nil {
			rp = *fp.RatePeriodCode
		}
		total += fp.Amount.MulRat(days[cat], int64(len(dates)))
	}
	return total, rp, true
}
