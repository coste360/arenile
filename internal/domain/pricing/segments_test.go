package pricing

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Split stays (ADR 0028 decision 1): each day uses the price category of the
// spot occupied that day.
func TestQuote_SegmentsPerDayCategory(t *testing.T) {
	rules := RuleSet{
		Rules: []Rule{
			baseRule("r_row1", 2000, 0, Conditions{PriceCategories: []string{"row1"}}),
			baseRule("r_premium", 3000, 0, Conditions{PriceCategories: []string{"premium"}}),
		},
		FixedPrices: []FixedDurationPrice{
			{ID: "f1", PackID: "p_base", Formula: FormulaWeek, PriceCategory: "row1", Amount: 10000},
			{ID: "f2", PackID: "p_base", Formula: FormulaWeek, PriceCategory: "premium", Amount: 17000},
		},
	}
	seg := func(spot, from, to string) LineSegment { return LineSegment{SpotID: spot, From: d(from), To: d(to)} }
	tests := []struct {
		name     string
		formula  Formula
		from, to string
		spot     string
		segments []LineSegment
		want     money.Cents
		wantErr  bool
	}{
		{name: "single spot", formula: FormulaMultiDay, from: "2027-07-10", to: "2027-07-12", spot: "s_a12", want: 6000},
		{
			name: "same category segments keep the price", formula: FormulaMultiDay, from: "2027-07-10", to: "2027-07-12", spot: "s_a12",
			segments: []LineSegment{seg("s_a12", "2027-07-10", "2027-07-11"), seg("s_a12", "2027-07-12", "2027-07-12")}, want: 6000,
		},
		{
			name: "last day on premium", formula: FormulaMultiDay, from: "2027-07-10", to: "2027-07-12", spot: "s_a12",
			segments: []LineSegment{seg("s_a12", "2027-07-10", "2027-07-11"), seg("s_b5", "2027-07-12", "2027-07-12")}, want: 7000,
		},
		{
			name: "line spot premium, first day row1", formula: FormulaMultiDay, from: "2027-07-10", to: "2027-07-12", spot: "s_b5",
			segments: []LineSegment{seg("s_a12", "2027-07-10", "2027-07-10")}, want: 8000,
		},
		{
			name: "fixed week prorated by category", formula: FormulaWeek, from: "2027-07-10", to: "2027-07-16", spot: "s_a12",
			segments: []LineSegment{seg("s_a12", "2027-07-10", "2027-07-14"), seg("s_b5", "2027-07-15", "2027-07-16")},
			want:     money.Cents(10000).MulRat(5, 7) + money.Cents(17000).MulRat(2, 7),
		},
		{
			name: "fixed week one category", formula: FormulaWeek, from: "2027-07-10", to: "2027-07-16", spot: "s_a12",
			segments: []LineSegment{seg("s_b5", "2027-07-10", "2027-07-16")}, want: 17000,
		},
		{
			name: "overlapping segments", formula: FormulaMultiDay, from: "2027-07-10", to: "2027-07-12", spot: "s_a12",
			segments: []LineSegment{seg("s_a12", "2027-07-10", "2027-07-11"), seg("s_b5", "2027-07-11", "2027-07-12")}, wantErr: true,
		},
		{
			name: "segment outside the stay", formula: FormulaMultiDay, from: "2027-07-10", to: "2027-07-12", spot: "s_a12",
			segments: []LineSegment{seg("s_b5", "2027-07-12", "2027-07-13")}, wantErr: true,
		},
		{
			name: "unknown spot", formula: FormulaMultiDay, from: "2027-07-10", to: "2027-07-12", spot: "s_a12",
			segments: []LineSegment{seg("s_x", "2027-07-12", "2027-07-12")}, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := QuoteInput{
				Channel: ChannelDesk, StartsOn: d(tt.from), EndsOn: d(tt.to), Slots: calendar.SlotsFull, Formula: tt.formula,
				Today: d("2027-06-01"), Lines: []LineInput{{PackID: "p_base", SpotID: tt.spot, Segments: tt.segments}},
			}
			bd, err := Quote(in, testCatalog(), rules)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("err = %v, want invalid input", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if bd.Total != tt.want {
				t.Fatalf("total = %d, want %d", bd.Total, tt.want)
			}
		})
	}
}

// A discount restricted to a price category applies only to the days spent
// on a spot of that category.
func TestQuote_SegmentsDiscountPerDayCategory(t *testing.T) {
	rules := RuleSet{Rules: []Rule{
		baseRule("r_row1", 2000, 0, Conditions{PriceCategories: []string{"row1"}}),
		baseRule("r_premium", 3000, 0, Conditions{PriceCategories: []string{"premium"}}),
		{
			ID: "disc", Name: "disc", TargetType: TargetPack, Stage: StageDiscount, Active: true,
			Conditions: Conditions{PriceCategories: []string{"premium"}},
			Action:     Action{Type: ActionPercentOff, Percent: dec("10")},
		},
	}}
	in := QuoteInput{
		Channel: ChannelDesk, StartsOn: d("2027-07-10"), EndsOn: d("2027-07-11"), Slots: calendar.SlotsFull, Formula: FormulaMultiDay,
		Today: d("2027-06-01"), Lines: []LineInput{{PackID: "p_base", SpotID: "s_a12", Segments: []LineSegment{
			{SpotID: "s_a12", From: d("2027-07-10"), To: d("2027-07-10")}, {SpotID: "s_b5", From: d("2027-07-11"), To: d("2027-07-11")},
		}}},
	}
	bd, err := Quote(in, testCatalog(), rules)
	if err != nil {
		t.Fatal(err)
	}
	if want := money.Cents(2000 + 3000 - 300); bd.Total != want {
		t.Fatalf("total = %d, want %d", bd.Total, want)
	}
}
