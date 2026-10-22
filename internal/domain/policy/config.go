package policy

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
)

// BalanceDueOnArrival is the balance_due value meaning "pay the rest at the desk
// on the first day".
const BalanceDueOnArrival = "on_arrival"

const balanceDueDaysBeforePrefix = "days_before:"

// BalanceDue is the parsed form of Payment.BalanceDue.
type BalanceDue struct {
	OnArrival  bool
	DaysBefore int // meaningful when OnArrival is false
}

// ParseBalanceDue parses "on_arrival" or "days_before:N" (N >= 0).
func ParseBalanceDue(s string) (BalanceDue, error) {
	if s == BalanceDueOnArrival {
		return BalanceDue{OnArrival: true}, nil
	}
	rest, ok := strings.CutPrefix(s, balanceDueDaysBeforePrefix)
	if !ok {
		return BalanceDue{}, fmt.Errorf("%w: balance_due %q", ErrInvalidRules, s)
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 {
		return BalanceDue{}, fmt.Errorf("%w: balance_due %q", ErrInvalidRules, s)
	}
	return BalanceDue{DaysBefore: n}, nil
}

// String renders the canonical form.
func (b BalanceDue) String() string {
	if b.OnArrival {
		return BalanceDueOnArrival
	}
	return balanceDueDaysBeforePrefix + strconv.Itoa(b.DaysBefore)
}

// parseClock parses "HH:MM" into minutes after midnight.
func parseClock(s string) (int, error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, fmt.Errorf("%w: time %q is not HH:MM", ErrInvalidRules, s)
	}
	h, err1 := strconv.Atoi(s[:2])
	m, err2 := strconv.Atoi(s[3:])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("%w: time %q is not HH:MM", ErrInvalidRules, s)
	}
	return h*60 + m, nil
}

// CheckConfig verifies that the rules are well formed (used when a manager
// saves a policy). It returns an error wrapping ErrInvalidRules listing every
// problem, or nil.
func (r Rules) CheckConfig() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("%w: "+format, append([]any{ErrInvalidRules}, args...)...))
	}
	nonNeg := func(name string, p *int) {
		if p != nil && *p < 0 {
			bad("%s must be >= 0", name)
		}
	}
	pct := func(name string, v int) {
		if v < 0 || v > 100 {
			bad("%s must be between 0 and 100", name)
		}
	}
	if m := r.MinLeadTime; m != nil {
		nonNeg("min_lead_time.days", m.Days)
		if m.CutoffTime != nil {
			if _, err := parseClock(*m.CutoffTime); err != nil {
				errs = append(errs, err)
			}
		}
	}
	nonNeg("max_lead_time_days", r.MaxLeadTimeDays)
	if s := r.SameDayBooking; s != nil {
		if s.Until != nil {
			if _, err := parseClock(*s.Until); err != nil {
				errs = append(errs, err)
			}
		}
		if s.OnlySlot != nil {
			switch *s.OnlySlot {
			case "", OnlySlotAM, OnlySlotPM, OnlySlotPMAfter:
			default:
				bad("same_day_booking.only_slot %q", *s.OnlySlot)
			}
			if *s.OnlySlot == OnlySlotPMAfter && s.Until == nil {
				bad("same_day_booking.only_slot PM_after requires until")
			}
		}
	}
	if d := r.Duration; d != nil {
		for _, f := range d.AllowedFormulas {
			if !f.Valid() {
				bad("duration.allowed_formulas: unknown formula %q", f)
			}
		}
		nonNeg("duration.min_days", d.MinDays)
		nonNeg("duration.max_days", d.MaxDays)
		if d.MinDays != nil && d.MaxDays != nil && *d.MaxDays < *d.MinDays {
			bad("duration.max_days < duration.min_days")
		}
		if w := d.FixedStartWeekday; w != nil && (*w < 0 || *w > 7) {
			bad("duration.fixed_start_weekday must be 0 (any) or 1..7")
		}
	}
	if p := r.Payment; p != nil {
		if p.Mode != nil {
			switch *p.Mode {
			case PaymentFull, PaymentNone:
			case PaymentDeposit:
				if p.Deposit == nil || p.Deposit.Type == nil || p.Deposit.Value == nil {
					bad("payment.mode deposit requires deposit.type and deposit.value")
				}
			default:
				bad("payment.mode %q", *p.Mode)
			}
		}
		if dep := p.Deposit; dep != nil {
			if dep.Type != nil && *dep.Type != DepositPercent && *dep.Type != DepositAmount {
				bad("payment.deposit.type %q", *dep.Type)
			}
			if dep.Value != nil {
				if *dep.Value < 0 {
					bad("payment.deposit.value must be >= 0")
				}
				if dep.Type != nil && *dep.Type == DepositPercent && *dep.Value > 100 {
					bad("payment.deposit.value must be <= 100 for percent")
				}
			}
		}
		if p.BalanceDue != nil {
			if _, err := ParseBalanceDue(*p.BalanceDue); err != nil {
				errs = append(errs, err)
			}
		}
	}
	seen := map[int]bool{}
	for i, t := range r.Cancellation {
		if t.UntilDaysBefore < 0 {
			bad("cancellation[%d].until_days_before must be >= 0", i)
		}
		if seen[t.UntilDaysBefore] {
			bad("cancellation[%d]: duplicate until_days_before %d", i, t.UntilDaysBefore)
		}
		seen[t.UntilDaysBefore] = true
		pct(fmt.Sprintf("cancellation[%d].refund_percent", i), t.RefundPercent)
		if t.As != "" && t.As != AsRefund && t.As != AsCredit {
			bad("cancellation[%d].as %q", i, t.As)
		}
	}
	if m := r.Modification; m != nil {
		nonNeg("modification.allowed_until_days_before", m.AllowedUntilDaysBefore)
		if m.Fee != nil && *m.Fee < 0 {
			bad("modification.fee must be >= 0")
		}
	}
	if w := r.WeatherPolicy; w != nil && w.As != nil && *w.As != AsRefund && *w.As != AsCredit {
		bad("weather_policy.as %q", *w.As)
	}
	if m := r.MaxSpotsPerBooking; m != nil && *m < 1 {
		bad("max_spots_per_booking must be >= 1")
	}
	if c := r.SeasonReleaseCredit; c != nil && c.Percent != nil {
		pct("season_release_credit.percent", *c.Percent)
	}
	return errors.Join(errs...)
}

