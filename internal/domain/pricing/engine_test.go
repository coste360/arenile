package pricing

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

func TestQuote_ModifierActions(t *testing.T) {
	cases := []struct {
		name   string
		base   money.Cents
		action Action
		want   money.Cents
		effect string
	}{
		{"add", 3000, Action{Type: ActionAdd, Amount: 500}, 3500, "+500"},
		{"add negative", 3000, Action{Type: ActionAdd, Amount: -500}, 2500, "-500"},
		{"add negative clamps at zero", 3000, Action{Type: ActionAdd, Amount: -5000}, 0, "-5000"},
		{"multiply 1.2", 3000, Action{Type: ActionMultiply, Factor: dec("1.2")}, 3600, "x1.2"},
		{"multiply 1.15", 3000, Action{Type: ActionMultiply, Factor: dec("1.15")}, 3450, "x1.15"},
		{"multiply 0.333 rounds", 3001, Action{Type: ActionMultiply, Factor: dec("0.333")}, 999, "x0.333"},
		{"multiply half-even tie down", 3001, Action{Type: ActionMultiply, Factor: dec("0.5")}, 1500, "x0.5"},
		{"multiply half-even tie up", 3003, Action{Type: ActionMultiply, Factor: dec("0.5")}, 1502, "x0.5"},
		{"multiply by zero", 3000, Action{Type: ActionMultiply, Factor: dec("0")}, 0, "x0"},
		{"multiply by one", 3000, Action{Type: ActionMultiply, Factor: dec("1.000")}, 3000, "x1"},
		{"percent_off 10", 3000, Action{Type: ActionPercentOff, Percent: dec("10")}, 2700, "-10%"},
		{"percent_off decimal 12.5", 3000, Action{Type: ActionPercentOff, Percent: dec("12.5")}, 2625, "-12.5%"},
		{"percent_off rounding", 3001, Action{Type: ActionPercentOff, Percent: dec("12.5")}, 2626, "-12.5%"},
		{"percent_off 100", 3000, Action{Type: ActionPercentOff, Percent: dec("100")}, 0, "-100%"},
		{"percent_off 0.01", 10000, Action{Type: ActionPercentOff, Percent: dec("0.01")}, 9999, "-0.01%"},
		{"amount_off", 3000, Action{Type: ActionAmountOff, Amount: 1000}, 2000, "-1000"},
		{"amount_off per day explicit", 3000, Action{Type: ActionAmountOff, Amount: 1000, Per: PerDay}, 2000, "-1000"},
		{"amount_off clamps at zero", 3000, Action{Type: ActionAmountOff, Amount: 5000}, 0, "-5000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := ruleSet(baseRule("r_day", tc.base, 0, Conditions{}), modRule("r_mod", tc.action, 0, true, Conditions{}))
			b := mustQuote(t, oneDay(), testCatalog(), rs)
			day := b.Lines[0].Days[0]
			if day.Base != tc.base || day.Total != tc.want {
				t.Fatalf("base %d total %d, want %d %d", day.Base, day.Total, tc.base, tc.want)
			}
			want := []AppliedRule{
				{Rule: "r_day", Stage: StageBase, Effect: fmt.Sprintf("set %d", tc.base)},
				{Rule: "r_mod", Stage: StageModifier, Effect: tc.effect},
			}
			if !slices.Equal(day.Applied, want) {
				t.Fatalf("applied %+v, want %+v", day.Applied, want)
			}
		})
	}
}

func TestQuote_BaseSet(t *testing.T) {
	cases := []struct {
		name   string
		amount money.Cents
		days   int
		want   money.Cents
	}{
		{"set zero is free", 0, 1, 0},
		{"set one day", 2500, 1, 2500},
		{"set three days", 2500, 3, 7500},
		{"set large amount", 1_000_000, 2, 2_000_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := oneDay()
			in.EndsOn = in.StartsOn.AddDays(tc.days - 1)
			b := mustQuote(t, in, testCatalog(), ruleSet(baseRule("r", tc.amount, 0, Conditions{})))
			if b.Total != tc.want || len(b.Lines[0].Days) != tc.days {
				t.Fatalf("total %d days %d, want %d %d", b.Total, len(b.Lines[0].Days), tc.want, tc.days)
			}
		})
	}
}

