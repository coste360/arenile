package pricing

import (
	"fmt"
	"slices"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// maxQuoteDays bounds the length of a quoted range (a season is ~5 months).
const maxQuoteDays = 400

// pseudo-rule identifier recorded when the half-day factor is applied.
const halfDayFactorRule = "half_day_factor"

// Quote prices a booking. It is a pure function of its arguments: no I/O, no
// clock, no randomness. See the package documentation for the pipeline.
func Quote(in QuoteInput, catalog Catalog, rules RuleSet) (PriceBreakdown, error) {
	e, err := newEngine(catalog, rules)
	if err != nil {
		return PriceBreakdown{}, err
	}
	return e.quote(in)
}

// engine holds the validated, indexed catalog and price list.
type engine struct {
	settings  Settings
	holidays  calendar.Holidays
	factorNum int64
	factorDen int64
	packs     map[string]Pack
	extras    map[string]Extra
	zones     map[string]Zone
	spots     map[string]Spot
	rules     []Rule // active rules only, in input order
	periods   []RatePeriod
	fixed     []FixedDurationPrice
}

func newEngine(cat Catalog, rs RuleSet) (*engine, error) {
	if err := ValidateSettings(cat.Settings); err != nil {
		return nil, err
	}
	if err := ValidateRuleSet(rs); err != nil {
		return nil, err
	}
	e := &engine{
		settings: cat.Settings,
		holidays: calendar.NewHolidays(cat.Settings.LocalHolidays, cat.Settings.RecurringLocalHolidays),
		packs:    make(map[string]Pack, len(cat.Packs)),
		extras:   make(map[string]Extra, len(cat.Extras)),
		zones:    make(map[string]Zone, len(cat.Zones)),
		spots:    make(map[string]Spot, len(cat.Spots)),
		periods:  rs.RatePeriods,
		fixed:    rs.FixedPrices,
	}
	if e.settings.Currency == "" {
		e.settings.Currency = "EUR"
	}
	e.factorNum, e.factorDen = cat.Settings.HalfDayFactor.Rat()
	for _, p := range cat.Packs {
		if p.VATRateBP != nil && (*p.VATRateBP < 0 || *p.VATRateBP >= 10000) {
			return nil, invalidInput("pack %q: vat rate out of range", p.Code)
		}
		e.packs[p.ID] = p
	}
	for _, x := range cat.Extras {
		if !x.PricingUnit.Valid() {
			return nil, invalidInput("extra %q: unknown pricing unit %q", x.Code, x.PricingUnit)
		}
		if x.VATRateBP != nil && (*x.VATRateBP < 0 || *x.VATRateBP >= 10000) {
			return nil, invalidInput("extra %q: vat rate out of range", x.Code)
		}
		e.extras[x.ID] = x
	}
	for _, z := range cat.Zones {
		e.zones[z.ID] = z
	}
	for _, s := range cat.Spots {
		e.spots[s.ID] = s
	}
	for _, r := range rs.Rules {
		if r.Active {
			e.rules = append(e.rules, r)
		}
	}
	return e, nil
}

// bookingCtx holds the booking-level facts used by conditions.
type bookingCtx struct {
	in          *QuoteInput
	dates       []calendar.Date
	slots       []calendar.Slot
	leadTime    int
	agreementID string
}

// lineCtx holds the line-level facts used by conditions.
type lineCtx struct {
	pack     Pack
	zone     Zone
	spot     *Spot
	category string
}

// evalCtx is the full context a condition set is evaluated against.
type evalCtx struct {
	b     *bookingCtx
	line  *lineCtx // nil for booking-level extras
	date  calendar.Date
	slots []calendar.Slot
}

func (e *engine) ratePeriodOf(d calendar.Date) string {
	for _, p := range e.periods {
		for _, r := range p.Ranges {
			if r.Contains(d) {
				return p.Code
			}
		}
	}
	return ""
}

func anyIn[T comparable](have, want []T) bool {
	for _, h := range have {
		if slices.Contains(want, h) {
			return true
		}
	}
	return false
}

// matches reports whether every present condition holds in x.
func (e *engine) matches(c Conditions, x evalCtx) bool {
	in := x.b.in
	if len(c.RatePeriods) > 0 {
		rp := e.ratePeriodOf(x.date)
		if rp == "" || !slices.Contains(c.RatePeriods, rp) {
			return false
		}
	}
	if len(c.DateRanges) > 0 && !slices.ContainsFunc(c.DateRanges, func(r DateRange) bool { return r.Contains(x.date) }) {
		return false
	}
	if len(c.Weekdays) > 0 && !slices.Contains(c.Weekdays, x.date.ISOWeekday()) {
		return false
	}
	if c.Holidays != nil && e.holidays.IsHoliday(x.date) != *c.Holidays {
		return false
	}
	if len(c.Slots) > 0 {
		for _, s := range x.slots {
			if !slices.Contains(c.Slots, s) {
				return false
			}
		}
	}
	if len(c.Formulas) > 0 && !slices.Contains(c.Formulas, in.Formula) {
		return false
	}
	if len(c.PriceCategories) > 0 && (x.line == nil || !slices.Contains(c.PriceCategories, x.line.category)) {
		return false
	}
	if len(c.ZoneIDs) > 0 && (x.line == nil || !slices.Contains(c.ZoneIDs, x.line.zone.ID)) {
		return false
	}
	if len(c.SpotAttributes) > 0 && (x.line == nil || x.line.spot == nil || !anyIn(x.line.spot.Attributes, c.SpotAttributes)) {
		return false
	}
	if len(c.Channels) > 0 && !slices.Contains(c.Channels, in.Channel) {
		return false
	}
	if c.MinDays != nil && len(x.b.dates) < *c.MinDays {
		return false
	}
	if c.MaxDays != nil && len(x.b.dates) > *c.MaxDays {
		return false
	}
	if c.LeadTimeDays != nil && !c.LeadTimeDays.Contains(x.b.leadTime) {
		return false
	}
	if c.OccupancyPercent != nil {
		if x.line == nil {
			return false
		}
		v, ok := in.Occupancy[x.line.zone.ID][x.date]
		if !ok || !c.OccupancyPercent.Contains(v) {
			return false
		}
	}
	if len(c.CustomerTags) > 0 && !anyIn(in.Customer.Tags, c.CustomerTags) {
		return false
	}
	if len(c.AgreementIDs) > 0 && (x.b.agreementID == "" || !slices.Contains(c.AgreementIDs, x.b.agreementID)) {
		return false
	}
	return true
}

func (e *engine) newBookingCtx(in *QuoteInput) (*bookingCtx, error) {
	if !in.Channel.Valid() {
		return nil, invalidInput("unknown channel %q", in.Channel)
	}
	if !in.Formula.Valid() {
		return nil, invalidInput("unknown formula %q", in.Formula)
	}
	if !in.Slots.Valid() {
		return nil, invalidInput("unknown slots %q", in.Slots)
	}
	if in.StartsOn.IsZero() || in.EndsOn.IsZero() || in.Today.IsZero() {
		return nil, invalidInput("starts_on, ends_on and today are required")
	}
	if in.EndsOn.Before(in.StartsOn) {
		return nil, invalidInput("ends_on %s before starts_on %s", in.EndsOn, in.StartsOn)
	}
	if n := in.StartsOn.DaysUntil(in.EndsOn) + 1; n > maxQuoteDays {
		return nil, invalidInput("range of %d days exceeds %d", n, maxQuoteDays)
	}
	if in.Formula == FormulaHalfDay && in.Slots == calendar.SlotsFull {
		return nil, invalidInput("formula half_day requires slots AM or PM")
	}
	if len(in.Lines) == 0 {
		return nil, invalidInput("at least one line is required")
	}
	b := &bookingCtx{
		in:       in,
		dates:    calendar.Range(in.StartsOn, in.EndsOn),
		slots:    in.Slots.Expand(),
		leadTime: in.Today.DaysUntil(in.StartsOn),
	}
	if in.Agreement != nil && in.Agreement.Active {
		b.agreementID = in.Agreement.ID
	}
	return b, nil
}

func (e *engine) newLineCtx(li LineInput) (*lineCtx, error) {
	p, ok := e.packs[li.PackID]
	if !ok {
		return nil, invalidInput("unknown pack %q", li.PackID)
	}
	lc := &lineCtx{pack: p}
	zoneID := li.ZoneID
	if li.SpotID != "" {
		s, ok := e.spots[li.SpotID]
		if !ok {
			return nil, invalidInput("unknown spot %q", li.SpotID)
		}
		if zoneID == "" {
			zoneID = s.ZoneID
		} else if zoneID != s.ZoneID {
			return nil, invalidInput("spot %q is not in zone %q", s.Label, zoneID)
		}
		lc.spot = &s
	}
	z, ok := e.zones[zoneID]
	if !ok {
		return nil, invalidInput("unknown zone %q", zoneID)
	}
	lc.zone = z
	lc.category = z.PriceCategory
	if lc.spot != nil && lc.spot.PriceCategoryOverride != "" {
		lc.category = lc.spot.PriceCategoryOverride
	}
	return lc, nil
}

func (e *engine) vatOf(rate *int64) int64 {
	if rate != nil {
		return *rate
	}
	return e.settings.DefaultVATRateBP
}

// betterBase reports whether a beats b for the base stage: higher priority,
// then more specific, then more recent, then lower ID (determinism).
func betterBase(a, b Rule) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if sa, sb := a.specificity(), b.specificity(); sa != sb {
		return sa > sb
	}
	if a.Seq != b.Seq {
		return a.Seq > b.Seq
	}
	return a.ID < b.ID
}

