package policy

import (
	"slices"
	"strconv"
	"time"

	"github.com/coste360/arenile/internal/calendar"
)

// Request is the part of a booking request the policy rules look at.
type Request struct {
	StartsOn  calendar.Date
	EndsOn    calendar.Date // inclusive
	Slots     calendar.Slots
	Formula   Formula
	SpotCount int // number of spots/packs booked (booking lines)
}

// Validation is the outcome of Validate.
type Validation struct {
	// Violations lists every unmet rule, in a stable order.
	Violations []Violation
	// ApprovalRequired is true when the booking may proceed but must be
	// approved by the manager (status awaiting_approval). It is a flag, not a
	// violation.
	ApprovalRequired bool
}

// OK reports whether there are no violations.
func (v Validation) OK() bool { return len(v.Violations) == 0 }

// Err returns a *ViolationError (errors.Is ErrPolicyViolation) or nil.
func (v Validation) Err() error {
	if v.OK() {
		return nil
	}
	return &ViolationError{Violations: slices.Clone(v.Violations)}
}

// Validate checks req against the effective rules at instant now. Dates and
// times of day are evaluated in loc, the establishment timezone; loc must not be
// nil (see mustLocation).
// Malformed time strings in the rules (rejected by CheckConfig on save) make the
// corresponding check a no-op rather than blocking sales.
//
// Lead-time semantics (lead = start date - today, in days):
//   - lead < 0, or lead < min_lead_time.days: lead_time_too_short;
//   - lead == 0 (same day, allowed by min days 0): same_day_booking applies;
//   - lead >= 1: the booking must be placed before min_lead_time.cutoff_time on
//     day start - max(days, 1), otherwise cutoff_passed;
//   - lead > max_lead_time_days: lead_time_too_long.
func Validate(req Request, r Rules, now time.Time, loc *time.Location) Validation {
	mustLocation(loc)
	local := now.In(loc)
	today := calendar.In(now, loc)
	minuteOfDay := local.Hour()*60 + local.Minute()

	var out Validation
	add := func(c Code, kv ...string) {
		v := Violation{Code: c}
		if len(kv) > 0 {
			v.Params = make(map[string]string, len(kv)/2)
			for i := 0; i+1 < len(kv); i += 2 {
				v.Params[kv[i]] = kv[i+1]
			}
		}
		out.Violations = append(out.Violations, v)
	}
	itoa := strconv.Itoa

	lead := today.DaysUntil(req.StartsOn)
	minDays := 0
	var cutoff *string
	if m := r.MinLeadTime; m != nil {
		minDays = deref(m.Days, 0)
		cutoff = m.CutoffTime
	}

	switch {
	case lead < 0 || lead < minDays:
		add(CodeLeadTimeTooShort, "lead_days", itoa(lead), "min_days", itoa(minDays))
	case lead == 0:
		if reason, closed := sameDayClosed(r.SameDayBooking, req.Slots, minuteOfDay); closed {
			add(CodeSameDayClosed, "reason", reason)
		}
	default:
		if cutoff != nil {
			if c, err := parseClock(*cutoff); err == nil {
				deadlineDay := req.StartsOn.AddDays(-max(minDays, 1))
				if today.Equal(deadlineDay) && minuteOfDay >= c {
					add(CodeCutoffPassed, "cutoff_time", *cutoff, "deadline_day", deadlineDay.String())
				}
			}
		}
	}
	if r.MaxLeadTimeDays != nil && lead > *r.MaxLeadTimeDays {
		add(CodeLeadTimeTooLong, "lead_days", itoa(lead), "max_days", itoa(*r.MaxLeadTimeDays))
	}

	if d := r.Duration; d != nil {
		if len(d.AllowedFormulas) > 0 && !slices.Contains(d.AllowedFormulas, req.Formula) {
			add(CodeFormulaNotAllowed, "formula", string(req.Formula))
		}
		days := req.StartsOn.DaysUntil(req.EndsOn) + 1
		if req.EndsOn.Before(req.StartsOn) {
			add(CodeDurationTooShort, "days", itoa(days), "min_days", itoa(max(deref(d.MinDays, 1), 1)))
		} else if !req.Formula.HasFixedLength() {
			if d.MinDays != nil && days < *d.MinDays {
				add(CodeDurationTooShort, "days", itoa(days), "min_days", itoa(*d.MinDays))
			}
			if d.MaxDays != nil && days > *d.MaxDays {
				add(CodeDurationTooLong, "days", itoa(days), "max_days", itoa(*d.MaxDays))
			}
		}
		if w := deref(d.FixedStartWeekday, 0); w >= 1 && w <= 7 && req.StartsOn.ISOWeekday() != w {
			add(CodeStartWeekdayNotAllowed, "weekday", itoa(w), "start_weekday", itoa(req.StartsOn.ISOWeekday()))
		}
	}

	if r.MaxSpotsPerBooking != nil && req.SpotCount > *r.MaxSpotsPerBooking {
		add(CodeTooManySpots, "spots", itoa(req.SpotCount), "max_spots", itoa(*r.MaxSpotsPerBooking))
	}

	out.ApprovalRequired = r.RequiresApproval()
	return out
}

// sameDayClosed applies same_day_booking. A nil config means no restriction.
func sameDayClosed(s *SameDayBooking, slots calendar.Slots, minuteOfDay int) (reason string, closed bool) {
	if s == nil {
		return "", false
	}
	if !deref(s.Enabled, true) {
		return "disabled", true
	}
	only := deref(s.OnlySlot, "")
	if !slotsAllowedBy(slots, only) {
		return "slot", true
	}
	if s.Until != nil {
		until, err := parseClock(*s.Until)
		if err == nil && minuteOfDay >= until {
			if only == OnlySlotPMAfter && slots == calendar.SlotsPM {
				return "", false
			}
			return "time", true
		}
	}
	return "", false
}

// mustLocation panics on a nil location. Calendar dates are always in the
// establishment timezone: silently falling back to UTC would shift lead-time
// cut-offs and cancellation tiers by one or two hours around midnight
// (Europe/Rome is UTC+1/+2), so a missing timezone is a programming error.
func mustLocation(loc *time.Location) {
	if loc == nil {
		panic("policy: nil *time.Location: pass the establishment timezone")
	}
}
