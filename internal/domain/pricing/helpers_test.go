package pricing

import (
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Test fixture: a small establishment with three packs, four extras, three
// zones, two spots and a 2027 season with four rate periods.

func d(s string) calendar.Date { return calendar.MustParse(s) }
func ip(v int) *int            { return &v }
func bptr(v bool) *bool        { return &v }
func i64(v int64) *int64       { return &v }
func sp(s string) *string      { return &s }
func dec(s string) *Decimal    { v := MustParseDecimal(s); return &v }

func testCatalog() Catalog {
	return Catalog{
		Settings: Settings{
			Currency:         "EUR",
			HalfDayFactor:    MustParseDecimal("0.6"),
			Rounding:         RoundingNone,
			DefaultVATRateBP: 2200,
		},
		Packs: []Pack{
			{ID: "p_base", Code: "base", Active: true},
			{ID: "p_gaz", Code: "gazebo", Active: true},
			{ID: "p_vip", Code: "vip", VATRateBP: i64(1000), Active: true},
			{ID: "p_old", Code: "old", Active: false},
		},
		Extras: []Extra{
			{ID: "x_sunbed", Code: "sunbed", PricingUnit: PerDayUnit, Active: true},
			{ID: "x_cabin", Code: "cabin", PricingUnit: PerBookingUnit, Active: true},
			{ID: "x_park", Code: "parking", PricingUnit: PerSlotUnit, Active: true},
			{ID: "x_towel", Code: "towel", PricingUnit: PerDayUnit, VATRateBP: i64(1000), Active: true},
			{ID: "x_nop", Code: "noprice", PricingUnit: PerDayUnit, Active: true},
		},
		Zones: []Zone{
			{ID: "z1", Code: "row1", PriceCategory: "row1"},
			{ID: "z2", Code: "row2", PriceCategory: "row2"},
			{ID: "zp", Code: "pool", PriceCategory: "pool"},
		},
		Spots: []Spot{
			{ID: "s_a12", Label: "A12", ZoneID: "z1", Attributes: []string{"accessible"}},
			{ID: "s_b5", Label: "B5", ZoneID: "z2", PriceCategoryOverride: "premium", Attributes: []string{"shade_morning"}},
		},
	}
}

func testPeriods() []RatePeriod {
	return []RatePeriod{
		{ID: "rp_low", Code: "low", Ranges: []DateRange{{d("2027-05-01"), d("2027-06-30")}, {d("2027-09-01"), d("2027-09-30")}}},
		{ID: "rp_mid", Code: "mid", Ranges: []DateRange{{d("2027-07-01"), d("2027-07-31")}}},
		{ID: "rp_high", Code: "high", Ranges: []DateRange{{d("2027-08-01"), d("2027-08-09")}, {d("2027-08-21"), d("2027-08-31")}}},
		{ID: "rp_peak", Code: "peak", Ranges: []DateRange{{d("2027-08-10"), d("2027-08-20")}}},
	}
}

func baseRule(id string, amount money.Cents, prio int, c Conditions) Rule {
	return Rule{
		ID: id, Name: id, TargetType: TargetPack, Conditions: c,
		Action: Action{Type: ActionSet, Amount: amount}, Priority: prio, Stage: StageBase, Active: true,
	}
}

func extraBase(id, extraID string, amount money.Cents, prio int, c Conditions) Rule {
	r := baseRule(id, amount, prio, c)
	r.TargetType = TargetExtra
	r.TargetIDs = []string{extraID}
	return r
}

func modRule(id string, a Action, prio int, stackable bool, c Conditions) Rule {
	return Rule{
		ID: id, Name: id, TargetType: TargetPack, Conditions: c, Action: a,
		Priority: prio, Stage: StageModifier, Stackable: stackable, Active: true,
	}
}

func discRule(id string, a Action, prio int, stackable bool, c Conditions) Rule {
	r := modRule(id, a, prio, stackable, c)
	r.Stage = StageDiscount
	return r
}

// defaultRules: 30.00 per pack day, plus extra prices.
func defaultRules() []Rule {
	return []Rule{
		baseRule("r_day", 3000, 0, Conditions{}),
		extraBase("rx_sunbed", "x_sunbed", 800, 0, Conditions{}),
		extraBase("rx_cabin", "x_cabin", 1500, 0, Conditions{}),
		extraBase("rx_park", "x_park", 300, 0, Conditions{}),
		extraBase("rx_towel", "x_towel", 200, 0, Conditions{}),
	}
}

func ruleSet(rules ...Rule) RuleSet {
	return RuleSet{Rules: rules, RatePeriods: testPeriods()}
}

// oneDay is a desk booking of pack base in row1 on Monday 2027-07-05 (mid).
func oneDay() QuoteInput {
	return QuoteInput{
		Channel: ChannelDesk, StartsOn: d("2027-07-05"), EndsOn: d("2027-07-05"),
		Slots: calendar.SlotsFull, Formula: FormulaDay, Today: d("2027-06-01"),
		Lines: []LineInput{{PackID: "p_base", ZoneID: "z1"}},
	}
}

func span(from, to string, f Formula) QuoteInput {
	in := oneDay()
	in.StartsOn, in.EndsOn, in.Formula = d(from), d(to), f
	return in
}

func mustQuote(t *testing.T, in QuoteInput, cat Catalog, rs RuleSet) PriceBreakdown {
	t.Helper()
	b, err := Quote(in, cat, rs)
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	checkInvariants(t, b)
	return b
}

// fataler is satisfied by *testing.T and *rapid.T.
type fataler interface {
	Fatalf(format string, args ...any)
}

// checkInvariants verifies the internal consistency of a snapshot.
func checkInvariants(t fataler, b PriceBreakdown) {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	var sum money.Cents
	for _, l := range b.Lines {
		var ls money.Cents
		if l.Fixed != nil {
			ls += l.Fixed.Amount
		}
		for _, day := range l.Days {
			ls += day.Total
		}
		for _, x := range l.Extras {
			ls += x.Total
		}
		if ls != l.Total {
			t.Fatalf("line %s total %d != parts %d", l.Pack, l.Total, ls)
		}
		sum += l.Total
	}
	for _, x := range b.Extras {
		sum += x.Total
	}
	for _, dsc := range b.Discounts {
		if dsc.Amount >= 0 {
			t.Fatalf("discount %s not negative: %d", dsc.Source, dsc.Amount)
		}
		sum += dsc.Amount
	}
	sum += b.Rounding
	if sum != b.Total {
		t.Fatalf("lines+extras+discounts+rounding = %d, total %d", sum, b.Total)
	}
	if b.Total < 0 {
		t.Fatalf("negative total %d", b.Total)
	}
	var vat money.Cents
	for _, v := range b.VAT {
		vat += v.Taxable + v.Tax
	}
	if vat != b.Total {
		t.Fatalf("vat sum %d != total %d", vat, b.Total)
	}
}