func TestQuote_BaseConflicts(t *testing.T) {
	mid := Conditions{RatePeriods: []string{"mid"}}
	monday := Conditions{Weekdays: []int{1}}
	withSeq := func(r Rule, seq int64) Rule { r.Seq = seq; return r }
	withTargets := func(r Rule, ids ...string) Rule { r.TargetIDs = ids; return r }
	inactive := func(r Rule) Rule { r.Active = false; return r }
	cases := []struct {
		name   string
		rules  []Rule
		want   money.Cents
		winner string
	}{
		{"higher priority wins", []Rule{baseRule("a", 3000, 1, Conditions{}), baseRule("b", 3500, 5, Conditions{})}, 3500, "b"},
		{"higher priority wins regardless of order", []Rule{baseRule("b", 3500, 5, Conditions{}), baseRule("a", 3000, 1, Conditions{})}, 3500, "b"},
		{"higher priority beats more specific", []Rule{baseRule("a", 3000, 5, Conditions{}), baseRule("b", 3500, 1, mid)}, 3000, "a"},
		{"same priority more specific wins", []Rule{baseRule("a", 3000, 1, Conditions{}), baseRule("b", 3500, 1, mid)}, 3500, "b"},
		{"same priority two conditions beat one", []Rule{
			baseRule("a", 3000, 1, mid),
			baseRule("b", 3700, 1, Conditions{RatePeriods: []string{"mid"}, Weekdays: []int{1}}),
		}, 3700, "b"},
		{"target_ids count as specificity", []Rule{baseRule("a", 3000, 1, Conditions{}), withTargets(baseRule("b", 3300, 1, Conditions{}), "p_base")}, 3300, "b"},
		{"same priority and specificity most recent wins", []Rule{withSeq(baseRule("a", 3000, 1, mid), 2), withSeq(baseRule("b", 3500, 1, monday), 1)}, 3000, "a"},
		{"most recent wins regardless of order", []Rule{withSeq(baseRule("b", 3500, 1, monday), 9), withSeq(baseRule("a", 3000, 1, mid), 2)}, 3500, "b"},
		{"priority beats recency", []Rule{withSeq(baseRule("a", 3000, 1, Conditions{}), 9), withSeq(baseRule("b", 3500, 2, Conditions{}), 1)}, 3500, "b"},
		{"full tie lowest id wins", []Rule{baseRule("b", 3500, 1, Conditions{}), baseRule("a", 3000, 1, Conditions{})}, 3000, "a"},
		{"inactive rule ignored", []Rule{baseRule("a", 3000, 1, Conditions{}), inactive(baseRule("b", 3500, 9, Conditions{}))}, 3000, "a"},
		{"rule for another pack ignored", []Rule{baseRule("a", 3000, 1, Conditions{}), withTargets(baseRule("b", 9900, 9, Conditions{}), "p_gaz")}, 3000, "a"},
		{"non matching rule ignored", []Rule{baseRule("a", 3000, 1, Conditions{}), baseRule("b", 9900, 9, Conditions{RatePeriods: []string{"peak"}})}, 3000, "a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := mustQuote(t, oneDay(), testCatalog(), ruleSet(tc.rules...))
			day := b.Lines[0].Days[0]
			if day.Total != tc.want || day.Applied[0].Rule != tc.winner {
				t.Fatalf("total %d winner %s, want %d %s", day.Total, day.Applied[0].Rule, tc.want, tc.winner)
			}
		})
	}
}

