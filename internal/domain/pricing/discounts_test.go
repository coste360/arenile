package pricing

import (
	"errors"
	"slices"
	"testing"

	"github.com/coste360/arenile/internal/money"
)

func promo(code string, a Action, stackable bool) *PromoCode {
	return &PromoCode{ID: "pc_" + code, Code: code, Action: a, Stackable: stackable, Active: true}
}

func pctOff(p string) Action { return Action{Type: ActionPercentOff, Percent: dec(p)} }

func amountOff(v money.Cents, per AmountPer) Action {
	return Action{Type: ActionAmountOff, Amount: v, Per: per}
}

// fourEightyInput is 30.00 + sunbed 8.00 + an extra 10.00 weekend surcharge
// on Saturday 2027-07-10: line total 48.00.
func fourEightyInput() (QuoteInput, []Rule) {
	in := oneDay()
	in.StartsOn, in.EndsOn = d("2027-07-10"), d("2027-07-10")
	in.Lines[0].Extras = []ExtraInput{{"x_sunbed", 1}}
	rules := append(defaultRules(), modRule("r_weekend", Action{Type: ActionAdd, Amount: 1000}, 0, true, Conditions{Weekdays: []int{6, 7}}))
	return in, rules
}

type discountWant struct {
	source string
	amount money.Cents
}

func TestQuote_Discounts(t *testing.T) {
	agreement := func(a Action, stackable bool) *Agreement {
		return &Agreement{ID: "ag1", Code: "HOTEL", Action: a, Stackable: stackable, Active: true}
	}
	cases := []struct {
		name      string
		mutate    func(*QuoteInput)
		rules     []Rule
		want      money.Cents
		discounts []discountWant
	}{
		{name: "no discount", want: 4800, discounts: nil},
		{
			name: "promo percent", mutate: func(in *QuoteInput) { in.PromoCode = promo("ESTATE10", pctOff("10"), false) },
			want: 4320, discounts: []discountWant{{"promo:ESTATE10", -480}},
		},
		{
			name: "promo amount off per booking", mutate: func(in *QuoteInput) { in.PromoCode = promo("FIVE", amountOff(500, PerBooking), false) },
			want: 4300, discounts: []discountWant{{"promo:FIVE", -500}},
		},
		{
			name: "promo amount off default per booking", mutate: func(in *QuoteInput) { in.PromoCode = promo("FIVE", amountOff(500, ""), false) },
			want: 4300, discounts: []discountWant{{"promo:FIVE", -500}},
		},
		{
			name: "promo amount off capped at total", mutate: func(in *QuoteInput) { in.PromoCode = promo("BIG", amountOff(99999, PerBooking), false) },
			want: 0, discounts: []discountWant{{"promo:BIG", -4800}},
		},
		{name: "promo multiply", mutate: func(in *QuoteInput) {
			in.PromoCode = promo("X09", Action{Type: ActionMultiply, Factor: dec("0.9")}, false)
		}, want: 4320, discounts: []discountWant{{"promo:X09", -480}}},
		{name: "agreement then promo both stackable", mutate: func(in *QuoteInput) {
			in.Agreement = agreement(pctOff("10"), true)
			in.PromoCode = promo("FIVE", amountOff(500, PerBooking), true)
		}, want: 3820, discounts: []discountWant{{"agreement:HOTEL", -480}, {"promo:FIVE", -500}}},
		{name: "stackable percents compound", mutate: func(in *QuoteInput) {
			in.Agreement = agreement(pctOff("10"), true)
			in.PromoCode = promo("TEN", pctOff("10"), true)
		}, want: 3888, discounts: []discountWant{{"agreement:HOTEL", -480}, {"promo:TEN", -432}}},
		{
			name: "non-stackable best wins promo vs rule", mutate: func(in *QuoteInput) { in.PromoCode = promo("TEN", pctOff("10"), false) },
			rules: []Rule{discRule("d_15", pctOff("15"), 0, false, Conditions{})},
			want:  4200, discounts: []discountWant{{"rule:d_15", -600}},
		},
		{
			name: "non-stackable best wins promo better", mutate: func(in *QuoteInput) { in.PromoCode = promo("TWENTY", pctOff("20"), false) },
			rules: []Rule{discRule("d_15", pctOff("15"), 0, false, Conditions{})},
			want:  3840, discounts: []discountWant{{"promo:TWENTY", -960}},
		},
		{name: "non-stackable tie prefers agreement", mutate: func(in *QuoteInput) {
			in.Agreement = agreement(pctOff("10"), false)
			in.PromoCode = promo("TEN", pctOff("10"), false)
		}, want: 4320, discounts: []discountWant{{"agreement:HOTEL", -480}}},
		{
			name: "stackable plus non-stackable", mutate: func(in *QuoteInput) { in.PromoCode = promo("FIVE", amountOff(500, PerBooking), true) },
			rules: []Rule{discRule("d_10", pctOff("10"), 0, false, Conditions{}), discRule("d_300", amountOff(300, PerBooking), 0, false, Conditions{})},
			want:  3942, discounts: []discountWant{{"promo:FIVE", -500}, {"rule:d_10", -358}},
		},
		{
			name: "automatic long stay discount not reached", rules: []Rule{discRule("d_long", pctOff("10"), 0, true, Conditions{MinDays: ip(7)})},
			want: 4800,
		},
		{name: "automatic rule target extra only", rules: []Rule{func() Rule {
			r := discRule("d_x", pctOff("50"), 0, true, Conditions{})
			r.TargetType = TargetExtra
			return r
		}()}, want: 4400, discounts: []discountWant{{"rule:d_x", -400}}},
		{
			name: "automatic rule target pack only", rules: []Rule{discRule("d_p", pctOff("50"), 0, true, Conditions{})},
			want: 2800, discounts: []discountWant{{"rule:d_p", -2000}},
		},
		{name: "automatic rule other pack", rules: []Rule{func() Rule {
			r := discRule("d_g", pctOff("50"), 0, true, Conditions{})
			r.TargetIDs = []string{"p_gaz"}
			return r
		}()}, want: 4800},
		{name: "automatic rules sequential by priority", rules: []Rule{
			discRule("d_b", amountOff(1000, PerBooking), 2, true, Conditions{}),
			discRule("d_a", pctOff("50"), 1, true, Conditions{}),
		}, want: 1800, discounts: []discountWant{{"rule:d_a", -2000}, {"rule:d_b", -1000}}},
		{name: "inactive agreement ignored", mutate: func(in *QuoteInput) {
			in.Agreement = agreement(pctOff("10"), true)
			in.Agreement.Active = false
		}, want: 4800},
		{name: "agreement with unmatched conditions ignored", mutate: func(in *QuoteInput) {
			in.Agreement = agreement(pctOff("10"), true)
			in.Agreement.Conditions = Conditions{Channels: []Channel{ChannelOnline}}
		}, want: 4800},
		{name: "promo valid window", mutate: func(in *QuoteInput) {
			in.PromoCode = promo("TEN", pctOff("10"), false)
			in.PromoCode.ValidFrom, in.PromoCode.ValidTo = new(d("2027-06-01")), new(d("2027-06-30"))
		}, want: 4320, discounts: []discountWant{{"promo:TEN", -480}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, rules := fourEightyInput()
			if tc.mutate != nil {
				tc.mutate(&in)
			}
			b := mustQuote(t, in, testCatalog(), ruleSet(append(rules, tc.rules...)...))
			var got []discountWant
			for _, x := range b.Discounts {
				got = append(got, discountWant{x.Source, x.Amount})
			}
			if b.Total != tc.want || !slices.Equal(got, tc.discounts) {
				t.Fatalf("total %d discounts %v, want %d %v", b.Total, got, tc.want, tc.discounts)
			}
		})
	}
}

