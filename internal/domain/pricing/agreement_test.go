package pricing

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/money"
)

func TestQuote_AgreementPackPrices(t *testing.T) {
	pct := func(s string) Action { return Action{Type: ActionPercentOff, Percent: dec(s)} }
	threeDays := func() QuoteInput { return span("2027-07-05", "2027-07-07", FormulaDay) } // 3 x 30.00
	tests := []struct {
		name      string
		in        func() QuoteInput
		agreement Agreement
		want      money.Cents
	}{
		{
			name:      "fixed price per day replaces the list price",
			in:        threeDays,
			agreement: Agreement{PackPrices: []AgreementPackPrice{{PackID: "p_base", Amount: 1200}}},
			want:      3600,
		},
		{
			name:      "fixed price per line",
			in:        threeDays,
			agreement: Agreement{PackPrices: []AgreementPackPrice{{PackID: "p_base", Amount: 5000, Per: PerLine}}},
			want:      5000,
		},
		{
			name: "fixed price per booking covers every line of the pack",
			in: func() QuoteInput {
				in := threeDays()
				in.Lines = append(in.Lines, LineInput{PackID: "p_base", ZoneID: "z2"})
				return in
			},
			agreement: Agreement{PackPrices: []AgreementPackPrice{{PackID: "p_base", Amount: 10000, Per: PerBooking}}},
			want:      10000,
		},
		{
			name:      "a fixed price above the list price never raises it",
			in:        threeDays,
			agreement: Agreement{PackPrices: []AgreementPackPrice{{PackID: "p_base", Amount: 4000}}},
			want:      9000,
		},
		{
			name:      "other packs get the action",
			in:        func() QuoteInput { in := threeDays(); in.Lines[0].PackID = "p_gaz"; return in },
			agreement: Agreement{Action: pct("10"), PackPrices: []AgreementPackPrice{{PackID: "p_base", Amount: 1200}}},
			want:      8100,
		},
		{
			name: "scope excludes extras unless apply_to_extras",
			in: func() QuoteInput {
				in := oneDay()
				in.Lines[0].Extras = []ExtraInput{{ExtraID: "x_sunbed", Quantity: 1}}
				return in
			},
			agreement: Agreement{Action: pct("50"), Scope: &AgreementScope{}},
			want:      1500 + 800,
		},
		{
			name: "scope with apply_to_extras",
			in: func() QuoteInput {
				in := oneDay()
				in.Lines[0].Extras = []ExtraInput{{ExtraID: "x_sunbed", Quantity: 1}}
				return in
			},
			agreement: Agreement{Action: pct("50"), Scope: &AgreementScope{ApplyToExtras: true}},
			want:      1500 + 400,
		},
		{
			name:      "scope restricted to other packs leaves the price",
			in:        oneDay,
			agreement: Agreement{Action: pct("50"), Scope: &AgreementScope{PackIDs: []string{"p_vip"}}},
			want:      3000,
		},
		{
			name:      "nil scope keeps the behaviour before M8",
			in:        oneDay,
			agreement: Agreement{Action: pct("50")},
			want:      1500,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in()
			a := tt.agreement
			a.ID, a.Code, a.Active = "ag1", "RES", true
			in.Agreement = &a
			got := mustQuote(t, in, testCatalog(), ruleSet(defaultRules()...))
			if got.Total != tt.want {
				t.Fatalf("total = %d, want %d (discounts %+v)", got.Total, tt.want, got.Discounts)
			}
		})
	}
}

func TestValidateAgreement_PackPrices(t *testing.T) {
	ok := Agreement{ID: "a", Code: "X", PackPrices: []AgreementPackPrice{{PackID: "p", Amount: 100}}}
	if err := ValidateAgreement(ok); err != nil {
		t.Fatalf("fixed prices only: %v", err)
	}
	ok.Action = Action{Type: ActionPercentOff, Percent: dec("0")}
	if err := ValidateAgreement(ok); err != nil {
		t.Fatalf("fixed prices with a zero action: %v", err)
	}
	for _, bad := range [][]AgreementPackPrice{
		{{PackID: "", Amount: 1}},
		{{PackID: "p", Amount: -1}},
		{{PackID: "p", Amount: 1}, {PackID: "p", Amount: 2}},
		{{PackID: "p", Amount: 1, Per: "week"}},
	} {
		if err := ValidateAgreement(Agreement{ID: "a", Code: "X", PackPrices: bad}); err == nil {
			t.Fatalf("pack prices %+v accepted", bad)
		}
	}
}

// TestQuote_AgreementPackPriceProperty: with a fixed price per day the
// total is min(list, fixed) per day and never negative.
func TestQuote_AgreementPackPriceProperty(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		days := rapid.IntRange(1, 10).Draw(rt, "days")
		fixed := money.Cents(rapid.Int64Range(0, 6000).Draw(rt, "fixed"))
		in := oneDay()
		in.EndsOn = in.StartsOn.AddDays(days - 1)
		in.Agreement = &Agreement{ID: "a", Code: "X", Active: true, PackPrices: []AgreementPackPrice{{PackID: "p_base", Amount: fixed}}}
		b, err := Quote(in, testCatalog(), ruleSet(defaultRules()...))
		if err != nil {
			rt.Fatalf("Quote: %v", err)
		}
		checkInvariants(rt, b)
		want := min(fixed, 3000) * money.Cents(days)
		if b.Total != want {
			rt.Fatalf("total %d, want %d", b.Total, want)
		}
	})
}
