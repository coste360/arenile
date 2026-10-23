package cash_test

import (
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// TestSummarize_MatchesCorrispettivi: for a day served by a single cash
// session, the Z summary per VAT rate and the per-rate totals of the daily
// corrispettivi export agree to the cent (same scorporo granularity: per
// rate and method), with amounts chosen so that rounding per payment, per
// rate or per method would differ.
func TestSummarize_MatchesCorrispettivi(t *testing.T) {
	day := calendar.MustParse("2027-07-15")
	pay := func(method, dir string, shares ...store.VATShare) store.Payment {
		var amount money.Cents
		for i := range shares {
			shares[i].Taxable, shares[i].Tax = money.VATSplit(shares[i].Gross, shares[i].RateBP)
			amount += shares[i].Gross
		}
		return store.Payment{Method: method, Direction: dir, Amount: amount, Status: store.PaymentSucceeded, BusinessDate: day, VATBreakdown: shares}
	}
	payments := []store.Payment{
		pay(store.PaymentMethodCash, store.PaymentDirectionIn, store.VATShare{RateBP: 2200, Gross: 1001}),
		pay(store.PaymentMethodCash, store.PaymentDirectionIn, store.VATShare{RateBP: 2200, Gross: 333}, store.VATShare{RateBP: 1000, Gross: 777}),
		pay(store.PaymentMethodCard, store.PaymentDirectionIn, store.VATShare{RateBP: 2200, Gross: 1999}),
		pay(store.PaymentMethodCard, store.PaymentDirectionIn, store.VATShare{RateBP: 1000, Gross: 101}),
		pay(store.PaymentMethodCash, store.PaymentDirectionRefund, store.VATShare{RateBP: 2200, Gross: 123}),
		pay(store.PaymentMethodCredit, store.PaymentDirectionIn, store.VATShare{RateBP: 2200, Gross: 457}),
	}
	z := cash.Summarize(0, payments, nil, nil)
	rep := fiscal.BuildReport(payments, day, day, fiscal.GroupByDay, "EUR", 2200)
	if len(z.ByVAT) != len(rep.Totals) {
		t.Fatalf("rates: z %+v, export %+v", z.ByVAT, rep.Totals)
	}
	for i, v := range z.ByVAT {
		e := rep.Totals[i]
		if v.RateBP != e.RateBP || v.Gross != e.Gross || v.Taxable != e.Taxable || v.Tax != e.Tax {
			t.Fatalf("rate %d: z %+v, export %+v", v.RateBP, v, e)
		}
		if v.Taxable+v.Tax != v.Gross {
			t.Fatalf("INV-16: %+v", v)
		}
	}
	var gross money.Cents
	for _, v := range z.ByVAT {
		gross += v.Gross
	}
	if gross != rep.GrandTotal || gross != z.Takings.Sum() {
		t.Fatalf("gross %d, export %d, takings %d", gross, rep.GrandTotal, z.Takings.Sum())
	}
}
