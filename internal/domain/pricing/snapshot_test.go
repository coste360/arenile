package pricing

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/coste360/arenile/internal/money"
)

func TestQuote_Rounding(t *testing.T) {
	cases := []struct {
		name     string
		amount   money.Cents
		mode     Rounding
		total    money.Cents
		rounding money.Cents
	}{
		{"none keeps cents", 3333, RoundingNone, 3333, 0},
		{"empty mode is none", 3333, "", 3333, 0},
		{"euro down", 3333, RoundingEuro, 3300, -33},
		{"euro up", 3351, RoundingEuro, 3400, 49},
		{"euro just below half", 3349, RoundingEuro, 3300, -49},
		{"euro tie to even up", 3350, RoundingEuro, 3400, 50},
		{"euro tie to even down", 3250, RoundingEuro, 3200, -50},
		{"euro exact", 3300, RoundingEuro, 3300, 0},
		{"euro small amount to zero", 49, RoundingEuro, 0, -49},
		{"euro tie at fifty cents to zero", 50, RoundingEuro, 0, -50},
		{"euro tie at 1.50 to two", 150, RoundingEuro, 200, 50},
		{"euro zero", 0, RoundingEuro, 0, 0},
		{"fifty cents up", 3333, RoundingFiftyCents, 3350, 17},
		{"fifty cents down", 3324, RoundingFiftyCents, 3300, -24},
		{"fifty cents tie to even down", 3325, RoundingFiftyCents, 3300, -25},
		{"fifty cents tie to even up", 3375, RoundingFiftyCents, 3400, 25},
		{"fifty cents exact", 3350, RoundingFiftyCents, 3350, 0},
		{"fifty cents tiny to zero", 25, RoundingFiftyCents, 0, -25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := testCatalog()
			cat.Settings.Rounding = tc.mode
			b := mustQuote(t, oneDay(), cat, ruleSet(baseRule("r", tc.amount, 0, Conditions{})))
			if b.Total != tc.total || b.Rounding != tc.rounding {
				t.Fatalf("total %d rounding %d, want %d %d", b.Total, b.Rounding, tc.total, tc.rounding)
			}
		})
	}
}

func TestQuote_RoundingOnlyOnFinalTotal(t *testing.T) {
	// Three days at 33.33: daily totals keep cents, only the final total rounds.
	cat := testCatalog()
	cat.Settings.Rounding = RoundingEuro
	b := mustQuote(t, span("2027-07-05", "2027-07-07", FormulaMultiDay), cat, ruleSet(baseRule("r", 3333, 0, Conditions{})))
	if b.Lines[0].Days[0].Total != 3333 || b.Lines[0].Total != 9999 || b.Total != 10000 || b.Rounding != 1 {
		t.Fatalf("line %d total %d rounding %d", b.Lines[0].Total, b.Total, b.Rounding)
	}
}

func TestQuote_VAT(t *testing.T) {
	type vatWant struct {
		rate         string
		taxable, tax money.Cents
	}
	cases := []struct {
		name     string
		mutate   func(*QuoteInput)
		cat      func(*Catalog)
		want     []vatWant
		total    money.Cents
		rounding Rounding
	}{
		{name: "single rate 22", want: []vatWant{{"22", 2459, 541}}, total: 3000},
		{name: "pack rate 10", mutate: func(in *QuoteInput) { in.Lines[0].PackID = "p_vip" }, want: []vatWant{{"10", 2727, 273}}, total: 3000},
		{
			name: "two rates pack and towel", mutate: func(in *QuoteInput) { in.Lines[0].Extras = []ExtraInput{{"x_towel", 1}} },
			want: []vatWant{{"10", 182, 18}, {"22", 2459, 541}}, total: 3200,
		},
		{name: "two rates with percent discount", mutate: func(in *QuoteInput) {
			in.Lines[0].Extras = []ExtraInput{{"x_towel", 1}}
			in.PromoCode = promo("TEN", pctOff("10"), false)
		}, want: []vatWant{{"10", 164, 16}, {"22", 2213, 487}}, total: 2880},
		{name: "two rates with amount discount largest remainder", mutate: func(in *QuoteInput) {
			in.Lines[0].Extras = []ExtraInput{{"x_towel", 1}}
			in.PromoCode = promo("ODD", amountOff(101, PerBooking), false)
		}, want: []vatWant{{"10", 176, 18}, {"22", 2381, 524}}, total: 3099},
		{name: "two rates with rounding", mutate: func(in *QuoteInput) {
			in.Lines[0].Extras = []ExtraInput{{"x_towel", 1}}
			in.PromoCode = promo("ODD", amountOff(33, PerBooking), false)
		}, rounding: RoundingEuro, want: []vatWant{{"10", 182, 18}, {"22", 2459, 541}}, total: 3200},
		{name: "zero rate", cat: func(c *Catalog) { c.Settings.DefaultVATRateBP = 0 }, want: []vatWant{{"0", 3000, 0}}, total: 3000},
		{name: "decimal rate 5.5", cat: func(c *Catalog) { c.Settings.DefaultVATRateBP = 550 }, want: []vatWant{{"5.5", 2844, 156}}, total: 3000},
		{name: "free booking has no vat lines", mutate: func(in *QuoteInput) { in.PromoCode = promo("ALL", pctOff("100"), false) }, want: []vatWant{}, total: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := testCatalog()
			cat.Settings.Rounding = tc.rounding
			if tc.cat != nil {
				tc.cat(&cat)
			}
			in := oneDay()
			if tc.mutate != nil {
				tc.mutate(&in)
			}
			b := mustQuote(t, in, cat, ruleSet(defaultRules()...))
			got := []vatWant{}
			for _, v := range b.VAT {
				got = append(got, vatWant{v.Rate.String(), v.Taxable, v.Tax})
			}
			if b.Total != tc.total || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("total %d vat %+v, want %d %+v", b.Total, got, tc.total, tc.want)
			}
		})
	}
}

