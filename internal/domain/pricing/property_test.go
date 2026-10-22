package pricing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

var (
	genFactors  = []string{"0", "0.5", "0.85", "0.9", "1", "1.1", "1.15", "1.2", "1.333", "2"}
	genDiscFact = []string{"0", "0.5", "0.85", "0.9", "0.95", "1"}
	genPercents = []string{"1", "5", "10", "12.5", "33.33", "50", "99.99", "100"}
)

func genConditions(t *rapid.T, label string) Conditions {
	var c Conditions
	if rapid.Bool().Draw(t, label+"_has_period") {
		c.RatePeriods = []string{rapid.SampledFrom([]string{"low", "mid", "high", "peak"}).Draw(t, label+"_period")}
	}
	if rapid.Bool().Draw(t, label+"_has_weekday") {
		c.Weekdays = []int{rapid.IntRange(1, 7).Draw(t, label+"_weekday")}
	}
	if rapid.IntRange(0, 4).Draw(t, label+"_has_holiday") == 0 {
		c.Holidays = bptr(rapid.Bool().Draw(t, label+"_holiday"))
	}
	if rapid.IntRange(0, 4).Draw(t, label+"_has_cat") == 0 {
		c.PriceCategories = []string{rapid.SampledFrom([]string{"row1", "row2"}).Draw(t, label+"_cat")}
	}
	if rapid.IntRange(0, 4).Draw(t, label+"_has_min") == 0 {
		c.MinDays = ip(rapid.IntRange(1, 7).Draw(t, label+"_min"))
	}
	return c
}

func genModifierAction(t *rapid.T, label string) Action {
	switch rapid.IntRange(0, 3).Draw(t, label+"_type") {
	case 0:
		return Action{Type: ActionAdd, Amount: money.Cents(rapid.Int64Range(-2000, 2000).Draw(t, label+"_amount"))}
	case 1:
		return Action{Type: ActionMultiply, Factor: dec(rapid.SampledFrom(genFactors).Draw(t, label+"_factor"))}
	case 2:
		return Action{Type: ActionPercentOff, Percent: dec(rapid.SampledFrom(genPercents).Draw(t, label+"_pct"))}
	default:
		return Action{Type: ActionAmountOff, Amount: money.Cents(rapid.Int64Range(1, 3000).Draw(t, label+"_amount"))}
	}
}

func genDiscountAction(t *rapid.T, label string) Action {
	switch rapid.IntRange(0, 3).Draw(t, label+"_type") {
	case 0:
		return Action{Type: ActionPercentOff, Percent: dec(rapid.SampledFrom(genPercents).Draw(t, label+"_pct"))}
	case 1:
		return Action{Type: ActionMultiply, Factor: dec(rapid.SampledFrom(genDiscFact).Draw(t, label+"_factor"))}
	case 2:
		return Action{
			Type: ActionAmountOff, Amount: money.Cents(rapid.Int64Range(1, 20000).Draw(t, label+"_amount")),
			Per: rapid.SampledFrom([]AmountPer{PerBooking, PerLine, PerDay}).Draw(t, label+"_per"),
		}
	default:
		every := rapid.IntRange(2, 7).Draw(t, label+"_every")
		return Action{Type: ActionFreeDays, Every: every, Free: rapid.IntRange(1, every-1).Draw(t, label+"_free")}
	}
}