func TestQuote_ModifierStacking(t *testing.T) {
	add := func(v money.Cents) Action { return Action{Type: ActionAdd, Amount: v} }
	pct := func(p string) Action { return Action{Type: ActionPercentOff, Percent: dec(p)} }
	mul := func(f string) Action { return Action{Type: ActionMultiply, Factor: dec(f)} }
	seq := func(r Rule, s int64) Rule { r.Seq = s; return r }
	none := Conditions{}
	cases := []struct {
		name    string
		mods    []Rule
		want    money.Cents
		applied []string
	}{
		{"stackable in priority order add then multiply", []Rule{
			modRule("m_add", add(500), 1, true, none), modRule("m_mul", mul("1.1"), 2, true, none),
		}, 3850, []string{"m_add", "m_mul"}},
		{"stackable in priority order multiply then add", []Rule{
			modRule("m_add", add(500), 2, true, none), modRule("m_mul", mul("1.1"), 1, true, none),
		}, 3800, []string{"m_mul", "m_add"}},
		{"stackable same priority older first", []Rule{
			seq(modRule("m_add", add(500), 1, true, none), 5), seq(modRule("m_mul", mul("1.1"), 1, true, none), 3),
		}, 3800, []string{"m_mul", "m_add"}},
		{"three stackable sum", []Rule{
			modRule("a", add(100), 1, true, none), modRule("b", add(200), 2, true, none), modRule("c", add(300), 3, true, none),
		}, 3600, []string{"a", "b", "c"}},
		{"non-stackable best for customer among discounts", []Rule{
			modRule("n_pct", pct("10"), 1, false, none), modRule("n_amt", Action{Type: ActionAmountOff, Amount: 200}, 2, false, none),
		}, 2700, []string{"n_pct"}},
		{"non-stackable best for customer among surcharges", []Rule{
			modRule("n_500", add(500), 1, false, none), modRule("n_200", add(200), 2, false, none),
		}, 3200, []string{"n_200"}},
		{"non-stackable single applies", []Rule{modRule("n", add(500), 1, false, none)}, 3500, []string{"n"}},
		{"stackable plus best non-stackable", []Rule{
			modRule("s_add", add(500), 1, true, none),
			modRule("n_pct", pct("10"), 2, false, none),
			modRule("n_amt", Action{Type: ActionAmountOff, Amount: 400}, 3, false, none),
		}, 3100, []string{"s_add", "n_amt"}},
		{"non-stackable evaluated in position", []Rule{
			modRule("s_add", add(1000), 5, true, none),
			modRule("n_pct", pct("50"), 1, false, none),
			modRule("n_amt", Action{Type: ActionAmountOff, Amount: 1600}, 2, false, none),
		}, 2400, []string{"n_amt", "s_add"}},
		{"non-stackable tie prefers higher priority", []Rule{
			modRule("n_low", add(500), 1, false, none), modRule("n_high", add(500), 4, false, none),
		}, 3500, []string{"n_high"}},
		{"non matching modifier ignored", []Rule{
			modRule("m", add(500), 1, true, Conditions{Weekdays: []int{6}}),
		}, 3000, nil},
		{"modifier for other pack ignored", []Rule{
			func() Rule { r := modRule("m", add(500), 1, true, none); r.TargetIDs = []string{"p_gaz"}; return r }(),
		}, 3000, nil},
		{"extra modifier does not touch pack", []Rule{
			func() Rule { r := modRule("m", add(500), 1, true, none); r.TargetType = TargetExtra; return r }(),
		}, 3000, nil},
		{"clamp then add keeps zero floor per step", []Rule{
			modRule("a", add(-5000), 1, true, none), modRule("b", add(100), 2, true, none),
		}, 100, []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := ruleSet(append([]Rule{baseRule("r_day", 3000, 0, Conditions{})}, tc.mods...)...)
			day := mustQuote(t, oneDay(), testCatalog(), rs).Lines[0].Days[0]
			var got []string
			for _, a := range day.Applied[1:] {
				got = append(got, a.Rule)
			}
			if day.Total != tc.want || !slices.Equal(got, tc.applied) {
				t.Fatalf("total %d applied %v, want %d %v", day.Total, got, tc.want, tc.applied)
			}
		})
	}
}

