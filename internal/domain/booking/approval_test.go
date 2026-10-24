package booking

import "testing"

// TestDecideApproval_Rules covers rules R1-R5 of approval.go (ADR 0019 E).
func TestDecideApproval_Rules(t *testing.T) {
	tests := []struct {
		name string
		c    ApprovalCase
		want ApprovalVerdict
	}{
		// Confirm (create, manual Confirm, payment at the desk).
		{"confirm, no approval", ApprovalCase{Op: ApprovalOnConfirm, From: StatusHeld}, ApprovalProceed},
		{"confirm, no approval, forced", ApprovalCase{Op: ApprovalOnConfirm, From: StatusHeld, Forced: true}, ApprovalProceed},
		{"R2 confirm held needing approval waits", ApprovalCase{Op: ApprovalOnConfirm, From: StatusHeld, Needed: true}, ApprovalAwait},
		{"R1 force does not skip approval", ApprovalCase{Op: ApprovalOnConfirm, From: StatusHeld, Needed: true, Forced: true}, ApprovalAwait},
		{"R2 even the manager waits on confirm", ApprovalCase{Op: ApprovalOnConfirm, From: StatusDraft, Needed: true, Approver: true}, ApprovalAwait},
		{"R2 payment at desk waits", ApprovalCase{Op: ApprovalOnConfirm, From: StatusPendingPayment, Needed: true}, ApprovalAwait},
		{"R3 desk cannot confirm awaiting", ApprovalCase{Op: ApprovalOnConfirm, From: StatusAwaitingApproval, Needed: true}, ApprovalRefuse},
		{"R3 desk forcing cannot confirm awaiting", ApprovalCase{Op: ApprovalOnConfirm, From: StatusAwaitingApproval, Needed: true, Forced: true}, ApprovalRefuse},
		{"R3 manager confirming awaiting approves", ApprovalCase{Op: ApprovalOnConfirm, From: StatusAwaitingApproval, Needed: true, Approver: true}, ApprovalProceedApproved},
		{"R3 awaiting without current need still needs the manager", ApprovalCase{Op: ApprovalOnConfirm, From: StatusAwaitingApproval}, ApprovalRefuse},

		// Modifications.
		{"modify, no approval", ApprovalCase{Op: ApprovalOnModify, From: StatusConfirmed}, ApprovalProceed},
		{"R4 desk move onto approval spot refused", ApprovalCase{Op: ApprovalOnModify, From: StatusConfirmed, Needed: true}, ApprovalRefuse},
		{"R1/R4 desk forced move still refused", ApprovalCase{Op: ApprovalOnModify, From: StatusConfirmed, Needed: true, Forced: true}, ApprovalRefuse},
		{"R4 checked-in too", ApprovalCase{Op: ApprovalOnModify, From: StatusCheckedIn, Needed: true}, ApprovalRefuse},
		{"R4 manager modifies and approves", ApprovalCase{Op: ApprovalOnModify, From: StatusConfirmed, Needed: true, Approver: true}, ApprovalProceedApproved},
		{"R4 already approved booking may change", ApprovalCase{Op: ApprovalOnModify, From: StatusConfirmed, Needed: true, NeededBefore: true}, ApprovalProceed},
		{"R5 held: approval still ahead", ApprovalCase{Op: ApprovalOnModify, From: StatusHeld, Needed: true}, ApprovalProceed},
		{"R5 awaiting: approval still ahead", ApprovalCase{Op: ApprovalOnModify, From: StatusAwaitingApproval, Needed: true}, ApprovalProceed},
		{"R5 draft", ApprovalCase{Op: ApprovalOnModify, From: StatusDraft, Needed: true, Forced: true}, ApprovalProceed},
		{"unknown op refuses", ApprovalCase{Op: "other", Needed: true}, ApprovalRefuse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecideApproval(tt.c); got != tt.want {
				t.Fatalf("DecideApproval(%+v) = %s, want %s", tt.c, got, tt.want)
			}
		})
	}
}
