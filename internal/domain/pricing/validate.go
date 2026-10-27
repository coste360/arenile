package pricing

import (
	"fmt"
	"sort"

	"github.com/coste360/arenile/internal/calendar"
)

// ValidateRule checks a rule in isolation. It is meant to be called when a
// rule is saved; Quote calls it on every active rule.
func ValidateRule(r Rule) error {
	ruleErr := func(field, format string, args ...any) error {
		return &RuleError{RuleID: r.ID, Field: field, Reason: fmt.Sprintf(format, args...)}
	}
	if r.ID == "" {
		return ruleErr("id", "required")
	}
	if !r.TargetType.Valid() {
		return ruleErr("target_type", "unknown value %q", r.TargetType)
	}
	if !r.Stage.Valid() {
		return ruleErr("stage", "unknown value %q", r.Stage)
	}
	if err := validateConditions(r.ID, r.Conditions); err != nil {
		return err
	}
	return validateAction(r.ID, r.Stage, r.Action)
}

// ValidatePromoCode checks a promo code (its action behaves like a discount rule).
func ValidatePromoCode(p PromoCode) error {
	id := "promo:" + p.Code
	if p.Code == "" {
		return &RuleError{RuleID: id, Field: "code", Reason: "required"}
	}
	if p.ValidFrom != nil && p.ValidTo != nil && p.ValidTo.Before(*p.ValidFrom) {
		return &RuleError{RuleID: id, Field: "valid_to", Reason: "before valid_from"}
	}
	if err := validateConditions(id, p.Conditions); err != nil {
		return err
	}
	return validateAction(id, StageDiscount, p.Action)
}

// ValidateAgreement checks an agreement (its action behaves like a discount rule).
func ValidateAgreement(a Agreement) error {
	id := "agreement:" + a.Code
	if a.ID == "" {
		return &RuleError{RuleID: id, Field: "id", Reason: "required"}
	}
	if err := validateConditions(id, a.Conditions); err != nil {
		return err
	}
	if err := validatePackPrices(id, a.PackPrices); err != nil {
		return err
	}
	if len(a.PackPrices) > 0 && noAction(a.Action) {
		return nil // fixed prices only
	}
	return validateAction(id, StageDiscount, a.Action)
}

func validateConditions(id string, c Conditions) error {
	bad := func(field, format string, args ...any) error {
		return &RuleError{RuleID: id, Field: "conditions." + field, Reason: fmt.Sprintf(format, args...)}
	}
	for _, dr := range c.DateRanges {
		if dr.From.IsZero() || dr.To.IsZero() {
			return bad("date_ranges", "from and to are required")
		}
		if dr.To.Before(dr.From) {
			return bad("date_ranges", "to %s before from %s", dr.To, dr.From)
		}
	}
	for _, w := range c.Weekdays {
		if w < 1 || w > 7 {
			return bad("weekdays", "%d is not an ISO weekday 1-7", w)
		}
	}
	for _, s := range c.Slots {
		if s != calendar.AM && s != calendar.PM {
			return bad("slots", "unknown slot %q", s)
		}
	}
	for _, f := range c.Formulas {
		if !f.Valid() {
			return bad("formulas", "unknown formula %q", f)
		}
	}
	for _, ch := range c.Channels {
		if !ch.Valid() {
			return bad("channels", "unknown channel %q", ch)
		}
	}
	if c.MinDays != nil && *c.MinDays < 1 {
		return bad("min_days", "must be >= 1")
	}
	if c.MaxDays != nil && *c.MaxDays < 1 {
		return bad("max_days", "must be >= 1")
	}
	if c.MinDays != nil && c.MaxDays != nil && *c.MaxDays < *c.MinDays {
		return bad("max_days", "less than min_days")
	}
	if r := c.LeadTimeDays; r != nil {
		if r.Min == nil && r.Max == nil {
			return bad("lead_time_days", "min or max required")
		}
		if (r.Min != nil && *r.Min < 0) || (r.Max != nil && *r.Max < 0) {
			return bad("lead_time_days", "bounds must be >= 0")
		}
		if r.Min != nil && r.Max != nil && *r.Max < *r.Min {
			return bad("lead_time_days", "max less than min")
		}
	}
	if r := c.OccupancyPercent; r != nil {
		if r.Min == nil && r.Max == nil {
			return bad("occupancy_percent", "min or max required")
		}
		for _, v := range []*int{r.Min, r.Max} {
			if v != nil && (*v < 0 || *v > 100) {
				return bad("occupancy_percent", "bounds must be in 0..100")
			}
		}
		if r.Min != nil && r.Max != nil && *r.Max < *r.Min {
			return bad("occupancy_percent", "max less than min")
		}
	}
	return nil
}

