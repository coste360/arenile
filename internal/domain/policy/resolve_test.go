package policy

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestScopeMatches_Table(t *testing.T) {
	target := Target{
		Channel:     ChannelOnline,
		Zones:       []string{"z-front", "row1"},
		Packs:       []string{"p-std", "standard", "p-gaz", "gazebo"},
		RatePeriods: []string{"high", "peak"},
	}
	tests := []struct {
		name  string
		scope Scope
		want  bool
	}{
		{"empty scope matches all", Scope{}, true},
		{"channel match", Scope{Channels: []Channel{ChannelOnline}}, true},
		{"channel mismatch", Scope{Channels: []Channel{ChannelDesk}}, false},
		{"channel one of many", Scope{Channels: []Channel{ChannelDesk, ChannelOnline}}, true},
		{"zone by id", Scope{Zones: []string{"z-front"}}, true},
		{"zone by code", Scope{Zones: []string{"row1"}}, true},
		{"zone mismatch", Scope{Zones: []string{"pool"}}, false},
		{"pack any of booking", Scope{Packs: []string{"gazebo"}}, true},
		{"pack mismatch", Scope{Packs: []string{"tent"}}, false},
		{"rate period one of interval", Scope{RatePeriods: []string{"peak"}}, true},
		{"rate period mismatch", Scope{RatePeriods: []string{"low"}}, false},
		{"all dimensions match", Scope{Channels: []Channel{ChannelOnline}, Zones: []string{"row1"}, Packs: []string{"standard"}, RatePeriods: []string{"high"}}, true},
		{"one dimension fails", Scope{Channels: []Channel{ChannelOnline}, Zones: []string{"row1"}, Packs: []string{"tent"}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.scope.Matches(target); got != tc.want {
				t.Fatalf("Matches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolve_Table(t *testing.T) {
	def := DefaultRules()
	online := Policy{ID: "online", Priority: 10, Scope: Scope{Channels: []Channel{ChannelOnline}}, Rules: Rules{Payment: &Payment{Mode: Ptr(PaymentFull)}}}
	desk := Policy{ID: "desk", Priority: 10, Scope: Scope{Channels: []Channel{ChannelDesk}}, Rules: Rules{Payment: &Payment{Mode: Ptr(PaymentNone)}}}
	gazebo := Policy{ID: "gazebo", Priority: 20, Scope: Scope{Packs: []string{"gazebo"}}, Rules: Rules{ApprovalRequired: Ptr(true)}}
	peak := Policy{
		ID: "peak", Priority: 30, Scope: Scope{Channels: []Channel{ChannelOnline}, RatePeriods: []string{"peak"}},
		Rules: Rules{Duration: &Duration{AllowedFormulas: []Formula{FormulaWeek}, FixedStartWeekday: Ptr(6)}},
	}
	tieA := Policy{ID: "b-generic", Priority: 5, Scope: Scope{}, Rules: Rules{MaxSpotsPerBooking: Ptr(1)}}
	tieB := Policy{ID: "a-specific", Priority: 5, Scope: Scope{Channels: []Channel{ChannelOnline}}, Rules: Rules{MaxSpotsPerBooking: Ptr(2)}}
	tieC := Policy{ID: "a-generic", Priority: 5, Scope: Scope{}, Rules: Rules{MaxSpotsPerBooking: Ptr(3)}}
	all := []Policy{online, desk, gazebo, peak}

	tests := []struct {
		name     string
		policies []Policy
		target   Target
		wantID   string
		check    func(t *testing.T, r Rules)
	}{
		{"no policies uses default", nil, Target{Channel: ChannelOnline}, "", func(t *testing.T, r Rules) {
			if !reflect.DeepEqual(r, def) {
				t.Fatalf("rules differ from default")
			}
		}},
		{"desk channel", all, Target{Channel: ChannelDesk}, "desk", func(t *testing.T, r Rules) {
			if r.EffectivePaymentMode() != PaymentNone {
				t.Fatalf("mode = %s", r.EffectivePaymentMode())
			}
			if *r.MaxSpotsPerBooking != 4 {
				t.Fatalf("max spots not inherited")
			}
		}},
		{"gazebo beats channel by priority", all, Target{Channel: ChannelDesk, Packs: []string{"gazebo"}}, "gazebo", func(t *testing.T, r Rules) {
			if !r.RequiresApproval() {
				t.Fatalf("approval not required")
			}
			// payment inherited from the default, not from the desk policy
			if r.EffectivePaymentMode() != PaymentFull {
				t.Fatalf("mode = %s", r.EffectivePaymentMode())
			}
		}},
		{"peak online week only, duration merged", all, Target{Channel: ChannelOnline, RatePeriods: []string{"high", "peak"}}, "peak", func(t *testing.T, r Rules) {
			d := r.Duration
			if len(d.AllowedFormulas) != 1 || d.AllowedFormulas[0] != FormulaWeek || *d.FixedStartWeekday != 6 {
				t.Fatalf("duration = %+v", d)
			}
			if *d.MinDays != 1 || *d.MaxDays != 30 {
				t.Fatalf("min/max not inherited: %+v", d)
			}
		}},
		{"peak does not apply at desk", all, Target{Channel: ChannelDesk, RatePeriods: []string{"peak"}}, "desk", nil},
		{"no match uses default", []Policy{gazebo, peak}, Target{Channel: ChannelDesk}, "", nil},
		{"tie broken by specificity", []Policy{tieA, tieB}, Target{Channel: ChannelOnline}, "a-specific", func(t *testing.T, r Rules) {
			if *r.MaxSpotsPerBooking != 2 {
				t.Fatalf("max = %d", *r.MaxSpotsPerBooking)
			}
		}},
		{"tie broken by id", []Policy{tieA, tieC}, Target{Channel: ChannelOnline}, "a-generic", nil},
		{"tie order independent", []Policy{tieC, tieA}, Target{Channel: ChannelOnline}, "a-generic", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Resolve(tc.policies, def, tc.target)
			if res.PolicyID != tc.wantID {
				t.Fatalf("policy = %q, want %q", res.PolicyID, tc.wantID)
			}
			if tc.check != nil {
				tc.check(t, res.Rules)
			}
		})
	}
}

func TestMerge_Table(t *testing.T) {
	tests := []struct {
		name     string
		base     Rules
		override Rules
		check    func(t *testing.T, r Rules)
	}{
		{"override scalar", Rules{MaxLeadTimeDays: Ptr(10)}, Rules{MaxLeadTimeDays: Ptr(20)}, func(t *testing.T, r Rules) {
			if *r.MaxLeadTimeDays != 20 {
				t.Fatal(*r.MaxLeadTimeDays)
			}
		}},
		{"inherit scalar", Rules{MaxLeadTimeDays: Ptr(10)}, Rules{}, func(t *testing.T, r Rules) {
			if *r.MaxLeadTimeDays != 10 {
				t.Fatal(*r.MaxLeadTimeDays)
			}
		}},
		{"nested partial", Rules{MinLeadTime: &MinLeadTime{Days: Ptr(1), CutoffTime: Ptr("20:00")}}, Rules{MinLeadTime: &MinLeadTime{CutoffTime: Ptr("18:00")}}, func(t *testing.T, r Rules) {
			if *r.MinLeadTime.Days != 1 || *r.MinLeadTime.CutoffTime != "18:00" {
				t.Fatalf("%+v", r.MinLeadTime)
			}
		}},
		{"nested into empty base", Rules{}, Rules{Payment: &Payment{Deposit: &DepositRule{Value: Ptr(int64(30))}}}, func(t *testing.T, r Rules) {
			if *r.Payment.Deposit.Value != 30 || r.Payment.Mode != nil {
				t.Fatalf("%+v", r.Payment)
			}
		}},
		{"deposit partial", Rules{Payment: &Payment{Mode: Ptr(PaymentDeposit), Deposit: &DepositRule{Type: Ptr(DepositPercent), Value: Ptr(int64(30))}}}, Rules{Payment: &Payment{Deposit: &DepositRule{Value: Ptr(int64(50))}}}, func(t *testing.T, r Rules) {
			if *r.Payment.Deposit.Type != DepositPercent || *r.Payment.Deposit.Value != 50 || *r.Payment.Mode != PaymentDeposit {
				t.Fatalf("%+v", r.Payment.Deposit)
			}
		}},
		{"cancellation replaced", Rules{Cancellation: []CancellationTier{{UntilDaysBefore: 7, RefundPercent: 100}}}, Rules{Cancellation: []CancellationTier{{UntilDaysBefore: 1, RefundPercent: 10}}}, func(t *testing.T, r Rules) {
			if len(r.Cancellation) != 1 || r.Cancellation[0].UntilDaysBefore != 1 {
				t.Fatalf("%+v", r.Cancellation)
			}
		}},
		{"empty cancellation overrides", Rules{Cancellation: []CancellationTier{{UntilDaysBefore: 7, RefundPercent: 100}}}, Rules{Cancellation: []CancellationTier{}}, func(t *testing.T, r Rules) {
			if r.Cancellation == nil || len(r.Cancellation) != 0 {
				t.Fatalf("%+v", r.Cancellation)
			}
		}},
		{"bool false overrides true", Rules{ApprovalRequired: Ptr(true)}, Rules{ApprovalRequired: Ptr(false)}, func(t *testing.T, r Rules) {
			if r.RequiresApproval() {
				t.Fatal("still required")
			}
		}},
		{"all nested objects", Rules{}, DefaultRules(), func(t *testing.T, r Rules) {
			if !reflect.DeepEqual(r, DefaultRules()) {
				t.Fatal("merge over empty base is not identity")
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, Merge(tc.base, tc.override))
		})
	}
}

func TestMerge_DoesNotAliasInputs(t *testing.T) {
	base := DefaultRules()
	over := Rules{Duration: &Duration{AllowedFormulas: []Formula{FormulaWeek}}}
	out := Merge(base, over)
	*out.MaxSpotsPerBooking = 99
	out.Duration.AllowedFormulas[0] = FormulaSeason
	out.Cancellation[0].RefundPercent = 1
	if *base.MaxSpotsPerBooking != 4 || over.Duration.AllowedFormulas[0] != FormulaWeek || base.Cancellation[0].RefundPercent != 100 {
		t.Fatal("Merge result aliases its inputs")
	}
}

func TestRules_JSONRoundTrip(t *testing.T) {
	src := `{
  "min_lead_time":   {"days": 0, "cutoff_time": "20:00"},
  "max_lead_time_days": 120,
  "same_day_booking": {"enabled": true, "until": "13:00", "only_slot": "PM_after"},
  "duration": {"allowed_formulas": ["day", "multi_day", "week"], "min_days": 1, "max_days": 30, "fixed_start_weekday": null},
  "payment": {"mode": "deposit", "deposit": {"type": "percent", "value": 30}, "balance_due": "days_before:7"},
  "cancellation": [
    {"until_days_before": 14, "refund_percent": 100, "as": "refund"},
    {"until_days_before": 3,  "refund_percent": 50,  "as": "credit"},
    {"until_days_before": 0,  "refund_percent": 0}
  ],
  "modification": {"allowed_until_days_before": 2, "fee": 0},
  "weather_policy": {"enabled": true, "as": "credit"},
  "approval_required": false,
  "max_spots_per_booking": 4,
  "allow_spot_change_during_stay": false,
  "customer_can_release_season_days": true,
  "season_release_credit": {"enabled": false, "percent": 50}
}`
	var r Rules
	if err := json.Unmarshal([]byte(src), &r); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckConfig(); err != nil {
		t.Fatalf("spec example invalid: %v", err)
	}
	if r.Duration.FixedStartWeekday != nil || *r.Payment.Deposit.Value != 30 || r.Cancellation[2].As != "" || *r.SameDayBooking.OnlySlot != OnlySlotPMAfter {
		t.Fatalf("decoded = %+v", r)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back Rules
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r, back) {
		t.Fatalf("round trip differs:\n%s", b)
	}
}