// applyOrder sorts rules in evaluation order: priority ascending (higher is
// evaluated later), then older first, then ID.
func applyOrder(a, b Rule) int {
	if a.Priority != b.Priority {
		return a.Priority - b.Priority
	}
	if a.Seq != b.Seq {
		if a.Seq < b.Seq {
			return -1
		}
		return 1
	}
	switch {
	case a.ID < b.ID:
		return -1
	case a.ID > b.ID:
		return 1
	}
	return 0
}

func (e *engine) bestBase(t TargetType, itemID string, x evalCtx, slotSpecificOnly bool) (Rule, bool) {
	var best Rule
	found := false
	for _, r := range e.rules {
		if r.Stage != StageBase || !r.targets(t, itemID) {
			continue
		}
		if slotSpecificOnly && len(r.Conditions.Slots) == 0 {
			continue
		}
		if !e.matches(r.Conditions, x) {
			continue
		}
		if !found || betterBase(r, best) {
			best, found = r, true
		}
	}
	return best, found
}

// unitPrice computes the price of one item on one day: base (step 2) plus
// modifiers (step 3). halfDayLogic enables the slot-specific rule /
// half_day_factor fallback for single-slot contexts.
func (e *engine) unitPrice(t TargetType, itemID, code string, b *bookingCtx, lc *lineCtx, date calendar.Date, slots []calendar.Slot, halfDayLogic bool) (money.Cents, []AppliedRule, money.Cents, error) {
	x := evalCtx{b: b, line: lc, date: date, slots: slots}
	var (
		base    money.Cents
		applied []AppliedRule
	)
	if halfDayLogic && len(slots) == 1 {
		if r, ok := e.bestBase(t, itemID, x, true); ok {
			base = r.Action.Amount
			applied = append(applied, AppliedRule{Rule: r.ID, Stage: StageBase, Effect: fmt.Sprintf("set %d", r.Action.Amount)})
		} else {
			full := x
			full.slots = []calendar.Slot{calendar.AM, calendar.PM}
			r, ok := e.bestBase(t, itemID, full, false)
			if !ok {
				return 0, nil, 0, e.noPrice(t, itemID, code, lc, date, b)
			}
			base = r.Action.Amount.MulRat(e.factorNum, e.factorDen)
			applied = append(applied,
				AppliedRule{Rule: r.ID, Stage: StageBase, Effect: fmt.Sprintf("set %d", r.Action.Amount)},
				AppliedRule{Rule: halfDayFactorRule, Stage: StageBase, Effect: "x" + e.settings.HalfDayFactor.String()},
			)
		}
	} else {
		r, ok := e.bestBase(t, itemID, x, false)
		if !ok {
			return 0, nil, 0, e.noPrice(t, itemID, code, lc, date, b)
		}
		base = r.Action.Amount
		applied = append(applied, AppliedRule{Rule: r.ID, Stage: StageBase, Effect: fmt.Sprintf("set %d", r.Action.Amount)})
	}
	total, mods := e.applyModifiers(t, itemID, x, base)
	return base, append(applied, mods...), total, nil
}