func validateAction(id string, stage Stage, a Action) error {
	bad := func(field, format string, args ...any) error {
		return &RuleError{RuleID: id, Field: "action." + field, Reason: fmt.Sprintf(format, args...)}
	}
	allowed := map[Stage][]ActionType{
		StageBase:     {ActionSet},
		StageModifier: {ActionAdd, ActionMultiply, ActionPercentOff, ActionAmountOff},
		StageDiscount: {ActionPercentOff, ActionAmountOff, ActionMultiply, ActionFreeDays},
	}
	ok := false
	for _, t := range allowed[stage] {
		if t == a.Type {
			ok = true
		}
	}
	if !ok {
		return bad("type", "%q is not allowed in stage %q", a.Type, stage)
	}
	switch a.Type {
	case ActionSet:
		if a.Amount < 0 {
			return bad("amount", "must be >= 0")
		}
	case ActionAdd:
		// any amount, negative included; the day price is clamped at 0.
	case ActionMultiply:
		if a.Factor == nil {
			return bad("factor", "required")
		}
		if a.Factor.Sign() < 0 {
			return bad("factor", "must be >= 0")
		}
		if stage == StageDiscount && a.Factor.Cmp(NewDecimal(1, 0)) > 0 {
			return bad("factor", "a discount factor must be <= 1")
		}
	case ActionPercentOff:
		if a.Percent == nil {
			return bad("percent", "required")
		}
		bp, exact := a.Percent.BasisPoints()
		if !exact {
			return bad("percent", "at most two decimals")
		}
		if bp <= 0 || bp > 10000 {
			return bad("percent", "must be in (0, 100]")
		}
	case ActionAmountOff:
		if a.Amount <= 0 {
			return bad("amount", "must be > 0")
		}
		switch {
		case stage == StageModifier && a.Per != "" && a.Per != PerDay:
			return bad("per", "a modifier amount_off applies per day")
		case stage == StageDiscount && a.Per != "" && a.Per != PerBooking && a.Per != PerLine && a.Per != PerDay:
			return bad("per", "unknown value %q", a.Per)
		}
	case ActionFreeDays:
		if a.Every < 2 {
			return bad("every", "must be >= 2")
		}
		if a.Free < 1 || a.Free >= a.Every {
			return bad("free", "must be in [1, every)")
		}
	}
	return nil
}

// ValidateRuleSet validates every active rule, the rate periods (unique codes,
// well-formed and non-overlapping ranges) and the fixed prices.
func ValidateRuleSet(rs RuleSet) error {
	seen := make(map[string]bool, len(rs.Rules))
	for _, r := range rs.Rules {
		if seen[r.ID] && r.ID != "" {
			return &RuleError{RuleID: r.ID, Field: "id", Reason: "duplicate"}
		}
		seen[r.ID] = true
		if !r.Active {
			continue
		}
		if err := ValidateRule(r); err != nil {
			return err
		}
	}
	type span struct {
		code string
		r    DateRange
	}
	var spans []span
	codes := make(map[string]bool, len(rs.RatePeriods))
	for _, p := range rs.RatePeriods {
		id := "rate_period:" + p.Code
		if p.Code == "" {
			return &RuleError{RuleID: id, Field: "code", Reason: "required"}
		}
		if codes[p.Code] {
			return &RuleError{RuleID: id, Field: "code", Reason: "duplicate"}
		}
		codes[p.Code] = true
		for _, r := range p.Ranges {
			if r.From.IsZero() || r.To.IsZero() || r.To.Before(r.From) {
				return &RuleError{RuleID: id, Field: "ranges", Reason: "invalid range"}
			}
			spans = append(spans, span{p.Code, r})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].r.From.Before(spans[j].r.From) })
	for i := 1; i < len(spans); i++ {
		if !spans[i].r.From.After(spans[i-1].r.To) {
			return &RuleError{
				RuleID: "rate_period:" + spans[i].code, Field: "ranges",
				Reason: fmt.Sprintf("overlaps period %q", spans[i-1].code),
			}
		}
	}
	type fpKey struct{ pack, formula, cat, period string }
	fps := make(map[fpKey]bool, len(rs.FixedPrices))
	for _, fp := range rs.FixedPrices {
		id := "fixed_price:" + fp.ID
		if !fp.Formula.IsFixedDuration() {
			return &RuleError{RuleID: id, Field: "formula", Reason: fmt.Sprintf("%q is not a fixed-duration formula", fp.Formula)}
		}
		if fp.Amount < 0 {
			return &RuleError{RuleID: id, Field: "amount", Reason: "must be >= 0"}
		}
		k := fpKey{fp.PackID, string(fp.Formula), fp.PriceCategory, "\x00any"}
		if fp.RatePeriodCode != nil {
			k.period = *fp.RatePeriodCode
			if !codes[k.period] {
				return &RuleError{RuleID: id, Field: "rate_period_code", Reason: fmt.Sprintf("unknown period %q", k.period)}
			}
		}
		if fps[k] {
			return &RuleError{RuleID: id, Field: "pack_id", Reason: "duplicate fixed price for pack/formula/category/period"}
		}
		fps[k] = true
	}
	return nil
}

// ValidateSettings checks the establishment pricing settings.
func ValidateSettings(s Settings) error {
	if s.HalfDayFactor.Sign() <= 0 || s.HalfDayFactor.Cmp(NewDecimal(1, 0)) > 0 {
		return invalidInput("settings.half_day_factor must be in (0, 1], got %s", s.HalfDayFactor)
	}
	if !s.Rounding.Valid() {
		return invalidInput("settings.rounding: unknown value %q", s.Rounding)
	}
	if s.DefaultVATRateBP < 0 || s.DefaultVATRateBP >= 10000 {
		return invalidInput("settings.default_vat_rate_bp out of range: %d", s.DefaultVATRateBP)
	}
	return nil
}
