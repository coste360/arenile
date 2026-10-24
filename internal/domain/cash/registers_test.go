package cash

import (
	"testing"

	"github.com/coste360/arenile/internal/store"
)

func TestRegisterCode_Scenarios(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"simple", "Bar", "bar"},
		{"spaces and accents", "Bar Spiaggia è", "bar-spiaggia-e"},
		{"german", "Kasse Süd ß", "kasse-sud-s"},
		{"punctuation runs", "  Cassa #2 -- ingresso!! ", "cassa-2-ingresso"},
		{"nothing left", "€€€", "cassa"},
		{"empty", "", "cassa"},
		{"truncated to 40 without trailing dash", "abcdefghij abcdefghij abcdefghij abcdefghij", "abcdefghij-abcdefghij-abcdefghij-abcdefg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RegisterCode(tt.in)
			if got != tt.want {
				t.Fatalf("RegisterCode(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if !registerRe.MatchString(got) {
				t.Fatalf("code %q does not match the register pattern", got)
			}
		})
	}
}

// TestSummarize_TestPaymentsExcluded (INV-74): test-mode payments never
// reach the takings, the expected totals or the VAT split of the Z.
func TestSummarize_TestPaymentsExcluded(t *testing.T) {
	payments := []store.Payment{
		{
			Method: store.PaymentMethodCash, Direction: store.PaymentDirectionIn, Amount: 1000, Status: store.PaymentSucceeded,
			VATBreakdown: []store.VATShare{{RateBP: 2200, Gross: 1000}},
		},
		{
			Method: store.PaymentMethodCash, Direction: store.PaymentDirectionIn, Amount: 5000, Status: store.PaymentSucceeded, Test: true,
			VATBreakdown: []store.VATShare{{RateBP: 2200, Gross: 5000}},
		},
	}
	s := Summarize(500, payments, nil, nil)
	if s.Takings["cash"] != 1000 || s.Expected["cash"] != 1500 || s.Payments != 1 || len(s.ByVAT) != 1 || s.ByVAT[0].Gross != 1000 {
		t.Fatalf("summary: %+v", s)
	}
}