func TestQuote_DiscountUnits(t *testing.T) {
	cases := []struct {
		name  string
		in    QuoteInput
		rule  Rule
		promo *PromoCode
		want  money.Cents
	}{
		{
			name: "amount off per day", in: span("2027-07-05", "2027-07-07", FormulaMultiDay),
			promo: promo("D", amountOff(300, PerDay), false), want: 9000 - 900,
		},
		{name: "amount off per line two lines", in: func() QuoteInput {
			in := oneDay()
			in.Lines = append(in.Lines, LineInput{PackID: "p_base", ZoneID: "z2"})
			return in
		}(), promo: promo("L", amountOff(1000, PerLine), false), want: 6000 - 2000},
		{name: "amount off per day counts whole booking units", in: func() QuoteInput {
			in := span("2027-07-05", "2027-07-11", FormulaWeek)
			return in
		}(), promo: promo("D", amountOff(100, PerDay), false), want: 18000 - 700},
		{
			name: "discount restricted to rate period days", in: span("2027-07-30", "2027-08-02", FormulaMultiDay),
			rule: discRule("d_mid", pctOff("50"), 0, true, Conditions{RatePeriods: []string{"mid"}}), want: 12000 - 3000,
		},
		{
			name: "free days 7 for 6", in: span("2027-07-05", "2027-07-11", FormulaMultiDay),
			rule: discRule("d_free", Action{Type: ActionFreeDays, Every: 7, Free: 1}, 0, true, Conditions{}), want: 6 * 3000,
		},
		{
			name: "free days not enough days", in: span("2027-07-05", "2027-07-10", FormulaMultiDay),
			rule: discRule("d_free", Action{Type: ActionFreeDays, Every: 7, Free: 1}, 0, true, Conditions{}), want: 6 * 3000,
		},
		{
			name: "free days twice over 14 days", in: span("2027-07-05", "2027-07-18", FormulaMultiDay),
			rule: discRule("d_free", Action{Type: ActionFreeDays, Every: 7, Free: 1}, 0, true, Conditions{}), want: 12 * 3000,
		},
		{
			name: "free days 3 for 2 over 7 days", in: span("2027-07-05", "2027-07-11", FormulaMultiDay),
			rule: discRule("d_free", Action{Type: ActionFreeDays, Every: 3, Free: 1}, 0, true, Conditions{}), want: 5 * 3000,
		},
		{
			name: "free days skip fixed price lines", in: span("2027-07-05", "2027-07-11", FormulaWeek),
			rule: discRule("d_free", Action{Type: ActionFreeDays, Every: 7, Free: 1}, 0, true, Conditions{}), want: 18000,
		},
		{
			name: "percent on fixed price line", in: span("2027-07-05", "2027-07-11", FormulaWeek),
			promo: promo("TEN", pctOff("10"), false), want: 16200,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := []Rule{baseRule("r_day", 3000, 0, Conditions{})}
			if tc.rule.ID != "" {
				rules = append(rules, tc.rule)
			}
			rs := ruleSet(rules...)
			rs.FixedPrices = []FixedDurationPrice{{ID: "f", PackID: "p_base", Formula: FormulaWeek, PriceCategory: "row1", Amount: 18000}}
			in := tc.in
			in.PromoCode = tc.promo
			b := mustQuote(t, in, testCatalog(), rs)
			if b.Total != tc.want {
				t.Fatalf("total %d, want %d (discounts %+v)", b.Total, tc.want, b.Discounts)
			}
		})
	}
}

