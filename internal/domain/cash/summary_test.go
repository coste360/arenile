package cash

import (
	"testing"

	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

func TestSummarize_Scenarios(t *testing.T) {
	pm := func(method, dir string, amount money.Cents, rate int64) store.Payment {
		taxable, tax := money.VATSplit(amount, rate)
		return store.Payment{
			Method: method, Direction: dir, Amount: amount, Status: store.PaymentSucceeded,
			VATBreakdown: []store.VATShare{{RateBP: rate, Gross: amount, Taxable: taxable, Tax: tax}},
		}
	}
	payments := []store.Payment{
		pm(store.PaymentMethodCash, store.PaymentDirectionIn, 2000, 2200),
		pm(store.PaymentMethodCash, store.PaymentDirectionIn, 1100, 1000),
		pm(store.PaymentMethodCard, store.PaymentDirectionIn, 5000, 2200),
		pm(store.PaymentMethodCash, store.PaymentDirectionRefund, 500, 2200),
		{Method: store.PaymentMethodCard, Direction: store.PaymentDirectionIn, Amount: 999, Status: store.PaymentFailed},
	}
	movements := []store.CashMovement{
		{Kind: store.CashMovementIn, Amount: 1000},
		{Kind: store.CashMovementOut, Amount: 300},
	}
	tests := []struct {
		name     string
		counted  store.MethodTotals
		wantDiff money.Cents
		wantPer  store.MethodTotals
	}{
		{"open session", nil, 0, nil},
		{"exact count", store.MethodTotals{"cash": 13300, "card": 5000}, 0, store.MethodTotals{"cash": 0, "card": 0}},
		{"cash short, card missing counts as 0", store.MethodTotals{"cash": 13000}, -5300, store.MethodTotals{"cash": -300, "card": -5000}},
		{"extra method counted", store.MethodTotals{"cash": 13300, "card": 5000, "other": 100}, 100, store.MethodTotals{"cash": 0, "card": 0, "other": 100}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Summarize(10000, payments, movements, tt.counted)
			// float 100 + cash 20 + 11 - 5 + in 10 - out 3 = 133.00
			if s.Expected["cash"] != 13300 || s.Expected["card"] != 5000 {
				t.Fatalf("expected %+v", s.Expected)
			}
			if s.Takings["cash"] != 2600 || s.Takings["card"] != 5000 || s.Payments != 3 || s.Refunds != 1 {
				t.Fatalf("takings %+v %d %d", s.Takings, s.Payments, s.Refunds)
			}
			if s.MovementsIn != 1000 || s.MovementsOut != 300 {
				t.Fatalf("movements %d %d", s.MovementsIn, s.MovementsOut)
			}
			if len(s.ByVAT) != 2 || s.ByVAT[0] != (VATTotal{1000, 1100, 1000, 100}) || s.ByVAT[1].Gross != 6500 {
				t.Fatalf("vat %+v", s.ByVAT)
			}
			if got := s.Difference(); got != tt.wantDiff {
				t.Fatalf("difference %d, want %d", got, tt.wantDiff)
			}
			if tt.counted == nil {
				if s.Counted != nil || s.Differences != nil {
					t.Fatal("open session must not have counted totals")
				}
				return
			}
			// INV-25: difference per method and in total.
			var sum money.Cents
			for m, want := range tt.wantPer {
				if s.Differences[m] != want || s.Differences[m] != s.Counted[m]-s.Expected[m] {
					t.Fatalf("difference %s = %d, want %d", m, s.Differences[m], want)
				}
				sum += s.Differences[m]
			}
			if sum != s.Difference() || s.Counted.Sum()-s.Expected.Sum() != s.Difference() {
				t.Fatalf("total difference %d vs %d", sum, s.Difference())
			}
		})
	}
}
