package pricing

import (
	"errors"
	"sort"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// MatrixRequest selects the price-list matrix to compute (spec 8.7).
type MatrixRequest struct {
	// PackIDs to include; empty means every active pack in catalog order.
	PackIDs []string `json:"pack_ids,omitempty"`
	// Categories to include; empty means the distinct zone price categories in
	// catalog order.
	Categories []string `json:"categories,omitempty"`
	// RatePeriodCodes to include; empty means every period in rule set order.
	RatePeriodCodes []string `json:"rate_period_codes,omitempty"`
	// Channel defaults to desk.
	Channel Channel `json:"channel,omitempty"`
	// Slots defaults to FULL.
	Slots calendar.Slots `json:"slots,omitempty"`
	// Formula defaults to day (half_day for AM/PM); fixed-duration formulas
	// are not allowed because the matrix shows day prices.
	Formula Formula `json:"formula,omitempty"`
}

// MatrixCell is the resulting day price of a pack in a category and period.
type MatrixCell struct {
	PackID        string        `json:"pack_id"`
	PriceCategory string        `json:"price_category"`
	RatePeriod    string        `json:"rate_period"`
	ZoneID        string        `json:"zone_id"`
	Date          calendar.Date `json:"date"`
	Priced        bool          `json:"priced"`
	Price         money.Cents   `json:"price"`
}

// matrixZonePrefix marks a synthetic zone used when no catalog zone has the
// requested category.
const matrixZonePrefix = "matrix:"

// Matrix computes the pack x category x rate period price list with the same
// engine used by Quote. Each cell is the day total (base + modifiers, no
// booking discounts) of a one-day quote on the period's representative date:
// the earliest Monday-Friday non-holiday date of the period, or its earliest
// date when none exists. The zone is the first catalog zone with the category
// (so zone_ids rules apply), otherwise a synthetic zone. Today is set to the
// representative date (lead time 0); no customer tags, agreement, promo code
// or occupancy are used. A cell without base price has Priced = false.
func Matrix(req MatrixRequest, catalog Catalog, rules RuleSet) ([]MatrixCell, error) {
	if req.Channel == "" {
		req.Channel = ChannelDesk
	}
	if req.Slots == "" {
		req.Slots = calendar.SlotsFull
	}
	if req.Formula == "" {
		req.Formula = FormulaDay
		if req.Slots != calendar.SlotsFull {
			req.Formula = FormulaHalfDay
		}
	}
	if req.Formula.IsFixedDuration() {
		return nil, invalidInput("matrix: formula %q is not a day formula", req.Formula)
	}
	packs := selectPacks(req.PackIDs, catalog)
	cats := req.Categories
	if len(cats) == 0 {
		cats = catalogCategories(catalog)
	}
	periods := rules.RatePeriods
	if len(req.RatePeriodCodes) > 0 {
		periods = nil
		for _, code := range req.RatePeriodCodes {
			found := false
			for _, p := range rules.RatePeriods {
				if p.Code == code {
					periods = append(periods, p)
					found = true
				}
			}
			if !found {
				return nil, invalidInput("matrix: unknown rate period %q", code)
			}
		}
	}
	cat, zoneFor := withCategoryZones(catalog, cats)
	e, err := newEngine(cat, rules)
	if err != nil {
		return nil, err
	}
	cells := make([]MatrixCell, 0, len(packs)*len(cats)*len(periods))
	for _, packID := range packs {
		if _, ok := e.packs[packID]; !ok {
			return nil, invalidInput("matrix: unknown pack %q", packID)
		}
		for _, c := range cats {
			for _, p := range periods {
				d, ok := representativeDate(p, e.holidays)
				if !ok {
					return nil, invalidInput("matrix: rate period %q has no dates", p.Code)
				}
				cell := MatrixCell{PackID: packID, PriceCategory: c, RatePeriod: p.Code, ZoneID: zoneFor[c], Date: d}
				bd, err := e.quote(QuoteInput{
					Channel: req.Channel, StartsOn: d, EndsOn: d, Slots: req.Slots, Formula: req.Formula,
					Today: d, Lines: []LineInput{{PackID: packID, ZoneID: zoneFor[c]}},
				})
				switch {
				case errors.Is(err, ErrNoPrice):
				case err != nil:
					return nil, err
				default:
					cell.Priced = true
					cell.Price = bd.Lines[0].Days[0].Total
				}
				cells = append(cells, cell)
			}
		}
	}
	return cells, nil
}

func selectPacks(ids []string, cat Catalog) []string {
	if len(ids) > 0 {
		return ids
	}
	var out []string
	for _, p := range cat.Packs {
		if p.Active {
			out = append(out, p.ID)
		}
	}
	return out
}

func catalogCategories(cat Catalog) []string {
	seen := map[string]bool{}
	var out []string
	for _, z := range cat.Zones {
		if !seen[z.PriceCategory] {
			seen[z.PriceCategory] = true
			out = append(out, z.PriceCategory)
		}
	}
	return out
}

// withCategoryZones returns a catalog copy with a synthetic zone for every
// category not used by any zone, and the zone ID to use for each category.
func withCategoryZones(cat Catalog, cats []string) (Catalog, map[string]string) {
	zoneFor := make(map[string]string, len(cats))
	zones := append([]Zone(nil), cat.Zones...)
	for _, c := range cats {
		for _, z := range cat.Zones {
			if z.PriceCategory == c {
				zoneFor[c] = z.ID
				break
			}
		}
		if _, ok := zoneFor[c]; !ok {
			id := matrixZonePrefix + c
			zones = append(zones, Zone{ID: id, Code: c, PriceCategory: c})
			zoneFor[c] = id
		}
	}
	cat.Zones = zones
	return cat, zoneFor
}

func representativeDate(p RatePeriod, h calendar.Holidays) (calendar.Date, bool) {
	ranges := append([]DateRange(nil), p.Ranges...)
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].From.Before(ranges[j].From) })
	if len(ranges) == 0 {
		return calendar.Date{}, false
	}
	for _, r := range ranges {
		for _, d := range calendar.Range(r.From, r.To) {
			if d.ISOWeekday() <= 5 && !h.IsHoliday(d) {
				return d, true
			}
		}
	}
	return ranges[0].From, true
}

