package establishment

import (
	"testing"

	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

func TestParseSettings_Scenarios(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		factor   string
		rounding pricing.Rounding
		vat      int64
		wantErr  bool
	}{
		{"empty document", `{}`, "0.5", pricing.RoundingNone, 0, false},
		{"null", ``, "0.5", pricing.RoundingNone, 0, false},
		{"stored values", `{"pricing":{"half_day_factor":"0.6","rounding":"euro","default_vat_rate_bp":2200}}`, "0.6", pricing.RoundingEuro, 2200, false},
		{"malformed", `{"pricing":`, "0.5", pricing.RoundingNone, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := ParseSettings(store.JSON(tt.raw), "EUR")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if s.Pricing.HalfDayFactor.String() != tt.factor || s.Pricing.Rounding != tt.rounding || s.Pricing.DefaultVATRateBP != tt.vat {
				t.Fatalf("settings = %+v", s.Pricing)
			}
			if s.DefaultPolicy.MaxLeadTimeDays == nil {
				t.Fatal("default policy must be filled")
			}
			if !tt.wantErr && s.Pricing.Currency != "EUR" {
				t.Fatalf("currency %q", s.Pricing.Currency)
			}
		})
	}
}
