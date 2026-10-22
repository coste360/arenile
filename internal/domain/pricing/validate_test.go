package pricing

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
)

func TestValidateRule_Cases(t *testing.T) {
	ok := func(r Rule) Rule { return r }
	base := baseRule("r", 3000, 0, Conditions{})
	mod := func(a Action) Rule { return modRule("m", a, 0, true, Conditions{}) }
	disc := func(a Action) Rule { return discRule("d", a, 0, true, Conditions{}) }
	withCond := func(c Conditions) Rule { return baseRule("r", 3000, 0, c) }
	cases := []struct {
		name  string
		rule  Rule
		field string // empty = valid
	}{
		{"valid base", ok(base), ""},
		{"missing id", func() Rule { r := base; r.ID = ""; return r }(), "id"},
		{"bad target type", func() Rule { r := base; r.TargetType = "zone"; return r }(), "target_type"},
		{"bad stage", func() Rule { r := base; r.Stage = "final"; return r }(), "stage"},
		{"set negative", baseRule("r", -1, 0, Conditions{}), "action.amount"},
		{"set in modifier stage", mod(Action{Type: ActionSet, Amount: 100}), "action.type"},
		{"set in discount stage", disc(Action{Type: ActionSet, Amount: 100}), "action.type"},
		{"add in base stage", func() Rule { r := base; r.Action = Action{Type: ActionAdd, Amount: 1}; return r }(), "action.type"},
		{"add in discount stage", disc(Action{Type: ActionAdd, Amount: -100}), "action.type"},
		{"free_days in modifier stage", mod(Action{Type: ActionFreeDays, Every: 7, Free: 1}), "action.type"},
		{"unknown action", mod(Action{Type: "double"}), "action.type"},
		{"valid add negative", mod(Action{Type: ActionAdd, Amount: -100}), ""},
		{"multiply without factor", mod(Action{Type: ActionMultiply}), "action.factor"},
		{"multiply negative factor", mod(Action{Type: ActionMultiply, Factor: dec("-1")}), "action.factor"},
		{"valid modifier multiply above one", mod(Action{Type: ActionMultiply, Factor: dec("1.2")}), ""},
		{"discount multiply above one", disc(Action{Type: ActionMultiply, Factor: dec("1.2")}), "action.factor"},
		{"valid discount multiply", disc(Action{Type: ActionMultiply, Factor: dec("0.85")}), ""},
		{"percent missing", disc(Action{Type: ActionPercentOff}), "action.percent"},
		{"percent zero", disc(Action{Type: ActionPercentOff, Percent: dec("0")}), "action.percent"},
		{"percent over 100", disc(Action{Type: ActionPercentOff, Percent: dec("100.5")}), "action.percent"},
		{"percent three decimals", disc(Action{Type: ActionPercentOff, Percent: dec("12.345")}), "action.percent"},
		{"valid percent two decimals", disc(Action{Type: ActionPercentOff, Percent: dec("12.34")}), ""},
		{"amount_off zero", disc(Action{Type: ActionAmountOff}), "action.amount"},
		{"amount_off bad per", disc(Action{Type: ActionAmountOff, Amount: 100, Per: "week"}), "action.per"},
		{"modifier amount_off per booking", mod(Action{Type: ActionAmountOff, Amount: 100, Per: PerBooking}), "action.per"},
		{"valid amount_off per line", disc(Action{Type: ActionAmountOff, Amount: 100, Per: PerLine}), ""},
		{"free_days every one", disc(Action{Type: ActionFreeDays, Every: 1, Free: 1}), "action.every"},
		{"free_days free equals every", disc(Action{Type: ActionFreeDays, Every: 7, Free: 7}), "action.free"},
		{"free_days free zero", disc(Action{Type: ActionFreeDays, Every: 7}), "action.free"},
		{"valid free_days", disc(Action{Type: ActionFreeDays, Every: 7, Free: 1}), ""},
		{"weekday zero", withCond(Conditions{Weekdays: []int{0}}), "conditions.weekdays"},
		{"weekday eight", withCond(Conditions{Weekdays: []int{8}}), "conditions.weekdays"},
		{"bad slot", withCond(Conditions{Slots: []calendar.Slot{"FULL"}}), "conditions.slots"},
		{"bad formula", withCond(Conditions{Formulas: []Formula{"year"}}), "conditions.formulas"},
		{"bad channel", withCond(Conditions{Channels: []Channel{"phone"}}), "conditions.channels"},
		{"date range inverted", withCond(Conditions{DateRanges: []DateRange{{d("2027-08-20"), d("2027-08-10")}}}), "conditions.date_ranges"},
		{"date range missing end", withCond(Conditions{DateRanges: []DateRange{{From: d("2027-08-20")}}}), "conditions.date_ranges"},
		{"min days zero", withCond(Conditions{MinDays: ip(0)}), "conditions.min_days"},
		{"max days zero", withCond(Conditions{MaxDays: ip(0)}), "conditions.max_days"},
		{"max below min", withCond(Conditions{MinDays: ip(5), MaxDays: ip(3)}), "conditions.max_days"},
		{"lead time empty", withCond(Conditions{LeadTimeDays: &IntRange{}}), "conditions.lead_time_days"},
		{"lead time negative", withCond(Conditions{LeadTimeDays: &IntRange{Min: ip(-1)}}), "conditions.lead_time_days"},
		{"lead time inverted", withCond(Conditions{LeadTimeDays: &IntRange{Min: ip(5), Max: ip(1)}}), "conditions.lead_time_days"},
		{"occupancy over 100", withCond(Conditions{OccupancyPercent: &IntRange{Min: ip(120)}}), "conditions.occupancy_percent"},
		{"occupancy empty", withCond(Conditions{OccupancyPercent: &IntRange{}}), "conditions.occupancy_percent"},
		{"occupancy inverted", withCond(Conditions{OccupancyPercent: &IntRange{Min: ip(80), Max: ip(20)}}), "conditions.occupancy_percent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRule(tc.rule)
			if tc.field == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			var re *RuleError
			if !errors.As(err, &re) || !errors.Is(err, ErrInvalidRule) || re.Field != tc.field {
				t.Fatalf("err = %v, want RuleError on %s", err, tc.field)
			}
		})
	}
}

