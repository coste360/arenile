package policy

import (
	"slices"
	"sort"
)

// Target describes the booking a policy is being resolved for. Zones and
// Packs should contain every identifier of every zone and pack touched by the
// booking (both the UUID and the code), so that a scope written with either
// form matches.
type Target struct {
	Channel     Channel
	Zones       []string
	Packs       []string
	RatePeriods []string
}

// Resolution is the outcome of Resolve.
type Resolution struct {
	// PolicyID is the ID of the matched policy, empty when only the
	// establishment default applies.
	PolicyID string
	// Priority of the matched policy (0 when none matched).
	Priority int
	// Rules are the effective rules: matched policy over the default.
	Rules Rules
}

// Matches reports whether the scope applies to the target. Each non-empty
// dimension must intersect the target ("any" semantics); empty dimensions match
// everything.
func (s Scope) Matches(t Target) bool {
	if len(s.Channels) > 0 && !slices.Contains(s.Channels, t.Channel) {
		return false
	}
	return intersects(s.Zones, t.Zones) && intersects(s.Packs, t.Packs) && intersects(s.RatePeriods, t.RatePeriods)
}

// specificity is the number of constrained dimensions of the scope.
func (s Scope) specificity() int {
	n := 0
	for _, l := range []int{len(s.Channels), len(s.Zones), len(s.Packs), len(s.RatePeriods)} {
		if l > 0 {
			n++
		}
	}
	return n
}

func intersects(filter, values []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, v := range values {
		if slices.Contains(filter, v) {
			return true
		}
	}
	return false
}