func TestQuote_FreeDaysCheapestDays(t *testing.T) {
	// Mon-Sun with weekend at 40.00: the free day is the cheapest (Monday).
	rs := ruleSet(
		baseRule("r_day", 3000, 0, Conditions{}),
		baseRule("r_we", 4000, 1, Conditions{Weekdays: []int{6, 7}}),
		discRule("d_free", Action{Type: ActionFreeDays, Every: 7, Free: 2}, 0, true, Conditions{}),
	)
	b := mustQuote(t, span("2027-07-05", "2027-07-11", FormulaMultiDay), testCatalog(), rs)
	if b.Lines[0].Total != 5*3000+2*4000 || b.Discounts[0].Amount != -6000 || b.Total != 17000 {
		t.Fatalf("line %d discounts %+v total %d", b.Lines[0].Total, b.Discounts, b.Total)
	}
}

func TestQuote_PromoNotApplicable(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*PromoCode)
	}{
		{"inactive", func(p *PromoCode) { p.Active = false }},
		{"expired", func(p *PromoCode) { p.ValidTo = new(d("2027-05-31")) }},
		{"not yet valid", func(p *PromoCode) { p.ValidFrom = new(d("2027-06-02")) }},
		{"channel mismatch", func(p *PromoCode) { p.Conditions.Channels = []Channel{ChannelOnline} }},
		{"period mismatch", func(p *PromoCode) { p.Conditions.RatePeriods = []string{"peak"} }},
		{"min days not reached", func(p *PromoCode) { p.Conditions.MinDays = ip(5) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, rules := fourEightyInput()
			in.PromoCode = promo("TEN", pctOff("10"), false)
			tc.mutate(in.PromoCode)
			_, err := Quote(in, testCatalog(), ruleSet(rules...))
			if !errors.Is(err, ErrPromoNotApplicable) {
				t.Fatalf("err = %v, want ErrPromoNotApplicable", err)
			}
		})
	}
}

func TestQuote_InvalidPromoAndAgreement(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*QuoteInput)
	}{
		{"promo with set action", func(in *QuoteInput) { in.PromoCode = promo("X", Action{Type: ActionSet, Amount: 1}, false) }},
		{"promo without code", func(in *QuoteInput) { in.PromoCode = promo("", pctOff("10"), false) }},
		{"promo window inverted", func(in *QuoteInput) {
			in.PromoCode = promo("X", pctOff("10"), false)
			in.PromoCode.ValidFrom, in.PromoCode.ValidTo = new(d("2027-07-01")), new(d("2027-06-01"))
		}},
		{"agreement with add action", func(in *QuoteInput) {
			in.Agreement = &Agreement{ID: "a", Code: "A", Action: Action{Type: ActionAdd, Amount: 1}, Active: true}
		}},
		{"agreement percent too large", func(in *QuoteInput) {
			in.Agreement = &Agreement{ID: "a", Code: "A", Action: pctOff("101"), Active: true}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, rules := fourEightyInput()
			tc.mutate(&in)
			_, err := Quote(in, testCatalog(), ruleSet(rules...))
			if !errors.Is(err, ErrInvalidRule) {
				t.Fatalf("err = %v, want ErrInvalidRule", err)
			}
		})
	}
}
