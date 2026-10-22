package admin

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/http/api"
)

func TestPricingAction_RoundTrip(t *testing.T) {
	per := api.AmountPer("day")
	factor, percent := "1.25", "12.5"
	amount := int64(300)
	every, free := 7, 1
	tests := []api.PriceAction{
		{Type: "set", Amount: &amount},
		{Type: "multiply", Factor: &factor},
		{Type: "percent_off", Percent: &percent},
		{Type: "amount_off", Amount: &amount, Per: &per},
		{Type: "free_days", Every: &every, Free: &free},
	}
	for _, in := range tests {
		t.Run(string(in.Type), func(t *testing.T) {
			eng, err := pcActionFromAPI(in)
			if err != nil {
				t.Fatal(err)
			}
			out := pcActionToAPI(eng)
			if out.Type != in.Type || pcStrOr(out.Factor) != pcStrOr(in.Factor) || pcStrOr(out.Percent) != pcStrOr(in.Percent) ||
				pcIntOr(out.Every, 0) != pcIntOr(in.Every, 0) || pcIntOr(out.Free, 0) != pcIntOr(in.Free, 0) {
				t.Fatalf("round trip %+v -> %+v", in, out)
			}
			if (in.Amount == nil) != (out.Amount == nil) || (in.Amount != nil && *in.Amount != *out.Amount) {
				t.Fatalf("amount %+v -> %+v", in.Amount, out.Amount)
			}
		})
	}
	bad := "1e3"
	if _, err := pcActionFromAPI(api.PriceAction{Type: "multiply", Factor: &bad}); !errors.Is(err, configerr.ErrValidation) {
		t.Fatalf("exponent rejected: %v", err)
	}
}

func TestPricingConditions_RoundTrip(t *testing.T) {
	minDays := 3
	in := api.PriceConditions{
		RatePeriods: &[]string{"high"}, Weekdays: &[]int{6, 7}, Slots: &[]api.Slot{"AM"}, MinDays: &minDays,
		Channels: &[]api.Channel{"online"}, DateRanges: &[]api.DateRange{{From: calendar.MustParse("2027-08-10"), To: calendar.MustParse("2027-08-20")}},
	}
	eng, err := pcConditionsFromAPI(&in)
	if err != nil {
		t.Fatal(err)
	}
	if eng.Count() != 6 || eng.Slots[0] != calendar.AM || eng.Channels[0] != pricing.ChannelOnline {
		t.Fatalf("engine %+v", eng)
	}
	out := pcConditionsToAPI(eng)
	if (*out.DateRanges)[0].To != calendar.MustParse("2027-08-20") || *out.MinDays != 3 || out.ZoneIds != nil {
		t.Fatalf("api %+v", out)
	}
	if empty, err := pcConditionsFromAPI(nil); err != nil || empty.Count() != 0 {
		t.Fatalf("nil conditions: %+v %v", empty, err)
	}
}

func TestBreakdownToAPI_VATAsDecimalString(t *testing.T) {
	bd := pricing.PriceBreakdown{
		EngineVersion: 1, Currency: "EUR", Total: 1000,
		Lines: []pricing.LineBreakdown{{Pack: "base", Zone: "row1", Total: 1000, Days: []pricing.DayBreakdown{{
			Date: calendar.MustParse("2027-08-02"), Slots: calendar.SlotsFull, Base: 1000, Total: 1000,
			Applied: []pricing.AppliedRule{{Rule: "r", Stage: pricing.StageBase, Effect: "set 1000"}},
		}}}},
		Discounts: []pricing.DiscountBreakdown{},
		VAT:       []pricing.VATBreakdown{{Rate: pricing.NewDecimal(2200, 2), Taxable: 820, Tax: 180}},
	}
	out := BreakdownToAPI(bd)
	if out.Vat[0].Rate != "22" || out.Lines[0].Spot != nil || out.Extras != nil || len(out.Lines[0].Days[0].Applied) != 1 {
		t.Fatalf("breakdown %+v", out)
	}
}
