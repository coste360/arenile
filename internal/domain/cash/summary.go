package cash

import (
	"sort"

	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// VATTotal is the takings of a session for one VAT rate (refunds negative);
// Taxable + Tax == Gross. The scorporo runs per (rate, method) and the
// shares are summed, the same granularity as the rows of the corrispettivi
// export (fiscal.BuildReport), so that the Z summary of a one-session day
// and the daily export agree to the cent on taxable and tax.
type VATTotal struct {
	RateBP  int64
	Gross   money.Cents
	Taxable money.Cents
	Tax     money.Cents
}

// Summary is the Z-style report of a session (spec 14.1): takings by method
// and by VAT rate, cash movements, expected totals and, once counted, the
// counted totals and differences (INV-25).
type Summary struct {
	// Takings are the succeeded payments by method, refunds negative.
	Takings store.MethodTotals
	// Expected = Takings, plus the opening float and the movements on cash.
	Expected store.MethodTotals
	// Counted has every key of Expected (missing counted methods are 0);
	// nil until the session is counted.
	Counted store.MethodTotals
	// Differences = Counted - Expected per method; nil until counted.
	Differences  store.MethodTotals
	ByVAT        []VATTotal
	MovementsIn  money.Cents
	MovementsOut money.Cents
	Payments     int
	Refunds      int
}

// Difference is sum(Counted) - sum(Expected) (0 until counted).
func (s Summary) Difference() money.Cents {
	if s.Counted == nil {
		return 0
	}
	return s.Counted.Sum() - s.Expected.Sum()
}

// Summarize computes the summary of a session from its float, payments and
// movements; counted may be nil (session still open). Payments flagged as
// test (test mode, INV-74) are left out. Pure function.
func Summarize(openingFloat money.Cents, payments []store.Payment, movements []store.CashMovement, counted store.MethodTotals) Summary {
	s := Summary{
		Takings:  store.MethodTotals{},
		Expected: store.MethodTotals{store.PaymentMethodCash: openingFloat},
		ByVAT:    []VATTotal{},
	}
	type vatKey struct {
		rate   int64
		method string
	}
	vat := map[vatKey]money.Cents{}
	for i := range payments {
		pm := &payments[i]
		// Test-mode payments (INV-74) move no real money: never in the Z.
		if pm.Status != store.PaymentSucceeded || pm.Test {
			continue
		}
		signed := pm.Signed()
		s.Takings[pm.Method] += signed
		s.Expected[pm.Method] += signed
		if pm.Direction == store.PaymentDirectionRefund {
			s.Refunds++
		} else {
			s.Payments++
		}
		sign := money.Cents(1)
		if pm.Direction == store.PaymentDirectionRefund {
			sign = -1
		}
		for _, sh := range pm.VATBreakdown {
			vat[vatKey{sh.RateBP, pm.Method}] += sign * sh.Gross
		}
	}
	for _, m := range movements {
		switch m.Kind {
		case store.CashMovementIn:
			s.MovementsIn += m.Amount
			s.Expected[store.PaymentMethodCash] += m.Amount
		case store.CashMovementOut:
			s.MovementsOut += m.Amount
			s.Expected[store.PaymentMethodCash] -= m.Amount
		}
	}
	byRate := map[int64]*VATTotal{}
	for k, gross := range vat {
		t := byRate[k.rate]
		if t == nil {
			t = &VATTotal{RateBP: k.rate}
			byRate[k.rate] = t
		}
		taxable, tax := money.VATSplit(gross, k.rate)
		t.Gross += gross
		t.Taxable += taxable
		t.Tax += tax
	}
	rates := make([]int64, 0, len(byRate))
	for r := range byRate {
		rates = append(rates, r)
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i] < rates[j] })
	for _, r := range rates {
		s.ByVAT = append(s.ByVAT, *byRate[r])
	}
	if counted != nil {
		s.Counted = store.MethodTotals{}
		s.Differences = store.MethodTotals{}
		for m, v := range counted {
			s.Counted[m] = v
		}
		for m := range s.Expected {
			if _, ok := s.Counted[m]; !ok {
				s.Counted[m] = 0
			}
		}
		for m, v := range s.Counted {
			s.Differences[m] = v - s.Expected[m]
			if _, ok := s.Expected[m]; !ok {
				s.Expected[m] = 0
			}
		}
	}
	return s
}
