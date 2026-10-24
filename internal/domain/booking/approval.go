package booking

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
)

// Approval rule (spec 9 approval_required, spec 10.2 awaiting_approval,
// ADR 0019 E). A booking needs the manager's approval when its policy
// (snapshot, or the re-quote of a modification) has approval_required or one
// of its packs has requires_approval.
//
//	R1  force waives policy violations only (lead time, duration, max
//	    spots...), never the approval: a forced booking that needs approval
//	    still waits for it.
//	R2  a booking that needs approval reaches confirmed only through Approve:
//	    creation, the manual Confirm of a draft/held/pending_payment booking
//	    and a payment collected at the desk (ConfirmFromPayment) move it to
//	    awaiting_approval with the long hold instead of confirming it.
//	R3  Approve is the manager's decision: permission establishment.configure
//	    (owner, manager), never the desk (cashier) alone; the manual Confirm
//	    of an awaiting_approval booking is an approval and follows the same
//	    rule. Reject stays with bookings.write (it only releases resources,
//	    like a cancellation).
//	R4  a modification (PATCH, move, forced or not) of a confirmed or
//	    checked-in booking that did not need approval and now does (a pack
//	    or a spot whose policy requires approval) is refused with
//	    ErrApprovalRequired unless the actor can approve (R3); the
//	    modification by an approver is audited with approved = true. A
//	    booking that already needed approval was approved: it may change.
//	R5  modifications of draft, held, pending_payment or awaiting_approval
//	    bookings are not decided here: the approval is still ahead (R2).

// ErrApprovalRequired: the operation needs the manager's approval and the
// actor cannot approve (rules R3, R4).
var ErrApprovalRequired = fmt.Errorf("%w: serve l'approvazione del gestore (pacchetto o zona con approvazione obbligatoria)", configerr.ErrConflict)

// ApprovalOp is the operation being decided.
type ApprovalOp string

const (
	// ApprovalOnConfirm: a transition towards confirmed (create, Confirm,
	// ConfirmFromPayment).
	ApprovalOnConfirm ApprovalOp = "confirm"
	// ApprovalOnModify: a modification of an existing booking.
	ApprovalOnModify ApprovalOp = "modify"
)

// ApprovalVerdict is the outcome of DecideApproval.
type ApprovalVerdict string

const (
	// ApprovalProceed: no approval involved, go on.
	ApprovalProceed ApprovalVerdict = "proceed"
	// ApprovalProceedApproved: the actor approves while modifying (audited).
	ApprovalProceedApproved ApprovalVerdict = "proceed_approved"
	// ApprovalAwait: move to awaiting_approval instead of confirmed.
	ApprovalAwait ApprovalVerdict = "await"
	// ApprovalRefuse: refuse with ErrApprovalRequired.
	ApprovalRefuse ApprovalVerdict = "refuse"
)

// ApprovalCase are the facts of DecideApproval.
type ApprovalCase struct {
	Op     ApprovalOp
	From   Status
	Needed bool // approval needed after the operation
	// NeededBefore: approval was already needed before a modification.
	NeededBefore bool
	// Approver: the actor holds auth.PermEstablishmentConfigure.
	Approver bool
	// Forced: the operation waives policy violations (never the approval, R1).
	Forced bool
}

// DecideApproval applies rules R1-R5. It is pure.
func DecideApproval(c ApprovalCase) ApprovalVerdict {
	if c.Op == ApprovalOnConfirm && c.From == StatusAwaitingApproval {
		// Confirming a booking awaiting approval is approving it (R3).
		if c.Approver {
			return ApprovalProceedApproved
		}
		return ApprovalRefuse
	}
	if !c.Needed {
		return ApprovalProceed
	}
	switch c.Op {
	case ApprovalOnConfirm:
		return ApprovalAwait // R1, R2: force and role do not matter
	case ApprovalOnModify:
		if c.From != StatusConfirmed && c.From != StatusCheckedIn {
			return ApprovalProceed // R5
		}
		if c.NeededBefore {
			return ApprovalProceed // already approved
		}
		if c.Approver {
			return ApprovalProceedApproved
		}
		return ApprovalRefuse // R4, whatever Forced says
	}
	return ApprovalRefuse
}

// canApprove reports whether p may approve bookings of es (R3); nil (the
// public channel) never can.
func canApprove(p *auth.Principal, es store.EstablishmentScope) bool {
	return p != nil && p.Can(es, auth.PermEstablishmentConfigure)
}

// checkModifyApproval applies R4/R5 to a modification of b whose re-quote
// needs approval (needed); before is computed on the booking as stored.
func (s *Service) checkModifyApproval(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, b *store.Booking, needed, forced bool, diff map[string]any) error {
	if !needed {
		return nil
	}
	before, err := s.needsApproval(ctx, tx, es, b)
	if err != nil {
		return err
	}
	switch DecideApproval(ApprovalCase{
		Op: ApprovalOnModify, From: Status(b.Status), Needed: needed, NeededBefore: before,
		Approver: canApprove(p, es), Forced: forced,
	}) {
	case ApprovalRefuse:
		return ErrApprovalRequired
	case ApprovalProceedApproved:
		if diff != nil {
			diff["approved"] = true
		}
	}
	return nil
}

// awaitApprovalLocked moves a draft, held or pending_payment booking that
// needs approval to awaiting_approval with the long hold (R2) instead of
// confirming it; a draft is allocated now with hold allocations.
func (s *Service) awaitApprovalLocked(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, reason string) (*outcome, error) {
	from := Status(b.Status)
	now := s.now()
	until := now.Add(s.d.ApprovalTTL)
	var alloc *inventory.Allocation
	switch from {
	case StatusDraft:
		if err := Transition(StatusDraft, StatusHeld); err != nil {
			return nil, err
		}
		if err := Transition(StatusHeld, StatusAwaitingApproval); err != nil {
			return nil, err
		}
		rows, extras, cat, err := s.currentLines(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		if alloc, err = s.d.Inventory.Allocate(ctx, tx, es, allocRequest(b, inventory.KindHold, &until, rows, extras, cat)); err != nil {
			return nil, err
		}
		if err := s.applyAllocation(ctx, tx, es, b, rows, alloc, inventory.KindHold, now); err != nil {
			return nil, err
		}
	default:
		if err := Transition(from, StatusAwaitingApproval); err != nil {
			return nil, err
		}
		if err := s.d.Inventory.ExtendHold(ctx, tx, es, b.ID, until); err != nil {
			return nil, err
		}
	}
	b.Status = string(StatusAwaitingApproval)
	b.HoldExpiresAt = &until
	return &outcome{action: eventApprovalRequested, diff: map[string]any{"from": string(from), "reason": reason}, alloc: alloc}, nil
}
