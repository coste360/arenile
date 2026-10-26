package renewal

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

var d = calendar.MustParse

func TestPassInterval_Scenarios(t *testing.T) {
	src := store.Season{Year: 2027, OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-19")}
	target := store.Season{Year: 2028, OpensOn: d("2028-05-13"), ClosesOn: d("2028-09-17")}
	for _, tc := range []struct {
		name     string
		from, to string
		want     [2]string
		ok       bool
	}{
		{"whole season", "2027-05-15", "2027-09-19", [2]string{"2028-05-13", "2028-09-17"}, true},
		{"sold late, until the end", "2027-07-10", "2027-09-19", [2]string{"2028-05-13", "2028-09-17"}, true},
		{"from the opening, ends in August", "2027-05-15", "2027-08-31", [2]string{"2028-05-13", "2028-08-31"}, true},
		{"July only: weekday kept, month end kept", "2027-07-03", "2027-07-31", [2]string{"2028-07-01", "2028-07-31"}, true},
		{"mid-July fortnight keeps the weekdays", "2027-07-10", "2027-07-24", [2]string{"2028-07-08", "2028-07-22"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, to, ok := PassInterval(src, target, d(tc.from), d(tc.to))
			if ok != tc.ok || (ok && [2]string{f.String(), to.String()} != tc.want) {
				t.Fatalf("got %s..%s ok=%v, want %v ok=%v", f, to, ok, tc.want, tc.ok)
			}
		})
	}
	// A target season shorter than the shifted period: clamped.
	short := store.Season{Year: 2028, OpensOn: d("2028-06-01"), ClosesOn: d("2028-07-15")}
	f, to, ok := PassInterval(src, short, d("2027-07-03"), d("2027-07-31"))
	if !ok || f.String() != "2028-07-01" || to.String() != "2028-07-15" {
		t.Fatalf("clamped: %s..%s %v", f, to, ok)
	}
	// Nothing left.
	if _, _, ok := PassInterval(src, short, d("2027-08-01"), d("2027-08-31")); ok {
		t.Fatal("want no interval")
	}
}

func TestValidateDeadline_Scenarios(t *testing.T) {
	now := time.Date(2027, 10, 1, 8, 0, 0, 0, time.UTC)
	opens := time.Date(2028, 5, 12, 22, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		deadline time.Time
		ok       bool
	}{
		{"future and before the opening", time.Date(2028, 3, 31, 21, 59, 0, 0, time.UTC), true},
		{"zero", time.Time{}, false},
		{"past", now.Add(-time.Minute), false},
		{"now", now, false},
		{"on the opening", opens, false},
		{"after the opening", opens.Add(time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDeadline("deadline_at", tc.deadline, now, opens)
			if (err == nil) != tc.ok {
				t.Fatalf("got %v, want ok=%v", err, tc.ok)
			}
			if err != nil && !errors.Is(err, configerr.ErrValidation) {
				t.Fatalf("want a validation error, got %v", err)
			}
		})
	}
}

func TestSkipReasonOf_Scenarios(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		reason string
		ok     bool
	}{
		{"conflict", &availability.ConflictError{Err: availability.ErrSpotUnavailable}, SkipSpotUnavailable, true},
		{"wrapped spot unavailable", fmt.Errorf("x: %w", availability.ErrSpotUnavailable), SkipSpotUnavailable, true},
		{"no price", fmt.Errorf("x: %w", pricing.ErrNoPrice), SkipNoPrice, true},
		{"validation", configerr.Invalid("formula", "no"), SkipNoPrice, true},
		{"other", errors.New("db down"), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := SkipReasonOf(tc.err)
			if r != tc.reason || ok != tc.ok {
				t.Fatalf("got %q %v, want %q %v", r, ok, tc.reason, tc.ok)
			}
		})
	}
}