func TestAllocate_LargestRemainder(t *testing.T) {
	cases := []struct {
		name    string
		total   int64
		weights []int64
		want    []int64
	}{
		{"proportional exact", 100, []int64{1, 1, 2}, []int64{25, 25, 50}},
		{"remainder to largest fraction", 100, []int64{1, 1, 1}, []int64{34, 33, 33}},
		{"zero total", 0, []int64{5, 5}, []int64{0, 0}},
		{"zero weights to first", 7, []int64{0, 0}, []int64{7, 0}},
		{"single weight", 9, []int64{3}, []int64{9}},
		{"large values no overflow", 9_000_000_000_000, []int64{9_000_000_000_000, 1}, []int64{8_999_999_999_999, 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := allocate(tc.total, tc.weights); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("allocate = %v, want %v", got, tc.want)
			}
		})
	}
}

// specExampleSnapshot is the snapshot of spec 8.6, verbatim.
const specExampleSnapshot = `{
  "engine_version": 1,
  "currency": "EUR",
  "lines": [{
    "pack": "base", "zone": "row1", "spot": "A12",
    "days": [
      {"date": "2027-08-14", "slots": "FULL", "base": 3500,
       "applied": [{"rule": "r_peak", "stage": "base", "effect": "set 3500"},
                   {"rule": "r_weekend", "stage": "modifier", "effect": "+500"}],
       "total": 4000}
    ],
    "extras": [{"code": "sunbed", "qty": 1, "total": 800}],
    "total": 4800
  }],
  "discounts": [{"source": "promo:ESTATE10", "amount": -480}],
  "rounding": 0,
  "total": 4320,
  "vat": [{"rate": 22, "taxable": 3541, "tax": 779}]
}`

func specExample() (QuoteInput, RuleSet) {
	in := oneDay()
	in.StartsOn, in.EndsOn = d("2027-08-14"), d("2027-08-14")
	in.Lines = []LineInput{{PackID: "p_base", SpotID: "s_a12", Extras: []ExtraInput{{"x_sunbed", 1}}}}
	in.PromoCode = promo("ESTATE10", pctOff("10"), false)
	rs := ruleSet(
		baseRule("r_day", 3000, 0, Conditions{}),
		baseRule("r_peak", 3500, 10, Conditions{RatePeriods: []string{"peak"}}),
		modRule("r_weekend", Action{Type: ActionAdd, Amount: 500}, 0, true, Conditions{Weekdays: []int{6, 7}}),
		extraBase("rx_sunbed", "x_sunbed", 800, 0, Conditions{}),
	)
	return in, rs
}

func TestQuote_SpecSnapshotExample(t *testing.T) {
	in, rs := specExample()
	b := mustQuote(t, in, testCatalog(), rs)
	got, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := json.Compact(&want, []byte(specExampleSnapshot)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("snapshot mismatch\n got %s\nwant %s", got, want.Bytes())
	}
}