// genScenario draws a random but valid price list and booking.
func genScenario(t *rapid.T) (QuoteInput, Catalog, RuleSet) {
	cat := testCatalog()
	cat.Settings.Rounding = rapid.SampledFrom([]Rounding{RoundingNone, RoundingEuro, RoundingFiftyCents}).Draw(t, "rounding")
	cat.Settings.HalfDayFactor = MustParseDecimal(rapid.SampledFrom([]string{"0.5", "0.6", "0.75", "1"}).Draw(t, "factor"))

	rules := []Rule{baseRule("r_default", money.Cents(rapid.Int64Range(0, 10000).Draw(t, "default")), 0, Conditions{})}
	for _, x := range []string{"x_sunbed", "x_cabin", "x_park", "x_towel"} {
		rules = append(rules, extraBase("rx_"+x, x, money.Cents(rapid.Int64Range(0, 2000).Draw(t, "price_"+x)), 0, Conditions{}))
	}
	nBase := rapid.IntRange(0, 4).Draw(t, "n_base")
	for i := range nBase {
		l := fmt.Sprintf("base%d", i)
		r := baseRule(l, money.Cents(rapid.Int64Range(0, 10000).Draw(t, l+"_amount")), rapid.IntRange(0, 3).Draw(t, l+"_prio"), genConditions(t, l))
		if rapid.IntRange(0, 3).Draw(t, l+"_am") == 0 {
			r.Conditions.Slots = []calendar.Slot{rapid.SampledFrom([]calendar.Slot{calendar.AM, calendar.PM}).Draw(t, l+"_slot")}
		}
		r.Seq = int64(i)
		rules = append(rules, r)
	}
	nMod := rapid.IntRange(0, 4).Draw(t, "n_mod")
	for i := range nMod {
		l := fmt.Sprintf("mod%d", i)
		r := modRule(l, genModifierAction(t, l), rapid.IntRange(0, 3).Draw(t, l+"_prio"), rapid.Bool().Draw(t, l+"_stack"), genConditions(t, l))
		if rapid.IntRange(0, 3).Draw(t, l+"_extra") == 0 {
			r.TargetType = TargetExtra
		}
		rules = append(rules, r)
	}
	nDisc := rapid.IntRange(0, 3).Draw(t, "n_disc")
	for i := range nDisc {
		l := fmt.Sprintf("disc%d", i)
		r := discRule(l, genDiscountAction(t, l), rapid.IntRange(0, 3).Draw(t, l+"_prio"), rapid.Bool().Draw(t, l+"_stack"), genConditions(t, l))
		if rapid.IntRange(0, 3).Draw(t, l+"_extra") == 0 {
			r.TargetType = TargetExtra
		}
		rules = append(rules, r)
	}
	rs := ruleSet(rules...)
	if rapid.Bool().Draw(t, "has_fixed") {
		rs.FixedPrices = []FixedDurationPrice{{
			ID: "f", PackID: "p_base", Formula: FormulaWeek, PriceCategory: "row1",
			Amount: money.Cents(rapid.Int64Range(0, 50000).Draw(t, "fixed")),
		}}
	}

	start := d("2027-05-01").AddDays(rapid.IntRange(0, 150).Draw(t, "start"))
	days := rapid.IntRange(1, 15).Draw(t, "days")
	in := QuoteInput{
		Channel:  rapid.SampledFrom([]Channel{ChannelDesk, ChannelOnline, ChannelAPI}).Draw(t, "channel"),
		StartsOn: start, EndsOn: start.AddDays(days - 1),
		Slots:   rapid.SampledFrom([]calendar.Slots{calendar.SlotsFull, calendar.SlotsAM, calendar.SlotsPM}).Draw(t, "slots"),
		Formula: rapid.SampledFrom([]Formula{FormulaDay, FormulaMultiDay, FormulaWeek}).Draw(t, "formula"),
		Today:   start.AddDays(-rapid.IntRange(0, 60).Draw(t, "lead")),
	}
	nLines := rapid.IntRange(1, 3).Draw(t, "n_lines")
	for i := range nLines {
		l := LineInput{
			PackID: rapid.SampledFrom([]string{"p_base", "p_vip"}).Draw(t, fmt.Sprintf("line%d_pack", i)),
			ZoneID: rapid.SampledFrom([]string{"z1", "z2"}).Draw(t, fmt.Sprintf("line%d_zone", i)),
		}
		nx := rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("line%d_nx", i))
		for j := range nx {
			l.Extras = append(l.Extras, ExtraInput{
				ExtraID:  rapid.SampledFrom([]string{"x_sunbed", "x_cabin", "x_park", "x_towel"}).Draw(t, fmt.Sprintf("line%d_x%d", i, j)),
				Quantity: rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("line%d_q%d", i, j)),
			})
		}
		in.Lines = append(in.Lines, l)
	}
	if rapid.Bool().Draw(t, "booking_extra") {
		in.Extras = []ExtraInput{{ExtraID: "x_cabin", Quantity: 1}}
	}
	return in, cat, rs
}