// RequiresApproval reports the effective approval_required flag (default false).
func (r Rules) RequiresApproval() bool { return deref(r.ApprovalRequired, false) }

// AllowsSpotChangeDuringStay reports allow_spot_change_during_stay (default false).
func (r Rules) AllowsSpotChangeDuringStay() bool { return deref(r.AllowSpotChangeDuringStay, false) }

// CustomerCanRelease reports customer_can_release_season_days (default false).
func (r Rules) CustomerCanRelease() bool { return deref(r.CustomerCanReleaseSeasonDays, false) }

// EffectivePaymentMode returns the payment mode, defaulting to "full" when the
// rules do not say otherwise (the conservative choice for online sales).
func (r Rules) EffectivePaymentMode() PaymentMode {
	if r.Payment == nil || r.Payment.Mode == nil {
		return PaymentFull
	}
	return *r.Payment.Mode
}

func deref[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// DefaultRules returns a sensible starting default policy for a new
// establishment (the example of spec section 9). Managers can change it.
func DefaultRules() Rules {
	return Rules{
		MinLeadTime:     &MinLeadTime{Days: Ptr(0), CutoffTime: Ptr("20:00")},
		MaxLeadTimeDays: Ptr(120),
		SameDayBooking:  &SameDayBooking{Enabled: Ptr(true), Until: Ptr("13:00"), OnlySlot: Ptr(OnlySlotPMAfter)},
		Duration: &Duration{
			AllowedFormulas: []Formula{FormulaHalfDay, FormulaDay, FormulaMultiDay, FormulaWeek, FormulaFortnight, FormulaMonth, FormulaSeason},
			MinDays:         Ptr(1),
			MaxDays:         Ptr(30),
		},
		Payment: &Payment{
			Mode:       Ptr(PaymentFull),
			BalanceDue: Ptr(BalanceDueOnArrival),
		},
		Cancellation: []CancellationTier{
			{UntilDaysBefore: 14, RefundPercent: 100, As: AsRefund},
			{UntilDaysBefore: 3, RefundPercent: 50, As: AsCredit},
			{UntilDaysBefore: 0, RefundPercent: 0},
		},
		Modification:                 &Modification{AllowedUntilDaysBefore: Ptr(2), Fee: Ptr(int64(0))},
		WeatherPolicy:                &WeatherPolicy{Enabled: Ptr(true), As: Ptr(AsCredit)},
		ApprovalRequired:             Ptr(false),
		MaxSpotsPerBooking:           Ptr(4),
		AllowSpotChangeDuringStay:    Ptr(false),
		CustomerCanReleaseSeasonDays: Ptr(true),
		SeasonReleaseCredit:          &SeasonReleaseCredit{Enabled: Ptr(false), Percent: Ptr(50)},
	}
}

// slotsAllowedBy reports whether the requested slots are compatible with an
// only_slot restriction ("AM" or "PM").
func slotsAllowedBy(s calendar.Slots, only string) bool {
	switch only {
	case OnlySlotAM:
		return s == calendar.SlotsAM
	case OnlySlotPM:
		return s == calendar.SlotsPM
	}
	return true
}