func TestPriceBreakdown_JSONRoundTrip(t *testing.T) {
	week := span("2027-07-05", "2027-07-11", FormulaWeek)
	week.Lines[0].Extras = []ExtraInput{{"x_towel", 2}}
	week.Extras = []ExtraInput{{"x_cabin", 1}}
	week.PromoCode = promo("ODD", amountOff(77, PerBooking), true)
	specIn, specRS := specExample()
	fixedRS := ruleSet(defaultRules()...)
	fixedRS.FixedPrices = []FixedDurationPrice{{ID: "f", PackID: "p_base", Formula: FormulaWeek, PriceCategory: "row1", RatePeriodCode: sp("mid"), Amount: 18000}}
	cases := []struct {
		name string
		in   QuoteInput
		rs   RuleSet
	}{
		{"spec example", specIn, specRS},
		{"fixed week with extras and booking extras", week, fixedRS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := testCatalog()
			cat.Settings.Rounding = RoundingFiftyCents
			b := mustQuote(t, tc.in, cat, tc.rs)
			raw, err := json.Marshal(b)
			if err != nil {
				t.Fatal(err)
			}
			var back PriceBreakdown
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(back, b) {
				t.Fatalf("round trip differs\n got %+v\nwant %+v", back, b)
			}
			raw2, _ := json.Marshal(back)
			if !bytes.Equal(raw, raw2) {
				t.Fatalf("re-marshal differs\n%s\n%s", raw, raw2)
			}
			checkInvariants(t, back)
		})
	}
}

func TestRule_JSONFromSpecShapes(t *testing.T) {
	cases := []struct {
		name  string
		json  string
		check func(Rule) bool
	}{
		{
			"set", `{"id":"r1","target_type":"pack","stage":"base","active":true,"conditions":{},"action":{"type":"set","amount":2500}}`,
			func(r Rule) bool { return r.Action.Type == ActionSet && r.Action.Amount == 2500 },
		},
		{
			"multiply factor exact", `{"id":"r1","target_type":"pack","stage":"modifier","conditions":{},"action":{"type":"multiply","factor":1.2}}`,
			func(r Rule) bool { n, den := r.Action.Factor.Rat(); return n == 12 && den == 10 },
		},
		{
			"percent decimal", `{"id":"r1","target_type":"pack","stage":"discount","conditions":{},"action":{"type":"percent_off","percent":12.5}}`,
			func(r Rule) bool { bp, ok := r.Action.Percent.BasisPoints(); return ok && bp == 1250 },
		},
		{
			"amount off per", `{"id":"r1","target_type":"pack","stage":"discount","conditions":{},"action":{"type":"amount_off","amount":1000,"per":"booking"}}`,
			func(r Rule) bool { return r.Action.Per == PerBooking },
		},
		{
			"free days", `{"id":"r1","target_type":"pack","stage":"discount","conditions":{},"action":{"type":"free_days","every":7,"free":1}}`,
			func(r Rule) bool { return r.Action.Every == 7 && r.Action.Free == 1 },
		},
		{
			"all conditions", `{"id":"r1","target_type":"pack","stage":"modifier","action":{"type":"add","amount":500},"conditions":{
			"rate_periods": ["high", "peak"], "date_ranges": [{"from": "2027-08-10", "to": "2027-08-20"}],
			"weekdays": [6, 7], "holidays": true, "slots": ["AM"], "formulas": ["day", "multi_day"],
			"price_categories": ["row1", "row2"], "zone_ids": ["z"], "spot_attributes": ["accessible"],
			"channels": ["online"], "min_days": 3, "max_days": 6, "lead_time_days": {"min": 0, "max": 1},
			"occupancy_percent": {"min": 80}, "customer_tags": ["storico"], "agreement_ids": ["a"]}}`,
			func(r Rule) bool {
				return r.Conditions.Count() == 16 && *r.Conditions.OccupancyPercent.Min == 80 && r.Conditions.OccupancyPercent.Max == nil &&
					r.Conditions.DateRanges[0].To.String() == "2027-08-20" && ValidateRule(r) == nil
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r Rule
			if err := json.Unmarshal([]byte(tc.json), &r); err != nil {
				t.Fatal(err)
			}
			if !tc.check(r) {
				t.Fatalf("unexpected rule %+v", r)
			}
			raw, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var back Rule
			if err := json.Unmarshal(raw, &back); err != nil || !reflect.DeepEqual(back, r) {
				t.Fatalf("round trip: %v %s", err, raw)
			}
		})
	}
}