func TestPublicActions_Scenarios(t *testing.T) {
	now := time.Date(2028, 2, 1, 10, 0, 0, 0, time.UTC)
	deadline := now.Add(48 * time.Hour)
	hold := deadline
	past := now.Add(-time.Minute)
	base := ActionsInput{
		Status: store.RenewalProposed, BookingStatus: "held", Deadline: deadline, HoldExpiresAt: &hold, Now: now,
		Total: 100000, Deposit: 30000, Online: true,
	}
	for _, tc := range []struct {
		name string
		mod  func(*ActionsInput)
		want Actions
	}{
		{"open with deposit", func(*ActionsInput) {}, Actions{Accept: true, Decline: true, PayDeposit: true, PayFull: true}},
		{"no deposit in the policy", func(in *ActionsInput) { in.Deposit = 0 }, Actions{Accept: true, Decline: true, PayFull: true}},
		{"deposit equal to the total", func(in *ActionsInput) { in.Deposit = 100000 }, Actions{Accept: true, Decline: true, PayFull: true}},
		{"deposit already paid", func(in *ActionsInput) { in.Paid = 30000 }, Actions{Accept: true, Decline: true, PayFull: true}},
		{"offline establishment", func(in *ActionsInput) { in.Online = false }, Actions{Accept: true, Decline: true}},
		{"credit", func(in *ActionsInput) { in.Credit, in.CanApply = 5000, true }, Actions{Accept: true, Decline: true, PayDeposit: true, PayFull: true, UseCredit: true}},
		{"credit without applier", func(in *ActionsInput) { in.Credit = 5000 }, Actions{Accept: true, Decline: true, PayDeposit: true, PayFull: true}},
		{"deadline passed", func(in *ActionsInput) { in.Deadline = past }, Actions{}},
		{"accepted after the deadline with a running payment hold", func(in *ActionsInput) {
			in.Status, in.BookingStatus, in.Deadline = store.RenewalAccepted, "pending_payment", past
		}, Actions{Accept: true, Decline: true, PayDeposit: true, PayFull: true}},
		{"hold elapsed", func(in *ActionsInput) { in.HoldExpiresAt = &past }, Actions{}},
		{"converted", func(in *ActionsInput) { in.Status, in.BookingStatus = store.RenewalConverted, "confirmed" }, Actions{}},
		{"declined", func(in *ActionsInput) { in.Status, in.BookingStatus = store.RenewalDeclined, "cancelled" }, Actions{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mod(&in)
			if got := PublicActions(in); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAcceptRefusal_Scenarios(t *testing.T) {
	for _, tc := range []struct {
		name, status, booking string
		want                  error
	}{
		{"deadline passed while proposed", store.RenewalProposed, "held", ErrExpired},
		{"hold elapsed after the acceptance", store.RenewalAccepted, "pending_payment", ErrExpired},
		{"booking cancelled, proposal still open", store.RenewalProposed, "cancelled", ErrExpired},
		{"confirmed by the desk before the link loaded", store.RenewalProposed, "confirmed", ErrNotOpen},
		{"paid online, not yet converted", store.RenewalAccepted, "confirmed", ErrNotOpen},
		{"already checked in", store.RenewalAccepted, "checked_in", ErrNotOpen},
		{"converted", store.RenewalConverted, "confirmed", ErrNotOpen},
		{"declined", store.RenewalDeclined, "cancelled", ErrNotOpen},
		{"expired", store.RenewalExpired, "cancelled", ErrExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AcceptRefusal(ActionsInput{Status: tc.status, BookingStatus: tc.booking}); !errors.Is(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAmountDue_Scenarios(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		payment              string
		total, paid, deposit money.Cents
		want                 money.Cents
	}{
		{"full", PaymentFull, 100000, 0, 30000, 100000},
		{"deposit", PaymentDeposit, 100000, 0, 30000, 30000},
		{"deposit partly paid by credit", PaymentDeposit, 100000, 10000, 30000, 20000},
		{"deposit covered", PaymentDeposit, 100000, 40000, 30000, 0},
		{"no deposit configured", PaymentDeposit, 100000, 0, 0, 100000},
		{"full after credit", PaymentFull, 100000, 25000, 30000, 75000},
		{"overpaid", PaymentFull, 100000, 120000, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AmountDue(tc.payment, tc.total, tc.paid, tc.deposit); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}
