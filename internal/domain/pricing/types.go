package pricing

import (
	"slices"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// EngineVersion is written in every snapshot. Bump it whenever the output for
// an existing input can change.
const EngineVersion = 1

// Formula is the duration type of a booking.
type Formula string

const (
	FormulaHalfDay   Formula = "half_day"
	FormulaDay       Formula = "day"
	FormulaMultiDay  Formula = "multi_day"
	FormulaWeek      Formula = "week"
	FormulaFortnight Formula = "fortnight"
	FormulaMonth     Formula = "month"
	FormulaSeason    Formula = "season"
)

// Valid reports whether f is a known formula.
func (f Formula) Valid() bool {
	switch f {
	case FormulaHalfDay, FormulaDay, FormulaMultiDay, FormulaWeek, FormulaFortnight, FormulaMonth, FormulaSeason:
		return true
	}
	return false
}

// IsFixedDuration reports whether f may be priced with a FixedDurationPrice.
func (f Formula) IsFixedDuration() bool {
	switch f {
	case FormulaWeek, FormulaFortnight, FormulaMonth, FormulaSeason:
		return true
	}
	return false
}

// Channel is the origin of a booking.
type Channel string

const (
	ChannelDesk   Channel = "desk"
	ChannelOnline Channel = "online"
	ChannelAPI    Channel = "api"
)

// Valid reports whether c is a known channel.
func (c Channel) Valid() bool { return c == ChannelDesk || c == ChannelOnline || c == ChannelAPI }

// Stage is the pipeline stage of a rule.
type Stage string

const (
	StageBase     Stage = "base"
	StageModifier Stage = "modifier"
	StageDiscount Stage = "discount"
)

// Valid reports whether s is a known stage.
func (s Stage) Valid() bool { return s == StageBase || s == StageModifier || s == StageDiscount }

// TargetType is the kind of catalog item a rule prices.
type TargetType string

const (
	TargetPack  TargetType = "pack"
	TargetExtra TargetType = "extra"
)

// Valid reports whether t is a known target type.
func (t TargetType) Valid() bool { return t == TargetPack || t == TargetExtra }

// ActionType is the kind of price action.
type ActionType string

const (
	ActionSet        ActionType = "set"
	ActionAdd        ActionType = "add"
	ActionMultiply   ActionType = "multiply"
	ActionPercentOff ActionType = "percent_off"
	ActionAmountOff  ActionType = "amount_off"
	ActionFreeDays   ActionType = "free_days"
)

// AmountPer is the unit an amount_off applies to.
type AmountPer string

const (
	PerBooking AmountPer = "booking"
	PerLine    AmountPer = "line"
	PerDay     AmountPer = "day"
)

// PricingUnit tells how an extra's unit price is multiplied.
type PricingUnit string

const (
	PerBookingUnit PricingUnit = "per_booking"
	PerDayUnit     PricingUnit = "per_day"
	PerSlotUnit    PricingUnit = "per_slot"
)

// Valid reports whether u is a known pricing unit.
func (u PricingUnit) Valid() bool {
	return u == PerBookingUnit || u == PerDayUnit || u == PerSlotUnit
}

// Rounding is the establishment rounding mode of the final total.
type Rounding string

const (
	RoundingNone       Rounding = "none"
	RoundingEuro       Rounding = "euro"
	RoundingFiftyCents Rounding = "fifty_cents"
)

// Valid reports whether r is a known rounding mode ("" means none).
func (r Rounding) Valid() bool {
	return r == "" || r == RoundingNone || r == RoundingEuro || r == RoundingFiftyCents
}

// DateRange is an inclusive range of dates.
type DateRange struct {
	From calendar.Date `json:"from"`
	To   calendar.Date `json:"to"`
}

// Contains reports whether d is inside the range (inclusive).
func (r DateRange) Contains(d calendar.Date) bool { return !d.Before(r.From) && !d.After(r.To) }

// IntRange is an inclusive integer interval; a nil bound is open.
type IntRange struct {
	Min *int `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
}

// Contains reports whether v satisfies both bounds.
func (r IntRange) Contains(v int) bool {
	if r.Min != nil && v < *r.Min {
		return false
	}
	if r.Max != nil && v > *r.Max {
		return false
	}
	return true
}

// Conditions are the optional filters of a rule (spec 8.2). A rule applies when
// every present condition is true. Empty lists count as absent.
type Conditions struct {
	RatePeriods      []string        `json:"rate_periods,omitempty"`
	DateRanges       []DateRange     `json:"date_ranges,omitempty"`
	Weekdays         []int           `json:"weekdays,omitempty"`
	Holidays         *bool           `json:"holidays,omitempty"`
	Slots            []calendar.Slot `json:"slots,omitempty"`
	Formulas         []Formula       `json:"formulas,omitempty"`
	PriceCategories  []string        `json:"price_categories,omitempty"`
	ZoneIDs          []string        `json:"zone_ids,omitempty"`
	SpotAttributes   []string        `json:"spot_attributes,omitempty"`
	Channels         []Channel       `json:"channels,omitempty"`
	MinDays          *int            `json:"min_days,omitempty"`
	MaxDays          *int            `json:"max_days,omitempty"`
	LeadTimeDays     *IntRange       `json:"lead_time_days,omitempty"`
	OccupancyPercent *IntRange       `json:"occupancy_percent,omitempty"`
	CustomerTags     []string        `json:"customer_tags,omitempty"`
	AgreementIDs     []string        `json:"agreement_ids,omitempty"`
}

// Count returns how many conditions are present (the rule "specificity").
func (c Conditions) Count() int {
	n := 0
	for _, present := range []bool{
		len(c.RatePeriods) > 0, len(c.DateRanges) > 0, len(c.Weekdays) > 0, c.Holidays != nil,
		len(c.Slots) > 0, len(c.Formulas) > 0, len(c.PriceCategories) > 0, len(c.ZoneIDs) > 0,
		len(c.SpotAttributes) > 0, len(c.Channels) > 0, c.MinDays != nil, c.MaxDays != nil,
		c.LeadTimeDays != nil, c.OccupancyPercent != nil, len(c.CustomerTags) > 0, len(c.AgreementIDs) > 0,
	} {
		if present {
			n++
		}
	}
	return n
}

// Action is what a rule does to the price (spec 8.3).
type Action struct {
	Type    ActionType  `json:"type"`
	Amount  money.Cents `json:"amount,omitempty"`
	Factor  *Decimal    `json:"factor,omitempty"`
	Percent *Decimal    `json:"percent,omitempty"`
	Per     AmountPer   `json:"per,omitempty"`
	Every   int         `json:"every,omitempty"`
	Free    int         `json:"free,omitempty"`
}

// Rule is a price rule (spec 6.4 price_rules).
type Rule struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	TargetType TargetType `json:"target_type"`
	// TargetIDs restricts the rule to these pack/extra IDs; nil or empty means all.
	TargetIDs  []string   `json:"target_ids"`
	Conditions Conditions `json:"conditions"`
	Action     Action     `json:"action"`
	// Priority: higher is evaluated later and wins base conflicts.
	Priority  int   `json:"priority"`
	Stage     Stage `json:"stage"`
	Stackable bool  `json:"stackable"`
	Active    bool  `json:"active"`
	// Seq orders rules by recency (higher = more recent); used as tie-break.
	Seq int64 `json:"seq"`
}

func (r Rule) targets(t TargetType, id string) bool {
	if r.TargetType != t {
		return false
	}
	return len(r.TargetIDs) == 0 || slices.Contains(r.TargetIDs, id)
}

// specificity is the number of conditions, plus one when the rule is
// restricted to explicit targets.
func (r Rule) specificity() int {
	n := r.Conditions.Count()
	if len(r.TargetIDs) > 0 {
		n++
	}
	return n
}

// RatePeriod is a named set of date ranges (spec 6.1 rate_periods).
type RatePeriod struct {
	ID     string      `json:"id"`
	Code   string      `json:"code"`
	Ranges []DateRange `json:"ranges"`
}

// FixedDurationPrice is a package price for week/fortnight/month/season.
type FixedDurationPrice struct {
	ID            string  `json:"id"`
	PackID        string  `json:"pack_id"`
	Formula       Formula `json:"formula"`
	PriceCategory string  `json:"price_category"`
	// RatePeriodCode nil means valid in every period.
	RatePeriodCode *string     `json:"rate_period_code"`
	Amount         money.Cents `json:"amount"`
}

// Settings are the pricing settings of an establishment.
type Settings struct {
	// Currency defaults to "EUR" when empty.
	Currency string `json:"currency"`
	// HalfDayFactor multiplies the day price for AM/PM bookings without a
	// slot-specific rule; 0 < factor <= 1.
	HalfDayFactor Decimal  `json:"half_day_factor"`
	Rounding      Rounding `json:"rounding"`
	// DefaultVATRateBP is used by packs/extras without their own rate (2200 = 22%).
	DefaultVATRateBP int64 `json:"default_vat_rate_bp"`
	// LocalHolidays are extra holidays on specific dates.
	LocalHolidays []calendar.Date `json:"local_holidays,omitempty"`
	// RecurringLocalHolidays repeat every year (e.g. the patron saint).
	RecurringLocalHolidays []calendar.MonthDay `json:"recurring_local_holidays,omitempty"`
}

// Pack is the pricing view of a sellable pack.
type Pack struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	VATRateBP *int64 `json:"vat_rate_bp,omitempty"`
	Active    bool   `json:"active"`
}

// Extra is the pricing view of an extra.
type Extra struct {
	ID          string      `json:"id"`
	Code        string      `json:"code"`
	PricingUnit PricingUnit `json:"pricing_unit"`
	VATRateBP   *int64      `json:"vat_rate_bp,omitempty"`
	Active      bool        `json:"active"`
}

// Zone is the pricing view of a zone.
type Zone struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	PriceCategory string `json:"price_category"`
}

// Spot is the pricing view of a spot.
type Spot struct {
	ID                    string   `json:"id"`
	Label                 string   `json:"label"`
	ZoneID                string   `json:"zone_id"`
	PriceCategoryOverride string   `json:"price_category_override,omitempty"`
	Attributes            []string `json:"attributes,omitempty"`
}

// Catalog is everything about the establishment the engine needs, loaded by
// the caller before quoting.
type Catalog struct {
	Settings Settings `json:"settings"`
	Packs    []Pack   `json:"packs"`
	Extras   []Extra  `json:"extras"`
	Zones    []Zone   `json:"zones"`
	Spots    []Spot   `json:"spots"`
}

// RuleSet is the price list of a season.
type RuleSet struct {
	Rules       []Rule               `json:"rules"`
	RatePeriods []RatePeriod         `json:"rate_periods"`
	FixedPrices []FixedDurationPrice `json:"fixed_prices"`
}

// PromoCode is a resolved promo code. Usage limits (max_uses...) are checked
// by the booking service, which owns the counters; the engine checks Active,
// the validity window against QuoteInput.Today and the conditions.
type PromoCode struct {
	ID         string         `json:"id"`
	Code       string         `json:"code"`
	Action     Action         `json:"action"`
	Conditions Conditions     `json:"conditions"`
	ValidFrom  *calendar.Date `json:"valid_from,omitempty"`
	ValidTo    *calendar.Date `json:"valid_to,omitempty"`
	Stackable  bool           `json:"stackable"`
	Active     bool           `json:"active"`
}

// Agreement is a resolved convention (convenzione) attached to the customer.
type Agreement struct {
	ID         string     `json:"id"`
	Code       string     `json:"code"`
	Action     Action     `json:"action"`
	Conditions Conditions `json:"conditions"`
	Stackable  bool       `json:"stackable"`
	Active     bool       `json:"active"`
}

// ExtraInput is a requested extra.
type ExtraInput struct {
	ExtraID  string `json:"extra_id"`
	Quantity int    `json:"quantity"`
}

// LineInput is a requested pack with its assignment.
type LineInput struct {
	PackID string `json:"pack_id"`
	// ZoneID may be empty when SpotID is set (derived from the spot).
	ZoneID string       `json:"zone_id"`
	SpotID string       `json:"spot_id,omitempty"`
	Extras []ExtraInput `json:"extras,omitempty"`
}

// CustomerInput carries the customer data relevant to pricing.
type CustomerInput struct {
	Tags []string `json:"tags,omitempty"`
}

// QuoteInput is the full request to price.
type QuoteInput struct {
	Channel  Channel        `json:"channel"`
	StartsOn calendar.Date  `json:"starts_on"`
	EndsOn   calendar.Date  `json:"ends_on"`
	Slots    calendar.Slots `json:"slots"`
	Formula  Formula        `json:"formula"`
	// Today is the current date in the establishment timezone (lead time,
	// promo validity). It is an input so that the engine stays pure.
	Today     calendar.Date `json:"today"`
	Lines     []LineInput   `json:"lines"`
	Extras    []ExtraInput  `json:"extras,omitempty"` // booking-level extras
	Customer  CustomerInput `json:"customer"`
	Agreement *Agreement    `json:"agreement,omitempty"`
	PromoCode *PromoCode    `json:"promo_code,omitempty"`
	// Occupancy is the optional occupancy percentage per zone ID and date,
	// used by occupancy_percent conditions.
	Occupancy map[string]map[calendar.Date]int `json:"occupancy,omitempty"`
}
