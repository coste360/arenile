package pricing

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

func matrixRules() RuleSet {
	return ruleSet(
		baseRule("r_day", 3000, 0, Conditions{}),
		baseRule("r_row2", 2500, 5, Conditions{PriceCategories: []string{"row2"}}),
		baseRule("r_peak", 4500, 10, Conditions{RatePeriods: []string{"peak"}}),
		baseRule("r_premium", 5000, 20, Conditions{PriceCategories: []string{"premium"}}),
		func() Rule { r := baseRule("r_gaz", 8000, 1, Conditions{}); r.TargetIDs = []string{"p_gaz"}; return r }(),
		modRule("m_high", Action{Type: ActionMultiply, Factor: dec("1.2")}, 0, true, Conditions{RatePeriods: []string{"high"}}),
		modRule("m_weekend", Action{Type: ActionAdd, Amount: 500}, 0, true, Conditions{Weekdays: []int{6, 7}}),
	)
}

func TestMatrix_Values(t *testing.T) {
	cells, err := Matrix(MatrixRequest{PackIDs: []string{"p_base", "p_gaz"}, Categories: []string{"row1", "row2", "premium"}}, testCatalog(), matrixRules())
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ pack, cat, period string }
	got := map[key]money.Cents{}
	dates := map[string]string{}
	for _, c := range cells {
		if !c.Priced {
			t.Fatalf("cell %+v not priced", c)
		}
		got[key{c.PackID, c.PriceCategory, c.RatePeriod}] = c.Price
		dates[c.RatePeriod] = c.Date.String()
	}
	if len(cells) != 2*3*4 {
		t.Fatalf("got %d cells", len(cells))
	}
	cases := []struct {
		k    key
		want money.Cents
	}{
		{key{"p_base", "row1", "low"}, 3000},
		{key{"p_base", "row1", "mid"}, 3000},
		{key{"p_base", "row1", "high"}, 3600},
		{key{"p_base", "row1", "peak"}, 4500},
		{key{"p_base", "row2", "low"}, 2500},
		{key{"p_base", "row2", "high"}, 3000},
		{key{"p_base", "row2", "peak"}, 4500},
		{key{"p_base", "premium", "mid"}, 5000},
		{key{"p_base", "premium", "high"}, 6000},
		{key{"p_gaz", "row1", "mid"}, 8000},
		{key{"p_gaz", "row1", "high"}, 9600},
	}
	for _, tc := range cases {
		if got[tc.k] != tc.want {
			t.Errorf("%+v = %d, want %d", tc.k, got[tc.k], tc.want)
		}
	}
	// Representative dates: earliest non-holiday weekday of each period.
	wantDates := map[string]string{"low": "2027-05-03", "mid": "2027-07-01", "high": "2027-08-02", "peak": "2027-08-10"}
	if !reflect.DeepEqual(dates, wantDates) {
		t.Fatalf("dates %v, want %v", dates, wantDates)
	}
}

// TestMatrix_MatchesQuote is the M2 acceptance check: every matrix cell equals
// the simulator (Quote) for the same pack, category, date and channel.
func TestMatrix_MatchesQuote(t *testing.T) {
	cat, rs := testCatalog(), matrixRules()
	for _, ch := range []Channel{ChannelDesk, ChannelOnline} {
		for _, slots := range []calendar.Slots{calendar.SlotsFull, calendar.SlotsAM} {
			cells, err := Matrix(MatrixRequest{Channel: ch, Slots: slots}, cat, rs)
			if err != nil {
				t.Fatal(err)
			}
			if len(cells) != 3*3*4 { // 3 active packs (p_vip included), 3 zone categories, 4 periods
				t.Fatalf("got %d cells", len(cells))
			}
			for _, c := range cells {
				formula := FormulaDay
				if slots != calendar.SlotsFull {
					formula = FormulaHalfDay
				}
				b, err := Quote(QuoteInput{
					Channel: ch, StartsOn: c.Date, EndsOn: c.Date, Slots: slots, Formula: formula, Today: c.Date,
					Lines: []LineInput{{PackID: c.PackID, ZoneID: c.ZoneID}},
				}, cat, rs)
				if err != nil {
					t.Fatalf("cell %+v: %v", c, err)
				}
				if b.Lines[0].Days[0].Total != c.Price {
					t.Fatalf("cell %+v != quote %d", c, b.Lines[0].Days[0].Total)
				}
			}
		}
	}
}

