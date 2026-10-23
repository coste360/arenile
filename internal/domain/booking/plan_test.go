package booking

import (
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
)

func TestDecideInitialState_Table(t *testing.T) {
	const ttl, appr = 10 * time.Minute, 72 * time.Hour
	cases := []struct {
		name string
		in   StateRequest
		want InitialState
		err  bool
	}{
		{"default is confirmed", StateRequest{}, InitialState{Status: StatusConfirmed, Kind: inventory.KindBooking, Event: eventConfirmed}, false},
		{"explicit confirmed", StateRequest{Requested: StatusConfirmed}, InitialState{Status: StatusConfirmed, Kind: inventory.KindBooking, Event: eventConfirmed}, false},
		{"approval required", StateRequest{ApprovalRequired: true}, InitialState{Status: StatusAwaitingApproval, Kind: inventory.KindHold, HoldTTL: appr, Event: eventApprovalRequested}, false},
		{"held", StateRequest{Requested: StatusHeld}, InitialState{Status: StatusHeld, Kind: inventory.KindHold, HoldTTL: ttl, Event: eventHeld}, false},
		{"held ignores approval", StateRequest{Requested: StatusHeld, ApprovalRequired: true}, InitialState{Status: StatusHeld, Kind: inventory.KindHold, HoldTTL: ttl, Event: eventHeld}, false},
		{"draft occupies nothing", StateRequest{Requested: StatusDraft}, InitialState{Status: StatusDraft}, false},
		{"season pass", StateRequest{SeasonPass: true}, InitialState{Status: StatusConfirmed, Kind: inventory.KindSeason, Event: eventConfirmed}, false},
		{"explicit season kind", StateRequest{Kind: inventory.KindSeason}, InitialState{Status: StatusConfirmed, Kind: inventory.KindSeason, Event: eventConfirmed}, false},
		{"cancelled is not initial", StateRequest{Requested: StatusCancelled}, InitialState{}, true},
		{"checked_in is not initial", StateRequest{Requested: StatusCheckedIn}, InitialState{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.HoldTTL, tc.in.ApprovalTTL = ttl, appr
			got, err := DecideInitialState(tc.in)
			if tc.err {
				if !errors.Is(err, ErrInvalidInitialStatus) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestBookingCode_Table(t *testing.T) {
	prefixes := []struct{ slug, want string }{
		{"lido-delfino", "LD"}, {"bagni-73-sole", "B7"}, {"aurora", "AU"}, {"x", "AR"}, {"", "AR"}, {"lido_sirena", "LS"},
	}
	for _, tc := range prefixes {
		if got := CodePrefix(tc.slug); got != tc.want {
			t.Errorf("CodePrefix(%q) = %q, want %q", tc.slug, got, tc.want)
		}
	}
	series := CodeSeries("LD", 2027)
	next := []struct{ last, want string }{
		{"", "LD-2027-000001"},
		{"LD-2027-000123", "LD-2027-000124"},
		{"LD-2026-000999", "LD-2027-000001"},
		{"LD-2027-garbage", "LD-2027-000001"},
		{"LD-2027-999999", "LD-2027-1000000"},
	}
	for _, tc := range next {
		if got := NextCode(series, tc.last); got != tc.want {
			t.Errorf("NextCode(%q) = %q, want %q", tc.last, got, tc.want)
		}
	}
}

func TestPricingLines_QuantityAndTotals(t *testing.T) {
	lines := []PricedLine{
		{PackID: "p1", ZoneID: "z1", SpotID: "s1", Quantity: 1, Extras: []ExtraInput{{ExtraID: "x", Quantity: 2}}},
		{PackID: "p2", ZoneID: "z2", Quantity: 3, Extras: []ExtraInput{{ExtraID: "y", Quantity: 1}}},
	}
	engine, owner := PricingLines(lines)
	if len(engine) != 4 || len(owner) != 4 {
		t.Fatalf("engine lines %d", len(engine))
	}
	if owner[0] != 0 || owner[1] != 1 || owner[3] != 1 {
		t.Fatalf("owners %v", owner)
	}
	if len(engine[1].Extras) != 1 || len(engine[2].Extras) != 0 {
		t.Fatalf("extras must go to the first unit only: %+v", engine)
	}
	bd := pricing.PriceBreakdown{Lines: []pricing.LineBreakdown{
		{Total: 1000, Extras: []pricing.ExtraBreakdown{{Code: "x", Total: 400}}},
		{Total: 700, Extras: []pricing.ExtraBreakdown{{Code: "y", Total: 100}}},
		{Total: 600},
		{Total: 600},
	}}
	totals, extras := LineTotals(bd, owner, len(lines))
	if totals[0] != 1000 || totals[1] != 1900 {
		t.Fatalf("totals %v", totals)
	}
	if len(extras[0]) != 1 || extras[0][0] != 400 || len(extras[1]) != 1 || extras[1][0] != 100 {
		t.Fatalf("extra totals %v", extras)
	}
}

func TestComputeRefund_Table(t *testing.T) {
	rules := policy.Rules{
		Cancellation: []policy.CancellationTier{
			{UntilDaysBefore: 14, RefundPercent: 100, As: policy.AsRefund},
			{UntilDaysBefore: 3, RefundPercent: 50, As: policy.AsCredit},
			{UntilDaysBefore: 0, RefundPercent: 0},
		},
		WeatherPolicy: &policy.WeatherPolicy{Enabled: policy.Ptr(true), As: policy.Ptr(policy.AsCredit)},
	}
	override := money.Cents(1234)
	tooMuch := money.Cents(99999)
	cases := []struct {
		name    string
		paid    money.Cents
		days    int
		req     CancelRequest
		amount  money.Cents
		as      Settlement
		percent int
	}{
		{"full refund far ahead", 6000, 20, CancelRequest{}, 6000, SettleRefund, 100},
		{"exactly 14 days", 6000, 14, CancelRequest{}, 6000, SettleRefund, 100},
		{"credit tier", 6000, 5, CancelRequest{}, 3000, SettleCredit, 50},
		{"last tier zero", 6000, 1, CancelRequest{}, 0, SettleRefund, 0},
		{"after start nothing", 6000, -1, CancelRequest{}, 0, SettleRefund, 0},
		{"nothing paid shows the tier", 0, 20, CancelRequest{}, 0, SettleRefund, 100},
		{"force credit", 6000, 20, CancelRequest{Settlement: SettleCredit}, 6000, SettleCredit, 100},
		{"force refund on credit tier", 6000, 5, CancelRequest{Settlement: SettleRefund}, 3000, SettleRefund, 50},
		{"none", 6000, 20, CancelRequest{Settlement: SettleNone}, 0, SettleNone, 0},
		{"override", 6000, 1, CancelRequest{Override: &override}, 1234, SettleRefund, 0},
		{"override capped at paid (INV-24)", 6000, 20, CancelRequest{Override: &tooMuch}, 6000, SettleRefund, 100},
		{"weather credit", 6000, 0, CancelRequest{Weather: true}, 6000, SettleCredit, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeRefund(rules, tc.paid, tc.days, tc.req)
			if got.Amount != tc.amount || got.As != tc.as || got.RefundPercent != tc.percent {
				t.Fatalf("got %+v", got)
			}
			if got.Amount > got.PaidAmount {
				t.Fatalf("refund %d above paid %d", got.Amount, got.PaidAmount)
			}
		})
	}
}

// TestBookingTransitions_ServiceActions checks that every service action
// uses an edge of the state machine (INV-17): the target of each action is
// reachable only from the statuses the API documents.
func TestBookingTransitions_ServiceActions(t *testing.T) {
	actions := map[string]struct {
		to   Status
		from []Status
	}{
		"confirm":  {StatusConfirmed, []Status{StatusHeld, StatusPendingPayment, StatusAwaitingApproval}},
		"reject":   {StatusRejected, []Status{StatusAwaitingApproval}},
		"check-in": {StatusCheckedIn, []Status{StatusConfirmed}},
		"no-show":  {StatusNoShow, []Status{StatusConfirmed}},
		"cancel":   {StatusCancelled, []Status{StatusDraft, StatusHeld, StatusAwaitingApproval, StatusConfirmed}},
		"expire":   {StatusExpired, []Status{StatusHeld, StatusPendingPayment, StatusAwaitingApproval}},
		"complete": {StatusCompleted, []Status{StatusConfirmed, StatusCheckedIn}},
	}
	for name, a := range actions {
		for _, from := range AllStatuses() {
			allowed := false
			for _, f := range a.from {
				allowed = allowed || f == from
			}
			err := Transition(from, a.to)
			if allowed != (err == nil) {
				t.Errorf("%s from %s: err = %v, allowed = %v", name, from, err, allowed)
			}
			var te *TransitionError
			if err != nil && (!errors.As(err, &te) || te.From != from) {
				t.Errorf("%s from %s: untyped error %v", name, from, err)
			}
		}
	}
}

func TestModifiable_Table(t *testing.T) {
	for _, st := range AllStatuses() {
		want := !st.IsTerminal()
		if got := modifiable(st); got != want {
			t.Errorf("modifiable(%s) = %v, want %v", st, got, want)
		}
	}
}
