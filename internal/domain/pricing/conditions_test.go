package pricing

import (
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// TestQuote_Conditions checks every condition type of spec 8.2: a conditional
// base rule (40.00, priority 10) competes with the default 30.00 rule; the
// first day total tells whether the condition matched.
func TestQuote_Conditions(t *testing.T) {
	const hit, miss money.Cents = 4000, 3000
	withSpot := func(spot string) func(*QuoteInput) {
		return func(in *QuoteInput) { in.Lines[0].SpotID = spot; in.Lines[0].ZoneID = "" }
	}
	onDate := func(s string) func(*QuoteInput) {
		return func(in *QuoteInput) { in.StartsOn, in.EndsOn = d(s), d(s) }
	}
	occ := func(zone, date string, pct int) func(*QuoteInput) {
		return func(in *QuoteInput) {
			in.Occupancy = map[string]map[calendar.Date]int{zone: {d(date): pct}}
		}
	}
	agreement := func(id string, active bool) func(*QuoteInput) {
		return func(in *QuoteInput) {
			in.Agreement = &Agreement{
				ID: id, Code: "HOTEL", Active: active,
				Action: Action{Type: ActionPercentOff, Percent: dec("5")},
			}
		}
	}
	cases := []struct {
		name   string
		cond   Conditions
		mutate func(*QuoteInput)
		cat    func(*Catalog)
		want   money.Cents
	}{
		{name: "no conditions always match", cond: Conditions{}, want: hit},
		{name: "rate_periods match", cond: Conditions{RatePeriods: []string{"mid"}}, want: hit},
		{name: "rate_periods any of list", cond: Conditions{RatePeriods: []string{"high", "mid"}}, want: hit},
		{name: "rate_periods miss", cond: Conditions{RatePeriods: []string{"high", "peak"}}, want: miss},
		{name: "rate_periods outside every period", cond: Conditions{RatePeriods: []string{"low"}}, mutate: onDate("2027-10-15"), want: miss},
		{name: "rate_periods second range of period", cond: Conditions{RatePeriods: []string{"low"}}, mutate: onDate("2027-09-10"), want: hit},
		{name: "date_ranges inside", cond: Conditions{DateRanges: []DateRange{{d("2027-07-01"), d("2027-07-10")}}}, want: hit},
		{name: "date_ranges from inclusive", cond: Conditions{DateRanges: []DateRange{{d("2027-07-05"), d("2027-07-10")}}}, want: hit},
		{name: "date_ranges to inclusive", cond: Conditions{DateRanges: []DateRange{{d("2027-07-01"), d("2027-07-05")}}}, want: hit},
		{name: "date_ranges outside", cond: Conditions{DateRanges: []DateRange{{d("2027-08-10"), d("2027-08-20")}}}, want: miss},
		{name: "date_ranges second range", cond: Conditions{DateRanges: []DateRange{{d("2027-08-10"), d("2027-08-20")}, {d("2027-07-04"), d("2027-07-06")}}}, want: hit},
		{name: "weekdays monday", cond: Conditions{Weekdays: []int{1}}, want: hit},
		{name: "weekdays weekend on monday", cond: Conditions{Weekdays: []int{6, 7}}, want: miss},
		{name: "weekdays weekend on saturday", cond: Conditions{Weekdays: []int{6, 7}}, mutate: onDate("2027-07-10"), want: hit},
		{name: "weekdays sunday is 7", cond: Conditions{Weekdays: []int{7}}, mutate: onDate("2027-07-11"), want: hit},
		{name: "holidays true on ferragosto", cond: Conditions{Holidays: bptr(true)}, mutate: onDate("2027-08-15"), want: hit},
		{name: "holidays true on ordinary day", cond: Conditions{Holidays: bptr(true)}, want: miss},
		{name: "holidays false on ordinary day", cond: Conditions{Holidays: bptr(false)}, want: hit},
		{name: "holidays false on 2 june", cond: Conditions{Holidays: bptr(false)}, mutate: onDate("2027-06-02"), want: miss},
		{
			name: "holidays true on local recurring patron", cond: Conditions{Holidays: bptr(true)},
			cat: func(c *Catalog) { c.Settings.RecurringLocalHolidays = []calendar.MonthDay{{Month: time.July, Day: 5}} }, want: hit,
		},
		{
			name: "holidays true on local specific date", cond: Conditions{Holidays: bptr(true)},
			cat: func(c *Catalog) { c.Settings.LocalHolidays = []calendar.Date{d("2027-07-05")} }, want: hit,
		},
		{name: "slots AM-AM on full day misses", cond: Conditions{Slots: []calendar.Slot{calendar.AM}}, want: miss},
		{name: "slots AM+PM on full day", cond: Conditions{Slots: []calendar.Slot{calendar.AM, calendar.PM}}, want: hit},
		{
			name: "slots AM on AM booking", cond: Conditions{Slots: []calendar.Slot{calendar.AM}},
			mutate: func(in *QuoteInput) { in.Slots, in.Formula = calendar.SlotsAM, FormulaHalfDay }, want: hit,
		},
		{
			name: "slots PM on AM booking falls back to factor", cond: Conditions{Slots: []calendar.Slot{calendar.PM}},
			mutate: func(in *QuoteInput) { in.Slots, in.Formula = calendar.SlotsAM, FormulaHalfDay }, want: 1800,
		},
		{name: "formulas miss", cond: Conditions{Formulas: []Formula{FormulaMultiDay}}, want: miss},
		{name: "formulas match", cond: Conditions{Formulas: []Formula{FormulaMultiDay, FormulaDay}}, want: hit},
		{
			name: "formulas multi_day booking", cond: Conditions{Formulas: []Formula{FormulaMultiDay}},
			mutate: func(in *QuoteInput) { in.Formula = FormulaMultiDay }, want: hit,
		},
		{name: "price_categories zone category", cond: Conditions{PriceCategories: []string{"row1"}}, want: hit},
		{name: "price_categories other", cond: Conditions{PriceCategories: []string{"row2", "pool"}}, want: miss},
		{
			name: "price_categories pool zone", cond: Conditions{PriceCategories: []string{"pool"}},
			mutate: func(in *QuoteInput) { in.Lines[0].ZoneID = "zp" }, want: hit,
		},
		{name: "price_categories spot override", cond: Conditions{PriceCategories: []string{"premium"}}, mutate: withSpot("s_b5"), want: hit},
		{name: "price_categories override hides zone category", cond: Conditions{PriceCategories: []string{"row2"}}, mutate: withSpot("s_b5"), want: miss},
		{name: "zone_ids match", cond: Conditions{ZoneIDs: []string{"z1"}}, want: hit},
		{name: "zone_ids miss", cond: Conditions{ZoneIDs: []string{"z2"}}, want: miss},
		{name: "zone_ids derived from spot", cond: Conditions{ZoneIDs: []string{"z2"}}, mutate: withSpot("s_b5"), want: hit},
		{name: "spot_attributes match", cond: Conditions{SpotAttributes: []string{"accessible"}}, mutate: withSpot("s_a12"), want: hit},
		{name: "spot_attributes no spot", cond: Conditions{SpotAttributes: []string{"accessible"}}, want: miss},
		{name: "spot_attributes other spot", cond: Conditions{SpotAttributes: []string{"accessible"}}, mutate: withSpot("s_b5"), want: miss},
		{name: "spot_attributes any of list", cond: Conditions{SpotAttributes: []string{"vip", "shade_morning"}}, mutate: withSpot("s_b5"), want: hit},
		{name: "channels online on desk", cond: Conditions{Channels: []Channel{ChannelOnline}}, want: miss},
		{
			name: "channels online on online", cond: Conditions{Channels: []Channel{ChannelOnline}},
			mutate: func(in *QuoteInput) { in.Channel = ChannelOnline }, want: hit,
		},
		{
			name: "channels api", cond: Conditions{Channels: []Channel{ChannelAPI, ChannelDesk}},
			mutate: func(in *QuoteInput) { in.Channel = ChannelAPI }, want: hit,
		},
		{name: "min_days not reached", cond: Conditions{MinDays: ip(3)}, want: miss},
		{
			name: "min_days reached", cond: Conditions{MinDays: ip(3)},
			mutate: func(in *QuoteInput) { in.EndsOn = d("2027-07-07"); in.Formula = FormulaMultiDay }, want: hit,
		},
		{name: "max_days within", cond: Conditions{MaxDays: ip(2)}, want: hit},
		{
			name: "max_days exceeded", cond: Conditions{MaxDays: ip(2)},
			mutate: func(in *QuoteInput) { in.EndsOn = d("2027-07-07"); in.Formula = FormulaMultiDay }, want: miss,
		},
		{
			name: "lead_time last minute same day", cond: Conditions{LeadTimeDays: &IntRange{Min: ip(0), Max: ip(1)}},
			mutate: func(in *QuoteInput) { in.Today = d("2027-07-05") }, want: hit,
		},
		{
			name: "lead_time last minute day before", cond: Conditions{LeadTimeDays: &IntRange{Min: ip(0), Max: ip(1)}},
			mutate: func(in *QuoteInput) { in.Today = d("2027-07-04") }, want: hit,
		},
		{name: "lead_time last minute too early", cond: Conditions{LeadTimeDays: &IntRange{Min: ip(0), Max: ip(1)}}, want: miss},
		{name: "lead_time early booking", cond: Conditions{LeadTimeDays: &IntRange{Min: ip(30)}}, want: hit},
		{
			name: "lead_time early booking too late", cond: Conditions{LeadTimeDays: &IntRange{Min: ip(30)}},
			mutate: func(in *QuoteInput) { in.Today = d("2027-06-20") }, want: miss,
		},
		{name: "occupancy above min", cond: Conditions{OccupancyPercent: &IntRange{Min: ip(80)}}, mutate: occ("z1", "2027-07-05", 85), want: hit},
		{name: "occupancy at min", cond: Conditions{OccupancyPercent: &IntRange{Min: ip(80)}}, mutate: occ("z1", "2027-07-05", 80), want: hit},
		{name: "occupancy below min", cond: Conditions{OccupancyPercent: &IntRange{Min: ip(80)}}, mutate: occ("z1", "2027-07-05", 50), want: miss},
		{name: "occupancy missing data", cond: Conditions{OccupancyPercent: &IntRange{Min: ip(0)}}, want: miss},
		{name: "occupancy other zone data", cond: Conditions{OccupancyPercent: &IntRange{Min: ip(80)}}, mutate: occ("z2", "2027-07-05", 90), want: miss},
		{name: "occupancy below max", cond: Conditions{OccupancyPercent: &IntRange{Max: ip(30)}}, mutate: occ("z1", "2027-07-05", 20), want: hit},
		{
			name: "customer_tags match", cond: Conditions{CustomerTags: []string{"storico"}},
			mutate: func(in *QuoteInput) { in.Customer.Tags = []string{"vip", "storico"} }, want: hit,
		},
		{name: "customer_tags none", cond: Conditions{CustomerTags: []string{"storico"}}, want: miss},
		{
			name: "customer_tags other", cond: Conditions{CustomerTags: []string{"storico"}},
			mutate: func(in *QuoteInput) { in.Customer.Tags = []string{"vip"} }, want: miss,
		},
		{name: "agreement_ids match", cond: Conditions{AgreementIDs: []string{"ag1"}}, mutate: agreement("ag1", true), want: hit},
		{name: "agreement_ids other agreement", cond: Conditions{AgreementIDs: []string{"ag1"}}, mutate: agreement("ag2", true), want: miss},
		{name: "agreement_ids inactive agreement", cond: Conditions{AgreementIDs: []string{"ag1"}}, mutate: agreement("ag1", false), want: miss},
		{name: "agreement_ids no agreement", cond: Conditions{AgreementIDs: []string{"ag1"}}, want: miss},
		{name: "combined all true", cond: Conditions{
			RatePeriods: []string{"mid"}, Weekdays: []int{1}, Channels: []Channel{ChannelDesk},
			PriceCategories: []string{"row1"}, MaxDays: ip(1), Holidays: bptr(false),
		}, want: hit},
		{name: "combined one false", cond: Conditions{
			RatePeriods: []string{"mid"}, Weekdays: []int{1}, Channels: []Channel{ChannelOnline},
		}, want: miss},
		{name: "empty lists are absent", cond: Conditions{Weekdays: []int{}, RatePeriods: []string{}}, want: hit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := testCatalog()
			if tc.cat != nil {
				tc.cat(&cat)
			}
			in := oneDay()
			if tc.mutate != nil {
				tc.mutate(&in)
			}
			rs := ruleSet(append(defaultRules(), baseRule("r_cond", hit, 10, tc.cond))...)
			b := mustQuote(t, in, cat, rs)
			if got := b.Lines[0].Days[0].Total; got != tc.want {
				t.Fatalf("day total = %d, want %d (applied %+v)", got, tc.want, b.Lines[0].Days[0].Applied)
			}
		})
	}
}

// TestQuote_ConditionsOnModifiers checks that conditions gate modifiers and
// discounts in the same way as base rules.
func TestQuote_ConditionsOnModifiers(t *testing.T) {
	weekend := Conditions{Weekdays: []int{6, 7}}
	cases := []struct {
		name string
		date string
		want money.Cents
	}{
		{"weekend modifier on friday", "2027-07-09", 3000},
		{"weekend modifier on saturday", "2027-07-10", 3500},
		{"weekend modifier on sunday", "2027-07-11", 3500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := oneDay()
			in.StartsOn, in.EndsOn = d(tc.date), d(tc.date)
			rs := ruleSet(append(defaultRules(), modRule("r_weekend", Action{Type: ActionAdd, Amount: 500}, 0, true, weekend))...)
			if got := mustQuote(t, in, testCatalog(), rs).Total; got != tc.want {
				t.Fatalf("total = %d, want %d", got, tc.want)
			}
		})
	}
}
