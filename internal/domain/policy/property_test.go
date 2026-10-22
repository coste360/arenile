package policy

import (
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

func genRules(t *rapid.T) Rules {
	r := Rules{}
	switch rapid.IntRange(0, 3).Draw(t, "mode") {
	case 0:
		// unset
	case 1:
		r.Payment = &Payment{Mode: Ptr(PaymentFull)}
	case 2:
		r.Payment = &Payment{Mode: Ptr(PaymentNone)}
	case 3:
		typ := rapid.SampledFrom([]DepositType{DepositPercent, DepositAmount}).Draw(t, "type")
		r.Payment = &Payment{Mode: Ptr(PaymentDeposit), Deposit: &DepositRule{
			Type: Ptr(typ), Value: Ptr(rapid.Int64Range(-1000, 1_000_000).Draw(t, "value")),
		}}
	}
	n := rapid.IntRange(0, 5).Draw(t, "tiers")
	for range n {
		r.Cancellation = append(r.Cancellation, CancellationTier{
			UntilDaysBefore: rapid.IntRange(0, 60).Draw(t, "until"),
			RefundPercent:   rapid.IntRange(-10, 150).Draw(t, "pct"),
			As:              rapid.SampledFrom([]Compensation{"", AsRefund, AsCredit}).Draw(t, "as"),
		})
	}
	return r
}

func TestDeposit_PropertyWithinTotal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		total := money.Cents(rapid.Int64Range(-10_000, 10_000_000).Draw(t, "total"))
		r := genRules(t)
		d := Deposit(total, r)
		if d < 0 || (total >= 0 && d > total) || (total <= 0 && d != 0) {
			t.Fatalf("deposit %d out of [0, %d]", d, total)
		}
	})
}

func TestCancellationOutcome_PropertyNeverExceedsPaid(t *testing.T) {
	start := calendar.MustParse("2027-08-15")
	rapid.Check(t, func(t *rapid.T) {
		paid := money.Cents(rapid.Int64Range(-10_000, 10_000_000).Draw(t, "paid"))
		r := genRules(t)
		offset := rapid.IntRange(-5*24, 90*24).Draw(t, "hours")
		now := time.Date(2027, 8, 15, 12, 0, 0, 0, time.UTC).Add(-time.Duration(offset) * time.Hour)
		res := CancellationOutcome(start, paid, now, time.UTC, r)
		if res.RefundAmount < 0 || res.CreditAmount < 0 {
			t.Fatalf("negative amounts %+v", res)
		}
		if res.RefundAmount > 0 && res.CreditAmount > 0 {
			t.Fatalf("both refund and credit %+v", res)
		}
		if limit := max(paid, 0); res.RefundAmount+res.CreditAmount > limit {
			t.Fatalf("refund+credit %d > paid %d", res.RefundAmount+res.CreditAmount, paid)
		}
	})
}

func TestCancellationOutcome_PropertyMonotonicWithNestedTiers(t *testing.T) {
	// With a well-formed schedule (refund percent non-increasing as the start
	// approaches) cancelling earlier never returns less money.
	start := calendar.MustParse("2027-08-15")
	r := DefaultRules()
	rapid.Check(t, func(t *rapid.T) {
		paid := money.Cents(rapid.Int64Range(0, 1_000_000).Draw(t, "paid"))
		a := rapid.IntRange(0, 60).Draw(t, "a")
		b := rapid.IntRange(0, 60).Draw(t, "b")
		early, late := max(a, b), min(a, b)
		at := func(days int) money.Cents {
			res := CancellationOutcome(start, paid, start.Time().AddDate(0, 0, -days), time.UTC, r)
			return res.RefundAmount + res.CreditAmount
		}
		if at(early) < at(late) {
			t.Fatalf("earlier cancellation (%d days) returns less than later (%d days)", early, late)
		}
	})
}

func TestMerge_PropertyIdentity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		r := genRules(t)
		m1 := Merge(r, Rules{})
		m2 := Merge(Rules{}, r)
		if Deposit(12345, m1) != Deposit(12345, r) || Deposit(12345, m2) != Deposit(12345, r) {
			t.Fatal("merge with empty rules changed the deposit")
		}
		if len(m1.Cancellation) != len(r.Cancellation) || len(m2.Cancellation) != len(r.Cancellation) {
			t.Fatal("merge with empty rules changed the cancellation schedule")
		}
	})
}