func (e *engine) noPrice(t TargetType, itemID, code string, lc *lineCtx, date calendar.Date, b *bookingCtx) error {
	np := &NoPriceError{Target: t, ItemID: itemID, ItemCode: code, Date: date, Slots: b.in.Slots}
	if lc != nil {
		np.PriceCategory = lc.category
	}
	return np
}

// applyModifierAction applies a modifier action and clamps at zero.
func applyModifierAction(a Action, p money.Cents) (money.Cents, string) {
	var effect string
	switch a.Type {
	case ActionAdd:
		p += a.Amount
		effect = fmt.Sprintf("%+d", a.Amount)
	case ActionMultiply:
		n, d := a.Factor.Rat()
		p = p.MulRat(n, d)
		effect = "x" + a.Factor.String()
	case ActionPercentOff:
		bp, _ := a.Percent.BasisPoints()
		p -= p.PercentOfBasisPoints(bp)
		effect = "-" + a.Percent.String() + "%"
	case ActionAmountOff:
		p -= a.Amount
		effect = fmt.Sprintf("-%d", a.Amount)
	}
	if p < 0 {
		p = 0
	}
	return p, effect
}

func runModifiers(seq []Rule, base money.Cents) (money.Cents, []AppliedRule) {
	p := base
	applied := make([]AppliedRule, 0, len(seq))
	for _, r := range seq {
		var eff string
		p, eff = applyModifierAction(r.Action, p)
		applied = append(applied, AppliedRule{Rule: r.ID, Stage: StageModifier, Effect: eff})
	}
	return p, applied
}