func TestQuote_HalfDay(t *testing.T) {
	am := Conditions{Slots: []calendar.Slot{calendar.AM}}
	pm := Conditions{Slots: []calendar.Slot{calendar.PM}}
	cases := []struct {
		name    string
		slots   calendar.Slots
		factor  string
		rules   []Rule
		want    money.Cents
		applied []string
	}{
		{"AM via factor", calendar.SlotsAM, "0.6", []Rule{baseRule("r_day", 3000, 0, Conditions{})}, 1800, []string{"r_day", halfDayFactorRule}},
		{"PM via factor", calendar.SlotsPM, "0.6", []Rule{baseRule("r_day", 3000, 0, Conditions{})}, 1800, []string{"r_day", halfDayFactorRule}},
		{"PM via slot rule", calendar.SlotsPM, "0.6", []Rule{baseRule("r_day", 3000, 0, Conditions{}), baseRule("r_pm", 2000, 0, pm)}, 2000, []string{"r_pm"}},
		{"AM ignores PM slot rule", calendar.SlotsAM, "0.6", []Rule{baseRule("r_day", 3000, 0, Conditions{}), baseRule("r_pm", 2000, 0, pm)}, 1800, []string{"r_day", halfDayFactorRule}},
		{"slot rule wins over higher priority day rule", calendar.SlotsAM, "0.6", []Rule{baseRule("r_day", 3000, 9, Conditions{}), baseRule("r_am", 1500, 0, am)}, 1500, []string{"r_am"}},
		{"full day ignores AM rule", calendar.SlotsFull, "0.6", []Rule{baseRule("r_day", 3000, 0, Conditions{}), baseRule("r_am", 1500, 9, am)}, 3000, []string{"r_day"}},
		{"factor half-even tie down", calendar.SlotsAM, "0.5", []Rule{baseRule("r_day", 3001, 0, Conditions{})}, 1500, []string{"r_day", halfDayFactorRule}},
		{"factor half-even tie up", calendar.SlotsAM, "0.5", []Rule{baseRule("r_day", 3003, 0, Conditions{})}, 1502, []string{"r_day", halfDayFactorRule}},
		{"factor one", calendar.SlotsAM, "1", []Rule{baseRule("r_day", 3000, 0, Conditions{})}, 3000, []string{"r_day", halfDayFactorRule}},
		{"modifier after factor", calendar.SlotsAM, "0.6", []Rule{
			baseRule("r_day", 3000, 0, Conditions{}), modRule("m", Action{Type: ActionAdd, Amount: 500}, 0, true, Conditions{}),
		}, 2300, []string{"r_day", halfDayFactorRule, "m"}},
		{"slot modifier on half day", calendar.SlotsPM, "0.6", []Rule{
			baseRule("r_day", 3000, 0, Conditions{}), modRule("m_pm", Action{Type: ActionAmountOff, Amount: 300}, 0, true, pm),
		}, 1500, []string{"r_day", halfDayFactorRule, "m_pm"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := testCatalog()
			cat.Settings.HalfDayFactor = MustParseDecimal(tc.factor)
			in := oneDay()
			in.Slots = tc.slots
			if tc.slots != calendar.SlotsFull {
				in.Formula = FormulaHalfDay
			}
			day := mustQuote(t, in, cat, ruleSet(tc.rules...)).Lines[0].Days[0]
			var got []string
			for _, a := range day.Applied {
				got = append(got, a.Rule)
			}
			if day.Total != tc.want || !slices.Equal(got, tc.applied) || day.Slots != tc.slots {
				t.Fatalf("total %d applied %v slots %s, want %d %v", day.Total, got, day.Slots, tc.want, tc.applied)
			}
		})
	}
}

func TestQuote_HalfDayNoPrice(t *testing.T) {
	in := oneDay()
	in.Slots, in.Formula = calendar.SlotsAM, FormulaHalfDay
	_, err := Quote(in, testCatalog(), ruleSet(baseRule("r_pm", 2000, 0, Conditions{Slots: []calendar.Slot{calendar.PM}})))
	if !errors.Is(err, ErrNoPrice) {
		t.Fatalf("err = %v, want ErrNoPrice", err)
	}
}