func TestValidateRuleSet_Cases(t *testing.T) {
	cases := []struct {
		name    string
		rs      RuleSet
		wantErr bool
	}{
		{"valid", ruleSet(defaultRules()...), false},
		{"duplicate period code", RuleSet{RatePeriods: []RatePeriod{{Code: "a"}, {Code: "a"}}}, true},
		{"empty period code", RuleSet{RatePeriods: []RatePeriod{{Code: ""}}}, true},
		{"inverted period range", RuleSet{RatePeriods: []RatePeriod{{Code: "a", Ranges: []DateRange{{d("2027-08-02"), d("2027-08-01")}}}}}, true},
		{"touching periods allowed", RuleSet{RatePeriods: []RatePeriod{
			{Code: "a", Ranges: []DateRange{{d("2027-08-01"), d("2027-08-09")}}},
			{Code: "b", Ranges: []DateRange{{d("2027-08-10"), d("2027-08-20")}}},
		}}, false},
		{"overlapping periods", RuleSet{RatePeriods: []RatePeriod{
			{Code: "a", Ranges: []DateRange{{d("2027-08-01"), d("2027-08-10")}}},
			{Code: "b", Ranges: []DateRange{{d("2027-08-10"), d("2027-08-20")}}},
		}}, true},
		{"fixed price non fixed formula", RuleSet{FixedPrices: []FixedDurationPrice{{ID: "f", PackID: "p", Formula: FormulaDay}}}, true},
		{"fixed price negative", RuleSet{FixedPrices: []FixedDurationPrice{{ID: "f", PackID: "p", Formula: FormulaWeek, Amount: -1}}}, true},
		{"fixed price unknown period", RuleSet{FixedPrices: []FixedDurationPrice{{ID: "f", PackID: "p", Formula: FormulaWeek, RatePeriodCode: sp("x")}}}, true},
		{"fixed price duplicate", RuleSet{FixedPrices: []FixedDurationPrice{
			{ID: "f1", PackID: "p", Formula: FormulaWeek, PriceCategory: "row1"},
			{ID: "f2", PackID: "p", Formula: FormulaWeek, PriceCategory: "row1"},
		}}, true},
		{"fixed price generic and period ok", RuleSet{
			RatePeriods: testPeriods(),
			FixedPrices: []FixedDurationPrice{
				{ID: "f1", PackID: "p", Formula: FormulaWeek, PriceCategory: "row1"},
				{ID: "f2", PackID: "p", Formula: FormulaWeek, PriceCategory: "row1", RatePeriodCode: sp("mid")},
			},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRuleSet(tc.rs)
			if (err != nil) != tc.wantErr || (err != nil && !errors.Is(err, ErrInvalidRule)) {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDecimal_Parse(t *testing.T) {
	cases := []struct {
		in      string
		num     int64
		den     int64
		str     string
		wantErr bool
	}{
		{in: "1.2", num: 12, den: 10, str: "1.2"},
		{in: "1.20", num: 12, den: 10, str: "1.2"},
		{in: "0.6", num: 6, den: 10, str: "0.6"},
		{in: "12.5", num: 125, den: 10, str: "12.5"},
		{in: "10", num: 10, den: 1, str: "10"},
		{in: "0", num: 0, den: 1, str: "0"},
		{in: "-0.05", num: -5, den: 100, str: "-0.05"},
		{in: "0.000000001", num: 1, den: 1_000_000_000, str: "0.000000001"},
		{in: "1e2", wantErr: true},
		{in: "1.", wantErr: true},
		{in: ".5", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "", wantErr: true},
		{in: "0.0000000001", wantErr: true},
		{in: "99999999999999999999", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			v, err := ParseDecimal(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			n, den := v.Rat()
			if n != tc.num || den != tc.den || v.String() != tc.str {
				t.Fatalf("got %d/%d %q", n, den, v.String())
			}
		})
	}
}

func TestDecimal_JSON(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`1.2`, `1.2`},
		{`"1.2"`, `1.2`},
		{`22`, `22`},
		{`0.50`, `0.5`},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			var v Decimal
			if err := json.Unmarshal([]byte(tc.in), &v); err != nil {
				t.Fatal(err)
			}
			out, _ := json.Marshal(v)
			if string(out) != tc.want {
				t.Fatalf("got %s want %s", out, tc.want)
			}
		})
	}
}

func TestDecimal_BasisPointsAndCmp(t *testing.T) {
	cases := []struct {
		in string
		bp int64
		ok bool
	}{
		{"10", 1000, true},
		{"12.5", 1250, true},
		{"0.01", 1, true},
		{"100", 10000, true},
		{"0.001", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			bp, ok := MustParseDecimal(tc.in).BasisPoints()
			if bp != tc.bp || ok != tc.ok {
				t.Fatalf("got %d %v", bp, ok)
			}
		})
	}
	if MustParseDecimal("1.2").Cmp(MustParseDecimal("1.20")) != 0 || MustParseDecimal("0.9").Cmp(NewDecimal(1, 0)) != -1 {
		t.Fatal("Cmp mismatch")
	}
}