// applyModifiers runs step 3: all matching stackable modifiers plus the single
// non-stackable modifier whose inclusion gives the lowest price, all applied
// in priority order.
func (e *engine) applyModifiers(t TargetType, itemID string, x evalCtx, base money.Cents) (money.Cents, []AppliedRule) {
	var stack, nonStack []Rule
	for _, r := range e.rules {
		if r.Stage != StageModifier || !r.targets(t, itemID) || !e.matches(r.Conditions, x) {
			continue
		}
		if r.Stackable {
			stack = append(stack, r)
		} else {
			nonStack = append(nonStack, r)
		}
	}
	if len(nonStack) == 0 {
		slices.SortFunc(stack, applyOrder)
		return runModifiers(stack, base)
	}
	var (
		bestTotal   money.Cents
		bestApplied []AppliedRule
		bestRule    Rule
	)
	for i, n := range nonStack {
		seq := append(slices.Clone(stack), n)
		slices.SortFunc(seq, applyOrder)
		total, applied := runModifiers(seq, base)
		if i == 0 || total < bestTotal || (total == bestTotal && betterBase(n, bestRule)) {
			bestTotal, bestApplied, bestRule = total, applied, n
		}
	}
	return bestTotal, bestApplied
}

// unit is an internal priced amount used to apply booking discounts and to
// split VAT. amount is the remaining amount after the discounts applied so far.
type unit struct {
	line   int // index into lines; -1 for booking-level extras
	date   calendar.Date
	whole  bool // fixed-duration pack or per_booking extra: covers every day
	target TargetType
	itemID string
	vatBP  int64
	amount money.Cents
}

func (e *engine) lookupFixed(packID string, f Formula, category, period string) (FixedDurationPrice, bool) {
	var generic *FixedDurationPrice
	for i := range e.fixed {
		fp := &e.fixed[i]
		if fp.PackID != packID || fp.Formula != f || fp.PriceCategory != category {
			continue
		}
		if fp.RatePeriodCode == nil {
			generic = fp
			continue
		}
		if *fp.RatePeriodCode == period && period != "" {
			return *fp, true
		}
	}
	if generic != nil {
		return *generic, true
	}
	return FixedDurationPrice{}, false
}