// CoverageRequest selects what the coverage check verifies (spec 8.7).
type CoverageRequest struct {
	From calendar.Date `json:"from"`
	To   calendar.Date `json:"to"`
	// PackIDs to check; empty means every active pack.
	PackIDs []string `json:"pack_ids,omitempty"`
	// Categories to check; empty means the zone price categories.
	Categories []string `json:"categories,omitempty"`
	// Channels to check; empty means desk and online.
	Channels []Channel `json:"channels,omitempty"`
	// Slots defaults to FULL.
	Slots calendar.Slots `json:"slots,omitempty"`
}

// CoverageGap is a maximal run of consecutive days without a price.
type CoverageGap struct {
	PackID        string        `json:"pack_id"`
	PriceCategory string        `json:"price_category"`
	Channel       Channel       `json:"channel"`
	From          calendar.Date `json:"from"`
	To            calendar.Date `json:"to"`
}

// Coverage reports the days in [From, To] on which an active pack has no base
// price for a category and channel. Each day is checked with a plain one-day
// quote (formula day, or half_day for AM/PM; Today = that day; no customer
// tags, agreement or occupancy), i.e. the walk-in case. Rules restricted to
// specific zones count only for the first zone of the category, like Matrix.
// Gaps are ordered by pack, category, channel (request/catalog order), date.
func Coverage(req CoverageRequest, catalog Catalog, rules RuleSet) ([]CoverageGap, error) {
	if req.From.IsZero() || req.To.IsZero() || req.To.Before(req.From) {
		return nil, invalidInput("coverage: invalid range")
	}
	if req.Slots == "" {
		req.Slots = calendar.SlotsFull
	}
	formula := FormulaDay
	if req.Slots != calendar.SlotsFull {
		formula = FormulaHalfDay
	}
	channels := req.Channels
	if len(channels) == 0 {
		channels = []Channel{ChannelDesk, ChannelOnline}
	}
	cats := req.Categories
	if len(cats) == 0 {
		cats = catalogCategories(catalog)
	}
	cat, zoneFor := withCategoryZones(catalog, cats)
	e, err := newEngine(cat, rules)
	if err != nil {
		return nil, err
	}
	gaps := []CoverageGap{}
	for _, packID := range selectPacks(req.PackIDs, catalog) {
		if _, ok := e.packs[packID]; !ok {
			return nil, invalidInput("coverage: unknown pack %q", packID)
		}
		for _, c := range cats {
			for _, ch := range channels {
				var open *CoverageGap
				for _, d := range calendar.Range(req.From, req.To) {
					_, err := e.quote(QuoteInput{
						Channel: ch, StartsOn: d, EndsOn: d, Slots: req.Slots, Formula: formula, Today: d,
						Lines: []LineInput{{PackID: packID, ZoneID: zoneFor[c]}},
					})
					switch {
					case errors.Is(err, ErrNoPrice):
						if open != nil && open.To.AddDays(1).Equal(d) {
							open.To = d
						} else {
							gaps = append(gaps, CoverageGap{PackID: packID, PriceCategory: c, Channel: ch, From: d, To: d})
							open = &gaps[len(gaps)-1]
						}
					case err != nil:
						return nil, err
					}
				}
			}
		}
	}
	return gaps, nil
}