func quoteOrSkip(t *rapid.T, in QuoteInput, cat Catalog, rs RuleSet) PriceBreakdown {
	b, err := Quote(in, cat, rs)
	if errors.Is(err, ErrNoPrice) || errors.Is(err, ErrPromoNotApplicable) {
		t.Skip("scenario not priceable")
	}
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	return b
}

func TestQuoteProperty_TotalNonNegativeAndConsistent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in, cat, rs := genScenario(t)
		b := quoteOrSkip(t, in, cat, rs)
		checkInvariants(t, b)
		for _, l := range b.Lines {
			for _, day := range l.Days {
				if day.Total < 0 {
					t.Fatalf("negative day total %+v", day)
				}
			}
		}
	})
}

func TestQuoteProperty_DiscountNeverIncreasesTotal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in, cat, rs := genScenario(t)
		before := quoteOrSkip(t, in, cat, rs)
		withPromo := in
		withPromo.PromoCode = &PromoCode{
			ID: "pc", Code: "PROMO", Active: true, Action: genDiscountAction(t, "promo"),
			Stackable: rapid.Bool().Draw(t, "promo_stack"),
		}
		b1, err := Quote(withPromo, cat, rs)
		if err != nil && !errors.Is(err, ErrPromoNotApplicable) {
			t.Fatalf("Quote with promo: %v", err)
		}
		if err == nil && b1.Total > before.Total {
			t.Fatalf("promo increased total %d -> %d", before.Total, b1.Total)
		}
		rs2 := rs
		rs2.Rules = append(append([]Rule(nil), rs.Rules...), discRule("extra_disc", genDiscountAction(t, "rule"),
			rapid.IntRange(0, 3).Draw(t, "rule_prio"), rapid.Bool().Draw(t, "rule_stack"), genConditions(t, "rule")))
		b2 := quoteOrSkip(t, in, cat, rs2)
		if b2.Total > before.Total {
			t.Fatalf("discount rule increased total %d -> %d", before.Total, b2.Total)
		}
	})
}

func TestQuoteProperty_Deterministic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in, cat, rs := genScenario(t)
		b := quoteOrSkip(t, in, cat, rs)
		j1, _ := json.Marshal(b)
		b2 := quoteOrSkip(t, in, cat, rs)
		j2, _ := json.Marshal(b2)
		if !bytes.Equal(j1, j2) {
			t.Fatalf("non deterministic:\n%s\n%s", j1, j2)
		}
		// Rule order in the input must not matter either.
		shuffled := rs
		shuffled.Rules = rapid.Permutation(rs.Rules).Draw(t, "perm")
		b3 := quoteOrSkip(t, in, cat, shuffled)
		j3, _ := json.Marshal(b3)
		if !bytes.Equal(j1, j3) {
			t.Fatalf("rule order changed the result:\n%s\n%s", j1, j3)
		}
	})
}

func TestQuoteProperty_SnapshotRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in, cat, rs := genScenario(t)
		b := quoteOrSkip(t, in, cat, rs)
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		var back PriceBreakdown
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatal(err)
		}
		raw2, _ := json.Marshal(back)
		if !bytes.Equal(raw, raw2) {
			t.Fatalf("snapshot not reproducible:\n%s\n%s", raw, raw2)
		}
		checkInvariants(t, back)
	})
}

func TestAllocateProperty_ExactAndBounded(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		weights := rapid.SliceOfN(rapid.Int64Range(0, 1_000_000), 1, 10).Draw(t, "weights")
		var sum int64
		for _, w := range weights {
			sum += w
		}
		total := rapid.Int64Range(0, sum).Draw(t, "total")
		parts := allocate(total, weights)
		var got int64
		for i, p := range parts {
			if p < 0 || (sum > 0 && p > weights[i]) {
				t.Fatalf("part %d = %d outside [0,%d]", i, p, weights[i])
			}
			got += p
		}
		if got != total {
			t.Fatalf("parts sum %d != %d", got, total)
		}
	})
}