func (e *engine) quote(in QuoteInput) (PriceBreakdown, error) {
	b, err := e.newBookingCtx(&in)
	if err != nil {
		return PriceBreakdown{}, err
	}
	out := PriceBreakdown{
		EngineVersion: EngineVersion,
		Currency:      e.settings.Currency,
		Lines:         make([]LineBreakdown, 0, len(in.Lines)),
		Discounts:     []DiscountBreakdown{},
		VAT:           []VATBreakdown{},
	}
	var units []unit
	lines := make([]*lineCtx, len(in.Lines))
	for li, l := range in.Lines {
		lc, err := e.newLineCtx(l)
		if err != nil {
			return PriceBreakdown{}, err
		}
		lines[li] = lc
		lb := LineBreakdown{Pack: lc.pack.Code, Zone: lc.zone.Code, Days: []DayBreakdown{}, Extras: []ExtraBreakdown{}}
		if lc.spot != nil {
			lb.Spot = lc.spot.Label
		}
		packVAT := e.vatOf(lc.pack.VATRateBP)
		fixedDone := false
		if in.Formula.IsFixedDuration() {
			period := e.ratePeriodOf(in.StartsOn)
			if fp, ok := e.lookupFixed(lc.pack.ID, in.Formula, lc.category, period); ok {
				amount := fp.Amount
				if len(b.slots) == 1 {
					amount = amount.MulRat(e.factorNum, e.factorDen)
				}
				fb := &FixedBreakdown{Formula: in.Formula, Amount: amount}
				if fp.RatePeriodCode != nil {
					fb.RatePeriod = *fp.RatePeriodCode
				}
				lb.Fixed = fb
				lb.Total += amount
				units = append(units, unit{line: li, date: in.StartsOn, whole: true, target: TargetPack, itemID: lc.pack.ID, vatBP: packVAT, amount: amount})
				fixedDone = true
			}
		}
		if !fixedDone {
			for _, d := range b.dates {
				base, applied, total, err := e.unitPrice(TargetPack, lc.pack.ID, lc.pack.Code, b, lc, d, b.slots, true)
				if err != nil {
					return PriceBreakdown{}, err
				}
				lb.Days = append(lb.Days, DayBreakdown{Date: d, Slots: in.Slots, Base: base, Applied: applied, Total: total})
				lb.Total += total
				units = append(units, unit{line: li, date: d, target: TargetPack, itemID: lc.pack.ID, vatBP: packVAT, amount: total})
			}
		}
		for _, xi := range l.Extras {
			eb, us, err := e.priceExtra(xi, b, lc, li)
			if err != nil {
				return PriceBreakdown{}, err
			}
			lb.Extras = append(lb.Extras, eb)
			lb.Total += eb.Total
			units = append(units, us...)
		}
		out.Lines = append(out.Lines, lb)
	}
	for _, xi := range in.Extras {
		eb, us, err := e.priceExtra(xi, b, nil, -1)
		if err != nil {
			return PriceBreakdown{}, err
		}
		out.Extras = append(out.Extras, eb)
		units = append(units, us...)
	}

	discounts, units, err := e.applyDiscounts(b, lines, units)
	if err != nil {
		return PriceBreakdown{}, err
	}
	out.Discounts = discounts

	var subtotal money.Cents
	for _, u := range units {
		subtotal += u.amount
	}
	out.Total = roundTotal(subtotal, e.settings.Rounding)
	out.Rounding = out.Total - subtotal
	out.VAT = vatSplit(units, out.Rounding)
	return out, nil
}

// priceExtra runs step 4 for one requested extra.
func (e *engine) priceExtra(xi ExtraInput, b *bookingCtx, lc *lineCtx, li int) (ExtraBreakdown, []unit, error) {
	x, ok := e.extras[xi.ExtraID]
	if !ok {
		return ExtraBreakdown{}, nil, invalidInput("unknown extra %q", xi.ExtraID)
	}
	if xi.Quantity < 1 {
		return ExtraBreakdown{}, nil, invalidInput("extra %q: quantity must be >= 1", x.Code)
	}
	qty := money.Cents(xi.Quantity)
	vat := e.vatOf(x.VATRateBP)
	eb := ExtraBreakdown{Code: x.Code, Qty: xi.Quantity}
	var units []unit
	switch x.PricingUnit {
	case PerBookingUnit:
		_, _, p, err := e.unitPrice(TargetExtra, x.ID, x.Code, b, lc, b.in.StartsOn, b.slots, false)
		if err != nil {
			return ExtraBreakdown{}, nil, err
		}
		amt := p * qty
		units = append(units, unit{line: li, date: b.in.StartsOn, whole: true, target: TargetExtra, itemID: x.ID, vatBP: vat, amount: amt})
		eb.Total = amt
	case PerDayUnit:
		for _, d := range b.dates {
			_, _, p, err := e.unitPrice(TargetExtra, x.ID, x.Code, b, lc, d, b.slots, true)
			if err != nil {
				return ExtraBreakdown{}, nil, err
			}
			amt := p * qty
			units = append(units, unit{line: li, date: d, target: TargetExtra, itemID: x.ID, vatBP: vat, amount: amt})
			eb.Total += amt
		}
	case PerSlotUnit:
		for _, d := range b.dates {
			var day money.Cents
			for _, s := range b.slots {
				_, _, p, err := e.unitPrice(TargetExtra, x.ID, x.Code, b, lc, d, []calendar.Slot{s}, false)
				if err != nil {
					return ExtraBreakdown{}, nil, err
				}
				day += p
			}
			amt := day * qty
			units = append(units, unit{line: li, date: d, target: TargetExtra, itemID: x.ID, vatBP: vat, amount: amt})
			eb.Total += amt
		}
	}
	return eb, units, nil
}
