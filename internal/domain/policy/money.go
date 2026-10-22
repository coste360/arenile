package policy

import (
	"sort"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Deposit returns the amount due when the booking is made, given its total:
//   - mode "full" (or unset): the total;
//   - mode "none": zero;
//   - mode "deposit", type "percent": value% of the total, rounded half-even
//     to the cent;
//   - mode "deposit", type "amount": value cents, capped at the total.
//
// The result is always in [0, total] (0 when total <= 0). A deposit mode
// without a usable deposit configuration falls back to the full total.
func Deposit(total money.Cents, r Rules) money.Cents {
	if total <= 0 {
		return 0
	}
	switch r.EffectivePaymentMode() {
	case PaymentNone:
		return 0
	case PaymentDeposit:
		d := r.Payment.Deposit
		if d == nil || d.Type == nil || d.Value == nil {
			return total
		}
		var due money.Cents
		switch *d.Type {
		case DepositPercent:
			due = total.MulRat(*d.Value, 100)
		case DepositAmount:
			due = money.Cents(*d.Value)
		default:
			return total
		}
		return clamp(due, 0, total)
	default:
		return total
	}
}

// BalanceDueDate returns the date by which the balance (total - deposit) must
// be paid. ok is false when no balance is expected (mode "full" or "none").
// "on_arrival" (also the default) yields the start date; "days_before:N"
// yields start - N.
func BalanceDueDate(start calendar.Date, r Rules) (due calendar.Date, ok bool) {
	if r.EffectivePaymentMode() != PaymentDeposit {
		return calendar.Date{}, false
	}
	if r.Payment.BalanceDue == nil {
		return start, true
	}
	b, err := ParseBalanceDue(*r.Payment.BalanceDue)
	if err != nil || b.OnArrival {
		return start, true
	}
	return start.AddDays(-b.DaysBefore), true
}

// CancellationResult is the money returned to a customer on cancellation.
// RefundAmount + CreditAmount <= paid always holds.
type CancellationResult struct {
	RefundAmount money.Cents `json:"refund_amount"`
	CreditAmount money.Cents `json:"credit_amount"`
	// TierApplied is the index in Rules.Cancellation of the tier used, or -1.
	TierApplied int `json:"tier_applied"`
	// DaysBefore is start date - cancellation date (negative after the start).
	DaysBefore int `json:"days_before"`
}

// CancellationOutcome computes refund and credit for a cancellation at instant
// now (dates in loc) of a booking starting on start with paid already paid.
// The applicable tier is the one with the largest until_days_before not
// greater than the days left before the start. When no tier applies (or the
// stay has already started) nothing is returned.
func CancellationOutcome(start calendar.Date, paid money.Cents, now time.Time, loc *time.Location, r Rules) CancellationResult {
	if loc == nil {
		loc = time.UTC
	}
	daysBefore := calendar.In(now, loc).DaysUntil(start)
	res := CancellationResult{TierApplied: -1, DaysBefore: daysBefore}
	if daysBefore < 0 {
		return res
	}
	idx := make([]int, len(r.Cancellation))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return r.Cancellation[idx[a]].UntilDaysBefore > r.Cancellation[idx[b]].UntilDaysBefore
	})
	for _, i := range idx {
		t := r.Cancellation[i]
		if daysBefore < t.UntilDaysBefore {
			continue
		}
		res.TierApplied = i
		amount := percentOf(paid, t.RefundPercent)
		if t.As == AsCredit {
			res.CreditAmount = amount
		} else {
			res.RefundAmount = amount
		}
		break
	}
	return res
}

// ModificationOutcome tells whether a customer may change a booking.
type ModificationOutcome struct {
	Allowed bool        `json:"allowed"`
	Fee     money.Cents `json:"fee"`
}

// ModificationAllowed reports whether the customer may modify, from the manage
// link, a booking starting on start at instant now. Without a modification
// rule customers cannot self-modify (the panel can always force a change).
func ModificationAllowed(start calendar.Date, now time.Time, loc *time.Location, r Rules) ModificationOutcome {
	if loc == nil {
		loc = time.UTC
	}
	m := r.Modification
	if m == nil || m.AllowedUntilDaysBefore == nil {
		return ModificationOutcome{}
	}
	daysBefore := calendar.In(now, loc).DaysUntil(start)
	if daysBefore < 0 || daysBefore < *m.AllowedUntilDaysBefore {
		return ModificationOutcome{}
	}
	return ModificationOutcome{Allowed: true, Fee: max(money.Cents(deref(m.Fee, 0)), 0)}
}

// WeatherResult is the compensation for days declared unusable by weather.
type WeatherResult struct {
	Applies      bool        `json:"applies"`
	RefundAmount money.Cents `json:"refund_amount"`
	CreditAmount money.Cents `json:"credit_amount"`
}

// WeatherOutcome returns the compensation for the paid value of the affected
// days (computed by the caller from the price snapshot). It applies only when
// weather_policy.enabled is true; "as" defaults to refund.
func WeatherOutcome(affectedPaid money.Cents, r Rules) WeatherResult {
	w := r.WeatherPolicy
	if w == nil || !deref(w.Enabled, false) || affectedPaid <= 0 {
		return WeatherResult{}
	}
	if deref(w.As, AsRefund) == AsCredit {
		return WeatherResult{Applies: true, CreditAmount: affectedPaid}
	}
	return WeatherResult{Applies: true, RefundAmount: affectedPaid}
}

// SeasonReleaseCreditAmount returns the credit accrued by a season pass holder when
// a released day is resold for resoldAmount (the total of the resold booking
// line for that day). Zero unless season_release_credit.enabled is true.
func SeasonReleaseCreditAmount(resoldAmount money.Cents, r Rules) money.Cents {
	c := r.SeasonReleaseCredit
	if c == nil || !deref(c.Enabled, false) {
		return 0
	}
	return percentOf(resoldAmount, deref(c.Percent, 0))
}

// percentOf returns pct% of amount rounded half-even, within [0, amount].
func percentOf(amount money.Cents, pct int) money.Cents {
	if amount <= 0 || pct <= 0 {
		return 0
	}
	if pct >= 100 {
		return amount
	}
	return clamp(amount.MulRat(int64(pct), 100), 0, amount)
}

func clamp(v, lo, hi money.Cents) money.Cents {
	return min(max(v, lo), hi)
}