func TestQuote_FixedDuration(t *testing.T) {
	week := span("2027-07-05", "2027-07-11", FormulaWeek)
	fp := func(id, pack string, f Formula, cat string, period *string, amount money.Cents) FixedDurationPrice {
		return FixedDurationPrice{ID: id, PackID: pack, Formula: f, PriceCategory: cat, RatePeriodCode: period, Amount: amount}
	}
	cases := []struct {
		name       string
		in         QuoteInput
		fixed      []FixedDurationPrice
		wantTotal  money.Cents
		wantFixed  bool
		wantPeriod string
	}{
		{"week with period price", week, []FixedDurationPrice{fp("f1", "p_base", FormulaWeek, "row1", sp("mid"), 18000)}, 18000, true, "mid"},
		{"week generic price", week, []FixedDurationPrice{fp("f1", "p_base", FormulaWeek, "row1", nil, 17000)}, 17000, true, ""},
		{"period price preferred over generic", week, []FixedDurationPrice{
			fp("f0", "p_base", FormulaWeek, "row1", nil, 17000), fp("f1", "p_base", FormulaWeek, "row1", sp("mid"), 18000),
		}, 18000, true, "mid"},
		{"generic used when period differs", week, []FixedDurationPrice{
			fp("f0", "p_base", FormulaWeek, "row1", nil, 17000), fp("f1", "p_base", FormulaWeek, "row1", sp("high"), 25000),
		}, 17000, true, ""},
		{"period of start date decides", span("2027-07-29", "2027-08-04", FormulaWeek), []FixedDurationPrice{
			fp("f1", "p_base", FormulaWeek, "row1", sp("mid"), 18000), fp("f2", "p_base", FormulaWeek, "row1", sp("high"), 25000),
		}, 18000, true, "mid"},
		{"other category falls back to days", week, []FixedDurationPrice{fp("f1", "p_base", FormulaWeek, "row2", nil, 17000)}, 21000, false, ""},
		{"other pack falls back to days", week, []FixedDurationPrice{fp("f1", "p_gaz", FormulaWeek, "row1", nil, 17000)}, 21000, false, ""},
		{"other formula falls back to days", week, []FixedDurationPrice{fp("f1", "p_base", FormulaMonth, "row1", nil, 50000)}, 21000, false, ""},
		{"day formula ignores fixed prices", span("2027-07-05", "2027-07-11", FormulaMultiDay), []FixedDurationPrice{fp("f1", "p_base", FormulaWeek, "row1", nil, 17000)}, 21000, false, ""},
		{"season price", span("2027-05-01", "2027-09-30", FormulaSeason), []FixedDurationPrice{fp("f1", "p_base", FormulaSeason, "row1", nil, 150000)}, 150000, true, ""},
		{"fortnight price", span("2027-07-05", "2027-07-18", FormulaFortnight), []FixedDurationPrice{fp("f1", "p_base", FormulaFortnight, "row1", nil, 33000)}, 33000, true, ""},
		{
			"half-day week uses factor", func() QuoteInput { in := week; in.Slots = calendar.SlotsAM; return in }(),
			[]FixedDurationPrice{fp("f1", "p_base", FormulaWeek, "row1", nil, 17000)},
			10200, true, "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := ruleSet(defaultRules()...)
			rs.FixedPrices = tc.fixed
			b := mustQuote(t, tc.in, testCatalog(), rs)
			l := b.Lines[0]
			if b.Total != tc.wantTotal || (l.Fixed != nil) != tc.wantFixed {
				t.Fatalf("total %d fixed %+v, want %d fixed=%v", b.Total, l.Fixed, tc.wantTotal, tc.wantFixed)
			}
			if tc.wantFixed && (len(l.Days) != 0 || l.Fixed.RatePeriod != tc.wantPeriod || l.Fixed.Formula != tc.in.Formula) {
				t.Fatalf("fixed line %+v days %d", l.Fixed, len(l.Days))
			}
		})
	}
}

