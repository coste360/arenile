package priceoverride

import (
	"encoding/json"
	"errors"
	"testing"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

func vat(rate int64, gross money.Cents) pricing.VATBreakdown {
	taxable, tax := money.VATSplit(gross, rate*100)
	return pricing.VATBreakdown{Rate: pricing.NewDecimal(rate, 0), Taxable: taxable, Tax: tax}
}

func breakdown(total money.Cents, vats ...pricing.VATBreakdown) pricing.PriceBreakdown {
	return pricing.PriceBreakdown{
		EngineVersion: 1, Currency: "EUR", Total: total, VAT: vats,
		Lines:     []pricing.LineBreakdown{{Pack: "std", Zone: "z1", Total: total}},
		Discounts: []pricing.DiscountBreakdown{},
	}
}

func vatSum(v []pricing.VATBreakdown) money.Cents {
	var s money.Cents
	for _, x := range v {
		s += x.Taxable + x.Tax
	}
	return s
}

func TestValidateOverride_Table(t *testing.T) {
	tests := []struct {
		name   string
		amount money.Cents
		reason string
		field  string
	}{
		{"ok", 90000, "sconto fedeltà", ""},
		{"zero is a free pass", 0, "omaggio", ""},
		{"negative", -1, "errore", "amount"},
		{"too large", MaxAmount + 1, "errore", "amount"},
		{"reason missing", 1000, "", "reason"},
		{"reason too short", 1000, "ab", "reason"},
		{"reason too long", 1000, string(make([]rune, 501)), "reason"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.amount, tt.reason)
			if tt.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			assertField(t, err, tt.field)
		})
	}
}

func TestApplyOverride_Scenarios(t *testing.T) {
	tests := []struct {
		name   string
		engine pricing.PriceBreakdown
		amount money.Cents
	}{
		{"discount single rate", breakdown(120000, vat(22, 120000)), 100000},
		{"raise single rate", breakdown(120000, vat(22, 120000)), 130000},
		{"two rates", breakdown(150000, vat(22, 100000), vat(10, 50000)), 99999},
		{"free pass", breakdown(120000, vat(22, 120000)), 0},
		{"no vat lines", breakdown(120000), 50000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := Apply(tt.engine, nil, tt.amount)
			if err != nil {
				t.Fatal(err)
			}
			var got Breakdown
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if got.Total != tt.amount {
				t.Fatalf("total %d, want %d", got.Total, tt.amount)
			}
			if len(tt.engine.VAT) > 0 && vatSum(got.VAT) != tt.amount {
				t.Fatalf("vat sums to %d, want %d", vatSum(got.VAT), tt.amount)
			}
			last := got.Discounts[len(got.Discounts)-1]
			if last.Source != Source || last.Amount != tt.amount-tt.engine.Total {
				t.Fatalf("override line %+v", last)
			}
			if got.Override == nil || got.Override.Engine.Total != tt.engine.Total || string(got.Override.Previous) != "null" {
				t.Fatalf("origin %+v", got.Override)
			}
			// Readers that decode price_current as a plain breakdown see the override total.
			var plain pricing.PriceBreakdown
			if err := json.Unmarshal(raw, &plain); err != nil || plain.Total != tt.amount {
				t.Fatalf("plain decode %v %d", err, plain.Total)
			}
		})
	}
}

func TestOverride_RoundTripRestoresEngine(t *testing.T) {
	engine := breakdown(120000, vat(22, 120000))
	snap, _ := json.Marshal(engine)
	tests := []struct {
		name    string
		current store.JSON
	}{
		{"priced by the snapshot", nil},
		{"priced by a re-quote", store.JSON(snap)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &store.Booking{PriceSnapshot: store.JSON(snap), PriceCurrent: tt.current, TotalAmount: 120000}
			eng, prev, err := EngineOf(b)
			if err != nil || eng.Total != 120000 {
				t.Fatalf("engine %v %d", err, eng.Total)
			}
			cur, err := Apply(eng, prev, 90000)
			if err != nil {
				t.Fatal(err)
			}
			b.PriceCurrent, b.TotalAmount = cur, 90000
			// A second override keeps the original engine origin.
			eng2, prev2, err := EngineOf(b)
			if err != nil || eng2.Total != 120000 || string(prev2) != string(prevOrNull(prev)) {
				t.Fatalf("second override origin %v %d %s", err, eng2.Total, prev2)
			}
			restored, total, err := Restore(b)
			if err != nil || total != 120000 {
				t.Fatalf("restore %v %d", err, total)
			}
			if string(restored) != string(tt.current) {
				t.Fatalf("restored price_current %s, want %s", restored, tt.current)
			}
		})
	}
}

func prevOrNull(p json.RawMessage) json.RawMessage {
	if p == nil {
		return json.RawMessage("null")
	}
	return p
}

// TestRescaleVAT_Property: the re-split always sums to the new total and
// every rate keeps taxable + tax == its gross share.
func TestRescaleVAT_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 4).Draw(t, "rates")
		rates := []int64{4, 5, 10, 22}
		var v []pricing.VATBreakdown
		for i := 0; i < n; i++ {
			v = append(v, vat(rates[i], money.Cents(rapid.Int64Range(0, 5_000_000).Draw(t, "gross"))))
		}
		total := money.Cents(rapid.Int64Range(0, int64(MaxAmount)).Draw(t, "total"))
		out := RescaleVAT(v, total)
		if vatSum(out) != total {
			t.Fatalf("sum %d, want %d", vatSum(out), total)
		}
		for _, x := range out {
			if x.Taxable < 0 || x.Tax < 0 {
				t.Fatalf("negative share %+v", x)
			}
		}
	})
}

func assertField(t *testing.T, err error, field string) {
	t.Helper()
	var ve *configerr.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want validation error on %q, got %v", field, err)
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return
		}
	}
	t.Fatalf("no error on %q: %+v", field, ve.Fields)
}