// Resolve returns the effective rules for target: the matching policy with the
// highest priority (ties broken by higher specificity, then by lowest ID) is
// merged over def, the establishment default. Fields the policy does not
// specify are inherited from def.
func Resolve(policies []Policy, def Rules, t Target) Resolution {
	var matching []Policy
	for _, p := range policies {
		if p.Scope.Matches(t) {
			matching = append(matching, p)
		}
	}
	if len(matching) == 0 {
		return Resolution{Rules: def.Clone()}
	}
	sort.SliceStable(matching, func(i, j int) bool {
		a, b := matching[i], matching[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if sa, sb := a.Scope.specificity(), b.Scope.specificity(); sa != sb {
			return sa > sb
		}
		return a.ID < b.ID
	})
	best := matching[0]
	return Resolution{PolicyID: best.ID, Priority: best.Priority, Rules: Merge(def, best.Rules)}
}

// Merge returns base overlaid with override: every field set in override wins,
// unset fields come from base. Nested objects are merged field by field; the
// cancellation schedule and the allowed formulas are replaced as a whole.
// Neither input is modified and the result shares no memory with them.
func Merge(base, override Rules) Rules {
	out := base.Clone()
	o := override.Clone()
	if o.MinLeadTime != nil {
		if out.MinLeadTime == nil {
			out.MinLeadTime = &MinLeadTime{}
		}
		setIf(&out.MinLeadTime.Days, o.MinLeadTime.Days)
		setIf(&out.MinLeadTime.CutoffTime, o.MinLeadTime.CutoffTime)
	}
	setIf(&out.MaxLeadTimeDays, o.MaxLeadTimeDays)
	if o.SameDayBooking != nil {
		if out.SameDayBooking == nil {
			out.SameDayBooking = &SameDayBooking{}
		}
		setIf(&out.SameDayBooking.Enabled, o.SameDayBooking.Enabled)
		setIf(&out.SameDayBooking.Until, o.SameDayBooking.Until)
		setIf(&out.SameDayBooking.OnlySlot, o.SameDayBooking.OnlySlot)
	}
	if o.Duration != nil {
		if out.Duration == nil {
			out.Duration = &Duration{}
		}
		if o.Duration.AllowedFormulas != nil {
			out.Duration.AllowedFormulas = o.Duration.AllowedFormulas
		}
		setIf(&out.Duration.MinDays, o.Duration.MinDays)
		setIf(&out.Duration.MaxDays, o.Duration.MaxDays)
		setIf(&out.Duration.FixedStartWeekday, o.Duration.FixedStartWeekday)
	}
	if o.Payment != nil {
		if out.Payment == nil {
			out.Payment = &Payment{}
		}
		setIf(&out.Payment.Mode, o.Payment.Mode)
		setIf(&out.Payment.BalanceDue, o.Payment.BalanceDue)
		if o.Payment.Deposit != nil {
			if out.Payment.Deposit == nil {
				out.Payment.Deposit = &DepositRule{}
			}
			setIf(&out.Payment.Deposit.Type, o.Payment.Deposit.Type)
			setIf(&out.Payment.Deposit.Value, o.Payment.Deposit.Value)
		}
	}
	if o.Cancellation != nil {
		out.Cancellation = o.Cancellation
	}
	if o.Modification != nil {
		if out.Modification == nil {
			out.Modification = &Modification{}
		}
		setIf(&out.Modification.AllowedUntilDaysBefore, o.Modification.AllowedUntilDaysBefore)
		setIf(&out.Modification.Fee, o.Modification.Fee)
	}
	if o.WeatherPolicy != nil {
		if out.WeatherPolicy == nil {
			out.WeatherPolicy = &WeatherPolicy{}
		}
		setIf(&out.WeatherPolicy.Enabled, o.WeatherPolicy.Enabled)
		setIf(&out.WeatherPolicy.As, o.WeatherPolicy.As)
	}
	setIf(&out.ApprovalRequired, o.ApprovalRequired)
	setIf(&out.MaxSpotsPerBooking, o.MaxSpotsPerBooking)
	setIf(&out.AllowSpotChangeDuringStay, o.AllowSpotChangeDuringStay)
	setIf(&out.CustomerCanReleaseSeasonDays, o.CustomerCanReleaseSeasonDays)
	if o.SeasonReleaseCredit != nil {
		if out.SeasonReleaseCredit == nil {
			out.SeasonReleaseCredit = &SeasonReleaseCredit{}
		}
		setIf(&out.SeasonReleaseCredit.Enabled, o.SeasonReleaseCredit.Enabled)
		setIf(&out.SeasonReleaseCredit.Percent, o.SeasonReleaseCredit.Percent)
	}
	setIf(&out.CreditValidityDays, o.CreditValidityDays)
	return out
}

func setIf[T any](dst **T, src *T) {
	if src != nil {
		v := *src
		*dst = &v
	}
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// Clone returns a deep copy of r.
func (r Rules) Clone() Rules {
	out := Rules{
		MaxLeadTimeDays:              clonePtr(r.MaxLeadTimeDays),
		ApprovalRequired:             clonePtr(r.ApprovalRequired),
		MaxSpotsPerBooking:           clonePtr(r.MaxSpotsPerBooking),
		AllowSpotChangeDuringStay:    clonePtr(r.AllowSpotChangeDuringStay),
		CustomerCanReleaseSeasonDays: clonePtr(r.CustomerCanReleaseSeasonDays),
	}
	if r.MinLeadTime != nil {
		out.MinLeadTime = &MinLeadTime{Days: clonePtr(r.MinLeadTime.Days), CutoffTime: clonePtr(r.MinLeadTime.CutoffTime)}
	}
	if r.SameDayBooking != nil {
		out.SameDayBooking = &SameDayBooking{
			Enabled:  clonePtr(r.SameDayBooking.Enabled),
			Until:    clonePtr(r.SameDayBooking.Until),
			OnlySlot: clonePtr(r.SameDayBooking.OnlySlot),
		}
	}
	if r.Duration != nil {
		out.Duration = &Duration{
			MinDays:           clonePtr(r.Duration.MinDays),
			MaxDays:           clonePtr(r.Duration.MaxDays),
			FixedStartWeekday: clonePtr(r.Duration.FixedStartWeekday),
		}
		if r.Duration.AllowedFormulas != nil {
			out.Duration.AllowedFormulas = slices.Clone(r.Duration.AllowedFormulas)
		}
	}
	if r.Payment != nil {
		out.Payment = &Payment{Mode: clonePtr(r.Payment.Mode), BalanceDue: clonePtr(r.Payment.BalanceDue)}
		if r.Payment.Deposit != nil {
			out.Payment.Deposit = &DepositRule{Type: clonePtr(r.Payment.Deposit.Type), Value: clonePtr(r.Payment.Deposit.Value)}
		}
	}
	if r.Cancellation != nil {
		out.Cancellation = slices.Clone(r.Cancellation)
	}
	if r.Modification != nil {
		out.Modification = &Modification{
			AllowedUntilDaysBefore: clonePtr(r.Modification.AllowedUntilDaysBefore),
			Fee:                    clonePtr(r.Modification.Fee),
		}
	}
	if r.WeatherPolicy != nil {
		out.WeatherPolicy = &WeatherPolicy{Enabled: clonePtr(r.WeatherPolicy.Enabled), As: clonePtr(r.WeatherPolicy.As)}
	}
	if r.SeasonReleaseCredit != nil {
		out.SeasonReleaseCredit = &SeasonReleaseCredit{
			Enabled: clonePtr(r.SeasonReleaseCredit.Enabled),
			Percent: clonePtr(r.SeasonReleaseCredit.Percent),
		}
	}
	return out
}