func TestQuote_FixedDurationWithExtras(t *testing.T) {
	rs := ruleSet(defaultRules()...)
	rs.FixedPrices = []FixedDurationPrice{{ID: "f", PackID: "p_base", Formula: FormulaWeek, PriceCategory: "row1", Amount: 18000}}
	in := span("2027-07-05", "2027-07-11", FormulaWeek)
	in.Lines[0].Extras = []ExtraInput{{ExtraID: "x_sunbed", Quantity: 1}}
	b := mustQuote(t, in, testCatalog(), rs)
	if b.Lines[0].Total != 18000+7*800 || b.Lines[0].Extras[0].Total != 5600 {
		t.Fatalf("line %+v", b.Lines[0])
	}
}

func TestQuote_Extras(t *testing.T) {
	am := Conditions{Slots: []calendar.Slot{calendar.AM}}
	pm := Conditions{Slots: []calendar.Slot{calendar.PM}}
	cases := []struct {
		name  string
		days  int
		slots calendar.Slots
		extra ExtraInput
		rules []Rule
		want  money.Cents
	}{
		{"per_day one day", 1, calendar.SlotsFull, ExtraInput{"x_sunbed", 1}, nil, 800},
		{"per_day three days", 3, calendar.SlotsFull, ExtraInput{"x_sunbed", 1}, nil, 2400},
		{"per_day quantity two", 3, calendar.SlotsFull, ExtraInput{"x_sunbed", 2}, nil, 4800},
		{"per_day half day uses factor", 1, calendar.SlotsAM, ExtraInput{"x_sunbed", 1}, nil, 480},
		{"per_day half day slot rule", 1, calendar.SlotsAM, ExtraInput{"x_sunbed", 1}, []Rule{extraBase("rx_sb_am", "x_sunbed", 500, 0, am)}, 500},
		{"per_booking once", 3, calendar.SlotsFull, ExtraInput{"x_cabin", 1}, nil, 1500},
		{"per_booking quantity", 3, calendar.SlotsFull, ExtraInput{"x_cabin", 2}, nil, 3000},
		{"per_booking half day no factor", 1, calendar.SlotsAM, ExtraInput{"x_cabin", 1}, nil, 1500},
		{"per_slot full day", 1, calendar.SlotsFull, ExtraInput{"x_park", 1}, nil, 600},
		{"per_slot half day", 1, calendar.SlotsPM, ExtraInput{"x_park", 1}, nil, 300},
		{"per_slot two days quantity", 2, calendar.SlotsFull, ExtraInput{"x_park", 3}, nil, 3600},
		{"per_slot slot-specific price", 1, calendar.SlotsFull, ExtraInput{"x_park", 1}, []Rule{extraBase("rx_park_pm", "x_park", 400, 1, pm)}, 700},
		{"extra modifier", 1, calendar.SlotsFull, ExtraInput{"x_sunbed", 2}, []Rule{func() Rule {
			r := modRule("mx", Action{Type: ActionAdd, Amount: 200}, 0, true, Conditions{})
			r.TargetType, r.TargetIDs = TargetExtra, []string{"x_sunbed"}
			return r
		}()}, 2000},
		{"extra base priority", 1, calendar.SlotsFull, ExtraInput{"x_sunbed", 1}, []Rule{extraBase("rx_sb_cat", "x_sunbed", 1000, 1, Conditions{PriceCategories: []string{"row1"}})}, 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := oneDay()
			in.EndsOn = in.StartsOn.AddDays(tc.days - 1)
			in.Slots = tc.slots
			if tc.days > 1 {
				in.Formula = FormulaMultiDay
			}
			if tc.slots != calendar.SlotsFull {
				in.Formula = FormulaHalfDay
			}
			in.Lines[0].Extras = []ExtraInput{tc.extra}
			b := mustQuote(t, in, testCatalog(), ruleSet(append(defaultRules(), tc.rules...)...))
			x := b.Lines[0].Extras[0]
			if x.Total != tc.want || x.Qty != tc.extra.Quantity {
				t.Fatalf("extra %+v, want total %d", x, tc.want)
			}
		})
	}
}