func TestMatrix_Errors(t *testing.T) {
	cases := []struct {
		name string
		req  MatrixRequest
		rs   RuleSet
		want error
	}{
		{"unknown period", MatrixRequest{RatePeriodCodes: []string{"winter"}}, matrixRules(), ErrInvalidInput},
		{"fixed formula", MatrixRequest{Formula: FormulaWeek}, matrixRules(), ErrInvalidInput},
		{"unknown pack", MatrixRequest{PackIDs: []string{"nope"}}, matrixRules(), ErrInvalidInput},
		{"invalid rules", MatrixRequest{}, ruleSet(modRule("x", Action{Type: ActionSet}, 0, true, Conditions{})), ErrInvalidRule},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Matrix(tc.req, testCatalog(), tc.rs); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMatrix_UnpricedCell(t *testing.T) {
	rs := ruleSet(baseRule("r_summer", 3000, 0, Conditions{RatePeriods: []string{"mid", "high", "peak"}}))
	cells, err := Matrix(MatrixRequest{PackIDs: []string{"p_base"}, Categories: []string{"row1"}}, testCatalog(), rs)
	if err != nil {
		t.Fatal(err)
	}
	if cells[0].RatePeriod != "low" || cells[0].Priced || !cells[1].Priced {
		t.Fatalf("cells %+v", cells)
	}
}

func TestCoverage_Gaps(t *testing.T) {
	july := Conditions{DateRanges: []DateRange{{d("2027-07-01"), d("2027-07-31")}}}
	cases := []struct {
		name string
		req  CoverageRequest
		rs   RuleSet
		want []CoverageGap
	}{
		{
			"fully covered",
			CoverageRequest{From: d("2027-07-01"), To: d("2027-07-31"), PackIDs: []string{"p_base"}},
			ruleSet(baseRule("r", 3000, 0, Conditions{})),
			[]CoverageGap{},
		},
		{
			"gap before and after",
			CoverageRequest{From: d("2027-06-28"), To: d("2027-08-02"), PackIDs: []string{"p_base"}, Categories: []string{"row1"}, Channels: []Channel{ChannelDesk}},
			ruleSet(baseRule("r", 3000, 0, july)),
			[]CoverageGap{
				{PackID: "p_base", PriceCategory: "row1", Channel: ChannelDesk, From: d("2027-06-28"), To: d("2027-06-30")},
				{PackID: "p_base", PriceCategory: "row1", Channel: ChannelDesk, From: d("2027-08-01"), To: d("2027-08-02")},
			},
		},
		{
			"gap only on one channel",
			CoverageRequest{From: d("2027-07-01"), To: d("2027-07-02"), PackIDs: []string{"p_base"}, Categories: []string{"row1"}},
			ruleSet(baseRule("r", 3000, 0, Conditions{Channels: []Channel{ChannelDesk}})),
			[]CoverageGap{
				{PackID: "p_base", PriceCategory: "row1", Channel: ChannelOnline, From: d("2027-07-01"), To: d("2027-07-02")},
			},
		},
		{
			"gap only for one category",
			CoverageRequest{From: d("2027-07-01"), To: d("2027-07-01"), PackIDs: []string{"p_base"}, Channels: []Channel{ChannelDesk}},
			ruleSet(baseRule("r", 3000, 0, Conditions{PriceCategories: []string{"row1", "row2"}})),
			[]CoverageGap{
				{PackID: "p_base", PriceCategory: "pool", Channel: ChannelDesk, From: d("2027-07-01"), To: d("2027-07-01")},
			},
		},
		{
			"weekday hole",
			CoverageRequest{From: d("2027-07-05"), To: d("2027-07-18"), PackIDs: []string{"p_base"}, Categories: []string{"row1"}, Channels: []Channel{ChannelDesk}},
			ruleSet(baseRule("r", 3000, 0, Conditions{Weekdays: []int{1, 2, 3, 4, 5, 6}})),
			[]CoverageGap{
				{PackID: "p_base", PriceCategory: "row1", Channel: ChannelDesk, From: d("2027-07-11"), To: d("2027-07-11")},
				{PackID: "p_base", PriceCategory: "row1", Channel: ChannelDesk, From: d("2027-07-18"), To: d("2027-07-18")},
			},
		},
		{
			"inactive packs skipped by default",
			CoverageRequest{From: d("2027-07-01"), To: d("2027-07-01"), Categories: []string{"row1"}, Channels: []Channel{ChannelDesk}},
			ruleSet(func() Rule {
				r := baseRule("r", 3000, 0, Conditions{})
				r.TargetIDs = []string{"p_base", "p_gaz"}
				return r
			}()),
			[]CoverageGap{
				{PackID: "p_vip", PriceCategory: "row1", Channel: ChannelDesk, From: d("2027-07-01"), To: d("2027-07-01")},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Coverage(tc.req, testCatalog(), tc.rs)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("gaps %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCoverage_InvalidRange(t *testing.T) {
	_, err := Coverage(CoverageRequest{From: d("2027-07-02"), To: d("2027-07-01")}, testCatalog(), ruleSet())
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v", err)
	}
}

func TestShiftDate_Rule(t *testing.T) {
	cases := []struct {
		name  string
		date  string
		years int
		isEnd bool
		want  string
	}{
		{"saturday keeps weekday", "2027-07-10", 1, false, "2028-07-08"},
		{"two years keeps weekday", "2027-07-10", 2, false, "2029-07-07"},
		{"backwards keeps weekday", "2027-07-10", -1, false, "2026-07-11"},
		{"ferragosto calendar", "2027-08-15", 1, false, "2028-08-15"},
		{"end before ferragosto calendar", "2027-08-14", 1, true, "2028-08-14"},
		{"start before ferragosto weekday", "2027-08-14", 1, false, "2028-08-12"},
		{"start after ferragosto calendar", "2027-08-16", 1, false, "2028-08-16"},
		{"first of month start calendar", "2027-07-01", 1, false, "2028-07-01"},
		{"first of month end weekday", "2027-07-01", 1, true, "2028-06-29"},
		{"end of month end calendar", "2027-07-31", 1, true, "2028-07-31"},
		{"easter sunday", "2027-03-28", 1, false, "2028-04-16"},
		{"easter monday", "2027-03-29", 1, true, "2028-04-17"},
		{"saturday before easter", "2027-03-27", 1, false, "2028-04-15"},
		{"leap day weekday", "2028-02-29", 1, false, "2029-02-27"},
		{"2 june holiday calendar", "2027-06-02", 1, false, "2028-06-02"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ShiftDate(d(tc.date), tc.years, tc.isEnd)
			if got.String() != tc.want {
				t.Fatalf("ShiftDate(%s, %d, %v) = %s, want %s", tc.date, tc.years, tc.isEnd, got, tc.want)
			}
		})
	}
}

func TestCopySeason_PriceList(t *testing.T) {
	rules := []Rule{
		baseRule("r_day", 3000, 0, Conditions{}),
		func() Rule {
			r := baseRule("r_event", 5000, 9, Conditions{DateRanges: []DateRange{{d("2027-07-10"), d("2027-07-11")}}, Weekdays: []int{6, 7}})
			r.Seq = 42
			return r
		}(),
	}
	fixed := []FixedDurationPrice{{ID: "f1", PackID: "p_base", Formula: FormulaWeek, PriceCategory: "row1", RatePeriodCode: sp("mid"), Amount: 18000}}
	periods := testPeriods()
	res, err := CopySeason(CopySeasonInput{
		FromYear: 2027, ToYear: 2028, Rules: rules, RatePeriods: periods, FixedPrices: fixed,
		NewID: func(kind, old string) string { return kind + ":" + old + ":2028" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings %v", res.Warnings)
	}
	wantPeriods := map[string][]string{
		"low":  {"2028-05-01", "2028-06-30", "2028-09-01", "2028-09-30"},
		"mid":  {"2028-07-01", "2028-07-31"},
		"high": {"2028-08-01", "2028-08-07", "2028-08-19", "2028-08-31"},
		"peak": {"2028-08-08", "2028-08-18"},
	}
	for _, p := range res.RatePeriods {
		var got []string
		for _, r := range p.Ranges {
			got = append(got, r.From.String(), r.To.String())
		}
		if !reflect.DeepEqual(got, wantPeriods[p.Code]) || !strings.HasSuffix(p.ID, ":2028") {
			t.Errorf("period %s (%s) = %v, want %v", p.Code, p.ID, got, wantPeriods[p.Code])
		}
	}
	ev := res.Rules[1]
	if ev.ID != "rule:r_event:2028" || ev.Seq != 42 || ev.Priority != 9 ||
		ev.Conditions.DateRanges[0].From.String() != "2028-07-08" || ev.Conditions.DateRanges[0].To.String() != "2028-07-09" {
		t.Fatalf("event rule %+v", ev)
	}
	if res.Rules[0].Conditions.DateRanges != nil {
		t.Fatalf("rule without ranges got ranges")
	}
	if res.FixedPrices[0].ID != "fixed_price:f1:2028" || *res.FixedPrices[0].RatePeriodCode != "mid" || res.FixedPrices[0].Amount != 18000 {
		t.Fatalf("fixed %+v", res.FixedPrices[0])
	}
	// Source untouched (deep copy).
	if rules[1].Conditions.DateRanges[0].From.String() != "2027-07-10" || rules[1].ID != "r_event" {
		t.Fatal("source rules modified")
	}
	// The copied list must be a valid rule set and price the new season.
	rs := RuleSet{Rules: res.Rules, RatePeriods: res.RatePeriods, FixedPrices: res.FixedPrices}
	b := mustQuote(t, QuoteInput{
		Channel: ChannelDesk, StartsOn: d("2028-07-08"), EndsOn: d("2028-07-08"), Slots: calendar.SlotsFull,
		Formula: FormulaDay, Today: d("2028-06-01"), Lines: []LineInput{{PackID: "p_base", ZoneID: "z1"}},
	}, testCatalog(), rs)
	if b.Total != 5000 {
		t.Fatalf("copied event rule not applied: %d", b.Total)
	}
	*res.FixedPrices[0].RatePeriodCode = "x"
	if *fixed[0].RatePeriodCode != "mid" {
		t.Fatal("fixed price period shared with source")
	}
}

func TestCopySeason_Warnings(t *testing.T) {
	periods := []RatePeriod{
		{ID: "a", Code: "pre", Ranges: []DateRange{{d("2027-03-20"), d("2027-03-26")}}},
		{ID: "b", Code: "easter", Ranges: []DateRange{{d("2027-03-27"), d("2027-03-30")}}},
	}
	res, err := CopySeason(CopySeasonInput{FromYear: 2027, ToYear: 2028, RatePeriods: periods})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) == 0 || !strings.Contains(res.Warnings[0], "no longer adjacent") {
		t.Fatalf("warnings %v", res.Warnings)
	}
	if res.RatePeriods[0].ID != "a" {
		t.Fatal("nil NewID must keep IDs")
	}
}

func TestCopySeason_InvalidYears(t *testing.T) {
	cases := []struct {
		name     string
		from, to int
	}{
		{"same year", 2027, 2027},
		{"zero year", 0, 2028},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := CopySeason(CopySeasonInput{FromYear: tc.from, ToYear: tc.to}); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
