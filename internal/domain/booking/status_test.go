package booking

import (
	"errors"
	"testing"
)

func TestBookingTransitions_OnlyAllowed(t *testing.T) {
	allowed := map[[2]Status]bool{
		{StatusDraft, StatusHeld}:                      true,
		{StatusDraft, StatusCancelled}:                 true,
		{StatusHeld, StatusPendingPayment}:             true,
		{StatusHeld, StatusConfirmed}:                  true,
		{StatusHeld, StatusAwaitingApproval}:           true,
		{StatusHeld, StatusExpired}:                    true,
		{StatusHeld, StatusCancelled}:                  true,
		{StatusPendingPayment, StatusConfirmed}:        true,
		{StatusPendingPayment, StatusAwaitingApproval}: true,
		{StatusPendingPayment, StatusExpired}:          true,
		{StatusAwaitingApproval, StatusConfirmed}:      true,
		{StatusAwaitingApproval, StatusRejected}:       true,
		{StatusAwaitingApproval, StatusExpired}:        true,
		{StatusAwaitingApproval, StatusCancelled}:      true,
		{StatusConfirmed, StatusCheckedIn}:             true,
		{StatusConfirmed, StatusCancelled}:             true,
		{StatusConfirmed, StatusNoShow}:                true,
		{StatusConfirmed, StatusCompleted}:             true,
		{StatusCheckedIn, StatusCompleted}:             true,
	}
	all := append(AllStatuses(), Status("bogus"), Status(""))
	n := 0
	for _, from := range all {
		for _, to := range all {
			want := allowed[[2]Status{from, to}]
			name := string(from) + "->" + string(to)
			t.Run(name, func(t *testing.T) {
				if got := CanTransition(from, to); got != want {
					t.Fatalf("CanTransition = %v, want %v", got, want)
				}
				err := Transition(from, to)
				if want {
					if err != nil {
						t.Fatalf("Transition err = %v", err)
					}
					return
				}
				if !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("Transition err = %v, want ErrInvalidTransition", err)
				}
				var te *TransitionError
				if !errors.As(err, &te) || te.From != from || te.To != to {
					t.Fatalf("TransitionError = %+v", te)
				}
			})
			n++
		}
	}
	if n != 13*13 {
		t.Fatalf("checked %d pairs", n)
	}
}

func TestStatus_Properties(t *testing.T) {
	tests := []struct {
		s                        Status
		terminal, occupies, hold bool
	}{
		{StatusDraft, false, false, false},
		{StatusHeld, false, true, true},
		{StatusPendingPayment, false, true, true},
		{StatusAwaitingApproval, false, true, true},
		{StatusConfirmed, false, true, false},
		{StatusCheckedIn, false, true, false},
		{StatusCompleted, true, false, false},
		{StatusCancelled, true, false, false},
		{StatusNoShow, true, false, false},
		{StatusExpired, true, false, false},
		{StatusRejected, true, false, false},
		{Status("bogus"), false, false, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.s), func(t *testing.T) {
			if tc.s.IsTerminal() != tc.terminal || tc.s.OccupiesResources() != tc.occupies || tc.s.IsHold() != tc.hold {
				t.Fatalf("terminal=%v occupies=%v hold=%v", tc.s.IsTerminal(), tc.s.OccupiesResources(), tc.s.IsHold())
			}
			if tc.s.IsTerminal() && len(tc.s.NextStatuses()) != 0 {
				t.Fatal("terminal status has successors")
			}
		})
	}
}

func TestParseStatus_Scenarios(t *testing.T) {
	for _, s := range AllStatuses() {
		got, err := ParseStatus(string(s))
		if err != nil || got != s {
			t.Fatalf("ParseStatus(%q) = %q, %v", s, got, err)
		}
	}
	for _, bad := range []string{"", "Confirmed", "paid"} {
		if _, err := ParseStatus(bad); !errors.Is(err, ErrUnknownStatus) {
			t.Fatalf("ParseStatus(%q) err = %v", bad, err)
		}
	}
}

func TestNextStatuses_IsCopy(t *testing.T) {
	n := StatusHeld.NextStatuses()
	n[0] = StatusCompleted
	if CanTransition(StatusHeld, StatusCompleted) {
		t.Fatal("NextStatuses exposes the internal table")
	}
}

func TestStatus_EveryNonTerminalReachableFromDraft(t *testing.T) {
	seen := map[Status]bool{StatusDraft: true}
	queue := []Status{StatusDraft}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		for _, n := range s.NextStatuses() {
			if !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	for _, s := range AllStatuses() {
		if !seen[s] {
			t.Fatalf("%s unreachable from draft", s)
		}
	}
}