func TestQuote_BookingLevelExtras(t *testing.T) {
	in := oneDay()
	in.Extras = []ExtraInput{{"x_cabin", 1}}
	rs := ruleSet(append(defaultRules(),
		// line-level conditions never match booking-level extras
		extraBase("rx_cabin_row1", "x_cabin", 9900, 5, Conditions{PriceCategories: []string{"row1"}}))...)
	b := mustQuote(t, in, testCatalog(), rs)
	if len(b.Extras) != 1 || b.Extras[0].Total != 1500 || b.Total != 4500 {
		t.Fatalf("extras %+v total %d", b.Extras, b.Total)
	}
}

func TestQuote_Errors(t *testing.T) {
	good := ruleSet(defaultRules()...)
	cases := []struct {
		name    string
		mutate  func(*QuoteInput)
		cat     func(*Catalog)
		rs      *RuleSet
		wantErr error
	}{
		{name: "no base rule", rs: &RuleSet{RatePeriods: testPeriods()}, wantErr: ErrNoPrice},
		{name: "base rule only some days", rs: func() *RuleSet {
			r := ruleSet(baseRule("r_jul", 3000, 0, Conditions{DateRanges: []DateRange{{d("2027-07-01"), d("2027-07-06")}}}))
			return &r
		}(), mutate: func(in *QuoteInput) { in.EndsOn = d("2027-07-08") }, wantErr: ErrNoPrice},
		{name: "base rule other category", rs: func() *RuleSet {
			r := ruleSet(baseRule("r", 3000, 0, Conditions{PriceCategories: []string{"row2"}}))
			return &r
		}(), wantErr: ErrNoPrice},
		{name: "extra without price", mutate: func(in *QuoteInput) { in.Lines[0].Extras = []ExtraInput{{"x_nop", 1}} }, wantErr: ErrNoPrice},
		{name: "unknown pack", mutate: func(in *QuoteInput) { in.Lines[0].PackID = "nope" }, wantErr: ErrInvalidInput},
		{name: "unknown zone", mutate: func(in *QuoteInput) { in.Lines[0].ZoneID = "nope" }, wantErr: ErrInvalidInput},
		{name: "unknown spot", mutate: func(in *QuoteInput) { in.Lines[0].SpotID = "nope" }, wantErr: ErrInvalidInput},
		{name: "spot in other zone", mutate: func(in *QuoteInput) { in.Lines[0].SpotID = "s_b5" }, wantErr: ErrInvalidInput},
		{name: "unknown extra", mutate: func(in *QuoteInput) { in.Lines[0].Extras = []ExtraInput{{"nope", 1}} }, wantErr: ErrInvalidInput},
		{name: "zero quantity", mutate: func(in *QuoteInput) { in.Extras = []ExtraInput{{"x_cabin", 0}} }, wantErr: ErrInvalidInput},
		{name: "ends before starts", mutate: func(in *QuoteInput) { in.EndsOn = d("2027-07-01") }, wantErr: ErrInvalidInput},
		{name: "missing today", mutate: func(in *QuoteInput) { in.Today = calendar.Date{} }, wantErr: ErrInvalidInput},
		{name: "unknown channel", mutate: func(in *QuoteInput) { in.Channel = "fax" }, wantErr: ErrInvalidInput},
		{name: "unknown formula", mutate: func(in *QuoteInput) { in.Formula = "year" }, wantErr: ErrInvalidInput},
		{name: "unknown slots", mutate: func(in *QuoteInput) { in.Slots = "NIGHT" }, wantErr: ErrInvalidInput},
		{name: "half_day formula full slots", mutate: func(in *QuoteInput) { in.Formula = FormulaHalfDay }, wantErr: ErrInvalidInput},
		{name: "no lines", mutate: func(in *QuoteInput) { in.Lines = nil }, wantErr: ErrInvalidInput},
		{name: "range too long", mutate: func(in *QuoteInput) { in.EndsOn = d("2029-01-01") }, wantErr: ErrInvalidInput},
		{name: "half day factor zero", cat: func(c *Catalog) { c.Settings.HalfDayFactor = Decimal{} }, wantErr: ErrInvalidInput},
		{name: "half day factor above one", cat: func(c *Catalog) { c.Settings.HalfDayFactor = MustParseDecimal("1.5") }, wantErr: ErrInvalidInput},
		{name: "unknown rounding", cat: func(c *Catalog) { c.Settings.Rounding = "dime" }, wantErr: ErrInvalidInput},
		{name: "vat out of range", cat: func(c *Catalog) { c.Settings.DefaultVATRateBP = 10000 }, wantErr: ErrInvalidInput},
		{name: "pack vat out of range", cat: func(c *Catalog) { c.Packs[0].VATRateBP = i64(-1) }, wantErr: ErrInvalidInput},
		{name: "extra pricing unit invalid", cat: func(c *Catalog) { c.Extras[0].PricingUnit = "per_hour" }, wantErr: ErrInvalidInput},
		{name: "invalid active rule", rs: func() *RuleSet {
			r := ruleSet(baseRule("r", 3000, 0, Conditions{}), modRule("bad", Action{Type: ActionSet, Amount: 1}, 0, true, Conditions{}))
			return &r
		}(), wantErr: ErrInvalidRule},
		{name: "duplicate rule id", rs: func() *RuleSet {
			r := ruleSet(baseRule("r", 3000, 0, Conditions{}), baseRule("r", 3100, 0, Conditions{}))
			return &r
		}(), wantErr: ErrInvalidRule},
		{name: "overlapping periods", rs: func() *RuleSet {
			r := ruleSet(defaultRules()...)
			r.RatePeriods = append(r.RatePeriods, RatePeriod{Code: "ferragosto", Ranges: []DateRange{{d("2027-08-14"), d("2027-08-16")}}})
			return &r
		}(), wantErr: ErrInvalidRule},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := oneDay()
			if tc.mutate != nil {
				tc.mutate(&in)
			}
			cat := testCatalog()
			if tc.cat != nil {
				tc.cat(&cat)
			}
			rs := good
			if tc.rs != nil {
				rs = *tc.rs
			}
			_, err := Quote(in, cat, rs)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestQuote_NoPriceErrorDetails(t *testing.T) {
	rs := ruleSet(baseRule("r_jul", 3000, 0, Conditions{DateRanges: []DateRange{{d("2027-07-01"), d("2027-07-06")}}}))
	in := span("2027-07-05", "2027-07-08", FormulaMultiDay)
	_, err := Quote(in, testCatalog(), rs)
	var np *NoPriceError
	if !errors.As(err, &np) {
		t.Fatalf("err = %v, want *NoPriceError", err)
	}
	if np.Target != TargetPack || np.ItemCode != "base" || np.PriceCategory != "row1" || np.Date.String() != "2027-07-07" {
		t.Fatalf("details %+v", np)
	}
}

func TestQuote_InactiveInvalidRuleIgnored(t *testing.T) {
	bad := modRule("bad", Action{Type: ActionSet}, 0, true, Conditions{})
	bad.Active = false
	b := mustQuote(t, oneDay(), testCatalog(), ruleSet(baseRule("r", 3000, 0, Conditions{}), bad))
	if b.Total != 3000 {
		t.Fatalf("total %d", b.Total)
	}
}

func TestQuote_MultipleLines(t *testing.T) {
	in := span("2027-07-05", "2027-07-06", FormulaMultiDay)
	in.Lines = append(in.Lines, LineInput{PackID: "p_gaz", SpotID: "s_b5"})
	rs := ruleSet(append(defaultRules(), func() Rule {
		r := baseRule("r_gaz", 6000, 1, Conditions{})
		r.TargetIDs = []string{"p_gaz"}
		return r
	}())...)
	b := mustQuote(t, in, testCatalog(), rs)
	if len(b.Lines) != 2 || b.Lines[1].Total != 12000 || b.Lines[1].Spot != "B5" || b.Lines[1].Zone != "row2" || b.Total != 18000 {
		t.Fatalf("lines %+v total %d", b.Lines, b.Total)
	}
}
